package hotspot

import (
	"container/list"
	"sync"
	"time"
)

// localCache 是一个简单的 LRU + TTL 组合缓存，用于热点帖子在本地内存中的快速读取。
//
// 设计目标：
//  1. 控制缓存容量，避免热点帖过多时占用过多内存；
//  2. 支持 TTL，在热点衰减后能够自动淘汰；
//  3. 尽量保持实现简单，减少对现有结构的侵入。
type localCache struct {
	mu       sync.RWMutex
	ttl      time.Duration
	capacity int
	items    map[int64]*list.Element
	list     *list.List
}

type cacheEntry struct {
	key    int64
	value  interface{}
	expire time.Time
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
