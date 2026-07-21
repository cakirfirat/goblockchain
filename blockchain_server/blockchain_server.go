package main

import (
	"blockchain/block"
	"blockchain/utils"
	"blockchain/wallet"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

var (
	cache    map[string]*block.Blockchain = make(map[string]*block.Blockchain)
	cacheMux sync.Mutex
)

type BlockchainServer struct {
	port    uint16
	p2p     *utils.P2PNetwork
	dataDir string

	// Checkpoint anahtarları: priv doluysa bu node otorite (imzalar),
	// pub doluysa peer checkpoint'leri doğrulanıp uygulanır
	checkpointPriv *ecdsa.PrivateKey
	checkpointPub  *ecdsa.PublicKey
}

// CORS başlıklarını ekleyen yardımcı fonksiyon
func setCORSHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
}

func NewBlockchainServer(port uint16) *BlockchainServer {
	return NewBlockchainServerWithBootstrap(port, "")
}

func NewBlockchainServerWithBootstrap(port uint16, bootstrapURL string) *BlockchainServer {
	p2p := utils.NewP2PNetwork(port, 20, bootstrapURL)
	return &BlockchainServer{port: port, p2p: p2p, dataDir: "data"}
}

// SetDataDir, zincir ve cüzdan dosyalarının yazılacağı dizini ayarlar
func (bcs *BlockchainServer) SetDataDir(dir string) {
	bcs.dataDir = dir
}

// SetCheckpointKeys, checkpoint otorite/doğrulama anahtarlarını ayarlar
func (bcs *BlockchainServer) SetCheckpointKeys(priv *ecdsa.PrivateKey, pub *ecdsa.PublicKey) {
	bcs.checkpointPriv = priv
	if pub == nil && priv != nil {
		pub = &priv.PublicKey
	}
	bcs.checkpointPub = pub
}

func (bcs *BlockchainServer) Port() uint16 {
	return bcs.port
}

// minerWalletFile, node'un mining ödüllerini alacağı cüzdanın disk formatı
type minerWalletFile struct {
	PrivateKey        string `json:"private_key"`
	PublicKey         string `json:"public_key"`
	BlockchainAddress string `json:"blockchain_address"`
}

// loadOrCreateMinerWallet, miner cüzdanını diskten yükler; yoksa oluşturup kaydeder.
// Böylece node yeniden başladığında ödüller aynı adreste birikmeye devam eder.
func (bcs *BlockchainServer) loadOrCreateMinerWallet() string {
	path := filepath.Join(bcs.dataDir, fmt.Sprintf("wallet_%d.json", bcs.Port()))

	if data, err := os.ReadFile(path); err == nil {
		var f minerWalletFile
		if err := json.Unmarshal(data, &f); err == nil && f.BlockchainAddress != "" {
			log.Printf("Miner cüzdanı diskten yüklendi: %s", f.BlockchainAddress)
			return f.BlockchainAddress
		}
	}

	minersWallet := wallet.NewWallet()
	f := minerWalletFile{
		PrivateKey:        minersWallet.PrivateKeyStr(),
		PublicKey:         minersWallet.PublicKeyStr(),
		BlockchainAddress: minersWallet.BlockchainAddress(),
	}
	if data, err := json.MarshalIndent(&f, "", "  "); err == nil {
		if err := os.WriteFile(path, data, 0600); err != nil {
			log.Printf("UYARI: miner cüzdanı diske yazılamadı: %v", err)
		}
	}
	log.Printf("Yeni miner cüzdanı oluşturuldu: %s (anahtarlar: %s)", f.BlockchainAddress, path)
	return f.BlockchainAddress
}

func (bcs *BlockchainServer) GetBlockchain() *block.Blockchain {
	cacheMux.Lock()
	defer cacheMux.Unlock()

	bc, ok := cache["blockchain"]
	if !ok {
		if err := os.MkdirAll(bcs.dataDir, 0700); err != nil {
			log.Printf("UYARI: veri dizini oluşturulamadı (%s), kalıcılık devre dışı: %v", bcs.dataDir, err)
		}
		minerAddress := bcs.loadOrCreateMinerWallet()
		chainFile := filepath.Join(bcs.dataDir, fmt.Sprintf("blockchain_%d.json", bcs.Port()))
		bc = block.NewBlockchainWithPersistence(minerAddress, bcs.Port(), chainFile)
		bc.SetPeerProvider(bcs.p2p)
		if bcs.checkpointPriv != nil || bcs.checkpointPub != nil {
			bc.SetCheckpointKeys(bcs.checkpointPriv, bcs.checkpointPub)
		}
		cache["blockchain"] = bc
		log.Printf("blockchain_address %v", minerAddress)
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
		io.WriteString(w, string(bc.ChainJSON()))
	default:
		log.Printf("ERROR: Invalid HTTP Method")

	}
}

// Checkpoint, bilinen en güncel imzalı checkpoint'i döndürür (GET /checkpoint)
func (bcs *BlockchainServer) Checkpoint(w http.ResponseWriter, req *http.Request) {
	setCORSHeaders(w)
	if req.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	switch req.Method {
	case http.MethodGet:
		cp := bcs.GetBlockchain().Checkpoint()
		if cp == nil {
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, string(utils.JsonStatus("no checkpoint")))
			return
		}
		w.Header().Add("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cp)
	default:
		log.Println("ERROR: Invalid HTTP Method")
		w.WriteHeader(http.StatusBadRequest)
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
			*t.RecipientBlockchainAddress, *t.Value, t.FeeOrZero(), *t.Timestamp, publicKey, signature)

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
			*t.RecipientBlockchainAddress, *t.Value, t.FeeOrZero(), *t.Timestamp, publicKey, signature)

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

func (bcs *BlockchainServer) StopMine(w http.ResponseWriter, req *http.Request) {
	setCORSHeaders(w)
	if req.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	switch req.Method {
	case http.MethodGet:
		bcs.GetBlockchain().StopMining()
		w.Header().Add("Content-Type", "application/json")
		io.WriteString(w, string(utils.JsonStatus("success")))
	default:
		log.Println("ERROR: Invalid HTTP Method")
		w.WriteHeader(http.StatusBadRequest)
	}
}

func (bcs *BlockchainServer) MineStatus(w http.ResponseWriter, req *http.Request) {
	setCORSHeaders(w)
	if req.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	switch req.Method {
	case http.MethodGet:
		bc := bcs.GetBlockchain()
		w.Header().Add("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Mining bool `json:"mining"`
			Height int  `json:"height"`
		}{
			Mining: bc.IsMining(),
			Height: len(bc.GetBlocks()) - 1,
		})
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

		ar := block.NewAmountResponse(amount)
		m, _ := json.Marshal(ar)

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

// Submit, bir peer'ın İTTİĞİ zinciri değerlendirir (POST /submit).
// NAT arkasındaki madencilerin kazdığı bloklar ağa bu kanaldan ulaşır;
// zincir tam doğrulamadan (ValidChain + iş kuralı + checkpoint) geçer.
func (bcs *BlockchainServer) Submit(w http.ResponseWriter, req *http.Request) {
	setCORSHeaders(w)
	if req.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	switch req.Method {
	case http.MethodPost:
		var bcResp block.Blockchain
		if err := json.NewDecoder(req.Body).Decode(&bcResp); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, string(utils.JsonStatus("fail")))
			return
		}
		w.Header().Add("Content-Type", "application/json")
		if bcs.GetBlockchain().ConsiderChain(bcResp.Chain()) {
			io.WriteString(w, string(utils.JsonStatus("success")))
		} else {
			io.WriteString(w, string(utils.JsonStatus("not adopted")))
		}
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
	mux.HandleFunc("/chain", bcs.GetChain)
	mux.HandleFunc("/transactions", bcs.Transactions)
	mux.HandleFunc("/mine", bcs.Mine)
	mux.HandleFunc("/mine/start", bcs.StartMine)
	mux.HandleFunc("/mine/stop", bcs.StopMine)
	mux.HandleFunc("/mine/status", bcs.MineStatus)
	mux.HandleFunc("/amount", bcs.Amount)
	mux.HandleFunc("/consensus", bcs.Consensus)

	// P2P endpoint'leri
	mux.HandleFunc("/peers", bcs.Peers)
	mux.HandleFunc("/p2p/status", bcs.P2PStatus)
	mux.HandleFunc("/block", bcs.Block)
	mux.HandleFunc("/checkpoint", bcs.Checkpoint)
	mux.HandleFunc("/submit", bcs.Submit)

	// Log bilgilerini yazdır ve sunucuyu başlat
	log.Printf("Blockchain Server starting on port %d with P2P network", bcs.Port())
	log.Printf("My P2P address: %s", bcs.p2p.MyAddress)
	log.Printf("DNS Seeds: %v, Enabled: %v", utils.DNSSeeds, bcs.p2p.UseDNS)

	// Blokzinciri eşzamanlamaya başla
	bc := bcs.GetBlockchain()
	bc.Run()

	server := &http.Server{
		Addr:              "0.0.0.0:" + strconv.Itoa(int(bcs.Port())),
		Handler:           limitBody(mux),
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	log.Fatal(server.ListenAndServe())
}

// limitBody, istek gövdelerine üst sınır koyar (sınırsız gövde belleği doldurup
// node'u çökertebilir). Zincir transferi (/submit, /consensus, /chain) büyük
// olabildiğinden 64 MB, diğer tüm uçlar 1 MB ile sınırlanır.
func limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var limit int64 = 1 << 20 // 1 MB
		switch r.URL.Path {
		case "/submit", "/consensus", "/chain", "/":
			limit = 64 << 20 // 64 MB
		}
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, limit)
		}
		next.ServeHTTP(w, r)
	})
}
