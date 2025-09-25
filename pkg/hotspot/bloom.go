package hotspot

import (
	"hash/fnv"
	"math/big"
	"sync"
)

// bloomFilter 是一个极简实现，用于防止缓存穿透。
// 布隆过滤器是一种概率型数据结构，用于快速判断一个元素是否可能存在于集合中。
// 特点：
// 1. 如果布隆过滤器说某个元素不存在，那么它一定不存在（无假阴性）
// 2. 如果布隆过滤器说某个元素存在，那么它可能存在（有假阳性）
// 3. 空间效率高，查询速度快
type bloomFilter struct {
	m    uint         // 位数组的大小（位数）
	k    uint         // 哈希函数的数量
	bits *big.Int     // 位数组，使用大整数来存储位
	mu   sync.RWMutex // 读写锁，保证并发安全
}

// newBloomFilter 创建一个新的布隆过滤器
// 参数：
//
//	m: 位数组大小，默认为 2^20 (1,048,576 位)
//	k: 哈希函数数量，默认为 3
func newBloomFilter(m, k uint) *bloomFilter {
	// 设置默认值
	if m == 0 {
		m = 1 << 20 // 2^20 = 1,048,576 位
	}
	if k == 0 {
		k = 3 // 使用3个哈希函数
	}
	return &bloomFilter{
		m:    m,
		k:    k,
		bits: big.NewInt(0), // 初始化为全0的位数组
	}
}

// Add 向布隆过滤器中添加一个元素
// 对输入数据进行k次哈希，将对应的位设置为1
func (b *bloomFilter) Add(data []byte) {
	b.mu.Lock() // 写操作需要加写锁
	defer b.mu.Unlock()

	// 获取该数据对应的k个位置
	for _, loc := range b.locations(data) {
		// 将对应位置的位设置为1
		b.bits.SetBit(b.bits, int(loc), 1)
	}
}

// Test 检查一个元素是否可能存在于布隆过滤器中
// 返回true表示可能存在，false表示一定不存在
func (b *bloomFilter) Test(data []byte) bool {
	b.mu.RLock() // 读操作加读锁
	defer b.mu.RUnlock()

	// 检查该数据对应的所有k个位置
	for _, loc := range b.locations(data) {
		// 如果任何一个位置为0，说明该元素一定不存在
		if b.bits.Bit(int(loc)) == 0 {
			return false
		}
	}
	// 所有位置都为1，说明该元素可能存在（但可能有假阳性）
	return true
}

// locations 计算输入数据在布隆过滤器中的k个位置
// 使用FNV哈希算法生成基础哈希值，然后通过线性组合生成k个不同的位置
func (b *bloomFilter) locations(data []byte) []uint {
	results := make([]uint, 0, b.k)

	// 使用FNV-1a哈希算法计算基础哈希值
	h := fnv.New64a()
	_, _ = h.Write(data)
	base := h.Sum64() // 得到64位哈希值

	// 通过线性组合生成k个不同的位置
	// 使用黄金比例常数 0x9e3779b97f4a7c15 来增加随机性
	for i := uint(0); i < b.k; i++ {
		// 线性组合：base + i * 黄金比例常数
		combined := base + uint64(i)*0x9e3779b97f4a7c15
		// 取模运算确保位置在[0, m)范围内
		results = append(results, uint(combined%uint64(b.m)))
	}
	return results
}
