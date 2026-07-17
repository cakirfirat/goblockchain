package wallet

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"blockchain/utils"
)

type Wallet struct {
	privateKey       *ecdsa.PrivateKey
	publicKey        *ecdsa.PublicKey
	blockchainAdress string
}

func NewWallet() *Wallet {
	w := new(Wallet)
	privateKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	w.privateKey = privateKey
	w.publicKey = &w.privateKey.PublicKey
	w.blockchainAdress = utils.AddressFromPublicKey(w.publicKey)
	return w
}

func (w *Wallet) PrivateKey() *ecdsa.PrivateKey {

	return w.privateKey

}
func (w *Wallet) PrivateKeyStr() string {

	return fmt.Sprintf("%x", w.privateKey.D.Bytes())

}

func (w *Wallet) PublicKey() *ecdsa.PublicKey {

	return w.publicKey

}
func (w *Wallet) PublicKeyStr() string {

	return fmt.Sprintf("%064x%064x", w.publicKey.X.Bytes(), w.publicKey.Y.Bytes())

}

func (w *Wallet) BlockchainAddress() string {
	return w.blockchainAdress
}

func (w *Wallet) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		PrivateKey        string `json:"private_key"`
		PublicKey         string `json:"public_key"`
		BlockchainAddress string `json:"blockchain_address"`
	}{
		PrivateKey:        w.PrivateKeyStr(),
		PublicKey:         w.PublicKeyStr(),
		BlockchainAddress: w.BlockchainAddress(),
	})
}

// Transaction: tutar int64 ALT BİRİMDİR (1 FLATUN = 1e8; bkz. utils.UNITS_PER_FLATUN)
type Transaction struct {
	senderPrivateKey           *ecdsa.PrivateKey
	senderPublicKey            *ecdsa.PublicKey
	senderBlockchainAddress    string
	recipientBlockchainAddress string
	value                      int64
	fee                        int64
	timestamp                  int64
}

func NewTransaction(privateKey *ecdsa.PrivateKey, publicKey *ecdsa.PublicKey,
	sender string, recipient string, value int64) *Transaction {
	return NewTransactionWithFee(privateKey, publicKey, sender, recipient, value, 0)
}

func NewTransactionWithFee(privateKey *ecdsa.PrivateKey, publicKey *ecdsa.PublicKey,
	sender string, recipient string, value int64, fee int64) *Transaction {
	return &Transaction{privateKey, publicKey, sender, recipient, value, fee, time.Now().UnixNano()}
}

// Timestamp, imzaya dahil edilen zaman damgası; node tarafında replay
// korumasının parçası olarak işlemi benzersizleştirir
func (t *Transaction) Timestamp() int64 {
	return t.timestamp
}

func (t *Transaction) Fee() int64 {
	return t.fee
}

func (t *Transaction) GenerateSignature() *utils.Signature {
	h := sha256.Sum256(t.signingPayload())
	r, s, _ := ecdsa.Sign(rand.Reader, t.senderPrivateKey, h[:])
	return &utils.Signature{R: r, S: s}
}

// signingPayload, block.Transaction.SigningPayload ile ALAN SIRASI DAHİL
// birebir aynı JSON'u üretmek ZORUNDADIR; imza bu çıktı üzerinden atılır
// ve node aynı çıktıyı yeniden kurup doğrular. Ağ kimliği (NetworkID)
// zincirler arası replay'i engeller.
func (t *Transaction) signingPayload() []byte {
	m, _ := json.Marshal(struct {
		Network   string `json:"network"`
		Sender    string `json:"sender_blockchain_address"`
		Recipient string `json:"recipient_blockchain_address"`
		Value     int64  `json:"value"`
		Fee       int64  `json:"fee"`
		Timestamp int64  `json:"timestamp"`
	}{
		Network:   utils.NetworkID,
		Sender:    t.senderBlockchainAddress,
		Recipient: t.recipientBlockchainAddress,
		Value:     t.value,
		Fee:       t.fee,
		Timestamp: t.timestamp,
	})
	return m
}

type TransactionRequest struct {
	SenderPrivateKey           *string `json:"sender_private_key"`
	SenderBlockchainAddress    *string `json:"sender_blockchain_address"`
	RecipientBlockchainAddress *string `json:"recipient_blockchain_address"`
	SenderPublicKey            *string `json:"sender_public_key"`
	Value                      *string `json:"value"`
}

func (tr *TransactionRequest) Validate() bool {
	if tr.SenderPrivateKey == nil ||
		tr.SenderBlockchainAddress == nil ||
		tr.RecipientBlockchainAddress == nil ||
		tr.SenderPublicKey == nil ||
		tr.Value == nil {
		return false
	}
	return true
}
