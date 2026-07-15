package utils

import (
	"math/big"
)

// BytesToBigInt, bayt dizisini big.Int türüne dönüştürür
func BytesToBigInt(b []byte) *big.Int {
	return new(big.Int).SetBytes(b)
}
