package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// DigitalOcean API'si için konfigürasyon
type DOConfig struct {
	APIToken  string
	Domain    string
	SubDomain string
}

// DigitalOcean DNS kaydı
type DODNSRecord struct {
	ID       string `json:"id,omitempty"`
	Type     string `json:"type"`
	Name     string `json:"name"`
	Data     string `json:"data"`
	Priority int    `json:"priority,omitempty"`
	Port     int    `json:"port,omitempty"`
	TTL      int    `json:"ttl"`
	Weight   int    `json:"weight,omitempty"`
	Flags    int    `json:"flags,omitempty"`
	Tag      string `json:"tag,omitempty"`
}

// DigitalOcean API Yanıtı
type DOAPIResponse struct {
	DomainRecord  DODNSRecord   `json:"domain_record,omitempty"`
	DomainRecords []DODNSRecord `json:"domain_records,omitempty"`
}

// Aktif düğüm
type ActiveNode struct {
	IP   string
	Port int
}

// DigitalOcean API istemcisi
type DOClient struct {
	Config DOConfig
	Client *http.Client
}

// Yeni DigitalOcean istemcisi oluştur
func NewDOClient(apiToken, domain, subDomain string) *DOClient {
	return &DOClient{
		Config: DOConfig{
			APIToken:  apiToken,
			Domain:    domain,
			SubDomain: subDomain,
		},
		Client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// DigitalOcean API'sine istek gönder
func (c *DOClient) sendRequest(method, url string, body []byte) ([]byte, error) {
	req, err := http.NewRequest(method, url, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.Config.APIToken)

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("API hatası: %s", resp.Status)
	}

	return ioutil.ReadAll(resp.Body)
}

// DNS kayıtlarını getir
func (c *DOClient) GetDNSRecords() ([]DODNSRecord, error) {
	url := fmt.Sprintf("https://api.digitalocean.com/v2/domains/%s/records?per_page=100", c.Config.Domain)

	body, err := c.sendRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	var response struct {
		DomainRecords []DODNSRecord `json:"domain_records"`
	}
	err = json.Unmarshal(body, &response)
	if err != nil {
		return nil, err
	}

	return response.DomainRecords, nil
}

// A kaydı oluştur
func (c *DOClient) CreateDNSRecord(name, ip string) (DODNSRecord, error) {
	url := fmt.Sprintf("https://api.digitalocean.com/v2/domains/%s/records", c.Config.Domain)

	record := DODNSRecord{
		Type: "A",
		Name: name,
		Data: ip,
		TTL:  300,
	}

	jsonData, err := json.Marshal(record)
	if err != nil {
		return DODNSRecord{}, err
	}

	body, err := c.sendRequest("POST", url, jsonData)
	if err != nil {
		return DODNSRecord{}, err
	}

	var response struct {
		DomainRecord DODNSRecord `json:"domain_record"`
	}
	err = json.Unmarshal(body, &response)
	if err != nil {
		return DODNSRecord{}, err
	}

	return response.DomainRecord, nil
}

// DNS kaydını sil
func (c *DOClient) DeleteDNSRecord(recordID string) error {
	url := fmt.Sprintf("https://api.digitalocean.com/v2/domains/%s/records/%s", c.Config.Domain, recordID)

	_, err := c.sendRequest("DELETE", url, nil)
	return err
}

// Düğümün aktif olup olmadığını kontrol et
func isNodeActive(ip string, port int) bool {
	address := fmt.Sprintf("%s:%d", ip, port)
	conn, err := net.DialTimeout("tcp", address, 5*time.Second)
	if err != nil {
		return false
	}
	defer conn.Close()
	return true
}

// Düğümleri dosyadan oku
func readNodesFromFile(filePath string) ([]ActiveNode, error) {
	if filePath == "" {
		return []ActiveNode{}, nil
	}

	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(data), "\n")
	nodes := []ActiveNode{}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Split(line, ":")
		if len(parts) != 2 {
			log.Printf("Geçersiz düğüm formatı: %s", line)
			continue
		}

		port, err := strconv.Atoi(parts[1])
		if err != nil {
			log.Printf("Geçersiz port: %s", parts[1])
			continue
		}

		nodes = append(nodes, ActiveNode{
			IP:   parts[0],
			Port: port,
		})
	}

	return nodes, nil
}

// Ana fonksiyon
func main() {
	// Komut satırı parametreleri
	apiToken := flag.String("token", "", "DigitalOcean API Token")
	domain := flag.String("domain", "flatuncoin.com", "Alan adı")
	subDomain := flag.String("subdomain", "seed", "Alt alan adı")
	nodeFile := flag.String("nodes", "", "Düğüm listesi dosyası (her satırda ip:port)")
	checkInterval := flag.Int("interval", 15, "Kontrol aralığı (dakika)")
	defaultPort := flag.Int("port", 5001, "Varsayılan port")
	flag.Parse()

	// Token kontrol et
	if *apiToken == "" {
		// Çevre değişkeninden token almayı dene
		*apiToken = os.Getenv("DO_API_TOKEN")
		if *apiToken == "" {
			log.Fatal("DigitalOcean API token gerekli. --token parametresi veya DO_API_TOKEN çevre değişkeni kullanın.")
		}
	}

	// DigitalOcean istemcisi oluştur
	client := NewDOClient(*apiToken, *domain, *subDomain)

	// DNS güncellemesi yapacak fonksiyon
	updateDNS := func() {
		log.Println("DNS güncellemesi başlatılıyor...")

		// Mevcut DNS kayıtlarını al
		records, err := client.GetDNSRecords()
		if err != nil {
			log.Printf("DNS kayıtları alınamadı: %v", err)
			return
		}

		// Alt alan adına ait DNS kayıtlarını filtrele
		seedRecords := make(map[string]DODNSRecord)
		for _, record := range records {
			if record.Type == "A" && record.Name == *subDomain {
				seedRecords[record.Data] = record // IP -> Record
			}
		}

		log.Printf("Mevcut DNS kayıtları: %d (%s.%s)", len(seedRecords), *subDomain, *domain)

		// Aktif düğümleri kontrol et
		var activeNodes []ActiveNode

		// Dosyada belirtilen düğümleri kontrol et
		if *nodeFile != "" {
			fileNodes, err := readNodesFromFile(*nodeFile)
			if err != nil {
				log.Printf("Düğüm dosyası okunamadı: %v", err)
			} else {
				for _, node := range fileNodes {
					if isNodeActive(node.IP, node.Port) {
						activeNodes = append(activeNodes, node)
						log.Printf("Aktif düğüm bulundu: %s:%d", node.IP, node.Port)
					} else {
						log.Printf("Düğüm aktif değil: %s:%d", node.IP, node.Port)
					}
				}
			}
		}

		// Aktif olan ve DNS'te olmayan düğümleri ekle
		for _, node := range activeNodes {
			if _, exists := seedRecords[node.IP]; !exists {
				log.Printf("Yeni DNS kaydı ekleniyor: %s -> %s.%s", node.IP, *subDomain, *domain)
				_, err := client.CreateDNSRecord(*subDomain, node.IP)
				if err != nil {
					log.Printf("DNS kaydı eklenemedi: %v", err)
				} else {
					log.Printf("DNS kaydı eklendi: %s -> %s.%s", node.IP, *subDomain, *domain)
				}
			} else {
				log.Printf("DNS kaydı zaten var: %s -> %s.%s", node.IP, *subDomain, *domain)
				// Mevcut kayıtlar listesinden çıkar (sonra silinmemesi için)
				delete(seedRecords, node.IP)
			}
		}

		// DNS'te olan ama artık aktif olmayan düğümleri sil
		for ip, record := range seedRecords {
			if isNodeActive(ip, *defaultPort) {
				log.Printf("Mevcut düğüm hala aktif: %s:%d", ip, *defaultPort)
			} else {
				log.Printf("Eski DNS kaydı siliniyor: %s (%s)", ip, record.ID)
				err := client.DeleteDNSRecord(record.ID)
				if err != nil {
					log.Printf("DNS kaydı silinemedi: %v", err)
				} else {
					log.Printf("DNS kaydı silindi: %s", ip)
				}
			}
		}

		log.Println("DNS güncellemesi tamamlandı.")
	}

	// İlk güncellemeyi yap
	updateDNS()

	// Periyodik güncelleme
	log.Printf("Periyodik DNS güncellemesi başlatıldı. Aralık: %d dakika", *checkInterval)
	ticker := time.NewTicker(time.Duration(*checkInterval) * time.Minute)
	for {
		<-ticker.C
		updateDNS()
	}
}
