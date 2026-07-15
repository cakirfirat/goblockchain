package main

import (
	"blockchain/block"
	"blockchain/utils"
	"blockchain/wallet"
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"path"
	"strconv"
)

const tempDir = "templates"

type WalletServer struct {
	port     uint16
	gatewawy string
}

func NewWalletServer(port uint16, gateway string) *WalletServer {
	return &WalletServer{port, gateway}
}
func (ws *WalletServer) Port() uint16 {
	return ws.port
}

func (ws *WalletServer) Gateway() string {
	return ws.gatewawy
}

func (ws *WalletServer) Index(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		t, err := template.ParseFiles(path.Join(tempDir, "index.html"))
		if err != nil {
			log.Printf("ERROR: Template parsing failed - %s", err.Error())
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		err = t.Execute(w, nil)
		if err != nil {
			log.Printf("ERROR: Template execution failed - %s", err.Error())
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
	default:
		log.Printf("ERROR: Invalid HTTP Method")
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
		value, err := strconv.ParseFloat(*t.Value, 32)
		if err != nil {
			log.Println("ERROR: parse error")
			io.WriteString(w, string(utils.JsonStatus("fail")))
			return
		}
		value32 := float32(value)

		w.Header().Add("Content-Type", "application/json")

		transaction := wallet.NewTransaction(privateKey, publicKey,
			*t.SenderBlockchainAddress, *t.RecipientBlockchainAddress, value32)
		signature := transaction.GenerateSignature()
		signatureStr := signature.String()
		timestamp := transaction.Timestamp()

		bt := &block.TransactionRequest{
			SenderBlockchainAddress:    t.SenderBlockchainAddress,
			RecipientBlockchainAddress: t.RecipientBlockchainAddress,
			SenderPublicKey:            t.SenderPublicKey,
			Value:                      &value32,
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
			Mnemonic                   string  `json:"mnemonic"`
			Passphrase                 string  `json:"passphrase,omitempty"`
			RecipientBlockchainAddress string  `json:"recipient_blockchain_address"`
			Value                      float32 `json:"value"`
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

		// HD cüzdan oluştur
		hdWallet := wallet.NewHDWalletFromMnemonic(t.Mnemonic, t.Passphrase)

		// İşlem oluştur
		transaction := hdWallet.CreateTransaction(t.RecipientBlockchainAddress, t.Value)
		signature := transaction.GenerateSignature()
		signatureStr := signature.String()

		// Blockchain sunucusuna gönder
		pubKeyStr := hdWallet.PublicKeyStr()
		timestamp := transaction.Timestamp()
		bt := &block.TransactionRequest{
			SenderBlockchainAddress:    &hdWallet.BlockchainAddress,
			RecipientBlockchainAddress: &t.RecipientBlockchainAddress,
			SenderPublicKey:            &pubKeyStr,
			Value:                      &t.Value,
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
				Message string  `json:"message"`
				Amount  float32 `json:"amount"`
			}{
				Message: "success",
				Amount:  bar.Amount,
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

func (ws *WalletServer) Run() {
	http.HandleFunc("/", ws.Index)
	http.HandleFunc("/wallet", ws.Wallet)
	http.HandleFunc("/wallet/amount", ws.WalletAmount)
	http.HandleFunc("/transaction", ws.CreateTransaction)

	// Yeni HD cüzdan endpointleri
	http.HandleFunc("/wallet/hd/import", ws.ImportHDWallet)
	http.HandleFunc("/wallet/hd/derive", ws.DeriveAddresses)
	http.HandleFunc("/transaction/hd", ws.CreateHDTransaction)

	log.Fatal(http.ListenAndServe("0.0.0.0:"+strconv.Itoa(int(ws.Port())), nil))
}
