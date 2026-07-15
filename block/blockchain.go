package block

import (
	"blockchain/utils"
	"bytes"
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	MINING_DIFFICULTY = 3
	// PoW mutex altında çalıştığı için zorluk tavanı şart; aksi halde
	// difficulty artışı node'u kilitleyip tüm HTTP isteklerini dondurur
	MAX_MINING_DIFFICULTY = 5
	MINING_SENDER         = "THE BLOCKCHAIN"
	MINING_REWARD         = 1.0
	MINING_TIMER_SEC      = 20

	// Dinamik zorluk ayarları — hedef süre mining timer ile tutarlı olmalı,
	// yoksa zorluk her ayarlamada tırmanır
	TARGET_BLOCK_TIME_SEC          = 20
	DIFFICULTY_ADJUSTMENT_INTERVAL = 10 // Her 10 blokta bir difficulty yeniden hesaplanır

	// Otorite node her CHECKPOINT_INTERVAL blokta bir checkpoint imzalar
	CHECKPOINT_INTERVAL = 10

	BLOCKCHAIN_PORT_RANGE_START      = 5001
	BLOCKCHAIN_PORT_RANGE_END        = 5003
	NEIGHBOR_IP_RANGE_START          = 0
	NEIGHBOR_IP_RANGE_END            = 1
	BLOCKCHIN_NEIGHBOR_SYNC_TIME_SEC = 20
)

// Tüm peer istekleri için zaman aşımı olan ortak HTTP istemcisi;
// varsayılan istemci zaman aşımı olmadığı için kapalı bir peer node'u sonsuza dek bekletebilir
var httpClient = &http.Client{Timeout: 10 * time.Second}

// Mining zamanlayıcısına eklenen rastgele gecikme için; node'lar kilitli adımda
// kazarsa eşit uzunlukta fork'lar uzun süre yaşar
var rnd = rand.New(rand.NewSource(time.Now().UnixNano()))

type Block struct {
	timestamp    int64
	nonce        int
	previousHash [32]byte
	transactions []*Transaction
	difficulty   int   // Blok oluşturulduğundaki zorluk seviyesi
	miningTime   int64 // Blok mining sürecinin ne kadar sürdüğü
}

func NewBlock(nonce int, previousHash [32]byte, transactions []*Transaction, difficulty int, miningTime int64) *Block {
	b := new(Block)
	b.timestamp = time.Now().UnixNano()
	b.nonce = nonce
	b.previousHash = previousHash
	b.transactions = transactions
	b.difficulty = difficulty
	b.miningTime = miningTime
	return b
}

func (b *Block) PreviousHash() [32]byte {
	return b.previousHash
}

func (b *Block) Nonce() int {
	return b.nonce
}

func (b *Block) Transactions() []*Transaction {
	return b.transactions
}

func (b *Block) Difficulty() int {
	return b.difficulty
}

func (b *Block) MiningTime() int64 {
	return b.miningTime
}

func (b *Block) Print() {
	fmt.Printf("timestamp       %d\n", b.timestamp)
	fmt.Printf("nonce           %d\n", b.nonce)
	fmt.Printf("previous_hash   %x\n", b.previousHash)
	fmt.Printf("difficulty      %d\n", b.difficulty)
	fmt.Printf("mining_time     %d ms\n", b.miningTime/1000000)
	for _, t := range b.transactions {
		t.Print()
	}
}

func (b *Block) Hash() [32]byte {
	m, _ := json.Marshal(b)
	return sha256.Sum256([]byte(m))
}

func (b *Block) HashStr() string {
	return fmt.Sprintf("%x", b.Hash())
}

func (b *Block) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Timestamp    int64          `json:"timestamp"`
		Nonce        int            `json:"nonce"`
		PreviousHash string         `json:"previous_hash"`
		Transactions []*Transaction `json:"transactions"`
		Difficulty   int            `json:"difficulty"`
		MiningTime   int64          `json:"mining_time"`
	}{
		Timestamp:    b.timestamp,
		Nonce:        b.nonce,
		PreviousHash: fmt.Sprintf("%x", b.previousHash),
		Transactions: b.transactions,
		Difficulty:   b.difficulty,
		MiningTime:   b.miningTime,
	})
}

func (b *Block) UnmarshalJSON(data []byte) error {
	var previousHash string
	v := &struct {
		Timestamp    *int64          `json:"timestamp"`
		Nonce        *int            `json:"nonce"`
		PreviousHash *string         `json:"previous_hash"`
		Transactions *[]*Transaction `json:"transactions"`
		Difficulty   *int            `json:"difficulty"`
		MiningTime   *int64          `json:"mining_time"`
	}{
		Timestamp:    &b.timestamp,
		Nonce:        &b.nonce,
		PreviousHash: &previousHash,
		Transactions: &b.transactions,
		Difficulty:   &b.difficulty,
		MiningTime:   &b.miningTime,
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	ph, err := hex.DecodeString(*v.PreviousHash)
	if err != nil || len(ph) != 32 {
		return fmt.Errorf("geçersiz previous_hash: %q", *v.PreviousHash)
	}
	copy(b.previousHash[:], ph[:32])
	return nil
}

// PeerProvider, senkronizasyon ve yayın için kullanılacak aktif peer listesini sağlar.
// utils.P2PNetwork bu arayüzü karşılar; böylece DNS/bootstrap ile bulunan
// peer'lar da konsensüse katılır.
type PeerProvider interface {
	GetActiveNodes() []string
}

type Blockchain struct {
	transactionPool   []*Transaction
	chain             []*Block
	blockchainAddress string
	port              uint16
	mux               sync.Mutex

	neighbors    []string // eski yerel ağ taraması (peerProvider yoksa devrede)
	muxNeighbors sync.Mutex
	peerProvider PeerProvider

	currentDifficulty int   // Mevcut zorluk seviyesi
	lastMiningTime    int64 // Son mining süresini tutan değişken

	dataFile string // boş ise kalıcılık kapalı

	// Kabul edilmiş her işlemin hash'i; aynı imzalı işlemin
	// tekrar oynatılmasını (replay) engeller
	seenTransactions map[string]bool

	checkpoint           *Checkpoint       // bilinen en yüksek doğrulanmış checkpoint
	checkpointPrivateKey *ecdsa.PrivateKey // doluysa bu node otorite: checkpoint imzalar
	checkpointPublicKey  *ecdsa.PublicKey  // doluysa peer checkpoint'leri doğrulanır ve uygulanır
}

// blockchainFile, diske yazılan kalıcı durum
type blockchainFile struct {
	Chain      []*Block    `json:"chain"`
	Checkpoint *Checkpoint `json:"checkpoint,omitempty"`
}

func NewBlockchain(blockchainAddress string, port uint16) *Blockchain {
	return NewBlockchainWithPersistence(blockchainAddress, port, "")
}

// NewBlockchainWithPersistence, dataFile doluysa zinciri diskten yükler;
// dosya yoksa genesis bloğu oluşturup kaydeder.
func NewBlockchainWithPersistence(blockchainAddress string, port uint16, dataFile string) *Blockchain {
	bc := new(Blockchain)
	bc.blockchainAddress = blockchainAddress
	bc.port = port
	bc.currentDifficulty = MINING_DIFFICULTY
	bc.dataFile = dataFile
	bc.seenTransactions = make(map[string]bool)

	if dataFile != "" {
		if data, err := os.ReadFile(dataFile); err == nil {
			var f blockchainFile
			if err := json.Unmarshal(data, &f); err == nil && len(f.Chain) > 0 {
				bc.chain = f.Chain
				bc.checkpoint = f.Checkpoint
				bc.currentDifficulty = clampDifficulty(f.Chain[len(f.Chain)-1].Difficulty())
				bc.rebuildSeenLocked()
				log.Printf("Zincir diskten yüklendi: %d blok (%s)", len(f.Chain), dataFile)
				return bc
			}
			log.Printf("UYARI: zincir dosyası okunamadı, yeni zincir başlatılıyor: %v", err)
		}
	}

	b := &Block{}
	bc.CreateBlock(0, b.Hash())
	return bc
}

func clampDifficulty(d int) int {
	if d < 1 {
		return 1
	}
	if d > MAX_MINING_DIFFICULTY {
		return MAX_MINING_DIFFICULTY
	}
	return d
}

func (bc *Blockchain) Chain() []*Block {
	return bc.chain
}

// SetPeerProvider, peer listesini P2P katmanına devreder (DNS + bootstrap + yerel tarama)
func (bc *Blockchain) SetPeerProvider(p PeerProvider) {
	bc.peerProvider = p
}

// SetCheckpointKeys, checkpoint imzalama (otorite) ve/veya doğrulama anahtarlarını ayarlar
func (bc *Blockchain) SetCheckpointKeys(priv *ecdsa.PrivateKey, pub *ecdsa.PublicKey) {
	bc.checkpointPrivateKey = priv
	if pub == nil && priv != nil {
		pub = &priv.PublicKey
	}
	bc.checkpointPublicKey = pub
}

// Checkpoint, bilinen en güncel checkpoint'i döndürür (yoksa nil)
func (bc *Blockchain) Checkpoint() *Checkpoint {
	bc.mux.Lock()
	defer bc.mux.Unlock()
	return bc.checkpoint
}

// Peers, senkronizasyon ve yayın için kullanılacak peer adreslerini döndürür
func (bc *Blockchain) Peers() []string {
	if bc.peerProvider != nil {
		return bc.peerProvider.GetActiveNodes()
	}
	bc.muxNeighbors.Lock()
	defer bc.muxNeighbors.Unlock()
	return append([]string(nil), bc.neighbors...)
}

func (bc *Blockchain) Run() {
	// Peer keşfi P2P katmanına devredildiyse eski yerel taramayı başlatma
	if bc.peerProvider == nil {
		bc.StartSyncNeighbors()
	}
	bc.ResolveConflicts()
	bc.StartMining()
}

func (bc *Blockchain) SetNeighbors() {
	bc.neighbors = utils.FindNeighbors(
		utils.GetHost(), bc.port,
		NEIGHBOR_IP_RANGE_START, NEIGHBOR_IP_RANGE_END,
		BLOCKCHAIN_PORT_RANGE_START, BLOCKCHAIN_PORT_RANGE_END)
	log.Printf("%v", bc.neighbors)
}

func (bc *Blockchain) SyncNeighbors() {
	bc.muxNeighbors.Lock()
	defer bc.muxNeighbors.Unlock()
	bc.SetNeighbors()
}

func (bc *Blockchain) StartSyncNeighbors() {
	bc.SyncNeighbors()
	_ = time.AfterFunc(time.Second*BLOCKCHIN_NEIGHBOR_SYNC_TIME_SEC, bc.StartSyncNeighbors)
}

func (bc *Blockchain) TransactionPool() []*Transaction {
	bc.mux.Lock()
	defer bc.mux.Unlock()
	return append([]*Transaction(nil), bc.transactionPool...)
}

func (bc *Blockchain) ClearTransactionPool() {
	bc.mux.Lock()
	defer bc.mux.Unlock()
	bc.transactionPool = bc.transactionPool[:0]
}

func (bc *Blockchain) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Blocks []*Block `json:"chain"`
	}{
		Blocks: bc.chain,
	})
}

func (bc *Blockchain) UnmarshalJSON(data []byte) error {
	v := &struct {
		Blocks *[]*Block `json:"chain"`
	}{
		Blocks: &bc.chain,
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	return nil
}

// ChainJSON, zincirin JSON halini kilit altında üretir (HTTP handler'ları için)
func (bc *Blockchain) ChainJSON() []byte {
	bc.mux.Lock()
	defer bc.mux.Unlock()
	m, _ := bc.MarshalJSON()
	return m
}

func (bc *Blockchain) CreateBlock(nonce int, previousHash [32]byte) *Block {
	bc.mux.Lock()
	defer bc.mux.Unlock()
	return bc.createBlockLocked(nonce, previousHash)
}

func (bc *Blockchain) createBlockLocked(nonce int, previousHash [32]byte) *Block {
	b := NewBlock(nonce, previousHash, bc.transactionPool, bc.currentDifficulty, bc.lastMiningTime)
	bc.chain = append(bc.chain, b)
	bc.transactionPool = []*Transaction{}
	bc.currentDifficulty = bc.adjustDifficultyLocked()
	bc.signCheckpointLocked()
	bc.saveLocked()
	return b
}

// signCheckpointLocked: otorite anahtarı varsa ve blok yüksekliği aralığa denk geliyorsa
// yeni checkpoint imzala
func (bc *Blockchain) signCheckpointLocked() {
	if bc.checkpointPrivateKey == nil {
		return
	}
	height := len(bc.chain) - 1
	if height <= 0 || height%CHECKPOINT_INTERVAL != 0 {
		return
	}
	cp, err := SignCheckpoint(bc.checkpointPrivateKey, height, bc.chain[height].HashStr())
	if err != nil {
		log.Printf("UYARI: checkpoint imzalanamadı: %v", err)
		return
	}
	bc.checkpoint = cp
	log.Printf("Checkpoint imzalandı: yükseklik=%d hash=%s...", cp.Height, cp.BlockHash[:12])
}

// saveLocked, zinciri atomik olarak diske yazar (kalıcılık kapalıysa no-op)
func (bc *Blockchain) saveLocked() {
	if bc.dataFile == "" {
		return
	}
	f := blockchainFile{Chain: bc.chain, Checkpoint: bc.checkpoint}
	data, err := json.Marshal(&f)
	if err != nil {
		log.Printf("HATA: zincir serileştirilemedi: %v", err)
		return
	}
	tmp := bc.dataFile + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		log.Printf("HATA: zincir diske yazılamadı: %v", err)
		return
	}
	if err := os.Rename(tmp, bc.dataFile); err != nil {
		log.Printf("HATA: zincir dosyası taşınamadı: %v", err)
	}
}

// rebuildSeenLocked, replay korumasının hash setini zincirden yeniden kurar
// ve havuzdaki işlemleri de işaretler
func (bc *Blockchain) rebuildSeenLocked() {
	bc.seenTransactions = make(map[string]bool)
	for _, b := range bc.chain {
		for _, t := range b.transactions {
			bc.seenTransactions[t.HashStr()] = true
		}
	}
	for _, t := range bc.transactionPool {
		bc.seenTransactions[t.HashStr()] = true
	}
}

// prunePoolLocked, zincire girmiş işlemleri havuzdan düşürür (zincir değişiminden sonra)
func (bc *Blockchain) prunePoolLocked(chainTxs map[string]bool) {
	remaining := make([]*Transaction, 0, len(bc.transactionPool))
	for _, t := range bc.transactionPool {
		if !chainTxs[t.HashStr()] {
			remaining = append(remaining, t)
		}
	}
	bc.transactionPool = remaining
}

func (bc *Blockchain) LastBlock() *Block {
	return bc.chain[len(bc.chain)-1]
}

func (bc *Blockchain) Print() {
	for i, block := range bc.chain {
		fmt.Printf("%s Chain %d %s\n", strings.Repeat("=", 25), i,
			strings.Repeat("=", 25))
		block.Print()
	}
	fmt.Printf("%s\n", strings.Repeat("*", 25))
}

func (bc *Blockchain) CreateTransaction(sender string, recipient string, value float32,
	timestamp int64, senderPublicKey *ecdsa.PublicKey, s *utils.Signature) bool {
	isTransacted := bc.AddTransaction(sender, recipient, value, timestamp, senderPublicKey, s)

	if isTransacted {
		publicKeyStr := fmt.Sprintf("%064x%064x", senderPublicKey.X.Bytes(),
			senderPublicKey.Y.Bytes())
		signatureStr := s.String()
		bt := &TransactionRequest{
			SenderBlockchainAddress:    &sender,
			RecipientBlockchainAddress: &recipient,
			SenderPublicKey:            &publicKeyStr,
			Value:                      &value,
			Timestamp:                  &timestamp,
			Signature:                  &signatureStr,
		}
		m, _ := json.Marshal(bt)
		for _, n := range bc.Peers() {
			endpoint := fmt.Sprintf("http://%s/transactions", n)
			req, err := http.NewRequest(http.MethodPut, endpoint, bytes.NewBuffer(m))
			if err != nil {
				continue
			}
			req.Header.Set("Content-Type", "application/json")
			resp, err := httpClient.Do(req)
			if err != nil {
				log.Printf("UYARI: işlem %s adresine iletilemedi: %v", n, err)
				continue
			}
			resp.Body.Close()
		}
	}

	return isTransacted
}

func (bc *Blockchain) AddTransaction(sender string, recipient string, value float32,
	timestamp int64, senderPublicKey *ecdsa.PublicKey, s *utils.Signature) bool {
	bc.mux.Lock()
	defer bc.mux.Unlock()
	return bc.addTransactionLocked(sender, recipient, value, timestamp, senderPublicKey, s)
}

func (bc *Blockchain) addTransactionLocked(sender string, recipient string, value float32,
	timestamp int64, senderPublicKey *ecdsa.PublicKey, s *utils.Signature) bool {

	if sender == MINING_SENDER {
		t := NewTransaction(sender, recipient, value)
		bc.transactionPool = append(bc.transactionPool, t)
		bc.seenTransactions[t.HashStr()] = true
		return true
	}

	// Değer negatif/sıfır/NaN olamaz; negatif değer para basmaya denk gelir
	if !(value > 0) {
		log.Println("ERROR: Gecersiz islem tutari")
		return false
	}
	if timestamp <= 0 {
		log.Println("ERROR: Islemde zaman damgasi eksik")
		return false
	}

	t := NewTransactionWithTimestamp(sender, recipient, value, timestamp)

	// Replay koruması: aynı imzalı işlem (aynı hash) yalnızca bir kez kabul edilir
	hashStr := t.HashStr()
	if bc.seenTransactions[hashStr] {
		log.Println("ERROR: Islem zaten islendi (replay reddedildi)")
		return false
	}

	if !bc.VerifyTransactionSignature(senderPublicKey, s, t) {
		log.Println("ERROR: Verify Transaction")
		return false
	}

	// Bakiye kontrolü havuzda bekleyen harcamaları da hesaba katar,
	// yoksa aynı bakiye havuz içinde iki kez harcanabilir
	available := bc.calculateTotalAmountLocked(sender) - bc.pendingSpendLocked(sender)
	if available < value {
		log.Println("ERROR: Not enough balance in a wallet")
		return false
	}

	bc.transactionPool = append(bc.transactionPool, t)
	bc.seenTransactions[hashStr] = true
	return true
}

// pendingSpendLocked, göndericinin havuzda bekleyen toplam harcamasını döndürür
func (bc *Blockchain) pendingSpendLocked(sender string) float32 {
	var total float32
	for _, t := range bc.transactionPool {
		if t.senderBlockchainAddress == sender {
			total += t.value
		}
	}
	return total
}

func (bc *Blockchain) VerifyTransactionSignature(
	senderPublicKey *ecdsa.PublicKey, s *utils.Signature, t *Transaction) bool {
	if senderPublicKey == nil || s == nil || s.R == nil || s.S == nil {
		return false
	}
	m, _ := json.Marshal(t)
	h := sha256.Sum256([]byte(m))
	return ecdsa.Verify(senderPublicKey, h[:], s.R, s.S)
}

func (bc *Blockchain) CopyTransactionPool() []*Transaction {
	transactions := make([]*Transaction, 0)
	for _, t := range bc.transactionPool {
		transactions = append(transactions,
			NewTransactionWithTimestamp(t.senderBlockchainAddress,
				t.recipientBlockchainAddress,
				t.value,
				t.timestamp))
	}
	return transactions
}

func (bc *Blockchain) proofOfWorkLocked() int {
	transactions := bc.CopyTransactionPool()
	previousHash := bc.LastBlock().Hash()
	nonce := 0

	miningStart := time.Now().UnixNano()

	for !bc.ValidProof(nonce, previousHash, transactions, bc.currentDifficulty) {
		nonce += 1
	}

	bc.lastMiningTime = time.Now().UnixNano() - miningStart
	log.Printf("Mining completed in %d ms with difficulty %d", bc.lastMiningTime/1000000, bc.currentDifficulty)

	return nonce
}

func (bc *Blockchain) ValidProof(nonce int, previousHash [32]byte, transactions []*Transaction, difficulty int) bool {
	// Kötü niyetli bir blok saçma bir difficulty ile slice taşması/panik tetikleyemesin
	if difficulty < 1 || difficulty > 32 {
		return false
	}
	zeros := strings.Repeat("0", difficulty)
	guessBlock := Block{0, nonce, previousHash, transactions, difficulty, 0}
	guessHashStr := fmt.Sprintf("%x", guessBlock.Hash())
	return guessHashStr[:difficulty] == zeros
}

func (bc *Blockchain) Mining() bool {
	bc.mux.Lock()

	bc.addTransactionLocked(MINING_SENDER, bc.blockchainAddress, MINING_REWARD, 0, nil, nil)
	nonce := bc.proofOfWorkLocked()
	previousHash := bc.LastBlock().Hash()
	bc.createBlockLocked(nonce, previousHash)
	bc.mux.Unlock()

	log.Println("action=mining, status=success")

	// Ağ çağrıları kilit dışında: peer'lara yeni zinciri almalarını söyle
	for _, n := range bc.Peers() {
		endpoint := fmt.Sprintf("http://%s/consensus", n)
		req, err := http.NewRequest(http.MethodPut, endpoint, nil)
		if err != nil {
			continue
		}
		resp, err := httpClient.Do(req)
		if err != nil {
			log.Printf("UYARI: konsensus bildirimi %s adresine ulaşmadı: %v", n, err)
			continue
		}
		resp.Body.Close()
	}

	return true
}

func (bc *Blockchain) StartMining() {
	bc.Mining()
	// Jitter: node'ların aynı anda blok üretip sürekli eşit uzunlukta
	// fork oluşturmasını engeller
	jitter := time.Duration(rnd.Intn(5000)) * time.Millisecond
	_ = time.AfterFunc(time.Second*MINING_TIMER_SEC+jitter, bc.StartMining)
}

func (bc *Blockchain) CalculateTotalAmount(blockchainAddress string) float32 {
	bc.mux.Lock()
	defer bc.mux.Unlock()
	return bc.calculateTotalAmountLocked(blockchainAddress)
}

func (bc *Blockchain) calculateTotalAmountLocked(blockchainAddress string) float32 {
	var totalAmount float32 = 0.0
	for _, b := range bc.chain {
		for _, t := range b.transactions {
			value := t.value
			if blockchainAddress == t.recipientBlockchainAddress {
				totalAmount += value
			}

			if blockchainAddress == t.senderBlockchainAddress {
				totalAmount -= value
			}
		}
	}
	return totalAmount
}

func (bc *Blockchain) ValidChain(chain []*Block) bool {
	if len(chain) == 0 {
		return false
	}
	preBlock := chain[0]
	currentIndex := 1
	for currentIndex < len(chain) {
		b := chain[currentIndex]
		if b.previousHash != preBlock.Hash() {
			return false
		}

		// Geçerli bloğun kendi zorluk seviyesiyle doğrulanması
		if !bc.ValidProof(b.Nonce(), b.PreviousHash(), b.Transactions(), b.Difficulty()) {
			return false
		}

		preBlock = b
		currentIndex += 1
	}
	return true
}

// chainSatisfiesCheckpointLocked: aday zincir, bilinen checkpoint ile uyumlu mu?
// Checkpoint'in gerisi asla yeniden yazılamaz — kiralık hash gücüyle gelen
// saldırgan geçmişi değiştiremez.
func (bc *Blockchain) chainSatisfiesCheckpointLocked(chain []*Block) bool {
	cp := bc.checkpoint
	if cp == nil {
		return true
	}
	if len(chain) <= cp.Height {
		return false
	}
	return chain[cp.Height].HashStr() == cp.BlockHash
}

// fetchPeerCheckpoint, peer'dan checkpoint çekip imzasını doğrular;
// mevcut checkpoint'ten yüksekse benimser
func (bc *Blockchain) fetchPeerCheckpoint(peer string) {
	if bc.checkpointPublicKey == nil {
		return
	}
	resp, err := httpClient.Get(fmt.Sprintf("http://%s/checkpoint", peer))
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return
	}
	var cp Checkpoint
	if err := json.NewDecoder(resp.Body).Decode(&cp); err != nil {
		return
	}
	if !cp.Verify(bc.checkpointPublicKey) {
		log.Printf("UYARI: %s geçersiz imzalı checkpoint gönderdi", peer)
		return
	}
	bc.mux.Lock()
	if bc.checkpoint == nil || cp.Height > bc.checkpoint.Height {
		bc.checkpoint = &cp
		log.Printf("Checkpoint güncellendi: yükseklik=%d (%s kaynağından)", cp.Height, peer)
		bc.saveLocked()
	}
	bc.mux.Unlock()
}

// chainWork, zincirin toplam işini hesaplar (blok başına 16^difficulty).
// "En uzun zincir" yerine "en çok iş" kuralı: düşük zorlukla üretilmiş
// uzun bir sahte zincir, yüksek zorluklu kısa zinciri yenemez.
func chainWork(chain []*Block) *big.Int {
	total := new(big.Int)
	for _, b := range chain {
		d := clampWorkDifficulty(b.Difficulty())
		total.Add(total, new(big.Int).Lsh(big.NewInt(1), uint(4*d)))
	}
	return total
}

func clampWorkDifficulty(d int) int {
	if d < 1 {
		return 1
	}
	if d > 32 {
		return 32
	}
	return d
}

// betterChain: aday zincir mevcut zincirden üstün mü?
// Önce toplam iş karşılaştırılır; eşitlikte tip hash'i küçük olan kazanır.
// Bu deterministik eşitlik bozucu olmadan, aynı anda kazan iki node eşit
// uzunlukta fork'larda süresiz takılı kalır.
func betterChain(candidateWork *big.Int, candidateTip string, currentWork *big.Int, currentTip string) bool {
	switch candidateWork.Cmp(currentWork) {
	case 1:
		return true
	case 0:
		return candidateTip < currentTip
	default:
		return false
	}
}

func (bc *Blockchain) ResolveConflicts() bool {
	peers := bc.Peers()

	bc.mux.Lock()
	bestWork := chainWork(bc.chain)
	bestTip := bc.LastBlock().HashStr()
	bc.mux.Unlock()

	var bestChain []*Block = nil

	for _, n := range peers {
		bc.fetchPeerCheckpoint(n)

		endpoint := fmt.Sprintf("http://%s/chain", n)
		resp, err := httpClient.Get(endpoint)
		if err != nil {
			log.Printf("UYARI: %s zinciri alınamadı: %v", n, err)
			continue
		}
		if resp.StatusCode == http.StatusOK {
			var bcResp Blockchain
			decoder := json.NewDecoder(resp.Body)
			if err := decoder.Decode(&bcResp); err != nil {
				log.Printf("UYARI: %s zinciri çözümlenemedi: %v", n, err)
				resp.Body.Close()
				continue
			}

			chain := bcResp.Chain()
			if len(chain) == 0 || !bc.ValidChain(chain) {
				resp.Body.Close()
				continue
			}

			w := chainWork(chain)
			tip := chain[len(chain)-1].HashStr()
			if betterChain(w, tip, bestWork, bestTip) {
				bestWork = w
				bestTip = tip
				bestChain = chain
			}
		}
		resp.Body.Close()
	}

	if bestChain != nil {
		bc.mux.Lock()
		defer bc.mux.Unlock()

		// Kilit alınana kadar kendi zincirimiz değişmiş olabilir; kuralı yeniden uygula
		myWork := chainWork(bc.chain)
		myTip := bc.LastBlock().HashStr()
		if !betterChain(bestWork, bestTip, myWork, myTip) {
			return false
		}
		if !bc.chainSatisfiesCheckpointLocked(bestChain) {
			log.Printf("Aday zincir checkpoint ile çelişiyor, reddedildi (uzunluk %d)", len(bestChain))
			return false
		}

		oldChain := bc.chain
		bc.chain = bestChain
		bc.currentDifficulty = clampDifficulty(bestChain[len(bestChain)-1].Difficulty())

		chainTxs := make(map[string]bool)
		for _, b := range bc.chain {
			for _, t := range b.transactions {
				chainTxs[t.HashStr()] = true
			}
		}

		// Reorg kurtarması: terk edilen fork'ta olup yeni zincirde olmayan
		// kullanıcı işlemlerini havuza geri ekle — yoksa fork yarışını
		// kaybeden bloklardaki transferler sessizce yok olur
		recovered := 0
		for _, b := range oldChain {
			for _, t := range b.transactions {
				if t.senderBlockchainAddress == MINING_SENDER {
					continue
				}
				if !chainTxs[t.HashStr()] {
					bc.transactionPool = append(bc.transactionPool, t)
					recovered++
				}
			}
		}
		if recovered > 0 {
			log.Printf("Reorg: terk edilen fork'tan %d işlem havuza geri alındı", recovered)
		}

		bc.prunePoolLocked(chainTxs)
		bc.filterPoolByBalanceLocked()
		bc.rebuildSeenLocked()
		bc.saveLocked()
		log.Printf("Resolve conflicts: zincir değiştirildi (%d blok)", len(bc.chain))
		return true
	}
	log.Printf("Resolve conflicts: zincir korundu")
	return false
}

// filterPoolByBalanceLocked, zincir değişiminden sonra havuzda artık
// karşılığı olmayan işlemleri (yetersiz bakiye) sırayla eler
func (bc *Blockchain) filterPoolByBalanceLocked() {
	pending := make(map[string]float32)
	kept := make([]*Transaction, 0, len(bc.transactionPool))
	for _, t := range bc.transactionPool {
		if t.senderBlockchainAddress == MINING_SENDER {
			kept = append(kept, t)
			continue
		}
		available := bc.calculateTotalAmountLocked(t.senderBlockchainAddress) - pending[t.senderBlockchainAddress]
		if available < t.value {
			log.Printf("Havuzdan elendi (yetersiz bakiye): %s -> %s (%.2f)",
				t.senderBlockchainAddress, t.recipientBlockchainAddress, t.value)
			continue
		}
		pending[t.senderBlockchainAddress] += t.value
		kept = append(kept, t)
	}
	bc.transactionPool = kept
}

type Transaction struct {
	senderBlockchainAddress    string
	recipientBlockchainAddress string
	value                      float32
	timestamp                  int64
}

func NewTransaction(sender string, recipient string, value float32) *Transaction {
	return &Transaction{sender, recipient, value, time.Now().UnixNano()}
}

func NewTransactionWithTimestamp(sender string, recipient string, value float32, timestamp int64) *Transaction {
	return &Transaction{sender, recipient, value, timestamp}
}

func (t *Transaction) Timestamp() int64 {
	return t.timestamp
}

// Hash, işlemin benzersiz kimliği (replay koruması için)
func (t *Transaction) Hash() [32]byte {
	m, _ := json.Marshal(t)
	return sha256.Sum256(m)
}

func (t *Transaction) HashStr() string {
	return fmt.Sprintf("%x", t.Hash())
}

func (t *Transaction) Print() {
	fmt.Printf("%s\n", strings.Repeat("-", 40))
	fmt.Printf(" sender_blockchain_address      %s\n", t.senderBlockchainAddress)
	fmt.Printf(" recipient_blockchain_address   %s\n", t.recipientBlockchainAddress)
	fmt.Printf(" value                          %.1f\n", t.value)
}

func (t *Transaction) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Sender    string  `json:"sender_blockchain_address"`
		Recipient string  `json:"recipient_blockchain_address"`
		Value     float32 `json:"value"`
		Timestamp int64   `json:"timestamp"`
	}{
		Sender:    t.senderBlockchainAddress,
		Recipient: t.recipientBlockchainAddress,
		Value:     t.value,
		Timestamp: t.timestamp,
	})
}

func (t *Transaction) UnmarshalJSON(data []byte) error {
	v := &struct {
		Sender    *string  `json:"sender_blockchain_address"`
		Recipient *string  `json:"recipient_blockchain_address"`
		Value     *float32 `json:"value"`
		Timestamp *int64   `json:"timestamp"`
	}{
		Sender:    &t.senderBlockchainAddress,
		Recipient: &t.recipientBlockchainAddress,
		Value:     &t.value,
		Timestamp: &t.timestamp,
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	return nil
}

type TransactionRequest struct {
	SenderBlockchainAddress    *string  `json:"sender_blockchain_address"`
	RecipientBlockchainAddress *string  `json:"recipient_blockchain_address"`
	SenderPublicKey            *string  `json:"sender_public_key"`
	Value                      *float32 `json:"value"`
	Timestamp                  *int64   `json:"timestamp"`
	Signature                  *string  `json:"signature"`
}

func (tr *TransactionRequest) Validate() bool {
	if tr.SenderBlockchainAddress == nil ||
		tr.RecipientBlockchainAddress == nil ||
		tr.SenderPublicKey == nil ||
		tr.Value == nil ||
		tr.Timestamp == nil ||
		tr.Signature == nil {
		return false
	}
	return true
}

type AmountResponse struct {
	Amount float32 `json:"amount"`
}

func (ar *AmountResponse) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Amount float32 `json:"amount"`
	}{
		Amount: ar.Amount,
	})
}

// adjustDifficultyLocked, dinamik difficulty hesaplar (1..MAX_MINING_DIFFICULTY aralığında)
func (bc *Blockchain) adjustDifficultyLocked() int {
	// Eğer yeterli blok yoksa veya ayarlama aralığında değilse, mevcut değeri kullan
	if len(bc.chain) < DIFFICULTY_ADJUSTMENT_INTERVAL || len(bc.chain)%DIFFICULTY_ADJUSTMENT_INTERVAL != 0 {
		return bc.currentDifficulty
	}

	lastIndex := len(bc.chain) - 1
	prevAdjustmentBlock := bc.chain[lastIndex-(DIFFICULTY_ADJUSTMENT_INTERVAL-1)]
	lastBlock := bc.chain[lastIndex]

	timeExpected := int64(TARGET_BLOCK_TIME_SEC * DIFFICULTY_ADJUSTMENT_INTERVAL * 1000000000)
	timeActual := lastBlock.timestamp - prevAdjustmentBlock.timestamp

	// Süre beklenenin çok altındaysa zorluğu artır (tavana kadar)
	if timeActual < timeExpected/2 && bc.currentDifficulty < MAX_MINING_DIFFICULTY {
		newDifficulty := bc.currentDifficulty + 1
		log.Printf("Difficulty increased: %d -> %d", bc.currentDifficulty, newDifficulty)
		return newDifficulty
	}

	// Süre beklenenin çok üstündeyse zorluğu azalt (1'in altına düşme)
	if timeActual > timeExpected*2 && bc.currentDifficulty > 1 {
		newDifficulty := bc.currentDifficulty - 1
		log.Printf("Difficulty decreased: %d -> %d", bc.currentDifficulty, newDifficulty)
		return newDifficulty
	}

	return bc.currentDifficulty
}

// GetBlocks returns all blocks in the blockchain
func (bc *Blockchain) GetBlocks() []*Block {
	return bc.chain
}
