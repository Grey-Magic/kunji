package utils

import (
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"io"
	"math"
	"os"
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

func (bf *BloomFilter) Save(filePath string) error {
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer file.Close()

	if err := binary.Write(file, binary.LittleEndian, bf.m); err != nil {
		return err
	}
	if err := binary.Write(file, binary.LittleEndian, bf.k); err != nil {
		return err
	}
	for _, b := range bf.bitset {
		if err := binary.Write(file, binary.LittleEndian, b); err != nil {
			return err
		}
	}
	return nil
}

func LoadBloomFilter(filePath string) (*BloomFilter, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var m, k uint64
	if err := binary.Read(file, binary.LittleEndian, &m); err != nil {
		return nil, err
	}
	if err := binary.Read(file, binary.LittleEndian, &k); err != nil {
		return nil, err
	}

	bitset := make([]uint64, m/64)
	for i := range bitset {
		if err := binary.Read(file, binary.LittleEndian, &bitset[i]); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
	}

	return &BloomFilter{
		bitset: bitset,
		m:      m,
		k:      k,
	}, nil
}

func (bf *BloomFilter) hash(data string, seed uint64) uint64 {
	h := fnv.New64a()
	h.Write([]byte(data))
	val := h.Sum64()
	return val ^ (seed * 0xbf58476d1ce4e5b9)
}

func (bf *BloomFilter) Clear() {
	for i := range bf.bitset {
		bf.bitset[i] = 0
	}
}

func (bf *BloomFilter) Merge(other *BloomFilter) error {
	if bf.m != other.m || bf.k != other.k {
		return fmt.Errorf("incompatible bloom filters")
	}
	for i := range bf.bitset {
		bf.bitset[i] |= other.bitset[i]
	}
	return nil
}
