package main

import (
	"blockchain/wallet"
	"fmt"
)

func main() {
	// Yeni bir HD cüzdan oluştur
	hdWallet := wallet.NewHDWallet()

	fmt.Println("==== Yeni HD Cüzdan ====")
	fmt.Println("Mnemonic (Seed Phrase):", hdWallet.Mnemonic)
	fmt.Println("Blockchain Adresi:", hdWallet.BlockchainAddress)
	fmt.Println("Özel Anahtar:", hdWallet.PrivateKeyStr())
	fmt.Println("Açık Anahtar:", hdWallet.PublicKeyStr())

	// Aynı mnemonik ile yeni bir cüzdan oluşturarak içe aktarma işlemini test et
	importedWallet := wallet.NewHDWalletFromMnemonic(hdWallet.Mnemonic, "")

	fmt.Println("\n==== İçe Aktarılan Aynı HD Cüzdan ====")
	fmt.Println("Mnemonic (Seed Phrase):", importedWallet.Mnemonic)
	fmt.Println("Blockchain Adresi:", importedWallet.BlockchainAddress)
	fmt.Println("Özel Anahtar:", importedWallet.PrivateKeyStr())
	fmt.Println("Açık Anahtar:", importedWallet.PublicKeyStr())

	// Farklı hesaplar/yollar için cüzdan adresleri türet
	fmt.Println("\n==== BIP44 Yolları ile Türetilmiş Adresler ====")
	for i := uint32(0); i < 5; i++ {
		address, _, _ := hdWallet.DerivePath(0, 0, i)
		fmt.Printf("m/44'/0'/0'/0/%d: %s\n", i, address)
	}

	// Farklı hesaplar için adresler türet
	fmt.Println("\n==== Farklı Hesaplardan Türetilmiş Adresler ====")
	for i := uint32(0); i < 3; i++ {
		address, _, _ := hdWallet.DerivePath(i, 0, 0)
		fmt.Printf("m/44'/0'/%d'/0/0: %s\n", i, address)
	}

	// Toplu adres türetme işlemini test et
	fmt.Println("\n==== Toplu Adres Türetme ====")
	addresses := hdWallet.DeriveAddresses(0, 0, 3)
	for i, address := range addresses {
		fmt.Printf("Adres %d: %s\n", i, address)
	}
}
