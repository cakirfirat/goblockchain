package wallet

import (
	"strings"
	"testing"
)

func TestNormalizeMnemonic(t *testing.T) {
	cases := []struct{ in, want string }{
		{"  abandon  ability   able  ", "abandon ability able"},
		{"ABANDON Ability\table", "abandon ability able"},
		{"tek", "tek"},
		{"", ""},
		{"   \n\t  ", ""},
	}
	for _, c := range cases {
		if got := NormalizeMnemonic(c.in); got != c.want {
			t.Errorf("NormalizeMnemonic(%q) = %q, beklenen %q", c.in, got, c.want)
		}
	}
}

func TestValidateMnemonic(t *testing.T) {
	// Üretilen her mnemonic geçerli olmalı
	hd := NewHDWallet()
	if !ValidateMnemonic(hd.Mnemonic) {
		t.Fatal("üretilen mnemonic geçersiz sayıldı")
	}

	// Büyük harf/boşluk farkı doğrulamayı bozmamalı
	messy := "  " + strings.ToUpper(hd.Mnemonic) + "  "
	if !ValidateMnemonic(messy) {
		t.Error("normalize edilebilir mnemonic geçersiz sayıldı")
	}

	// Tek kelimelik yazım hatası (checksum) reddedilmeli
	words := strings.Fields(hd.Mnemonic)
	words[0] = "zoo" // sözlükte var ama checksum tutmaz (çok düşük olasılıkla tutabilir)
	typo := strings.Join(words, " ")
	if typo != hd.Mnemonic && ValidateMnemonic(typo) {
		t.Error("checksum'u bozuk mnemonic kabul edildi")
	}

	// Sözlük dışı kelime reddedilmeli
	words[0] = "flatunchain"
	if ValidateMnemonic(strings.Join(words, " ")) {
		t.Error("sözlük dışı kelime içeren mnemonic kabul edildi")
	}

	if ValidateMnemonic("") {
		t.Error("boş mnemonic kabul edildi")
	}
}

// TestMnemonicRoundTrip: aynı mnemonic'in normalize edilmiş ve ham hâli
// aynı adresi üretmeli (onboarding'de normalize edip saklıyoruz).
func TestMnemonicRoundTrip(t *testing.T) {
	hd := NewHDWallet()
	again := NewHDWalletFromMnemonic(NormalizeMnemonic(hd.Mnemonic), "")
	if hd.BlockchainAddress != again.BlockchainAddress {
		t.Fatalf("adres uyuşmadı: %s != %s", hd.BlockchainAddress, again.BlockchainAddress)
	}
}
