package wallet

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math/big"

	"blockchain/utils"

	"github.com/tyler-smith/go-bip32"
	"github.com/tyler-smith/go-bip39"
)

// HDWallet, BIP-32, BIP-39 ve BIP-44 standartlarına uygun Hierarchical Deterministic (HD) cüzdan yapısı
type HDWallet struct {
	Seed              []byte            `json:"seed"`
	Mnemonic          string            `json:"mnemonic"`
	MasterKey         *bip32.Key        `json:"-"`
	PrivateKey        *ecdsa.PrivateKey `json:"-"`
	PublicKey         *ecdsa.PublicKey  `json:"-"`
	BlockchainAddress string            `json:"blockchain_address"`
}

// NewHDWallet, yeni bir HD cüzdan oluşturur
func NewHDWallet() *HDWallet {
	// 1. BIP-39 için entropy oluşturup seed phrase (mnemonic) türetme
	entropy, err := bip39.NewEntropy(256) // 256 bit entropi (24 kelime)
	if err != nil {
		log.Panic(err)
	}

	mnemonic, err := bip39.NewMnemonic(entropy)
	if err != nil {
		log.Panic(err)
	}

	return NewHDWalletFromMnemonic(mnemonic, "")
}

// NewHDWalletFromMnemonic, mevcut bir BIP-39 mnemonik cümlesinden HD cüzdan oluşturur
func NewHDWalletFromMnemonic(mnemonic string, passphrase string) *HDWallet {
	seed := bip39.NewSeed(mnemonic, passphrase)

	// 2. BIP-32 kullanarak HD cüzdan kök anahtarı oluştur
	masterKey, err := bip32.NewMasterKey(seed)
	if err != nil {
		log.Panic(err)
	}

	// 3. İlk adres için özel anahtar türet (BIP-44 path: m/44'/0'/0'/0/0)
	// Not: Burada basitlik için ilk yolu (m/44'/0'/0'/0/0) kullanıyoruz
	// Tam BIP-44 desteği için yol işlemeyi genişletebiliriz
	purposeKey, err := masterKey.NewChildKey(bip32.FirstHardenedChild + 44) // 44' (hardened)
	if err != nil {
		log.Panic(err)
	}

	coinTypeKey, err := purposeKey.NewChildKey(bip32.FirstHardenedChild + 0) // 0' (Bitcoin)
	if err != nil {
		log.Panic(err)
	}

	accountKey, err := coinTypeKey.NewChildKey(bip32.FirstHardenedChild + 0) // 0' (İlk hesap)
	if err != nil {
		log.Panic(err)
	}

	changeKey, err := accountKey.NewChildKey(0) // 0 (external chain)
	if err != nil {
		log.Panic(err)
	}

	addressKey, err := changeKey.NewChildKey(0) // 0 (ilk adres indeksi)
	if err != nil {
		log.Panic(err)
	}

	// ECDSA özel anahtarı BIP-32 özel anahtarından oluştur
	prvKey, pubKey := btcKeyToEcdsa(addressKey)

	// Blockchain adresi oluştur (node ile aynı türetme: utils.AddressFromPublicKey)
	address := utils.AddressFromPublicKey(pubKey)

	return &HDWallet{
		Seed:              seed,
		Mnemonic:          mnemonic,
		MasterKey:         masterKey,
		PrivateKey:        prvKey,
		PublicKey:         pubKey,
		BlockchainAddress: address,
	}
}

// PrivateKeyStr, özel anahtarı hexadecimal string olarak döndürür
func (w *HDWallet) PrivateKeyStr() string {
	return fmt.Sprintf("%x", w.PrivateKey.D.Bytes())
}

// PublicKeyStr, açık anahtarı hexadecimal string olarak döndürür
func (w *HDWallet) PublicKeyStr() string {
	return fmt.Sprintf("%064x%064x", w.PublicKey.X.Bytes(), w.PublicKey.Y.Bytes())
}

// DerivePath, belirli bir BIP-44 yolundan yeni bir özel/açık anahtar çifti ve adres türetir
func (w *HDWallet) DerivePath(account, change, addressIndex uint32) (string, *ecdsa.PrivateKey, *ecdsa.PublicKey) {
	purposeKey, _ := w.MasterKey.NewChildKey(bip32.FirstHardenedChild + 44)
	coinTypeKey, _ := purposeKey.NewChildKey(bip32.FirstHardenedChild + 0)
	accountKey, _ := coinTypeKey.NewChildKey(bip32.FirstHardenedChild + account)
	changeKey, _ := accountKey.NewChildKey(change)
	addressKey, _ := changeKey.NewChildKey(addressIndex)

	prvKey, pubKey := btcKeyToEcdsa(addressKey)
	address := utils.AddressFromPublicKey(pubKey)

	return address, prvKey, pubKey
}

// MarshalJSON, HDWallet nesnesini JSON formatına dönüştürür
func (w *HDWallet) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Mnemonic          string `json:"mnemonic"`
		PrivateKey        string `json:"private_key"`
		PublicKey         string `json:"public_key"`
		BlockchainAddress string `json:"blockchain_address"`
	}{
		Mnemonic:          w.Mnemonic,
		PrivateKey:        w.PrivateKeyStr(),
		PublicKey:         w.PublicKeyStr(),
		BlockchainAddress: w.BlockchainAddress,
	})
}

// btcKeyToEcdsa, BIP-32 anahtar baytlarını P-256 ECDSA anahtar çiftine dönüştürür.
//
// NOT (bilinçli tasarım kararı, Temmuz 2026): FlatunChain P-256 eğrisinde
// standartlaşmıştır. BIP-32 türetmesinden gelen baytlar deterministik tohum
// olarak kullanılır ve P-256 mertebesine indirgenir (d ∈ [1, N-1] garanti);
// mnemonic yalnızca FlatunChain cüzdanlarında geri yüklenebilir, bu kabul
// edilmiş bir sınırlamadır.
func btcKeyToEcdsa(key *bip32.Key) (*ecdsa.PrivateKey, *ecdsa.PublicKey) {
	curve := elliptic.P256()

	// d = (bytes mod (N-1)) + 1 — geçersiz (0 veya ≥N) anahtar imkânsız hale gelir
	d := utils.BytesToBigInt(key.Key)
	nMinusOne := new(big.Int).Sub(curve.Params().N, big.NewInt(1))
	d.Mod(d, nMinusOne)
	d.Add(d, big.NewInt(1))

	privateKey := new(ecdsa.PrivateKey)
	privateKey.PublicKey.Curve = curve
	privateKey.D = d
	privateKey.PublicKey.X, privateKey.PublicKey.Y = curve.ScalarBaseMult(d.Bytes())
	return privateKey, &privateKey.PublicKey
}

// DeriveAddresses, belirli bir hesap için birden fazla adres türetir
func (w *HDWallet) DeriveAddresses(account, change, count uint32) []string {
	addresses := make([]string, count)
	for i := uint32(0); i < count; i++ {
		address, _, _ := w.DerivePath(account, change, i)
		addresses[i] = address
	}
	return addresses
}

// GetSeedHex, seed'i hexadecimal string olarak döndürür
func (w *HDWallet) GetSeedHex() string {
	return hex.EncodeToString(w.Seed)
}

// CreateTransaction, HD cüzdan kullanarak işlem oluşturur (value: alt birim)
func (w *HDWallet) CreateTransaction(recipientAddress string, value int64) *Transaction {
	return NewTransaction(w.PrivateKey, w.PublicKey, w.BlockchainAddress, recipientAddress, value)
}
