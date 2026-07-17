package main

import (
	"blockchain/utils"
	"flag"
	"log"
	"strings"
)

func init() {
	log.SetPrefix("Blockchain: ")
}

func main() {
	port := flag.Uint("port", 5001, "TCP Port Number for Blockchain Server")
	bootstrap := flag.String("bootstrap", "", "Bootstrap Server URL (optional)")
	useDNS := flag.Bool("dns", false, "Enable DNS seed discovery (default: false)")
	dnsSeeds := flag.String("dns-seeds", "seed.yoxar.com", "DNS seed hostname'leri (virgülle ayrılmış)")
	dataDir := flag.String("data-dir", "data", "Zincir ve cüzdan dosyalarının yazılacağı dizin")
	checkpointKey := flag.String("checkpoint-key", "", "Checkpoint otorite ÖZEL anahtarı (hex) — yalnızca otorite node'da")
	checkpointPubKey := flag.String("checkpoint-pubkey", "", "Checkpoint otorite AÇIK anahtarı (hex) — doğrulama için")
	flag.Parse()

	if *dnsSeeds != "" {
		utils.SetDNSSeeds(strings.Split(*dnsSeeds, ","))
	}

	var app *BlockchainServer

	if *bootstrap != "" {
		app = NewBlockchainServerWithBootstrap(uint16(*port), *bootstrap)
		log.Printf("Blockchain Server starting with bootstrap server: %s", *bootstrap)
	} else {
		app = NewBlockchainServer(uint16(*port))
		log.Printf("Blockchain Server starting without bootstrap server")
	}

	app.SetDataDir(*dataDir)

	// Checkpoint anahtarlarını yükle
	if *checkpointKey != "" {
		priv, err := utils.PrivateKeyFromHex(*checkpointKey)
		if err != nil {
			log.Fatalf("Checkpoint özel anahtarı çözümlenemedi: %v", err)
		}
		app.SetCheckpointKeys(priv, nil)
		log.Printf("Bu node checkpoint OTORİTESİ olarak çalışıyor")
	} else if *checkpointPubKey != "" {
		pub := utils.PublicKeyFromString(*checkpointPubKey)
		app.SetCheckpointKeys(nil, pub)
		log.Printf("Checkpoint doğrulama etkin")
	}

	// DNS seed desteğini aç
	if *useDNS {
		app.EnableDNSSeeds(true)
		log.Printf("DNS seed keşfi etkin: %v", utils.DNSSeeds)
	}

	app.Run()
}
