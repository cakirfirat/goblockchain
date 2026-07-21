package utils

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"encoding/hex"
	"fmt"
	"math/big"
)

type Signature struct {
	R *big.Int
	S *big.Int
}

func (s *Signature) String() string {
	return fmt.Sprintf("%064x%064x", s.R, s.S)
}

// String2BigIntTuple, 128 hex karakterlik (2x64) dizeyi iki big.Int'e çevirir.
// Girdi beklenen uzunlukta/biçimde değilse panic yerine sıfır çift döner;
// böylece halka açık uçlara gelen kısa/bozuk pubkey veya imza dizeleri
// (ör. s[:64] slice taşması) sunucu goroutine'ini düşüremez — imza
// doğrulaması sonrasında temizce başarısız olur.
func String2BigIntTuple(s string) (big.Int, big.Int) {
	var bix, biy big.Int
	if len(s) != 128 {
		return bix, biy
	}
	bx, err1 := hex.DecodeString(s[:64])
	by, err2 := hex.DecodeString(s[64:])
	if err1 != nil || err2 != nil {
		return big.Int{}, big.Int{}
	}
	bix.SetBytes(bx)
	biy.SetBytes(by)
	return bix, biy
}

func SignatureFromString(s string) *Signature {
	x, y := String2BigIntTuple(s)
	return &Signature{&x, &y}
}

func PublicKeyFromString(s string) *ecdsa.PublicKey {
	x, y := String2BigIntTuple(s)
	return &ecdsa.PublicKey{Curve: elliptic.P256(), X: &x, Y: &y}

}

func PrivateKeyFromString(s string, publicKey *ecdsa.PublicKey) *ecdsa.PrivateKey {
	b, _ := hex.DecodeString(s[:])
	var bi big.Int
	_ = bi.SetBytes(b)
	return &ecdsa.PrivateKey{PublicKey: *publicKey, D: &bi}
}

// PrivateKeyFromHex, hex özel anahtardan açık anahtarı da türeterek tam bir
// anahtar çifti oluşturur (checkpoint otorite anahtarı için)
func PrivateKeyFromHex(s string) (*ecdsa.PrivateKey, error) {
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("geçersiz özel anahtar hex: %w", err)
	}
	priv := new(ecdsa.PrivateKey)
	priv.Curve = elliptic.P256()
	priv.D = new(big.Int).SetBytes(b)
	priv.PublicKey.X, priv.PublicKey.Y = priv.Curve.ScalarBaseMult(b)
	return priv, nil
}
