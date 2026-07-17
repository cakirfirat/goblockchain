// txgen, bir cüzdan dosyasından imzalı işlem isteği (TransactionRequest JSON) üretir.
// Node API'sini test etmek için geliştirme aracıdır:
//
//	go run ./cmd/txgen --wallet=data/wallet_5001.json --to=<ADRES> --amount=1.0 | \
//	  curl -s -X POST -H "Content-Type: application/json" -d @- http://localhost:5001/transactions
package main

import (
	"blockchain/block"
	"blockchain/utils"
	"blockchain/wallet"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
)

func main() {
	walletFile := flag.String("wallet", "", "Cüzdan dosyası (private_key/public_key/blockchain_address JSON)")
	to := flag.String("to", "", "Alıcı blockchain adresi")
	amount := flag.String("amount", "1", "Gönderilecek miktar (FLATUN, örn. 1.5)")
	fee := flag.String("fee", "0", "İşlem ücreti (FLATUN)")
	flag.Parse()

	if *walletFile == "" || *to == "" {
		log.Fatal("--wallet ve --to zorunludur")
	}

	data, err := os.ReadFile(*walletFile)
	if err != nil {
		log.Fatalf("Cüzdan dosyası okunamadı: %v", err)
	}

	var w struct {
		PrivateKey        string `json:"private_key"`
		PublicKey         string `json:"public_key"`
		BlockchainAddress string `json:"blockchain_address"`
	}
	if err := json.Unmarshal(data, &w); err != nil {
		log.Fatalf("Cüzdan dosyası çözümlenemedi: %v", err)
	}

	publicKey := utils.PublicKeyFromString(w.PublicKey)
	privateKey := utils.PrivateKeyFromString(w.PrivateKey, publicKey)

	value, err := utils.ParseFLATUN(*amount)
	if err != nil {
		log.Fatalf("Geçersiz --amount: %v", err)
	}
	feeUnits, err := utils.ParseFLATUN(*fee)
	if err != nil {
		log.Fatalf("Geçersiz --fee: %v", err)
	}

	t := wallet.NewTransactionWithFee(privateKey, publicKey, w.BlockchainAddress, *to, value, feeUnits)
	signatureStr := t.GenerateSignature().String()
	timestamp := t.Timestamp()

	bt := &block.TransactionRequest{
		SenderBlockchainAddress:    &w.BlockchainAddress,
		RecipientBlockchainAddress: to,
		SenderPublicKey:            &w.PublicKey,
		Value:                      &value,
		Fee:                        &feeUnits,
		Timestamp:                  &timestamp,
		Signature:                  &signatureStr,
	}
	out, _ := json.Marshal(bt)
	fmt.Println(string(out))
}
