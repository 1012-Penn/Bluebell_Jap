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
	"context"
	"strconv"
	"sync"
	"time"

	"bluebell/dao/mysql"
	redispkg "bluebell/dao/redis"
	"bluebell/models"

	redis "github.com/go-redis/redis/v8"
	"go.uber.org/zap"
)

// Manager 热点数据管理器
//
// 功能说明：
// 1. 统一管理热点数据的识别、缓存和批量处理
// 2. 组合多个组件，提供统一的外部接口
// 3. 实现热点帖子的自动识别和缓存
// 4. 管理点赞增量的缓冲和批量落库
// 5. 维护热点帖子的排行榜
//
// 字段说明：
// - cache: 帖子缓存组件，用于热点帖子缓存
// - likeBuffer: 点赞增量缓冲区，用于批量处理点赞
// - ranking: 排行榜管理器，维护热点帖子排名
// - client: Redis客户端，用于数据存储和缓存
// - decay: 热度衰减因子，用于时间衰减计算
// - hotScore: 热点阈值，超过此分数认为是热点
// - flushTicker: 定时器，用于定期刷新数据
// - once: 确保初始化只执行一次
// - mu: 读写锁，保护热点帖子集合
// - hotPosts: 热点帖子ID集合
//
// 技术亮点：
// - 组合模式，统一管理多个组件
// - 单例模式，避免重复创建
// - 定时刷新，确保数据及时落库
// - 线程安全，支持高并发访问
type Manager struct {
	cache      *PostCache      // 帖子缓存组件
	likeBuffer *likeBuffer     // 点赞增量缓冲区
	ranking    *rankingManager // 排行榜管理器
	client     *redis.Client   // Redis客户端

	decay       float64            // 热度衰减因子
	hotScore    float64            // 热点阈值
	flushTicker *time.Ticker       // 定时刷新器
	once        sync.Once          // 确保初始化只执行一次
	mu          sync.RWMutex       // 读写锁
	hotPosts    map[int64]struct{} // 热点帖子ID集合
}

// NewManager 创建热点管理器实例
//
// 功能说明：
// 1. 初始化所有子组件（缓存、缓冲区、排行榜等）
// 2. 设置合理的默认参数
// 3. 启动定时刷新循环
// 4. 返回完整配置的管理器实例
//
// 返回值：
// - *Manager: 热点管理器实例
//
// 技术亮点：
// - 组合模式，统一管理多个组件
// - 合理的默认参数，开箱即用
// - 自动启动定时任务，无需手动管理
// - 单例模式，避免重复创建
func NewManager() *Manager {
	client := redispkg.GetClient()
	m := &Manager{
		cache:      NewPostCache(512),                 // 帖子缓存，容量512
		likeBuffer: newLikeBuffer(50, 10*time.Second), // 点赞缓冲区，阈值50，窗口10秒
		ranking:    newRankingManager(200),            // 排行榜管理器，容量200
		client:     client,                            // Redis客户端
		decay:      0.1,                               // 热度衰减因子
		hotScore:   120,                               // 热点阈值
		hotPosts:   make(map[int64]struct{}),          // 热点帖子集合
	}

	// 启动定时刷新循环
	m.startFlushLoop()
	return m
}

// startFlushLoop 启动定时刷新循环
//
// 功能说明：
// 1. 创建定时器，每10分钟执行一次刷新
// 2. 在独立goroutine中运行，不阻塞主程序
// 3. 定期刷新排行榜和点赞缓冲区
// 4. 确保数据及时落库，避免丢失
//
// 技术亮点：
// - 使用sync.Once确保只启动一次
// - 独立goroutine，不阻塞主程序
// - 定时刷新，确保数据及时落库
// - 自动运行，无需手动管理
func (m *Manager) startFlushLoop() {
	m.once.Do(func() {
		// 技术亮点：每10分钟刷新一次，平衡实时性和性能
		m.flushTicker = time.NewTicker(10 * time.Minute)
		go func() {
			// 技术亮点：独立goroutine运行，不阻塞主程序
			for range m.flushTicker.C {
				// 刷新排行榜到Redis
				m.FlushRanking(context.Background())
				// 刷新点赞缓冲区到数据库
				m.FlushLikeBuffer(context.Background())
			}
		}()
	})
}

// Cache 获取帖子缓存组件
//
// 功能说明：
// 1. 返回帖子缓存组件，供外部使用
// 2. 用于帖子查询逻辑中的缓存操作
//
// 返回值：
// - *PostCache: 帖子缓存组件
//
// 技术亮点：
// - 提供统一的外部接口
// - 封装内部实现细节
func (m *Manager) Cache() *PostCache {
	return m.cache
}

// ObservePost 观察帖子访问，记录指标并识别热点
//
// 功能说明：
// 1. 记录帖子的访问指标（浏览量、点赞量等）
// 2. 计算帖子的热度分数
// 3. 识别热点帖子并加入缓存
// 4. 更新排行榜数据
//
// 参数说明：
// - ctx: 上下文
// - detail: 帖子详情，包含帖子基本信息
// - withView: 是否记录浏览量
//
// 技术亮点：
// - 实时热度计算，及时识别热点
// - 多指标综合评估（浏览量、点赞量、评论量）
// - 自动缓存热点帖子，提高访问效率
// - 更新排行榜，支持热点排序
func (m *Manager) ObservePost(ctx context.Context, detail *models.ApiPostDetail, withView bool) {
	// 1. 参数验证
	if detail == nil || detail.Post == nil {
		return
	}

	pid := detail.Post.ID

	// 2. 记录浏览量指标
	if withView {
		// 技术亮点：记录浏览量，参与热度计算
		m.addMetric(ctx, pid, detail.Post.CreateTime, "views", 1)
	}

	// 3. 计算并更新热度分数
	// 技术亮点：综合多个指标计算热度分数
	score := m.calculateAndUpdate(ctx, pid, detail.Post.CreateTime)

	// 4. 检查是否为热点帖子
	if score >= m.hotScore {
		// 技术亮点：达到热点阈值，标记为热点并加入缓存
		m.markHot(detail, score)
	}
}

func (m *Manager) markHot(detail *models.ApiPostDetail, score float64) {
	pid := detail.Post.ID
	m.mu.Lock()
	_, alreadyHot := m.hotPosts[pid]
	m.hotPosts[pid] = struct{}{}
	m.mu.Unlock()

	if !alreadyHot {
		// 将帖子写入本地热点缓存，提高命中率
		m.cache.PromoteHot(pid, detail, 5*time.Minute)
	}
	m.ranking.Update(pid, score)
}

// FlushRanking 将本地排名快照批量写入 Redis ZSET。
func (m *Manager) FlushRanking(ctx context.Context) {
	snapshot := m.ranking.Snapshot()
	if len(snapshot) == 0 {
		return
	}
	members := make([]*redis.Z, 0, len(snapshot))
	for _, item := range snapshot {
		members = append(members, &redis.Z{Score: item.score, Member: item.key})
	}
	_ = m.client.ZAdd(ctx, "bluebell:hot:ranking", members...).Err()
}

// FlushLikeBuffer 将热点帖子的点赞增量批量写入 Redis 和数据库。
func (m *Manager) FlushLikeBuffer(ctx context.Context) {
	entries := m.likeBuffer.ForceFlush()
	for pid, delta := range entries {
		m.persistLikeDelta(ctx, pid, delta)
	}
}

// HandleLikeEvent 处理点赞增量，热点帖子触发缓冲策略。
func (m *Manager) HandleLikeEvent(ctx context.Context, pid int64, createdAt time.Time, delta int64) {
	score := m.addMetric(ctx, pid, createdAt, "likes", delta)
	if score >= m.hotScore {
		m.mu.Lock()
		m.hotPosts[pid] = struct{}{}
		m.mu.Unlock()
	}
	if m.isHot(pid) {
		if flush, value := m.likeBuffer.Add(pid, delta); flush {
			m.persistLikeDelta(ctx, pid, value)
		}
	} else {
		m.persistLikeDelta(ctx, pid, delta)
	}
}

// HandleCommentEvent 评论同样参与热度计算。
func (m *Manager) HandleCommentEvent(ctx context.Context, pid int64, createdAt time.Time, delta int64) {
	m.addMetric(ctx, pid, createdAt, "comments", delta)
}

func (m *Manager) isHot(pid int64) bool {
	m.mu.RLock()
	_, ok := m.hotPosts[pid]
	m.mu.RUnlock()
	return ok
}

func (m *Manager) addMetric(ctx context.Context, pid int64, createdAt time.Time, field string, delta int64) float64 {
	if pid == 0 {
		return 0
	}
	key := "bluebell:hot:metric:" + strconv.FormatInt(pid, 10)
	_ = m.client.HIncrBy(ctx, key, field, delta).Err()
	_ = m.client.Expire(ctx, key, 24*time.Hour).Err()
	return m.calculateAndUpdate(ctx, pid, createdAt)
}

func (m *Manager) calculateAndUpdate(ctx context.Context, pid int64, createdAt time.Time) float64 {
	key := "bluebell:hot:metric:" + strconv.FormatInt(pid, 10)
	data, err := m.client.HGetAll(ctx, key).Result()
	if err != nil {
		return 0
	}
	views := parseInt64(data["views"])
	likes := parseInt64(data["likes"])
	comments := parseInt64(data["comments"])
	score := HeatScore(views, likes, comments, createdAt, m.decay)
	m.ranking.Update(pid, score)
	return score
}

func parseInt64(v string) int64 {
	if v == "" {
		return 0
	}
	value, _ := strconv.ParseInt(v, 10, 64)
	return value
}

func (m *Manager) persistLikeDelta(ctx context.Context, pid int64, delta int64) {
	if delta == 0 {
		return
	}
	field := "bluebell:hot:likes:" + strconv.FormatInt(pid, 10)
	_ = m.client.IncrBy(ctx, field, delta).Err()
	_ = m.client.Expire(ctx, field, 24*time.Hour).Err()
	if err := mysql.UpsertPostLikeStat(pid, delta); err != nil {
		zap.L().Error("UpsertPostLikeStat failed", zap.Int64("pid", pid), zap.Error(err))
	}
}

// GlobalManager 全局热点管理器
//
// 功能说明：
// 1. 提供全局单例热点管理器
// 2. 懒加载模式，首次调用时创建
// 3. 避免在不同逻辑中重复创建对象
// 4. 确保全局只有一个管理器实例
//
// 技术亮点：
// - 单例模式，确保全局唯一
// - 懒加载，按需创建
// - 线程安全，使用sync.Once
// - 内存优化，避免重复创建
var (
	globalManager     *Manager  // 全局管理器实例
	globalManagerOnce sync.Once // 确保只创建一次
)

// GetManager 获取全局热点管理器实例
//
// 功能说明：
// 1. 返回全局单例热点管理器
// 2. 首次调用时创建实例
// 3. 后续调用返回已创建的实例
//
// 返回值：
// - *Manager: 全局热点管理器实例
//
// 技术亮点：
// - 单例模式，确保全局唯一
// - 懒加载，按需创建
// - 线程安全，使用sync.Once
// - 性能优化，避免重复创建
func GetManager() *Manager {
	globalManagerOnce.Do(func() {
		// 技术亮点：使用sync.Once确保只创建一次，线程安全
		globalManager = NewManager()
	})
	return globalManager
}
