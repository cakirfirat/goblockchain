package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// FlatunCoin DNS seed listesi
var DNSSeeds = []string{
	"seed.flatuncoin.com",
	"seed2.flatuncoin.com",
}

// P2PNode represents a peer in the P2P network
type P2PNode struct {
	Address    string    `json:"address"`
	LastSeen   time.Time `json:"last_seen"`
	Reputation int       `json:"reputation"` // Node güvenilirliği için puanlama sistemi
}

// P2PNetwork manages the peer-to-peer network state
type P2PNetwork struct {
	Nodes        map[string]*P2PNode // Bilinen düğümlerin haritası
	MaxPeers     int                 // Maximum bağlantı sayısı
	MyAddress    string              // Bu düğümün adresi
	BootstrapURL string              // Ağa katılmak için bootstrap sunucusu (opsiyonel)
	Port         uint16              // Bu düğümün dinlediği port
	UseDNS       bool                // DNS seed kullanılsın mı?

	// Nodes haritası hem keşif goroutine'inden hem HTTP handler'larından
	// erişildiği için kilitle korunur
	mux sync.RWMutex
}

// NewP2PNetwork creates a new P2P network manager
func NewP2PNetwork(myPort uint16, maxPeers int, bootstrapURL string) *P2PNetwork {
	host := GetHost()
	return &P2PNetwork{
		Nodes:        make(map[string]*P2PNode),
		MaxPeers:     maxPeers,
		MyAddress:    fmt.Sprintf("%s:%d", host, myPort),
		BootstrapURL: bootstrapURL,
		Port:         myPort,
		UseDNS:       false, // Varsayılan olarak kapalı
	}
}

// SetUseDNS DNS seed kullanımını açar veya kapatır
func (p *P2PNetwork) SetUseDNS(useDNS bool) {
	p.UseDNS = useDNS
	log.Printf("DNS seed kullanımı: %v", useDNS)
}

// ResolveDNSSeeds DNS seed'lerden düğüm listesi alır
func (p *P2PNetwork) ResolveDNSSeeds() []string {
	var nodeAddresses []string

	for _, seed := range DNSSeeds {
		log.Printf("DNS seed sorgulanıyor: %s", seed)
		ips, err := net.LookupIP(seed)
		if err != nil {
			log.Printf("DNS seed sorgulama hatası (%s): %v", seed, err)
			continue
		}

		for _, ip := range ips {
			if ipv4 := ip.To4(); ipv4 != nil {
				// Varsayılan port 5001 olarak atandı, gerekirse değiştirilebilir
				nodeAddr := fmt.Sprintf("%s:5001", ipv4.String())
				log.Printf("DNS seed'den düğüm bulundu: %s", nodeAddr)
				nodeAddresses = append(nodeAddresses, nodeAddr)
			}
		}
	}

	if len(nodeAddresses) > 0 {
		log.Printf("DNS seedlerden toplam %d düğüm bulundu", len(nodeAddresses))
	} else {
		log.Printf("DNS seedlerden düğüm bulunamadı")
	}

	return nodeAddresses
}

// ConnectToDNSSeeds DNS seed'lerden bulunan düğümlere bağlanır
func (p *P2PNetwork) ConnectToDNSSeeds() int {
	if !p.UseDNS {
		return 0
	}

	addresses := p.ResolveDNSSeeds()
	connectedCount := 0

	for _, addr := range addresses {
		// Düğümün çalışıp çalışmadığını kontrol et
		parts := strings.Split(addr, ":")
		if len(parts) != 2 {
			continue
		}

		host := parts[0]
		port, err := strconv.ParseUint(parts[1], 10, 16)
		if err != nil {
			continue
		}

		// Düğüme bağlanmayı dene
		if IsFoundHost(host, uint16(port)) {
			p.AddNode(addr)
			connectedCount++
			log.Printf("DNS seed'den bulunan düğüme bağlanıldı: %s", addr)
		} else {
			log.Printf("DNS seed'den bulunan düğüm aktif değil: %s", addr)
		}
	}

	log.Printf("DNS seed'lerden toplam %d düğüme bağlanıldı", connectedCount)
	return connectedCount
}

// AddNode adds a new node to the network if it doesn't exist
func (p *P2PNetwork) AddNode(address string) {
	// Kendi adresini eklemeye çalışıyorsa atla
	if address == p.MyAddress {
		return
	}

	p.mux.Lock()
	defer p.mux.Unlock()

	// Node zaten varsa bilgilerini güncelle
	if node, exists := p.Nodes[address]; exists {
		node.LastSeen = time.Now()
		return
	}

	// Maksimum peer sayısına ulaşıldıysa en eski nodu çıkar
	if len(p.Nodes) >= p.MaxPeers {
		var oldestAddress string
		var oldestTime time.Time = time.Now()

		// En eski nodu bul
		for addr, node := range p.Nodes {
			if node.LastSeen.Before(oldestTime) {
				oldestTime = node.LastSeen
				oldestAddress = addr
			}
		}

		// En eski nodu kaldır
		if oldestAddress != "" {
			delete(p.Nodes, oldestAddress)
			log.Printf("Removed oldest node %s to make room for new node", oldestAddress)
		}
	}

	// Yeni nodu ekle
	p.Nodes[address] = &P2PNode{
		Address:    address,
		LastSeen:   time.Now(),
		Reputation: 0,
	}
	log.Printf("Added new node: %s", address)
}

// RemoveNode removes a node from the network
func (p *P2PNetwork) RemoveNode(address string) {
	p.mux.Lock()
	defer p.mux.Unlock()
	delete(p.Nodes, address)
	log.Printf("Removed node: %s", address)
}

// GetActiveNodes returns a list of all active nodes
func (p *P2PNetwork) GetActiveNodes() []string {
	p.mux.RLock()
	defer p.mux.RUnlock()

	activeNodes := make([]string, 0)
	now := time.Now()

	// Son 30 dakika içinde görülmüş tüm nodları getir
	for addr, node := range p.Nodes {
		if now.Sub(node.LastSeen) < 30*time.Minute {
			activeNodes = append(activeNodes, addr)
		}
	}

	return activeNodes
}

// ConnectToBootstrap attempts to connect to the bootstrap server
// to get a list of nodes to connect to
func (p *P2PNetwork) ConnectToBootstrap() error {
	if p.BootstrapURL == "" {
		return fmt.Errorf("no bootstrap URL provided")
	}

	// Bootstrap sunucusuna bağlanarak node listesi iste
	resp, err := http.Get(p.BootstrapURL + "/nodes")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var nodeList []string
	if err := json.NewDecoder(resp.Body).Decode(&nodeList); err != nil {
		return err
	}

	// Alınan node listesini ağa ekle
	for _, address := range nodeList {
		// Node'un aktif olup olmadığını kontrol et
		if IsFoundHost(strings.Split(address, ":")[0],
			parsePort(strings.Split(address, ":")[1])) {
			p.AddNode(address)
		}
	}

	// Bootstrap sunucusuna kendimizi kaydet
	p.RegisterWithBootstrap()

	return nil
}

// RegisterWithBootstrap registers this node with the bootstrap server
func (p *P2PNetwork) RegisterWithBootstrap() error {
	if p.BootstrapURL == "" {
		return fmt.Errorf("no bootstrap URL provided")
	}

	data, _ := json.Marshal(map[string]string{"address": p.MyAddress})
	resp, err := http.Post(
		p.BootstrapURL+"/register",
		"application/json",
		bytes.NewBuffer(data),
	)

	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

// DiscoverNodes tüm düğüm keşif yöntemlerini kullanır
func (p *P2PNetwork) DiscoverNodes(startIp, endIp uint8, startPort, endPort uint16) {
	log.Printf("Düğüm keşif işlemi başlatılıyor...")

	// 1. Yerel ağ taraması
	log.Printf("Yerel ağ taraması yapılıyor...")
	neighbors := FindNeighbors(
		strings.Split(p.MyAddress, ":")[0],
		p.Port,
		startIp, endIp,
		startPort, endPort,
	)

	for _, neighbor := range neighbors {
		p.AddNode(neighbor)
	}

	// 2. DNS seed'lerden düğüm keşfi
	if p.UseDNS {
		log.Printf("DNS seed'lerden düğüm keşfi yapılıyor...")
		p.ConnectToDNSSeeds()
	}

	// 3. Bootstrap sunucusundan düğüm keşfi
	if p.BootstrapURL != "" {
		log.Printf("Bootstrap sunucusundan düğüm keşfi yapılıyor: %s", p.BootstrapURL)
		p.ConnectToBootstrap()
	}

	// 4. Bilinen düğümlerden diğer düğümleri iste
	p.GetPeersFromNodes()

	log.Printf("Düğüm keşif işlemi tamamlandı. Bilinen düğüm sayısı: %d", len(p.Nodes))
}

// GetPeersFromNodes asks other nodes for their known peers
func (p *P2PNetwork) GetPeersFromNodes() {
	for _, address := range p.GetKnownPeers() {
		// Her node'dan peer listesini iste
		resp, err := http.Get(fmt.Sprintf("http://%s/peers", address))
		if err != nil {
			continue
		}

		var peers []string
		if err := json.NewDecoder(resp.Body).Decode(&peers); err != nil {
			resp.Body.Close()
			continue
		}
		resp.Body.Close()

		// Alınan peerleri ekle
		for _, peer := range peers {
			p.AddNode(peer)
		}
	}
}

// BroadcastData sends data to all active nodes
func (p *P2PNetwork) BroadcastData(endpoint string, data interface{}) map[string]error {
	results := make(map[string]error)
	peers := p.GetKnownPeers()

	jsonData, err := json.Marshal(data)
	if err != nil {
		for _, addr := range peers {
			results[addr] = err
		}
		return results
	}

	// Tüm aktif nodlara veriyi gönder (ağ çağrıları kilit dışında)
	for _, addr := range peers {
		url := fmt.Sprintf("http://%s%s", addr, endpoint)
		resp, err := http.Post(
			url,
			"application/json",
			bytes.NewBuffer(jsonData),
		)

		if err != nil {
			results[addr] = err
			p.adjustReputation(addr, -1)
			continue
		}

		io.Copy(io.Discard, resp.Body) // Yanıtı boşalt
		resp.Body.Close()

		p.adjustReputation(addr, +1)
		results[addr] = nil
	}

	return results
}

// adjustReputation, node güvenilirlik puanını günceller; çok düşük puanlıları kaldırır
func (p *P2PNetwork) adjustReputation(addr string, delta int) {
	p.mux.Lock()
	defer p.mux.Unlock()
	if node, exists := p.Nodes[addr]; exists {
		node.Reputation += delta
		if node.Reputation < -5 {
			delete(p.Nodes, addr)
		}
	}
}

// parsePort helper function to parse port string to uint16
func parsePort(portStr string) uint16 {
	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return 0
	}
	return uint16(port)
}

// RunNodeDiscovery starts a periodic node discovery process
func (p *P2PNetwork) RunNodeDiscovery(interval time.Duration,
	startIp, endIp uint8, startPort, endPort uint16) {
	ticker := time.NewTicker(interval)

	// İlk keşfi hemen başlat
	p.DiscoverNodes(startIp, endIp, startPort, endPort)

	go func() {
		for {
			<-ticker.C
			p.DiscoverNodes(startIp, endIp, startPort, endPort)
			p.CleanupInactiveNodes(1 * time.Hour)
		}
	}()
}

// CleanupInactiveNodes removes nodes that haven't been seen for a while
func (p *P2PNetwork) CleanupInactiveNodes(maxAge time.Duration) {
	p.mux.Lock()
	defer p.mux.Unlock()

	now := time.Now()
	for addr, node := range p.Nodes {
		if now.Sub(node.LastSeen) > maxAge {
			delete(p.Nodes, addr)
			log.Printf("Removed inactive node: %s", addr)
		}
	}
}

// GetKnownPeers returns a list of all known peers
func (p *P2PNetwork) GetKnownPeers() []string {
	p.mux.RLock()
	defer p.mux.RUnlock()

	peers := make([]string, 0, len(p.Nodes))
	for addr := range p.Nodes {
		peers = append(peers, addr)
	}
	return peers
}

// AddPeer adds a new peer to the network
func (p *P2PNetwork) AddPeer(address string) {
	p.AddNode(address)
}

// EnableDNSSeeder enables or disables DNS seeder functionality
func (p *P2PNetwork) EnableDNSSeeder(enable bool) {
	p.SetUseDNS(enable)
}

// IsDNSSeederEnabled returns whether DNS seeder is enabled
func (p *P2PNetwork) IsDNSSeederEnabled() bool {
	return p.UseDNS
}

// SyncBlockchain synchronizes the blockchain with peers
func (p *P2PNetwork) SyncBlockchain(bc interface{}) {
	// Bu metot normalde block.Blockchain tipi almalı ve tüm peerlardan
	// blockchain verisini alarak senkronize etmeli
	// Burada basitleştirilmiş bir implementasyon yapıyoruz
	log.Println("Blockchain senkronizasyonu başlatıldı")

	for _, peer := range p.GetActiveNodes() {
		log.Printf("Peer ile senkronizasyon deneniyor: %s", peer)
		// Burada blockchain senkronizasyonu yapılır
	}

	log.Println("Blockchain senkronizasyonu tamamlandı")
}
