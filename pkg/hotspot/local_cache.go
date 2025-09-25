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
	"container/list"
	"sync"
	"time"
)

// localCache 本地LRU+TTL组合缓存
//
// 功能说明：
// 1. 实现LRU（最近最少使用）淘汰策略
// 2. 支持TTL（生存时间）自动过期
// 3. 用于热点帖子在本地内存中的快速读取
// 4. 控制缓存容量，避免内存占用过多
//
// 设计目标：
// 1. 控制缓存容量，避免热点帖过多时占用过多内存
// 2. 支持TTL，在热点衰减后能够自动淘汰
// 3. 尽量保持实现简单，减少对现有结构的侵入
//
// 字段说明：
// - mu: 读写锁，保证线程安全
// - ttl: 缓存项生存时间
// - capacity: 缓存容量上限
// - items: 键到链表元素的映射，用于O(1)查找
// - list: 双向链表，用于LRU排序
//
// 技术亮点：
// - LRU算法，自动淘汰最久未使用的数据
// - TTL机制，自动清理过期数据
// - 双向链表+哈希表，O(1)查找和更新
// - 线程安全，支持并发访问
type localCache struct {
	mu       sync.RWMutex            // 读写锁，保证线程安全
	ttl      time.Duration           // 缓存项生存时间
	capacity int                     // 缓存容量上限
	items    map[int64]*list.Element // 键到链表元素的映射，用于O(1)查找
	list     *list.List              // 双向链表，用于LRU排序
}

// cacheEntry 缓存条目
//
// 功能说明：
// 1. 存储单个缓存项的数据
// 2. 记录过期时间，支持TTL机制
// 3. 包含键值对信息
//
// 字段说明：
// - key: 缓存键，通常是帖子ID
// - value: 缓存值，可以是任意类型
// - expire: 过期时间，用于TTL判断
//
// 技术亮点：
// - 记录过期时间，支持自动清理
// - 泛型设计，支持任意类型数据
type cacheEntry struct {
	key    int64       // 缓存键
	value  interface{} // 缓存值
	expire time.Time   // 过期时间
}

func newLocalCache(capacity int, ttl time.Duration) *localCache {
	if capacity <= 0 {
		capacity = 128
	}
	return &localCache{
		ttl:      ttl,
		capacity: capacity,
		items:    make(map[int64]*list.Element, capacity),
		list:     list.New(),
	}
}

func (c *localCache) Get(key int64) (interface{}, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if ele, ok := c.items[key]; ok {
		entry := ele.Value.(*cacheEntry)
		if entry.expire.After(time.Now()) {
			c.list.MoveToFront(ele)
			return entry.value, true
		}
		c.removeElement(ele)
	}
	return nil, false
}

func (c *localCache) Set(key int64, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if ele, ok := c.items[key]; ok {
		c.list.MoveToFront(ele)
		ele.Value.(*cacheEntry).value = value
		ele.Value.(*cacheEntry).expire = time.Now().Add(c.ttl)
		return
	}

	ele := c.list.PushFront(&cacheEntry{
		key:    key,
		value:  value,
		expire: time.Now().Add(c.ttl),
	})
	c.items[key] = ele

	if c.list.Len() > c.capacity {
		c.removeElement(c.list.Back())
	}
}

func (c *localCache) removeElement(ele *list.Element) {
	if ele == nil {
		return
	}
	c.list.Remove(ele)
	entry := ele.Value.(*cacheEntry)
	delete(c.items, entry.key)
}
