package hotspot

import (
	"container/heap"
	"sync"
)

// rankingManager 维护热点帖子的有限优先队列，结合 map + 小顶堆实现。
type rankingManager struct {
	mu      sync.Mutex
	maxSize int
	pq      priorityQueue
	index   map[int64]int
}

type element struct {
	key   int64
	score float64
}

type priorityQueue []element

func (pq priorityQueue) Len() int { return len(pq) }

func (pq priorityQueue) Less(i, j int) bool {
	// 小顶堆：分数越小优先被淘汰
	return pq[i].score < pq[j].score
}

func (pq priorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
}

func (pq *priorityQueue) Push(x interface{}) {
	*pq = append(*pq, x.(element))
}

func (pq *priorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	*pq = old[:n-1]
	return item
}

func newRankingManager(maxSize int) *rankingManager {
	if maxSize <= 0 {
		maxSize = 200
	}
	return &rankingManager{
		maxSize: maxSize,
		pq:      make(priorityQueue, 0, maxSize),
		index:   make(map[int64]int, maxSize),
	}
}

func (m *rankingManager) Update(key int64, score float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if idx, ok := m.index[key]; ok {
		m.pq[idx].score = score
		heap.Fix(&m.pq, idx)
		return
	}

	heap.Push(&m.pq, element{key: key, score: score})
	m.index[key] = len(m.pq) - 1
	if len(m.pq) > m.maxSize {
		removed := heap.Pop(&m.pq).(element)
		delete(m.index, removed.key)
		// 更新索引表（被移除元素之后的索引全部左移）
		for i := range m.pq {
			m.index[m.pq[i].key] = i
		}
	}
}

func (m *rankingManager) Snapshot() []element {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make([]element, len(m.pq))
	copy(result, m.pq)
	return result
}
