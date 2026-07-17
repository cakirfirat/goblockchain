// dns_updater, DNS seed kayıtlarını canlı tutar.
//
// Her döngüde bootstrap sunucusundan aktif node listesini çeker (ve/veya
// --nodes dosyasını okur), erişilebilir PUBLIC IPv4 adresleri için
// <subdomain>.<domain> altında A kaydı oluşturur; artık erişilemeyen
// node'ların kayıtlarını siler. Yalnızca --subdomain ile eşleşen A
// kayıtlarına dokunur — zone'daki diğer kayıtlar güvendedir.
//
// Kullanım (droplet üzerinde):
//
//	export DO_API_TOKEN=...
//	./dns_updater --domain=yoxar.com --subdomain=seed --bootstrap=http://127.0.0.1:8000 --interval=10
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// DigitalOcean DNS kaydı — ID, API'de SAYI olarak döner (string değil)
type DODNSRecord struct {
	ID   int64  `json:"id,omitempty"`
	Type string `json:"type"`
	Name string `json:"name"`
	Data string `json:"data"`
	TTL  int    `json:"ttl"`
}

// Aktif düğüm
type ActiveNode struct {
	IP   string
	Port int
}

// DigitalOcean API istemcisi
type DOClient struct {
	APIToken string
	Domain   string
	Client   *http.Client
}

func NewDOClient(apiToken, domain string) *DOClient {
	return &DOClient{
		APIToken: apiToken,
		Domain:   domain,
		Client:   &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *DOClient) sendRequest(method, url string, body []byte) ([]byte, error) {
	req, err := http.NewRequest(method, url, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIToken)

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("API hatası: %s — %s", resp.Status, truncate(string(respBody), 200))
	}

	return respBody, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// GetDNSRecords, domaindeki tüm kayıtları getirir
func (c *DOClient) GetDNSRecords() ([]DODNSRecord, error) {
	url := fmt.Sprintf("https://api.digitalocean.com/v2/domains/%s/records?per_page=200", c.Domain)

	body, err := c.sendRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	var response struct {
		DomainRecords []DODNSRecord `json:"domain_records"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}

	return response.DomainRecords, nil
}

// CreateDNSRecord, A kaydı oluşturur
func (c *DOClient) CreateDNSRecord(name, ip string, ttl int) error {
	url := fmt.Sprintf("https://api.digitalocean.com/v2/domains/%s/records", c.Domain)

	record := DODNSRecord{
		Type: "A",
		Name: name,
		Data: ip,
		TTL:  ttl,
	}

	jsonData, err := json.Marshal(record)
	if err != nil {
		return err
	}

	_, err = c.sendRequest("POST", url, jsonData)
	return err
}

// DeleteDNSRecord, kaydı ID ile siler
func (c *DOClient) DeleteDNSRecord(recordID int64) error {
	url := fmt.Sprintf("https://api.digitalocean.com/v2/domains/%s/records/%d", c.Domain, recordID)

	_, err := c.sendRequest("DELETE", url, nil)
	return err
}

// isNodeActive, node'a TCP bağlantısı kurmayı dener
func isNodeActive(ip string, port int) bool {
	address := net.JoinHostPort(ip, strconv.Itoa(port))
	conn, err := net.DialTimeout("tcp", address, 5*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// isPublicIPv4: DNS'e yalnızca genel (public) IPv4 adresleri yazılır;
// loopback/özel ağ adresleri seed kaydı olarak işe yaramaz
func isPublicIPv4(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil || ip.To4() == nil {
		return false
	}
	return !ip.IsLoopback() && !ip.IsPrivate() && !ip.IsLinkLocalUnicast() && !ip.IsUnspecified()
}

// nodesFromBootstrap, bootstrap sunucusundan kayıtlı aktif node listesini çeker
func nodesFromBootstrap(bootstrapURL string) ([]ActiveNode, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(strings.TrimRight(bootstrapURL, "/") + "/nodes")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var addresses []string
	if err := json.NewDecoder(resp.Body).Decode(&addresses); err != nil {
		return nil, err
	}

	nodes := make([]ActiveNode, 0, len(addresses))
	for _, addr := range addresses {
		host, portStr, err := net.SplitHostPort(addr)
		if err != nil {
			log.Printf("Geçersiz node adresi atlandı: %s", addr)
			continue
		}
		port, err := strconv.Atoi(portStr)
		if err != nil {
			continue
		}
		nodes = append(nodes, ActiveNode{IP: host, Port: port})
	}
	return nodes, nil
}

// nodesFromFile, düğümleri dosyadan okur (her satır ip:port, # ile yorum)
func nodesFromFile(filePath string) ([]ActiveNode, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	nodes := []ActiveNode{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		host, portStr, err := net.SplitHostPort(line)
		if err != nil {
			log.Printf("Geçersiz düğüm formatı: %s", line)
			continue
		}
		port, err := strconv.Atoi(portStr)
		if err != nil {
			log.Printf("Geçersiz port: %s", portStr)
			continue
		}
		nodes = append(nodes, ActiveNode{IP: host, Port: port})
	}
	return nodes, nil
}

func main() {
	apiToken := flag.String("token", "", "DigitalOcean API Token (boşsa DO_API_TOKEN ortam değişkeni)")
	domain := flag.String("domain", "yoxar.com", "Alan adı")
	subDomain := flag.String("subdomain", "seed", "Seed alt alan adı (yalnızca bu isimli A kayıtlarına dokunulur)")
	bootstrapURL := flag.String("bootstrap", "", "Bootstrap sunucusu URL'i (aktif node listesi buradan çekilir)")
	nodeFile := flag.String("nodes", "", "Ek düğüm listesi dosyası (her satırda ip:port) [isteğe bağlı]")
	checkInterval := flag.Int("interval", 15, "Kontrol aralığı (dakika)")
	defaultPort := flag.Int("port", 5001, "Mevcut DNS kayıtlarının canlılık kontrolünde kullanılacak port")
	ttl := flag.Int("ttl", 300, "Oluşturulan A kayıtlarının TTL değeri (saniye)")
	once := flag.Bool("once", false, "Tek sefer çalıştır ve çık (test için)")
	flag.Parse()

	if *apiToken == "" {
		*apiToken = os.Getenv("DO_API_TOKEN")
		if *apiToken == "" {
			log.Fatal("DigitalOcean API token gerekli: --token parametresi veya DO_API_TOKEN ortam değişkeni")
		}
	}
	if *bootstrapURL == "" && *nodeFile == "" {
		log.Fatal("Node kaynağı gerekli: --bootstrap ve/veya --nodes verin")
	}

	client := NewDOClient(*apiToken, *domain)

	updateDNS := func() {
		log.Println("DNS güncellemesi başlatılıyor...")

		// 1. Aday node'ları topla
		var candidates []ActiveNode
		if *bootstrapURL != "" {
			nodes, err := nodesFromBootstrap(*bootstrapURL)
			if err != nil {
				log.Printf("UYARI: bootstrap'tan node listesi alınamadı: %v", err)
			} else {
				log.Printf("Bootstrap'tan %d node alındı", len(nodes))
				candidates = append(candidates, nodes...)
			}
		}
		if *nodeFile != "" {
			nodes, err := nodesFromFile(*nodeFile)
			if err != nil {
				log.Printf("UYARI: düğüm dosyası okunamadı: %v", err)
			} else {
				candidates = append(candidates, nodes...)
			}
		}

		// 2. Public + erişilebilir olanları süz (tekilleştirerek)
		activeIPs := make(map[string]bool)
		for _, node := range candidates {
			if activeIPs[node.IP] {
				continue
			}
			if !isPublicIPv4(node.IP) {
				log.Printf("Atlandı (public IPv4 değil): %s", node.IP)
				continue
			}
			if !isNodeActive(node.IP, node.Port) {
				log.Printf("Node aktif değil: %s:%d", node.IP, node.Port)
				continue
			}
			log.Printf("Aktif node: %s:%d", node.IP, node.Port)
			activeIPs[node.IP] = true
		}

		// 3. Mevcut seed kayıtlarını al
		records, err := client.GetDNSRecords()
		if err != nil {
			log.Printf("HATA: DNS kayıtları alınamadı: %v", err)
			return
		}

		seedRecords := make(map[string]DODNSRecord) // IP -> kayıt
		for _, record := range records {
			if record.Type == "A" && record.Name == *subDomain {
				seedRecords[record.Data] = record
			}
		}
		log.Printf("Mevcut seed kaydı: %d (%s.%s)", len(seedRecords), *subDomain, *domain)

		// 4. Aktif olup kaydı olmayanları ekle
		for ip := range activeIPs {
			if _, exists := seedRecords[ip]; exists {
				delete(seedRecords, ip) // korunacak; silme adayı olmasın
				continue
			}
			if err := client.CreateDNSRecord(*subDomain, ip, *ttl); err != nil {
				log.Printf("HATA: DNS kaydı eklenemedi (%s): %v", ip, err)
			} else {
				log.Printf("DNS kaydı eklendi: %s.%s -> %s", *subDomain, *domain, ip)
			}
		}

		// 5. Kaydı olup artık aktif olmayanları sil (son bir canlılık kontrolüyle)
		for ip, record := range seedRecords {
			if isNodeActive(ip, *defaultPort) {
				log.Printf("Kayıtlı node hâlâ aktif: %s:%d", ip, *defaultPort)
				continue
			}
			if err := client.DeleteDNSRecord(record.ID); err != nil {
				log.Printf("HATA: DNS kaydı silinemedi (%s, id=%d): %v", ip, record.ID, err)
			} else {
				log.Printf("Eski DNS kaydı silindi: %s (id=%d)", ip, record.ID)
			}
		}

		log.Println("DNS güncellemesi tamamlandı.")
	}

	updateDNS()

	if *once {
		return
	}

	log.Printf("Periyodik DNS güncellemesi başlatıldı. Aralık: %d dakika", *checkInterval)
	ticker := time.NewTicker(time.Duration(*checkInterval) * time.Minute)
	for {
		<-ticker.C
		updateDNS()
	}
}
