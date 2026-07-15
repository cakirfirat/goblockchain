package main

import (
	"flag"
	"log"
)

func init() {
	log.SetPrefix("Blockchain: ")
}

func main() {
	port := flag.Uint("port", 5001, "TCP Port Number for Blockchain Server")
	bootstrap := flag.String("bootstrap", "", "Bootstrap Server URL (optional)")
	useDNS := flag.Bool("dns", false, "Enable DNS seed discovery (default: false)")
	flag.Parse()

	var app *BlockchainServer

	if *bootstrap != "" {
		app = NewBlockchainServerWithBootstrap(uint16(*port), *bootstrap)
		log.Printf("Blockchain Server starting with bootstrap server: %s", *bootstrap)
	} else {
		app = NewBlockchainServer(uint16(*port))
		log.Printf("Blockchain Server starting without bootstrap server")
	}

	// DNS seed desteğini aç
	if *useDNS {
		app.EnableDNSSeeds(true)
		log.Printf("DNS seed discovery enabled for flatuncoin.com")
	}

	app.Run()
}
