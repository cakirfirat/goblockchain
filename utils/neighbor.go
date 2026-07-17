package utils

import (
	"fmt"
	"net"
	"os"
	"regexp"
	"strconv"
	"time"
)

func IsFoundHost(host string, port uint16) bool {
	target := net.JoinHostPort(host, strconv.Itoa(int(port)))

	conn, err := net.DialTimeout("tcp", target, 1*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true

}

//192.168.0.10:5000
//192.168.0.11:5000
//192.168.0.12:5000
//192.168.0.10:5001
//192.168.0.10:5002
//192.168.0.10:5003

var PATTERN = regexp.MustCompile(`((25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?\.){3})(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)`)

func FindNeighbors(myHost string, myPort uint16, startIp uint8, endIp uint8, startPort uint16, endPort uint16) []string {
	address := fmt.Sprintf("%s:%d", myHost, myPort)

	m := PATTERN.FindStringSubmatch(myHost)
	if m == nil {
		return nil
	}
	prefixHost := m[1]
	lastIp, _ := strconv.Atoi(m[len(m)-1])
	neighbors := make([]string, 0)

	for port := startPort; port <= endPort; port += 1 {
		for ip := startIp; ip <= endIp; ip += 1 {
			guessHost := fmt.Sprintf("%s%d", prefixHost, lastIp+int(ip))
			guessTarget := fmt.Sprintf("%s:%d", guessHost, port)
			if guessTarget != address && IsFoundHost(guessHost, port) {
				neighbors = append(neighbors, guessTarget)
			}
		}
	}
	return neighbors

}

// GetHost, bu makinenin ağdaki IPv4 adresini döndürür.
// Önce ağ arayüzleri taranır (hostname çözümlemesi çoğu sistemde ::1 veya
// 127.0.0.1 döndürüp P2P adresini bozuyordu); global adres önceliklidir.
func GetHost() string {
	addrs, err := net.InterfaceAddrs()
	if err == nil {
		var privateIP string
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipnet.IP.To4()
			if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
				continue
			}
			if ip.IsPrivate() {
				if privateIP == "" {
					privateIP = ip.String()
				}
				continue
			}
			// Global (public) IPv4 bulundu — en iyi aday
			return ip.String()
		}
		if privateIP != "" {
			return privateIP
		}
	}

	// Eski davranışa geri düş
	hostname, err := os.Hostname()
	if err != nil {
		return "127.0.0.1"
	}
	address, err := net.LookupHost(hostname)
	if err != nil || len(address) == 0 {
		return "127.0.0.1"
	}
	return address[0]
}
