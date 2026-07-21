package wallet

import (
	"strings"

	"github.com/tyler-smith/go-bip39"
)

// NormalizeMnemonic, kullanıcı girdisindeki kurtarma kelimelerini kanonik
// biçime getirir: baş/son boşluklar atılır, tüm harfler küçültülür ve
// kelimeler tek boşlukla ayrılır. Doğrulama ve seed türetme her zaman bu
// normalize biçim üzerinden yapılmalıdır; aksi halde "aynı" mnemonic'in
// farklı yazımları farklı cüzdanlar üretir.
func NormalizeMnemonic(mnemonic string) string {
	return strings.Join(strings.Fields(strings.ToLower(mnemonic)), " ")
}

// ValidateMnemonic, BIP-39 sözlük ve checksum doğrulaması yapar.
// bip39.NewSeed HER metni kabul eder (checksum bakmaz); içe aktarma gibi
// giriş noktalarında bu doğrulama yapılmazsa tek harflik yazım hatası
// sessizce bambaşka (ve boş) bir cüzdan açar. Girdi önce normalize edilir.
func ValidateMnemonic(mnemonic string) bool {
	return bip39.IsMnemonicValid(NormalizeMnemonic(mnemonic))
}
