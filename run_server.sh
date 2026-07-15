#!/bin/bash

# Çıktı dosyasını belirleyin
output_file="output.txt"

# DNS seed kullanımını etkinleştirin
use_dns=true

# Bootstrap sunucusunu başlatın (8000 portunda)
(cd cmd/bootstrap_server && go run . --port=8000) &
bootstrap_pid=$!
echo "Bootstrap server started with PID: $bootstrap_pid"

# Birinci blockchain node'unu 5001 portu ile başlat
# DNS seed desteği eklenmiş halde başlat
if [ "$use_dns" = true ]; then
  (cd blockchain_server && go run . --port=5001 --bootstrap=http://localhost:8000 --dns=true) &
else
  (cd blockchain_server && go run . --port=5001 --bootstrap=http://localhost:8000) &
fi
blockchain1_pid=$!
echo "Blockchain node 1 started with PID: $blockchain1_pid"

# İkinci blockchain node'unu 5002 portu ile başlat
# DNS seed desteği eklenmiş halde başlat
if [ "$use_dns" = true ]; then
  (cd blockchain_server && go run . --port=5002 --bootstrap=http://localhost:8000 --dns=true) &
else
  (cd blockchain_server && go run . --port=5002 --bootstrap=http://localhost:8000) &
fi
blockchain2_pid=$!
echo "Blockchain node 2 started with PID: $blockchain2_pid"

# go run komutunu çalıştırırken çıktıları output_file'a yönlendirin
go run . > "$output_file" 2>&1 &
main_pid=$!
echo "Main application started with PID: $main_pid"

# wallet_server klasörüne girin ve go run . komutunu çalıştırın
(cd wallet_server && go run .) &
wallet_pid=$!
echo "Wallet server started with PID: $wallet_pid"

# DNS Updater'ı çalıştır (DigitalOcean API token varsa)
if [ -n "$DO_API_TOKEN" ]; then
  echo "DigitalOcean API token bulundu, DNS updater başlatılıyor..."
  (cd cmd/dns_updater && go run . --domain=flatuncoin.com --subdomain=seed --interval=5) &
  dns_updater_pid=$!
  echo "DNS updater started with PID: $dns_updater_pid"
else
  echo "DigitalOcean API token bulunamadı, DNS updater çalıştırılmadı"
  echo "DNS updater çalıştırmak için: export DO_API_TOKEN=your_token"
fi

# Bekleme fonksiyonu
read -p "Press Enter to stop all services..." key

# Tüm işlemleri sonlandır
kill $bootstrap_pid $blockchain1_pid $blockchain2_pid $main_pid $wallet_pid
# DNS updater çalışıyorsa onu da sonlandır
if [ -n "$dns_updater_pid" ]; then
  kill $dns_updater_pid
fi
echo "All services stopped"