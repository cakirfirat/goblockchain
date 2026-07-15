#!/bin/bash

# Sunucu adresini burada belirtin (IP adresi veya domain adı)
REMOTE_SERVER="147.182.187.4"  # Örnek: 192.168.1.100 veya example.com

# Uzak sunucudaki bootstrap port'u
BOOTSTRAP_PORT=8000

# Çıktı dosyasını belirleyin
output_file="local_node_output.txt"

# DNS seed kullanımını etkinleştirin
use_dns=true

# Yerel düğümün port numarası (uzak sunucuda kullanılanlardan farklı olmalı)
LOCAL_PORT=5003

echo "Uzak sunucuya bağlanacak yerel node başlatılıyor..."
echo "Uzak sunucu: $REMOTE_SERVER"
echo "Yerel port: $LOCAL_PORT"

# Blockchain node'unu başlat ve uzak bootstrap'a bağlan
if [ "$use_dns" = true ]; then
  (cd blockchain_server && go run . --port=$LOCAL_PORT --bootstrap=http://$REMOTE_SERVER:$BOOTSTRAP_PORT --dns=true) &
else
  (cd blockchain_server && go run . --port=$LOCAL_PORT --bootstrap=http://$REMOTE_SERVER:$BOOTSTRAP_PORT) &
fi
blockchain_pid=$!
echo "Blockchain node başlatıldı, PID: $blockchain_pid"

# Bekleme fonksiyonu
read -p "Durdurmak için Enter tuşuna basın..." key

# İşlemi sonlandır
kill $blockchain_pid
echo "Node durduruldu" 