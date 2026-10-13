package audit

import (
	"math"
	"unicode/utf8"
)

// CalculateShannonEntropy computes the randomness of a string in bits per
// character: H(X) = -Σ P(xᵢ) log₂ P(xᵢ), over the string's runes.
func CalculateShannonEntropy(input string) float64 {
	n := utf8.RuneCountInString(input)
	if n == 0 {
		return 0
	}
	freq := make(map[rune]int, min(n, 64))
	for _, r := range input {
		freq[r]++
	}
	length := float64(n)
	var entropy float64
	for _, count := range freq {
		p := float64(count) / length
		entropy -= p * math.Log2(p)
	}
	return entropy
}

func isHex(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return s != ""
}
