package utils

import (
	"math"
)

func CalculateShannonEntropy(str string) float64 {
	if len(str) == 0 {
		return 0
	}

	frequencies := make(map[rune]float64)
	for _, char := range str {
		frequencies[char]++
	}

	var entropy float64
	length := float64(len(str))
	for _, count := range frequencies {
		p := count / length
		entropy -= p * math.Log2(p)
	}

	return entropy
}
