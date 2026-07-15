package block

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"fmt"

	"blockchain/utils"
)

// Checkpoint, otorite node'un belirli bir blok yüksekliği için attığı imzadır.
// Konsensüs kuralı: checkpoint'in gerisindeki bloklar kesinleşmiştir; checkpoint
// ile çelişen aday zincirler ne kadar uzun olursa olsun reddedilir. Böylece ağ
// küçükken kiralık hash gücüyle yapılacak %51 saldırısı geçmişi yeniden yazamaz.
type Checkpoint struct {
	Height    int    `json:"height"`
	BlockHash string `json:"block_hash"`
	Signature string `json:"signature"`
}

// checkpointDigest, imzalanan mesajın özetini üretir
func checkpointDigest(height int, blockHash string) [32]byte {
	return sha256.Sum256([]byte(fmt.Sprintf("flatunchain-checkpoint:%d:%s", height, blockHash)))
}

// SignCheckpoint, otorite özel anahtarıyla (height, blockHash) çiftini imzalar
func SignCheckpoint(priv *ecdsa.PrivateKey, height int, blockHash string) (*Checkpoint, error) {
	h := checkpointDigest(height, blockHash)
	r, s, err := ecdsa.Sign(rand.Reader, priv, h[:])
	if err != nil {
		return nil, err
	}
	sig := &utils.Signature{R: r, S: s}
	return &Checkpoint{
		Height:    height,
		BlockHash: blockHash,
		Signature: sig.String(),
	}, nil
}

// Verify, checkpoint imzasını otorite açık anahtarıyla doğrular
func (cp *Checkpoint) Verify(pub *ecdsa.PublicKey) bool {
	if pub == nil || cp == nil {
		return false
	}
	if cp.Height < 0 || len(cp.BlockHash) != 64 || len(cp.Signature) != 128 {
		return false
	}
	sig := utils.SignatureFromString(cp.Signature)
	h := checkpointDigest(cp.Height, cp.BlockHash)
	return ecdsa.Verify(pub, h[:], sig.R, sig.S)
}
