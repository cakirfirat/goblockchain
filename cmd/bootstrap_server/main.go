package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

// normalizeAddress, node'un bildirdiği adresi bağlantının gerçek kaynağıyla düzeltir.
// Node kendi adresini loopback (::1, 127.x) olarak görebilir ya da NAT arkasında
// özel IP bildirebilir; bu durumda DNS'e ve peer listesine işe yaramaz adres girer.
// Kural: bildirilen host loopback ise, ya da özel IP olup bağlantı public bir
// IP'den geldiyse, bağlantının kaynak IP'si kullanılır (port bildirilen kalır).
func normalizeAddress(reported, remoteAddr string) string {
	host, port, err := net.SplitHostPort(reported)
	if err != nil {
		return reported
	}
	remoteHost, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return reported
	}

	reportedIP := net.ParseIP(host)
	remoteIP := net.ParseIP(remoteHost)
	if remoteIP == nil {
		return reported
	}

	replace := false
	if reportedIP == nil || reportedIP.IsLoopback() || reportedIP.IsUnspecified() {
		replace = true
	} else if reportedIP.IsPrivate() && !remoteIP.IsPrivate() && !remoteIP.IsLoopback() {
		replace = true
	}

	if replace && !remoteIP.IsLoopback() {
		corrected := net.JoinHostPort(remoteHost, port)
		log.Printf("Adres düzeltildi: %s -> %s", reported, corrected)
		return corrected
	}
	return reported
}

// RegisteredNode kayıtlı bir düğümü temsil eder
type RegisteredNode struct {
	Address  string    `json:"address"`
	LastSeen time.Time `json:"last_seen"`
}

// BootstrapServer bir bootstrap sunucusunu temsil eder
type BootstrapServer struct {
	nodes map[string]*RegisteredNode
	mutex sync.RWMutex
}

// NewBootstrapServer yeni bir bootstrap sunucusu oluşturur
func NewBootstrapServer() *BootstrapServer {
	return &BootstrapServer{
		nodes: make(map[string]*RegisteredNode),
	}
}

// RegisterNode yeni bir düğümü kaydeder veya mevcut bir düğümün son görülme zamanını günceller
func (bs *BootstrapServer) RegisterNode(addr string) {
	bs.mutex.Lock()
	defer bs.mutex.Unlock()

	bs.nodes[addr] = &RegisteredNode{
		Address:  addr,
		LastSeen: time.Now(),
	}
	log.Printf("Node registered/updated: %s", addr)
}

// GetNodes aktif düğümlerin bir listesini döndürür
func (bs *BootstrapServer) GetNodes() []string {
	bs.mutex.RLock()
	defer bs.mutex.RUnlock()

	now := time.Now()
	activeNodes := make([]string, 0)

	// Son 30 dakika içinde görülen düğümleri listele
	for addr, node := range bs.nodes {
		if now.Sub(node.LastSeen) < 30*time.Minute {
			activeNodes = append(activeNodes, addr)
		}
	}

	return activeNodes
}

// CleanupInactiveNodes eskimiş düğümleri kaldırır
func (bs *BootstrapServer) CleanupInactiveNodes() {
	bs.mutex.Lock()
	defer bs.mutex.Unlock()

	now := time.Now()
	for addr, node := range bs.nodes {
		// 1 saatten daha uzun süredir görülmeyen düğümleri kaldır
		if now.Sub(node.LastSeen) > 1*time.Hour {
			delete(bs.nodes, addr)
			log.Printf("Removed inactive node: %s", addr)
		}
	}
}

// StartCleanupTask düzenli aralıklarla temizleme görevi başlatır
func (bs *BootstrapServer) StartCleanupTask() {
	ticker := time.NewTicker(10 * time.Minute)
	go func() {
		for {
			<-ticker.C
			bs.CleanupInactiveNodes()
		}
	}()
}

func main() {
	port := flag.Int("port", 8000, "Bootstrap server port")
	flag.Parse()

	bs := NewBootstrapServer()
	bs.StartCleanupTask()

	// Register handler - POST /register
	http.HandleFunc("/register", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			Address string `json:"address"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		if req.Address == "" {
			http.Error(w, "Address is required", http.StatusBadRequest)
			return
		}

		bs.RegisterNode(normalizeAddress(req.Address, r.RemoteAddr))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok"}`))
	})

	// Nodes list handler - GET /nodes
	http.HandleFunc("/nodes", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		nodes := bs.GetNodes()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(nodes)
	})

	// Status handler - GET /status
	http.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		status := struct {
			NodeCount int `json:"node_count"`
		}{
			NodeCount: len(bs.GetNodes()),
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(status)
	})

	// Root handler - GET /
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Blockchain Bootstrap Server"))
	})

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("Bootstrap server starting on %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
