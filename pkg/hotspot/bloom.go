package hotspot

import (
	"hash/fnv"
	"math/big"
	"sync"
)

// bloomFilter 是一个极简实现，用于防止缓存穿透。
type bloomFilter struct {
	m    uint
	k    uint
	bits *big.Int
	mu   sync.RWMutex
}

func newBloomFilter(m, k uint) *bloomFilter {
	if m == 0 {
		m = 1 << 20
	}
	if k == 0 {
		k = 3
	}
	return &bloomFilter{
		m:    m,
		k:    k,
		bits: big.NewInt(0),
	}
}

func (b *bloomFilter) Add(data []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, loc := range b.locations(data) {
		b.bits.SetBit(b.bits, int(loc), 1)
	}
}

func (b *bloomFilter) Test(data []byte) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, loc := range b.locations(data) {
		if b.bits.Bit(int(loc)) == 0 {
			return false
		}
	}
	return true
}

func (b *bloomFilter) locations(data []byte) []uint {
	results := make([]uint, 0, b.k)
	h := fnv.New64a()
	_, _ = h.Write(data)
	base := h.Sum64()
	for i := uint(0); i < b.k; i++ {
		combined := base + uint64(i)*0x9e3779b97f4a7c15
		results = append(results, uint(combined%uint64(b.m)))
	}
	return results
}
