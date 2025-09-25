// Package hotspot 热点数据管理包
//
// 实现热点数据的识别、缓存和批量处理机制
// 主要功能：
// 1. 识别热点帖子（高访问量、高点赞量）
// 2. 本地缓存热点数据，减少数据库压力
// 3. 批量处理点赞增量，提高系统性能
// 4. 智能阈值控制，平衡实时性和性能
//
// 技术亮点：
// - 本地缓存热点数据，减少Redis和数据库访问
// - 批量处理机制，提高数据库写入效率
// - 智能阈值控制，平衡实时性和性能
// - 线程安全设计，支持高并发访问
package hotspot

import (
	"math"
	"sync"
	"time"
)

// likeBuffer 点赞增量缓冲区
//
// 功能说明：
// 1. 将热点帖子的点赞增量缓存在本地内存中
// 2. 当达到阈值或时间窗口时，批量写入数据库
// 3. 减少数据库写入频率，提高系统性能
// 4. 支持正负增量（点赞和取消点赞）
//
// 字段说明：
// - mu: 互斥锁，保证线程安全
// - threshold: 触发批量写入的阈值
// - window: 时间窗口，超过此时间强制写入
// - entries: 帖子ID到点赞增量的映射
//
// 技术亮点：
// - 本地缓存，减少数据库压力
// - 批量处理，提高写入效率
// - 阈值控制，平衡实时性和性能
// - 线程安全，支持高并发
type likeBuffer struct {
	mu        sync.Mutex           // 互斥锁，保证线程安全
	threshold int64                // 触发批量写入的阈值
	window    time.Duration        // 时间窗口，超过此时间强制写入
	entries   map[int64]*likeEntry // 帖子ID到点赞增量的映射
}

// likeEntry 点赞增量条目
//
// 功能说明：
// 1. 记录单个帖子的点赞增量
// 2. 记录首次添加时间，用于时间窗口控制
// 3. 支持正负增量（点赞和取消点赞）
//
// 字段说明：
// - delta: 点赞增量，正数表示点赞，负数表示取消点赞
// - firstTime: 首次添加时间，用于时间窗口控制
//
// 技术亮点：
// - 支持正负增量，处理点赞和取消点赞
// - 记录时间信息，支持时间窗口控制
type likeEntry struct {
	delta     int64     // 点赞增量
	firstTime time.Time // 首次添加时间
}

// newLikeBuffer 创建点赞增量缓冲区
//
// 功能说明：
// 1. 初始化点赞增量缓冲区
// 2. 设置阈值和时间窗口参数
// 3. 提供合理的默认值
//
// 参数说明：
// - threshold: 触发批量写入的阈值，默认50
// - window: 时间窗口，默认10秒
//
// 返回值：
// - *likeBuffer: 点赞增量缓冲区实例
//
// 技术亮点：
// - 提供合理的默认值，简化使用
// - 参数验证，确保配置正确
// - 预分配内存，提高性能
func newLikeBuffer(threshold int64, window time.Duration) *likeBuffer {
	// 设置默认阈值
	if threshold <= 0 {
		threshold = 50 // 默认阈值：50个点赞增量
	}

	// 设置默认时间窗口
	if window <= 0 {
		window = 10 * time.Second // 默认时间窗口：10秒
	}

	return &likeBuffer{
		threshold: threshold,
		window:    window,
		entries:   make(map[int64]*likeEntry), // 预分配内存
	}
}

// Add 添加点赞增量到缓冲区
//
// 功能说明：
// 1. 累积指定帖子的点赞增量
// 2. 检查是否达到阈值或时间窗口
// 3. 达到条件时返回需要落库的值
// 4. 支持正负增量（点赞和取消点赞）
//
// 参数说明：
// - postID: 帖子ID
// - delta: 点赞增量，正数表示点赞，负数表示取消点赞
//
// 返回值：
// - flush: 是否需要落库
// - flushValue: 需要落库的增量值
//
// 技术亮点：
// - 线程安全，使用互斥锁保护
// - 智能阈值控制，平衡实时性和性能
// - 支持正负增量，处理点赞和取消点赞
// - 时间窗口控制，确保数据及时落库
func (b *likeBuffer) Add(postID int64, delta int64) (flush bool, flushValue int64) {
	// 1. 参数验证
	if delta == 0 {
		return false, 0 // 增量为0，无需处理
	}

	// 2. 加锁保护，确保线程安全
	b.mu.Lock()
	defer b.mu.Unlock()

	// 3. 获取或创建帖子条目
	entry, ok := b.entries[postID]
	if !ok {
		// 技术亮点：首次添加时记录时间，用于时间窗口控制
		entry = &likeEntry{firstTime: time.Now()}
		b.entries[postID] = entry
	}

	// 4. 累积增量
	entry.delta += delta

	// 5. 处理增量归零的情况
	if entry.delta == 0 {
		// 技术亮点：增量归零时重置时间，避免立即触发
		entry.firstTime = time.Now()
		return false, 0
	}

	// 6. 检查是否达到触发条件
	// 技术亮点：使用绝对值检查，支持正负增量
	// 条件1：增量绝对值达到阈值
	// 条件2：时间窗口已过期
	if math.Abs(float64(entry.delta)) >= float64(b.threshold) || time.Since(entry.firstTime) >= b.window {
		// 7. 准备落库数据
		flushValue = entry.delta

		// 8. 重置条目状态
		// 技术亮点：重置增量和时间，为下次累积做准备
		entry.delta = 0
		entry.firstTime = time.Now()

		return true, flushValue
	}

	return false, 0
}

// ForceFlush 强制刷新所有缓存的增量数据
//
// 功能说明：
// 1. 将缓冲区中所有未落库的增量数据一次性返回
// 2. 通常用于定时器触发或服务关闭时调用
// 3. 确保数据不丢失，及时落库
// 4. 清空缓冲区，为下次累积做准备
//
// 返回值：
// - map[int64]int64: 帖子ID到增量的映射
//
// 技术亮点：
// - 线程安全，使用互斥锁保护
// - 批量处理，提高效率
// - 数据完整性，确保不丢失
// - 状态重置，为下次累积做准备
//
// 使用场景：
// - 定时器触发，定期落库
// - 服务关闭时，确保数据不丢失
// - 内存压力大时，强制落库
func (b *likeBuffer) ForceFlush() map[int64]int64 {
	// 1. 加锁保护，确保线程安全
	b.mu.Lock()
	defer b.mu.Unlock()

	// 2. 预分配结果映射，提高性能
	result := make(map[int64]int64, len(b.entries))

	// 3. 遍历所有条目，收集未落库的增量
	for id, entry := range b.entries {
		if entry.delta != 0 {
			// 技术亮点：只收集非零增量，避免无效数据
			result[id] = entry.delta

			// 4. 重置条目状态
			// 技术亮点：重置增量和时间，为下次累积做准备
			entry.delta = 0
			entry.firstTime = time.Now()
		}
	}

	return result
}
