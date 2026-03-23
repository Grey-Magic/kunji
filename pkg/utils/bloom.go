package utils

import (
	"hash/fnv"
	"math"
)

type BloomFilter struct {
	bitset []uint64
	m      uint64
	k      uint64
}

func NewBloomFilter(n uint64, p float64) *BloomFilter {
	m := uint64(math.Ceil(-float64(n) * math.Log(p) / math.Pow(math.Log(2), 2)))
	k := uint64(math.Ceil(float64(m) / float64(n) * math.Log(2)))

	m = (m + 63) / 64 * 64

	return &BloomFilter{
		bitset: make([]uint64, m/64),
		m:      m,
		k:      k,
	}
}

func (bf *BloomFilter) Add(data string) {
	for i := uint64(0); i < bf.k; i++ {
		idx := bf.hash(data, i) % bf.m
		bf.bitset[idx/64] |= (1 << (idx % 64))
	}
}

func (bf *BloomFilter) Test(data string) bool {
	for i := uint64(0); i < bf.k; i++ {
		idx := bf.hash(data, i) % bf.m
		if (bf.bitset[idx/64] & (1 << (idx % 64))) == 0 {
			return false
		}
	}
	return true
}

func (bf *BloomFilter) hash(data string, seed uint64) uint64 {
	h := fnv.New64a()
	h.Write([]byte(data))
	val := h.Sum64()
	return val ^ (seed * 0xbf58476d1ce4e5b9)
}
