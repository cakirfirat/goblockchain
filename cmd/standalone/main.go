package main

import (
	"blockchain/block"
	"blockchain/utils"
	"blockchain/wallet"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	walletPort     = flag.Uint("wallet-port", 8080, "Cüzdan sunucusu port numarası")
	blockchainPort = flag.Uint("blockchain-port", 5001, "Blockchain sunucusu port numarası")
	blockchainPeer = flag.String("peer", "", "Blockchain peer (ip:port) [isteğe bağlı]")
	minerMode      = flag.Bool("miner", false, "Mining modunu aktifleştir")
	openBrowser    = flag.Bool("open", true, "Tarayıcıyı otomatik aç")
	dnsSeeds       = flag.String("dns-seeds", "seed.yoxar.com", "DNS seed sunucuları (virgülle ayrılmış)")
	templatesDir   = flag.String("templates", "templates", "Template dosyaları dizini")
	dataDir        = flag.String("data-dir", "data", "Zincir ve cüzdan dosyalarının yazılacağı dizin")
	walletBind     = flag.String("bind", "127.0.0.1", "Cüzdan sunucusunun bağlanacağı adres (cüzdan API'sini dışarı AÇMAYIN)")
	nodeBind       = flag.String("node-bind", "0.0.0.0", "Blockchain API'sinin bağlanacağı adres")

	bootstrapURL     = flag.String("bootstrap", utils.DefaultBootstrapURL, "Bootstrap sunucusu URL'i")
	useDNS           = flag.Bool("dns", true, "DNS seed keşfini etkinleştir")
	checkpointPubKey = flag.String("checkpoint-pubkey", utils.DefaultCheckpointPubKeyHex, "Checkpoint otorite AÇIK anahtarı (hex)")
)

// BlockchainServer yapısı
type BlockchainServer struct {
	port uint16
	p2p  *utils.P2PNetwork
}

func NewBlockchainServer(port uint16, bootstrapURL string) *BlockchainServer {
	p2p := utils.NewP2PNetwork(port, 20, bootstrapURL)
	return &BlockchainServer{port, p2p}
}

type StandaloneServer struct {
	walletServer      *WalletServer
	blockchainServer  *BlockchainServer
	blockchain        *block.Blockchain
	p2p               *utils.P2PNetwork
	walletPort        uint16
	blockchainPort    uint16
	blockchainAddress string
	walletPrivateKey  string
	walletPublicKey   string
	miner             bool
	stopChan          chan os.Signal
	wg                sync.WaitGroup
}

// loadOrCreateHDWallet, standalone node'un mining cüzdanını diskte saklar;
// yeniden başlatmada aynı mnemonic'ten aynı cüzdan yüklenir ve ödüller kaybolmaz
func loadOrCreateHDWallet(dir string) *wallet.HDWallet {
	path := filepath.Join(dir, "standalone_wallet.json")

	if data, err := os.ReadFile(path); err == nil {
		var f struct {
			Mnemonic string `json:"mnemonic"`
		}
		if err := json.Unmarshal(data, &f); err == nil && f.Mnemonic != "" {
			hd := wallet.NewHDWalletFromMnemonic(f.Mnemonic, "")
			log.Printf("Cüzdan diskten yüklendi: %s", hd.BlockchainAddress)
			return hd
		}
	}

	hd := wallet.NewHDWallet()
	data, _ := json.MarshalIndent(struct {
		Mnemonic          string `json:"mnemonic"`
		BlockchainAddress string `json:"blockchain_address"`
	}{hd.Mnemonic, hd.BlockchainAddress}, "", "  ")
	if err := os.WriteFile(path, data, 0600); err != nil {
		log.Printf("UYARI: cüzdan diske yazılamadı: %v", err)
	}
	log.Printf("Yeni cüzdan oluşturuldu: %s (kurtarma: %s)", hd.BlockchainAddress, path)
	return hd
}

func NewStandaloneServer(walletPort, blockchainPort uint16, miner bool) *StandaloneServer {
	blockchainAddr := fmt.Sprintf("http://localhost:%d", blockchainPort)

	if err := os.MkdirAll(*dataDir, 0700); err != nil {
		log.Printf("UYARI: veri dizini oluşturulamadı: %v", err)
	}

	// Cüzdan oluştur/yükle (uygulamanın mining yapmak için kullanacağı cüzdan)
	hdWallet := loadOrCreateHDWallet(*dataDir)

	// P2P katmanı: bootstrap + DNS seed keşfi ile gerçek ağa katıl
	blockchainServer := NewBlockchainServer(blockchainPort, *bootstrapURL)
	blockchainServer.p2p.SetUseDNS(*useDNS)
	if *blockchainPeer != "" {
		blockchainServer.p2p.AddNode(*blockchainPeer)
	}

	// Blockchain nesnesini oluştur (kalıcı: her blok diske yazılır)
	chainFile := filepath.Join(*dataDir, fmt.Sprintf("blockchain_%d.json", blockchainPort))
	blockchain := block.NewBlockchainWithPersistence(hdWallet.BlockchainAddress, blockchainPort, chainFile)

	// Peer'lar konsensüse katılır; checkpoint imzası zincirin kimliğini doğrular
	blockchain.SetPeerProvider(blockchainServer.p2p)
	if *checkpointPubKey != "" {
		blockchain.SetCheckpointKeys(nil, utils.PublicKeyFromString(*checkpointPubKey))
	}

	// Cüzdan sunucusu oluştur ve yerel cüzdanı bağla (sidecar endpoint'leri için)
	walletServer := NewWalletServer(walletPort, blockchainAddr)
	walletServer.SetLocalWallet(hdWallet)

	return &StandaloneServer{
		walletServer:      walletServer,
		blockchainServer:  blockchainServer,
		blockchain:        blockchain,
		p2p:               blockchainServer.p2p,
		walletPort:        walletPort,
		blockchainPort:    blockchainPort,
		blockchainAddress: hdWallet.BlockchainAddress,
		walletPrivateKey:  hdWallet.PrivateKeyStr(),
		walletPublicKey:   hdWallet.PublicKeyStr(),
		miner:             miner,
		stopChan:          make(chan os.Signal, 1),
	}
}

func (s *StandaloneServer) Run() {
	// Uygulama kapatılırken düzgün temizlik yapmak için signal handler
	signal.Notify(s.stopChan, syscall.SIGINT, syscall.SIGTERM)

	// Blockchain sunucusunu başlat
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()

		// Mining modu aktifse otomatik mining başlat
		if s.miner {
			s.blockchain.StartMining()
		}

		// Server'ı çalıştır
		mux := http.NewServeMux()

		// CORS middleware'ini tanımla
		corsHandler := func(h http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Access-Control-Allow-Origin", "*")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "*")

				if r.Method == "OPTIONS" {
					w.WriteHeader(http.StatusOK)
					return
				}

				h.ServeHTTP(w, r)
			})
		}

		// Blockchain API route'ları
		mux.HandleFunc("/chain", func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, string(s.blockchain.ChainJSON()))
		})

		mux.HandleFunc("/transactions", func(w http.ResponseWriter, req *http.Request) {
			switch req.Method {
			case http.MethodPost:
				// Cüzdan arayüzünden gelen imzalı işlem
				decoder := json.NewDecoder(req.Body)
				var t block.TransactionRequest
				if err := decoder.Decode(&t); err != nil || !t.Validate() {
					log.Printf("ERROR: geçersiz işlem isteği")
					w.WriteHeader(http.StatusBadRequest)
					io.WriteString(w, string(utils.JsonStatus("fail")))
					return
				}
				publicKey := utils.PublicKeyFromString(*t.SenderPublicKey)
				signature := utils.SignatureFromString(*t.Signature)
				isCreated := s.blockchain.CreateTransaction(*t.SenderBlockchainAddress,
					*t.RecipientBlockchainAddress, *t.Value, t.FeeOrZero(), *t.Timestamp, publicKey, signature)

				w.Header().Set("Content-Type", "application/json")
				if !isCreated {
					w.WriteHeader(http.StatusBadRequest)
					io.WriteString(w, string(utils.JsonStatus("fail")))
					return
				}
				w.WriteHeader(http.StatusCreated)
				io.WriteString(w, string(utils.JsonStatus("success")))
			case http.MethodPut:
				// Peer'dan iletilen (relay) imzalı işlem
				decoder := json.NewDecoder(req.Body)
				var t block.TransactionRequest
				if err := decoder.Decode(&t); err != nil || !t.Validate() {
					w.WriteHeader(http.StatusBadRequest)
					io.WriteString(w, string(utils.JsonStatus("fail")))
					return
				}
				publicKey := utils.PublicKeyFromString(*t.SenderPublicKey)
				signature := utils.SignatureFromString(*t.Signature)
				isAdded := s.blockchain.AddTransaction(*t.SenderBlockchainAddress,
					*t.RecipientBlockchainAddress, *t.Value, t.FeeOrZero(), *t.Timestamp, publicKey, signature)

				w.Header().Set("Content-Type", "application/json")
				if !isAdded {
					w.WriteHeader(http.StatusBadRequest)
					io.WriteString(w, string(utils.JsonStatus("fail")))
					return
				}
				io.WriteString(w, string(utils.JsonStatus("success")))
			default:
				w.Header().Set("Content-Type", "application/json")
				transactions := s.blockchain.TransactionPool()
				m, _ := json.Marshal(struct {
					Transactions []*block.Transaction `json:"transactions"`
					Length       int                  `json:"length"`
				}{
					Transactions: transactions,
					Length:       len(transactions),
				})
				io.WriteString(w, string(m[:]))
			}
		})

		mux.HandleFunc("/amount", func(w http.ResponseWriter, req *http.Request) {
			blockchainAddress := req.URL.Query().Get("blockchain_address")
			amount := s.blockchain.CalculateTotalAmount(blockchainAddress)
			ar := block.NewAmountResponse(amount)
			m, _ := json.Marshal(ar)
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, string(m[:]))
		})

		// ---- P2P endpoint'leri: tam ağ üyeliği ----
		mux.HandleFunc("/consensus", func(w http.ResponseWriter, req *http.Request) {
			if req.Method != http.MethodPut {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			if s.blockchain.ResolveConflicts() {
				io.WriteString(w, string(utils.JsonStatus("success")))
			} else {
				io.WriteString(w, string(utils.JsonStatus("fail")))
			}
		})

		mux.HandleFunc("/checkpoint", func(w http.ResponseWriter, req *http.Request) {
			cp := s.blockchain.Checkpoint()
			if cp == nil {
				w.WriteHeader(http.StatusNotFound)
				io.WriteString(w, string(utils.JsonStatus("no checkpoint")))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(cp)
		})

		mux.HandleFunc("/submit", func(w http.ResponseWriter, req *http.Request) {
			if req.Method != http.MethodPost {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			var bcResp block.Blockchain
			if err := json.NewDecoder(req.Body).Decode(&bcResp); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				io.WriteString(w, string(utils.JsonStatus("fail")))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			if s.blockchain.ConsiderChain(bcResp.Chain()) {
				io.WriteString(w, string(utils.JsonStatus("success")))
			} else {
				io.WriteString(w, string(utils.JsonStatus("not adopted")))
			}
		})

		mux.HandleFunc("/peers", func(w http.ResponseWriter, req *http.Request) {
			switch req.Method {
			case http.MethodGet:
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(s.p2p.GetActiveNodes())
			case http.MethodPost:
				var pr struct {
					Address string `json:"address"`
				}
				if err := json.NewDecoder(req.Body).Decode(&pr); err != nil || pr.Address == "" {
					w.WriteHeader(http.StatusBadRequest)
					io.WriteString(w, string(utils.JsonStatus("fail")))
					return
				}
				s.p2p.AddNode(pr.Address)
				w.WriteHeader(http.StatusCreated)
				io.WriteString(w, string(utils.JsonStatus("success")))
			default:
				w.WriteHeader(http.StatusBadRequest)
			}
		})

		mux.HandleFunc("/mine", func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			isMined := s.blockchain.Mining()

			var m []byte
			if !isMined {
				m = utils.JsonStatus("fail")
			} else {
				m = utils.JsonStatus("success")
			}
			io.WriteString(w, string(m))
		})

		mux.HandleFunc("/mine/start", func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			s.blockchain.StartMining()
			m := utils.JsonStatus("success")
			io.WriteString(w, string(m))
		})

		mux.HandleFunc("/mine/stop", func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			s.blockchain.StopMining()
			io.WriteString(w, string(utils.JsonStatus("success")))
		})

		mux.HandleFunc("/mine/status", func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			m, _ := json.Marshal(struct {
				Mining bool `json:"mining"`
				Height int  `json:"height"`
			}{
				Mining: s.blockchain.IsMining(),
				Height: len(s.blockchain.GetBlocks()) - 1,
			})
			io.WriteString(w, string(m))
		})

		// CORS middleware'ini tüm route'lara uygula
		handler := corsHandler(mux)

		// Sunucuyu başlat
		log.Fatal(http.ListenAndServe(*nodeBind+":"+strconv.Itoa(int(s.blockchainPort)), handler))
	}()
	log.Printf("Blockchain sunucusu başlatıldı (Port: %d)", s.blockchainPort)

	// Cüzdan sunucusunu başlat
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.walletServer.Run()
	}()
	log.Printf("Cüzdan sunucusu başlatıldı (Port: %d)", s.walletPort)

	// P2P keşfi ve periyodik senkronizasyon. NAT arkasındaki bir node'a ağ
	// ulaşamaz; bu çekme döngüsü + mining sonrası itme (/submit) sayesinde
	// ev kullanıcısı da tam ağ üyesi olur.
	go func() {
		s.p2p.RunNodeDiscovery(5*time.Minute,
			block.NEIGHBOR_IP_RANGE_START, block.NEIGHBOR_IP_RANGE_END,
			block.BLOCKCHAIN_PORT_RANGE_START, block.BLOCKCHAIN_PORT_RANGE_END)
		s.blockchain.ResolveConflicts()
		ticker := time.NewTicker(45 * time.Second)
		for range ticker.C {
			s.blockchain.ResolveConflicts()
		}
	}()

	// Mining modunda başlatıldıysa bilgi ver
	if s.miner {
		log.Println("Mining modu aktif - Blok üretimi otomatik başlatıldı")
	}

	// Uygulama bilgilerini yazdır
	log.Println("=========================================")
	log.Println("FlatunChain başarıyla başlatıldı")
	log.Println("Cüzdan Arayüzü: http://localhost:" + strconv.Itoa(int(s.walletPort)))
	log.Println("Blockchain API: http://localhost:" + strconv.Itoa(int(s.blockchainPort)))
	log.Println("Blockchain Adresi: " + s.blockchainAddress)
	log.Println("=========================================")

	// Tarayıcıyı aç
	if *openBrowser {
		go func() {
			// Sunucuların başlaması için biraz bekle
			time.Sleep(1 * time.Second)
			openURL(fmt.Sprintf("http://localhost:%d", s.walletPort))
		}()
	}

	// Sinyal bekle
	<-s.stopChan
	log.Println("Uygulama kapatılıyor...")

	// Sunucu goroutine'leri ListenAndServe içinde sonsuza dek bloklar;
	// onları beklemek süreci asılı bırakır (Electron sidecar'ı hayalet kalır).
	// Zincir zaten her blokta diske yazılıyor — doğrudan çıkmak güvenli.
	os.Exit(0)
}

// Tarayıcıyı açmak için yardımcı fonksiyon
func openURL(url string) {
	var err error

	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		err = fmt.Errorf("Bilinmeyen işletim sistemi: %s", runtime.GOOS)
	}

	if err != nil {
		log.Printf("Tarayıcı açılamadı: %v", err)
	}
}

// WalletServer yapısı ve metodları
type WalletServer struct {
	port     uint16
	gateway  string
	mux      *http.ServeMux
	hdWallet *wallet.HDWallet // uygulamanın yerel cüzdanı (sidecar modu için)
}

func NewWalletServer(port uint16, gateway string) *WalletServer {
	return &WalletServer{port: port, gateway: gateway, mux: http.NewServeMux()}
}

// SetLocalWallet, /wallet/info ve /wallet/send endpoint'lerinin kullanacağı
// yerel cüzdanı bağlar (Electron sidecar modu)
func (ws *WalletServer) SetLocalWallet(hd *wallet.HDWallet) {
	ws.hdWallet = hd
}

func (ws *WalletServer) Port() uint16 {
	return ws.port
}

func (ws *WalletServer) Gateway() string {
	return ws.gateway
}

func (ws *WalletServer) Index(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		t, err := http.Dir(*templatesDir).Open("index.html")
		if err != nil {
			log.Printf("ERROR: Template açılamadı - %s", err.Error())
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		defer t.Close()

		// Dosyayı oku ve istemciye gönder
		http.ServeContent(w, req, "index.html", time.Now(), t)
	default:
		log.Printf("ERROR: Geçersiz HTTP Metodu")
	}
}

func (ws *WalletServer) Wallet(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodPost:
		w.Header().Add("Content-Type", "application/json")

		// HD cüzdan veya normal cüzdan oluşturup oluşturmama durumu
		walletType := req.URL.Query().Get("type")
		var walletData []byte
		var err error

		if walletType == "hd" {
			// HD cüzdan oluşturma
			hdWallet := wallet.NewHDWallet()
			walletData, err = hdWallet.MarshalJSON()
		} else {
			// Standart cüzdan oluşturma (geriye dönük uyumluluk)
			myWallet := wallet.NewWallet()
			walletData, err = myWallet.MarshalJSON()
		}

		if err != nil {
			log.Printf("ERROR: Wallet marshal failed - %s", err.Error())
			io.WriteString(w, string(utils.JsonStatus("fail")))
			return
		}

		io.WriteString(w, string(walletData[:]))
	default:
		w.WriteHeader(http.StatusBadRequest)
		log.Println("ERROR: Invalid http method")
	}
}

// ImportHDWallet, mevcut bir seed phrase ile HD cüzdan oluşturma
func (ws *WalletServer) ImportHDWallet(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodPost:
		w.Header().Add("Content-Type", "application/json")

		// İstek gövdesini ayrıştır
		decoder := json.NewDecoder(req.Body)
		var hdWalletRequest struct {
			Mnemonic   string `json:"mnemonic"`
			Passphrase string `json:"passphrase,omitempty"`
		}

		err := decoder.Decode(&hdWalletRequest)
		if err != nil {
			log.Printf("ERROR: %v", err)
			io.WriteString(w, string(utils.JsonStatus("fail")))
			return
		}

		// Mnemonic boş olmamalı
		if hdWalletRequest.Mnemonic == "" {
			log.Println("ERROR: Mnemonic is required")
			io.WriteString(w, string(utils.JsonStatus("fail")))
			return
		}

		// HD cüzdan oluştur
		hdWallet := wallet.NewHDWalletFromMnemonic(hdWalletRequest.Mnemonic, hdWalletRequest.Passphrase)
		walletData, err := hdWallet.MarshalJSON()

		if err != nil {
			log.Printf("ERROR: Wallet marshal failed - %s", err.Error())
			io.WriteString(w, string(utils.JsonStatus("fail")))
			return
		}

		io.WriteString(w, string(walletData[:]))
	default:
		w.WriteHeader(http.StatusBadRequest)
		log.Println("ERROR: Invalid HTTP Method")
	}
}

// DeriveAddresses, bir HD cüzdandan belirli sayıda adres türetme
func (ws *WalletServer) DeriveAddresses(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodPost:
		w.Header().Add("Content-Type", "application/json")

		// İstek gövdesini ayrıştır
		decoder := json.NewDecoder(req.Body)
		var deriveRequest struct {
			Mnemonic     string `json:"mnemonic"`
			Passphrase   string `json:"passphrase,omitempty"`
			Account      uint32 `json:"account,omitempty"`
			Change       uint32 `json:"change,omitempty"`
			AddressCount uint32 `json:"address_count"`
		}

		err := decoder.Decode(&deriveRequest)
		if err != nil {
			log.Printf("ERROR: %v", err)
			io.WriteString(w, string(utils.JsonStatus("fail")))
			return
		}

		// Mnemonic boş olmamalı
		if deriveRequest.Mnemonic == "" {
			log.Println("ERROR: Mnemonic is required")
			io.WriteString(w, string(utils.JsonStatus("fail")))
			return
		}

		// Adres sayısı default 1, maksimum 20 olabilir
		if deriveRequest.AddressCount == 0 {
			deriveRequest.AddressCount = 1
		} else if deriveRequest.AddressCount > 20 {
			deriveRequest.AddressCount = 20
		}

		// HD cüzdan oluştur
		hdWallet := wallet.NewHDWalletFromMnemonic(deriveRequest.Mnemonic, deriveRequest.Passphrase)

		// Belirlenen sayıda adres türet
		addresses := hdWallet.DeriveAddresses(deriveRequest.Account, deriveRequest.Change, deriveRequest.AddressCount)

		// Yanıt oluştur
		response, err := json.Marshal(struct {
			Message   string   `json:"message"`
			Addresses []string `json:"addresses"`
		}{
			Message:   "success",
			Addresses: addresses,
		})

		if err != nil {
			log.Printf("ERROR: Response marshal failed - %s", err.Error())
			io.WriteString(w, string(utils.JsonStatus("fail")))
			return
		}

		io.WriteString(w, string(response[:]))
	default:
		w.WriteHeader(http.StatusBadRequest)
		log.Println("ERROR: Invalid HTTP Method")
	}
}

func (ws *WalletServer) CreateTransaction(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodPost:
		decoder := json.NewDecoder(req.Body)
		var t wallet.TransactionRequest
		err := decoder.Decode(&t)
		if err != nil {
			log.Printf("ERROR: %v", err)
			io.WriteString(w, string(utils.JsonStatus("fail")))
			return
		}
		if !t.Validate() {
			log.Println("ERROR: missing field(s)")
			io.WriteString(w, string(utils.JsonStatus("fail")))
			return
		}

		publicKey := utils.PublicKeyFromString(*t.SenderPublicKey)
		privateKey := utils.PrivateKeyFromString(*t.SenderPrivateKey, publicKey)
		valueUnits, err := utils.ParseFLATUN(*t.Value)
		if err != nil {
			log.Printf("ERROR: tutar çözümlenemedi: %v", err)
			io.WriteString(w, string(utils.JsonStatus("fail")))
			return
		}

		w.Header().Add("Content-Type", "application/json")

		transaction := wallet.NewTransaction(privateKey, publicKey,
			*t.SenderBlockchainAddress, *t.RecipientBlockchainAddress, valueUnits)
		signature := transaction.GenerateSignature()
		signatureStr := signature.String()
		timestamp := transaction.Timestamp()
		fee := transaction.Fee()

		bt := &block.TransactionRequest{
			SenderBlockchainAddress:    t.SenderBlockchainAddress,
			RecipientBlockchainAddress: t.RecipientBlockchainAddress,
			SenderPublicKey:            t.SenderPublicKey,
			Value:                      &valueUnits,
			Fee:                        &fee,
			Timestamp:                  &timestamp,
			Signature:                  &signatureStr,
		}
		m, _ := json.Marshal(bt)
		buf := bytes.NewBuffer(m)
		resp, err := http.Post(ws.Gateway()+"/transactions", "application/json", buf)
		if err != nil {
			log.Printf("ERROR: %v", err)
			io.WriteString(w, string(utils.JsonStatus("fail")))
			return
		}
		if resp.StatusCode == 201 {
			io.WriteString(w, string(utils.JsonStatus("success")))
			return
		}
		io.WriteString(w, string(utils.JsonStatus("fail")))
	default:
		w.WriteHeader(http.StatusBadRequest)
		log.Println("ERROR: Invalid HTTP Method")
	}
}

// CreateHDTransaction, HD cüzdan kullanarak işlem oluşturma
func (ws *WalletServer) CreateHDTransaction(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodPost:
		decoder := json.NewDecoder(req.Body)
		var t struct {
			Mnemonic                   string `json:"mnemonic"`
			Passphrase                 string `json:"passphrase,omitempty"`
			RecipientBlockchainAddress string `json:"recipient_blockchain_address"`
			Value                      string `json:"value"`
		}

		err := decoder.Decode(&t)
		if err != nil {
			log.Printf("ERROR: %v", err)
			io.WriteString(w, string(utils.JsonStatus("fail")))
			return
		}

		// Mnemonic ve alıcı adresi boş olmamalı
		if t.Mnemonic == "" || t.RecipientBlockchainAddress == "" {
			log.Println("ERROR: missing mnemonic or recipient address")
			io.WriteString(w, string(utils.JsonStatus("fail")))
			return
		}

		valueUnits, err := utils.ParseFLATUN(t.Value)
		if err != nil {
			log.Printf("ERROR: tutar çözümlenemedi: %v", err)
			io.WriteString(w, string(utils.JsonStatus("fail")))
			return
		}

		// HD cüzdan oluştur
		hdWallet := wallet.NewHDWalletFromMnemonic(t.Mnemonic, t.Passphrase)

		// İşlem oluştur
		transaction := hdWallet.CreateTransaction(t.RecipientBlockchainAddress, valueUnits)
		signature := transaction.GenerateSignature()
		signatureStr := signature.String()

		// Blockchain sunucusuna gönder
		pubKeyStr := hdWallet.PublicKeyStr()
		timestamp := transaction.Timestamp()
		fee := transaction.Fee()
		bt := &block.TransactionRequest{
			SenderBlockchainAddress:    &hdWallet.BlockchainAddress,
			RecipientBlockchainAddress: &t.RecipientBlockchainAddress,
			SenderPublicKey:            &pubKeyStr,
			Value:                      &valueUnits,
			Fee:                        &fee,
			Timestamp:                  &timestamp,
			Signature:                  &signatureStr,
		}

		m, _ := json.Marshal(bt)
		buf := bytes.NewBuffer(m)
		resp, err := http.Post(ws.Gateway()+"/transactions", "application/json", buf)
		if err != nil {
			log.Printf("ERROR: %v", err)
			io.WriteString(w, string(utils.JsonStatus("fail")))
			return
		}

		w.Header().Add("Content-Type", "application/json")
		if resp.StatusCode == 201 {
			io.WriteString(w, string(utils.JsonStatus("success")))
			return
		}
		io.WriteString(w, string(utils.JsonStatus("fail")))
	default:
		w.WriteHeader(http.StatusBadRequest)
		log.Println("ERROR: Invalid HTTP Method")
	}
}

func (ws *WalletServer) WalletAmount(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		blockchainAddress := req.URL.Query().Get("blockchain_address")
		endpoint := fmt.Sprintf("%s/amount", ws.Gateway())

		client := &http.Client{}
		bcsReq, _ := http.NewRequest("GET", endpoint, nil)
		q := bcsReq.URL.Query()
		q.Add("blockchain_address", blockchainAddress)
		bcsReq.URL.RawQuery = q.Encode()

		bcsResp, err := client.Do(bcsReq)
		if err != nil {
			log.Printf("ERROR: %v", err)
			io.WriteString(w, string(utils.JsonStatus("fail")))
			return
		}

		w.Header().Add("Content-Type", "application/json")
		if bcsResp.StatusCode == 200 {
			decoder := json.NewDecoder(bcsResp.Body)
			var bar block.AmountResponse
			err := decoder.Decode(&bar)
			if err != nil {
				log.Printf("ERROR: %v", err)
				io.WriteString(w, string(utils.JsonStatus("fail")))
				return
			}

			m, _ := json.Marshal(struct {
				Message     string `json:"message"`
				Amount      string `json:"amount"`
				AmountUnits int64  `json:"amount_units"`
			}{
				Message:     "success",
				Amount:      bar.Amount,
				AmountUnits: bar.AmountUnits,
			})
			io.WriteString(w, string(m[:]))
		} else {
			io.WriteString(w, string(utils.JsonStatus("fail")))
		}
	default:
		log.Printf("ERROR: Invalid HTTP Method")
		w.WriteHeader(http.StatusBadRequest)
	}
}

func (ws *WalletServer) setupRoutes() {
	// CORS middleware'ini cüzdan sunucusu için tanımla
	corsMiddleware := func(handler http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")

			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}

			handler(w, r)
		}
	}

	// Route'ları CORS middleware ile birlikte ayarla
	ws.mux.HandleFunc("/", corsMiddleware(ws.Index))
	ws.mux.HandleFunc("/wallet", corsMiddleware(ws.Wallet))
	ws.mux.HandleFunc("/wallet/amount", corsMiddleware(ws.WalletAmount))
	ws.mux.HandleFunc("/transaction", corsMiddleware(ws.CreateTransaction))

	// HD cüzdan endpointleri
	ws.mux.HandleFunc("/wallet/hd/import", corsMiddleware(ws.ImportHDWallet))
	ws.mux.HandleFunc("/wallet/hd/derive", corsMiddleware(ws.DeriveAddresses))
	ws.mux.HandleFunc("/transaction/hd", corsMiddleware(ws.CreateHDTransaction))

	// Blockchain API için proxy ekle
	ws.mux.HandleFunc("/blockchain/", corsMiddleware(ws.BlockchainProxy))

	// Sidecar (Electron) endpoint'leri — yalnızca yerel cüzdan bağlıysa
	ws.mux.HandleFunc("/wallet/info", corsMiddleware(ws.LocalWalletInfo))
	ws.mux.HandleFunc("/wallet/send", corsMiddleware(ws.LocalWalletSend))
}

// LocalWalletInfo, uygulamanın yerel cüzdan kimliğini döndürür (mnemonic ASLA dönmez)
func (ws *WalletServer) LocalWalletInfo(w http.ResponseWriter, req *http.Request) {
	if ws.hdWallet == nil {
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, string(utils.JsonStatus("no local wallet")))
		return
	}
	w.Header().Add("Content-Type", "application/json")
	m, _ := json.Marshal(struct {
		BlockchainAddress string `json:"blockchain_address"`
		PublicKey         string `json:"public_key"`
	}{
		BlockchainAddress: ws.hdWallet.BlockchainAddress,
		PublicKey:         ws.hdWallet.PublicKeyStr(),
	})
	io.WriteString(w, string(m))
}

// LocalWalletSend, yerel cüzdandan imzalı işlem oluşturup node'a iletir.
// Cüzdan sunucusu 127.0.0.1'e bağlı olduğu sürece bu endpoint yalnızca
// aynı makinedeki uygulamadan çağrılabilir.
func (ws *WalletServer) LocalWalletSend(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if ws.hdWallet == nil {
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, string(utils.JsonStatus("no local wallet")))
		return
	}

	var body struct {
		Recipient string `json:"recipient"`
		Value     string `json:"value"` // FLATUN metni, örn. "1.5"
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil || body.Recipient == "" {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, string(utils.JsonStatus("fail")))
		return
	}
	valueUnits, err := utils.ParseFLATUN(body.Value)
	if err != nil || valueUnits <= 0 {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, string(utils.JsonStatus("invalid amount")))
		return
	}

	transaction := ws.hdWallet.CreateTransaction(body.Recipient, valueUnits)
	signatureStr := transaction.GenerateSignature().String()
	pubKeyStr := ws.hdWallet.PublicKeyStr()
	timestamp := transaction.Timestamp()
	fee := transaction.Fee()

	bt := &block.TransactionRequest{
		SenderBlockchainAddress:    &ws.hdWallet.BlockchainAddress,
		RecipientBlockchainAddress: &body.Recipient,
		SenderPublicKey:            &pubKeyStr,
		Value:                      &valueUnits,
		Fee:                        &fee,
		Timestamp:                  &timestamp,
		Signature:                  &signatureStr,
	}
	m, _ := json.Marshal(bt)
	resp, err := http.Post(ws.Gateway()+"/transactions", "application/json", bytes.NewBuffer(m))
	if err != nil {
		log.Printf("ERROR: node'a iletilemedi: %v", err)
		w.WriteHeader(http.StatusBadGateway)
		io.WriteString(w, string(utils.JsonStatus("fail")))
		return
	}
	defer resp.Body.Close()

	w.Header().Add("Content-Type", "application/json")
	if resp.StatusCode == http.StatusCreated {
		io.WriteString(w, string(utils.JsonStatus("success")))
		return
	}
	w.WriteHeader(http.StatusBadRequest)
	io.WriteString(w, string(utils.JsonStatus("fail")))
}

// BlockchainProxy, blockchain API'sine gelen istekleri yönlendirir
func (ws *WalletServer) BlockchainProxy(w http.ResponseWriter, req *http.Request) {
	// Blockchain API endpoint'i
	path := strings.TrimPrefix(req.URL.Path, "/blockchain")
	blockchainUrl := ws.Gateway() + path

	// URL parametrelerini ekle
	if req.URL.RawQuery != "" {
		blockchainUrl += "?" + req.URL.RawQuery
	}

	log.Printf("Proxy istek: %s -> %s (Method: %s)", req.URL.Path, blockchainUrl, req.Method)

	// İsteği blockchain sunucusuna yönlendir
	client := &http.Client{}

	// Yeni istek oluştur, body kopyala
	proxyReq, err := http.NewRequest(req.Method, blockchainUrl, req.Body)
	if err != nil {
		log.Printf("Proxy istek oluşturulamadı: %v", err)
		http.Error(w, "Proxy istek oluşturulamadı", http.StatusInternalServerError)
		return
	}

	// Header'ları kopyala
	for name, values := range req.Header {
		for _, value := range values {
			proxyReq.Header.Add(name, value)
		}
	}

	// İsteği gönder
	resp, err := client.Do(proxyReq)
	if err != nil {
		log.Printf("Blockchain sunucusuna erişilemiyor: %v", err)
		http.Error(w, "Blockchain sunucusuna erişilemiyor", http.StatusServiceUnavailable)
		return
	}
	defer resp.Body.Close()

	// Status kodunu ayarla
	w.WriteHeader(resp.StatusCode)

	// Body'yi kopyala
	io.Copy(w, resp.Body)
}

func (ws *WalletServer) Run() {
	ws.setupRoutes()
	log.Fatal(http.ListenAndServe(*walletBind+":"+strconv.Itoa(int(ws.Port())), ws.mux))
}

func main() {
	flag.Parse()

	if *dnsSeeds != "" {
		utils.SetDNSSeeds(strings.Split(*dnsSeeds, ","))
	}

	// Standalone sunucuyu oluştur ve başlat
	server := NewStandaloneServer(uint16(*walletPort), uint16(*blockchainPort), *minerMode)

	log.Println("FlatunChain başlatılıyor...")
	server.Run()
}
