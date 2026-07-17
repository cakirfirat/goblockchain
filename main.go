package main

import (
	"blockchain/block"
	"blockchain/utils"
	"blockchain/wallet"
	"fmt"
	"log"
)

func init() {
	log.SetPrefix("Blockchain: ")
}

func main() {

	walletM := wallet.NewWallet()
	walletA := wallet.NewWallet()
	walletB := wallet.NewWallet()

	//WALLET
	oneFlatun := utils.UNITS_PER_FLATUN
	t := wallet.NewTransaction(walletA.PrivateKey(), walletA.PublicKey(), walletA.BlockchainAddress(), walletB.BlockchainAddress(), oneFlatun)

	//Blockchain
	blockchain := block.NewBlockchain(walletM.BlockchainAddress(), 8080)
	isAdded := blockchain.AddTransaction(walletA.BlockchainAddress(), walletB.BlockchainAddress(), oneFlatun,
		0, t.Timestamp(), walletA.PublicKey(), t.GenerateSignature())
	fmt.Println("Added ?", isAdded)

	blockchain.Mining()
	blockchain.Print()
	fmt.Printf("A %s\n", utils.FormatFLATUN(blockchain.CalculateTotalAmount(walletA.BlockchainAddress())))
	fmt.Printf("B %s\n", utils.FormatFLATUN(blockchain.CalculateTotalAmount(walletB.BlockchainAddress())))
	fmt.Printf("M %s\n", utils.FormatFLATUN(blockchain.CalculateTotalAmount(walletM.BlockchainAddress())))

}
