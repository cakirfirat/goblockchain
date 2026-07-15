// checkpoint_keygen, checkpoint otoritesi için ECDSA anahtar çifti üretir.
//
// Kullanım:
//
//	go run ./cmd/checkpoint_keygen
//
// Çıktıdaki ÖZEL anahtar yalnızca otorite node'a (--checkpoint-key) verilir
// ve güvenli saklanır; AÇIK anahtar tüm node'lara (--checkpoint-pubkey) dağıtılır.
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"fmt"
	"log"
)

func main() {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		log.Fatalf("Anahtar üretilemedi: %v", err)
	}

	fmt.Println("Checkpoint otorite anahtar çifti üretildi.")
	fmt.Println()
	fmt.Printf("ÖZEL anahtar (gizli tutun, yalnızca otorite node):\n  %x\n\n", priv.D.Bytes())
	fmt.Printf("AÇIK anahtar (tüm node'lara dağıtın):\n  %064x%064x\n\n", priv.PublicKey.X.Bytes(), priv.PublicKey.Y.Bytes())
	fmt.Println("Otorite node:  ./blockchain_server --checkpoint-key=<ÖZEL>")
	fmt.Println("Diğer node'lar: ./blockchain_server --checkpoint-pubkey=<AÇIK>")
}
