package utils

import (
	"crypto/ecdsa"
	"crypto/sha256"

	"github.com/btcsuite/btcutil/base58"
	"golang.org/x/crypto/ripemd160"
)

// NetworkID, imza yüküne katılan ağ kimliği; bir ağ için imzalanan işlemin
// başka bir FlatunChain ağında (testnet vb.) tekrar oynatılmasını engeller.
const NetworkID = "flatunchain-1"

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
