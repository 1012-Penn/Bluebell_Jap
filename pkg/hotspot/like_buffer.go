package hotspot

import (
	"math"
	"sync"
	"time"
)

// likeBuffer 将热点帖子点赞的增量缓存在本地，满足阈值后再批量落库。
type likeBuffer struct {
	mu        sync.Mutex
	threshold int64
	window    time.Duration
	entries   map[int64]*likeEntry
}

type likeEntry struct {
	delta     int64
	firstTime time.Time
}

func newLikeBuffer(threshold int64, window time.Duration) *likeBuffer {
	if threshold <= 0 {
		threshold = 50
	}
	if window <= 0 {
		window = 10 * time.Second
	}
	return &likeBuffer{
		threshold: threshold,
		window:    window,
		entries:   make(map[int64]*likeEntry),
	}
}

// Add 会累积帖子 postID 的点赞增量，并在达到阈值或时间窗时返回需要落库的值。
func (b *likeBuffer) Add(postID int64, delta int64) (flush bool, flushValue int64) {
	if delta == 0 {
		return false, 0
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	entry, ok := b.entries[postID]
	if !ok {
		entry = &likeEntry{firstTime: time.Now()}
		b.entries[postID] = entry
	}
	entry.delta += delta
	if entry.delta == 0 {
		entry.firstTime = time.Now()
		return false, 0
	}

	if math.Abs(float64(entry.delta)) >= float64(b.threshold) || time.Since(entry.firstTime) >= b.window {
		flushValue = entry.delta
		entry.delta = 0
		entry.firstTime = time.Now()
		return true, flushValue
	}
	return false, 0
}

// ForceFlush 将缓存中所有未落库的增量一次性返回，通常用于定时器或服务关闭时调用。
func (b *likeBuffer) ForceFlush() map[int64]int64 {
	b.mu.Lock()
	defer b.mu.Unlock()

	result := make(map[int64]int64, len(b.entries))
	for id, entry := range b.entries {
		if entry.delta != 0 {
			result[id] = entry.delta
			entry.delta = 0
			entry.firstTime = time.Now()
		}
	}
	return result
}
