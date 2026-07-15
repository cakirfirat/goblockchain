package main

import (
	"blockchain/block"
	"blockchain/utils"
	"blockchain/wallet"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"
)

var cache map[string]*block.Blockchain = make(map[string]*block.Blockchain)

type BlockchainServer struct {
	port uint16
	p2p  *utils.P2PNetwork
}

// CORS başlıklarını ekleyen yardımcı fonksiyon
func setCORSHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
}

func NewBlockchainServer(port uint16) *BlockchainServer {
	// Bootstrap URL varsayılan olarak boş
	p2p := utils.NewP2PNetwork(port, 20, "")
	return &BlockchainServer{port, p2p}
}

func NewBlockchainServerWithBootstrap(port uint16, bootstrapURL string) *BlockchainServer {
	p2p := utils.NewP2PNetwork(port, 20, bootstrapURL)
	return &BlockchainServer{port, p2p}
}

func (bcs *BlockchainServer) Port() uint16 {
	return bcs.port
}

func (bcs *BlockchainServer) GetBlockchain() *block.Blockchain {
	bc, ok := cache["blockchain"]
	if !ok {
		minersWallet := wallet.NewWallet()
		bc = block.NewBlockchain(minersWallet.BlockchainAddress(), bcs.Port())
		cache["blockchain"] = bc
		log.Printf("private_key %v", minersWallet.PrivateKeyStr())
		log.Printf("publick_key %v", minersWallet.PublicKeyStr())
		log.Printf("blockchain_address %v", minersWallet.BlockchainAddress())
		//message := "Private Key" + minersWallet.PrivateKeyStr() + "Public Key" + minersWallet.PublicKeyStr() + "Blockchain Address" + minersWallet.BlockchainAddress()
		// utils.SendSms("5396655843", "message")
	}
	return bc
}

func (bcs *BlockchainServer) GetChain(w http.ResponseWriter, req *http.Request) {
	// CORS başlıklarını ekle
	setCORSHeaders(w)

	// OPTIONS metoduna yanıt ver
	if req.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	switch req.Method {
	case http.MethodGet:
		w.Header().Add("Content-Type", "application/json")
		bc := bcs.GetBlockchain()
		m, _ := bc.MarshalJSON()
		io.WriteString(w, string(m[:]))
	default:
		log.Printf("ERROR: Invalid HTTP Method")

	}
}

func (bcs *BlockchainServer) Transactions(w http.ResponseWriter, req *http.Request) {
	// CORS başlıklarını ekle
	setCORSHeaders(w)

	// OPTIONS metoduna yanıt ver
	if req.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	switch req.Method {
	case http.MethodGet:
		w.Header().Add("Content-Type", "application/json")
		bc := bcs.GetBlockchain()
		transactions := bc.TransactionPool()
		m, _ := json.Marshal(struct {
			Transactions []*block.Transaction `json:"transactions"`
			Length       int                  `json:"length"`
		}{
			Transactions: transactions,
			Length:       len(transactions),
		})
		io.WriteString(w, string(m[:]))

	case http.MethodPost:
		decoder := json.NewDecoder(req.Body)
		var t block.TransactionRequest
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
		signature := utils.SignatureFromString(*t.Signature)
		bc := bcs.GetBlockchain()
		isCreated := bc.CreateTransaction(*t.SenderBlockchainAddress,
			*t.RecipientBlockchainAddress, *t.Value, publicKey, signature)

		w.Header().Add("Content-Type", "application/json")
		var m []byte
		if !isCreated {
			w.WriteHeader(http.StatusBadRequest)
			m = utils.JsonStatus("fail")
		} else {
			w.WriteHeader(http.StatusCreated)
			m = utils.JsonStatus("success")
		}
		io.WriteString(w, string(m))
	case http.MethodPut:
		decoder := json.NewDecoder(req.Body)
		var t block.TransactionRequest
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
		signature := utils.SignatureFromString(*t.Signature)
		bc := bcs.GetBlockchain()
		isUpdated := bc.AddTransaction(*t.SenderBlockchainAddress,
			*t.RecipientBlockchainAddress, *t.Value, publicKey, signature)

		w.Header().Add("Content-Type", "application/json")
		var m []byte
		if !isUpdated {
			w.WriteHeader(http.StatusBadRequest)
			m = utils.JsonStatus("fail")
		} else {
			m = utils.JsonStatus("success")
		}
		io.WriteString(w, string(m))
	case http.MethodDelete:
		bc := bcs.GetBlockchain()
		bc.ClearTransactionPool()
		io.WriteString(w, string(utils.JsonStatus("success")))
	default:
		log.Println("ERROR: Invalid HTTP Method")
		w.WriteHeader(http.StatusBadRequest)
	}
}

func (bcs *BlockchainServer) Mine(w http.ResponseWriter, req *http.Request) {
	// CORS başlıklarını ekle
	setCORSHeaders(w)

	// OPTIONS metoduna yanıt ver
	if req.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	switch req.Method {
	case http.MethodGet:
		bc := bcs.GetBlockchain()
		isMined := bc.Mining()

		var m []byte
		if !isMined {
			w.WriteHeader(http.StatusBadRequest)
			m = utils.JsonStatus("fail")
		} else {
			m = utils.JsonStatus("success")
		}
		w.Header().Add("Content-Type", "application/json")
		io.WriteString(w, string(m))
	default:
		log.Println("ERROR: Invalid HTTP Method")
		w.WriteHeader(http.StatusBadRequest)
	}
}

func (bcs *BlockchainServer) StartMine(w http.ResponseWriter, req *http.Request) {
	// CORS başlıklarını ekle
	setCORSHeaders(w)

	// OPTIONS metoduna yanıt ver
	if req.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	switch req.Method {
	case http.MethodGet:
		bc := bcs.GetBlockchain()
		bc.StartMining()

		m := utils.JsonStatus("success")
		w.Header().Add("Content-Type", "application/json")
		io.WriteString(w, string(m))
	default:
		log.Println("ERROR: Invalid HTTP Method")
		w.WriteHeader(http.StatusBadRequest)
	}
}

func (bcs *BlockchainServer) Amount(w http.ResponseWriter, req *http.Request) {
	// CORS başlıklarını ekle
	setCORSHeaders(w)

	// OPTIONS metoduna yanıt ver
	if req.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	switch req.Method {
	case http.MethodGet:
		blockchainAddress := req.URL.Query().Get("blockchain_address")
		amount := bcs.GetBlockchain().CalculateTotalAmount(blockchainAddress)

		ar := &block.AmountResponse{amount}
		m, _ := ar.MarshalJSON()

		w.Header().Add("Content-Type", "application/json")
		io.WriteString(w, string(m[:]))

	default:
		log.Printf("ERROR: Invalid HTTP Method")
		w.WriteHeader(http.StatusBadRequest)
	}
}

func (bcs *BlockchainServer) Consensus(w http.ResponseWriter, req *http.Request) {
	// CORS başlıklarını ekle
	setCORSHeaders(w)

	// OPTIONS metoduna yanıt ver
	if req.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	switch req.Method {
	case http.MethodPut:
		bc := bcs.GetBlockchain()
		replaced := bc.ResolveConflicts()

		w.Header().Add("Content-Type", "application/json")
		if replaced {
			io.WriteString(w, string(utils.JsonStatus("success")))
		} else {
			io.WriteString(w, string(utils.JsonStatus("fail")))
		}
	default:
		log.Printf("ERROR: Invalid HTTP Method")
		w.WriteHeader(http.StatusBadRequest)
	}
}

// Yeni P2P endpoint'leri
func (bcs *BlockchainServer) Peers(w http.ResponseWriter, req *http.Request) {
	// CORS başlıklarını ekle
	setCORSHeaders(w)

	// OPTIONS metoduna yanıt ver
	if req.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	switch req.Method {
	case http.MethodGet:
		// Mevcut aktif peer listesini döndür
		activeNodes := bcs.p2p.GetActiveNodes()

		w.Header().Add("Content-Type", "application/json")
		json.NewEncoder(w).Encode(activeNodes)

	case http.MethodPost:
		// Yeni peer ekle
		var peerRequest struct {
			Address string `json:"address"`
		}

		if err := json.NewDecoder(req.Body).Decode(&peerRequest); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, string(utils.JsonStatus("fail")))
			return
		}

		if peerRequest.Address == "" {
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, string(utils.JsonStatus("fail")))
			return
		}

		bcs.p2p.AddNode(peerRequest.Address)
		w.WriteHeader(http.StatusCreated)
		io.WriteString(w, string(utils.JsonStatus("success")))

	default:
		log.Println("ERROR: Invalid HTTP Method")
		w.WriteHeader(http.StatusBadRequest)
	}
}

// P2P ağı durumunu rapor et
func (bcs *BlockchainServer) P2PStatus(w http.ResponseWriter, req *http.Request) {
	// CORS başlıklarını ekle
	setCORSHeaders(w)

	// OPTIONS metoduna yanıt ver
	if req.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	switch req.Method {
	case http.MethodGet:
		status := struct {
			MyAddress    string   `json:"my_address"`
			ActivePeers  []string `json:"active_peers"`
			PeerCount    int      `json:"peer_count"`
			BootstrapURL string   `json:"bootstrap_url"`
		}{
			MyAddress:    bcs.p2p.MyAddress,
			ActivePeers:  bcs.p2p.GetActiveNodes(),
			PeerCount:    len(bcs.p2p.GetActiveNodes()),
			BootstrapURL: bcs.p2p.BootstrapURL,
		}

		w.Header().Add("Content-Type", "application/json")
		json.NewEncoder(w).Encode(status)

	default:
		log.Println("ERROR: Invalid HTTP Method")
		w.WriteHeader(http.StatusBadRequest)
	}
}

// Yeni bir blok oluşturulduğunda diğer node'lara yayın yapmak için
func (bcs *BlockchainServer) broadcastNewBlock() {
	bc := bcs.GetBlockchain()
	lastBlock := bc.LastBlock()

	// Son bloğu diğer node'lara gönder
	blockData, _ := json.Marshal(lastBlock)

	// Yayın yap
	results := bcs.p2p.BroadcastData("/block", blockData)

	// Başarısız gönderimler için loglama
	for addr, err := range results {
		if err != nil {
			log.Printf("Failed to broadcast block to %s: %v", addr, err)
		}
	}
}

// Yeni bir işlem eklendiğinde diğer node'lara yayın yapmak için
func (bcs *BlockchainServer) broadcastTransaction(transaction *block.Transaction) {
	// İşlemi diğer node'lara gönder
	transactionData, _ := json.Marshal(transaction)

	// Yayın yap
	results := bcs.p2p.BroadcastData("/transaction", transactionData)

	// Başarısız gönderimler için loglama
	for addr, err := range results {
		if err != nil {
			log.Printf("Failed to broadcast transaction to %s: %v", addr, err)
		}
	}
}

// Mining işlemi sonrası chain'i diğer node'lara yayınla
func (bcs *BlockchainServer) broadcastChain() {
	bc := bcs.GetBlockchain()
	chainData, _ := bc.MarshalJSON()

	// Yayın yap
	results := bcs.p2p.BroadcastData("/consensus", chainData)

	// Başarısız gönderimler için loglama
	for addr, err := range results {
		if err != nil {
			log.Printf("Failed to broadcast chain to %s: %v", addr, err)
		}
	}
}

// Yeni blok handler'ı
func (bcs *BlockchainServer) Block(w http.ResponseWriter, req *http.Request) {
	// CORS başlıklarını ekle
	setCORSHeaders(w)

	// OPTIONS metoduna yanıt ver
	if req.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	switch req.Method {
	case http.MethodPost:
		// Diğer node'dan gelen bir blok
		var block block.Block
		if err := json.NewDecoder(req.Body).Decode(&block); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, string(utils.JsonStatus("fail")))
			return
		}

		// Bloğu doğrula
		bc := bcs.GetBlockchain()
		if !bc.ValidProof(block.Nonce(), block.PreviousHash(), block.Transactions(), block.Difficulty()) {
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, string(utils.JsonStatus("invalid block")))
			return
		}

		// Burada tam olarak ne yapılacağı, eksiksiz implementasyon için daha fazla iş gerektirir
		// Bu örnekte sadece bloğun geçerli olduğunu belirtiyoruz

		w.WriteHeader(http.StatusAccepted)
		io.WriteString(w, string(utils.JsonStatus("success")))

	default:
		log.Println("ERROR: Invalid HTTP Method")
		w.WriteHeader(http.StatusBadRequest)
	}
}

// P2P ağını DNS Seed kullanacak şekilde yapılandır
func (bcs *BlockchainServer) EnableDNSSeeds(enable bool) {
	bcs.p2p.SetUseDNS(enable)
	log.Printf("DNS seed desteği: %v", enable)
}

func (bcs *BlockchainServer) Run() {
	// P2P node keşif işlemini başlat
	bcs.p2p.RunNodeDiscovery(
		time.Minute*5, // 5 dakikada bir node keşfi yap
		0, 1,          // IP aralığı
		block.BLOCKCHAIN_PORT_RANGE_START, block.BLOCKCHAIN_PORT_RANGE_END, // Port aralığı
	)

	// HTTP sunucusunu başlat
	mux := http.NewServeMux()

	// Normal blockchain endpoint'leri
	mux.HandleFunc("/", bcs.GetChain)
	mux.HandleFunc("/transactions", bcs.Transactions)
	mux.HandleFunc("/mine", bcs.Mine)
	mux.HandleFunc("/mine/start", bcs.StartMine)
	mux.HandleFunc("/amount", bcs.Amount)
	mux.HandleFunc("/consensus", bcs.Consensus)

	// P2P endpoint'leri
	mux.HandleFunc("/peers", bcs.Peers)
	mux.HandleFunc("/p2p/status", bcs.P2PStatus)
	mux.HandleFunc("/block", bcs.Block)

	// Log bilgilerini yazdır ve sunucuyu başlat
	log.Printf("Blockchain Server starting on port %d with P2P network", bcs.Port())
	log.Printf("My P2P address: %s", bcs.p2p.MyAddress)
	log.Printf("DNS Seeds: %v, Enabled: %v", utils.DNSSeeds, bcs.p2p.UseDNS)

	// Blokzinciri eşzamanlamaya başla
	bc := bcs.GetBlockchain()
	bc.Run()

	server := &http.Server{
		Addr:    "0.0.0.0:" + strconv.Itoa(int(bcs.Port())),
		Handler: mux,
	}
	log.Fatal(server.ListenAndServe())
}
