package utils

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/sha256"

	"github.com/btcsuite/btcutil/base58"
	"golang.org/x/crypto/ripemd160"
)

// NetworkID, imza yüküne katılan ağ kimliği; bir ağ için imzalanan işlemin
// başka bir FlatunChain ağında (testnet vb.) tekrar oynatılmasını engeller.
const NetworkID = "flatunchain-1"

// DefaultCheckpointPubKeyHex, ana ağ checkpoint otoritesinin açık anahtarı
// (159.89.31.131 droplet'inde 17 Temmuz 2026'da üretildi). Node'lar ve
// desktop uygulaması, zincirin sahtesini gerçeğinden bu imzayla ayırır.
const DefaultCheckpointPubKeyHex = "8d66c875b609558e43676a574a2613015d9312093693ae2be5a61bd4ecb3d787ecc86570f220028e8b6ea06eb853702dfa82d9821f49df79e2518460ab0842a7"

// DefaultBootstrapURL, ağa katılım için varsayılan bootstrap sunucusu
const DefaultBootstrapURL = "http://seed.yoxar.com:8000"

// AddressFromPublicKey, açık anahtardan blockchain adresini türetir
// (SHA-256 → RIPEMD-160 → versiyon baytı + checksum → Base58).
// Cüzdan ve node aynı fonksiyonu kullanır; node bu sayede işlemdeki
// gönderici adresi ile açık anahtarın gerçekten eşleştiğini doğrular.
func AddressFromPublicKey(pub *ecdsa.PublicKey) string {
	h2 := sha256.New()
	h2.Write(pub.X.Bytes())
	h2.Write(pub.Y.Bytes())
	digest2 := h2.Sum(nil)

	h3 := ripemd160.New()
	h3.Write(digest2)
	digest3 := h3.Sum(nil)

	vd4 := make([]byte, 21)
	vd4[0] = 0x00
	copy(vd4[1:], digest3)

	h5 := sha256.Sum256(vd4)
	h6 := sha256.Sum256(h5[:])
	chsum := h6[:4]

	dc8 := make([]byte, 25)
	copy(dc8[:21], vd4)
	copy(dc8[21:], chsum)
	return base58.Encode(dc8)
}

// IsValidAddress, bir blockchain adresinin biçim ve checksum doğrulamasını yapar.
// AddressFromPublicKey ile üretilen adres 25 bayttır:
// [versiyon(1)] + [ripemd160(20)] + [checksum(4)]; checksum, ilk 21 baytın
// çift SHA-256'sının ilk 4 baytıdır. Yanlış yazılmış/bozuk bir alıcı adresine
// para gönderimi imzalanmadan reddedilir — böylece typo yüzünden para kaybı önlenir.
func IsValidAddress(address string) bool {
	if address == "" {
		return false
	}
	decoded := base58.Decode(address)
	if len(decoded) != 25 {
		return false
	}
	if decoded[0] != 0x00 {
		return false
	}
	versionedPayload := decoded[:21]
	checksum := decoded[21:]
	h1 := sha256.Sum256(versionedPayload)
	h2 := sha256.Sum256(h1[:])
	return bytes.Equal(h2[:4], checksum)
}
