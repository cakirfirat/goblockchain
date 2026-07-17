package utils

import (
	"fmt"
	"strconv"
	"strings"
)

// Para birimi: tüm tutarlar zincir üzerinde int64 ALT BİRİM olarak tutulur.
// 1 FLATUN = 100.000.000 alt birim (8 ondalık hane, Bitcoin'in satoshi modeli).
// float kullanılmaz — yuvarlama hatası parada kabul edilemez; toplam arz
// (~2,1e16 alt birim) float64'ün tam sayı hassasiyetini de aşar.
const UNITS_PER_FLATUN int64 = 100_000_000

const flatunDecimals = 8

// ParseFLATUN, "12.5" gibi ondalık FLATUN metnini alt birime çevirir.
// float kullanmadan, basamak basamak çözümler.
func ParseFLATUN(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("boş tutar")
	}
	neg := strings.HasPrefix(s, "-")
	if neg {
		return 0, fmt.Errorf("negatif tutar geçersiz")
	}

	intPart := s
	fracPart := ""
	if i := strings.IndexByte(s, '.'); i >= 0 {
		intPart, fracPart = s[:i], s[i+1:]
	}
	if intPart == "" {
		intPart = "0"
	}
	if len(fracPart) > flatunDecimals {
		return 0, fmt.Errorf("en fazla %d ondalık hane desteklenir", flatunDecimals)
	}

	whole, err := strconv.ParseInt(intPart, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("geçersiz tutar: %q", s)
	}

	// Kesir kısmını 8 haneye tamamla ("5" -> "50000000")
	fracPadded := fracPart + strings.Repeat("0", flatunDecimals-len(fracPart))
	var frac int64
	if fracPadded != "" {
		frac, err = strconv.ParseInt(fracPadded, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("geçersiz tutar: %q", s)
		}
	}

	if whole > (1<<62)/UNITS_PER_FLATUN {
		return 0, fmt.Errorf("tutar çok büyük")
	}
	return whole*UNITS_PER_FLATUN + frac, nil
}

// FormatFLATUN, alt birimi "12.5" gibi ondalık FLATUN metnine çevirir
// (gereksiz sıfırlar kırpılır, tam sayılar ondalıksız yazılır).
func FormatFLATUN(units int64) string {
	neg := units < 0
	if neg {
		units = -units
	}
	whole := units / UNITS_PER_FLATUN
	frac := units % UNITS_PER_FLATUN

	s := strconv.FormatInt(whole, 10)
	if frac != 0 {
		fracStr := fmt.Sprintf("%08d", frac)
		fracStr = strings.TrimRight(fracStr, "0")
		s += "." + fracStr
	}
	if neg {
		s = "-" + s
	}
	return s
}
