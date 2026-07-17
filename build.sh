#!/bin/bash

# Renklendirme için
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${YELLOW}FlatunChain Derleme Aracı${NC}"
echo "========================================"

# Mevcut dizin
DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
cd $DIR

# Build klasörünü oluştur
echo -e "${GREEN}Build klasörü hazırlanıyor...${NC}"
mkdir -p build

# Not: cüzdan arayüzü artık binary'ye gömülü (webui paketi) — harici
# template kopyalamaya gerek yok

# Her sistem için derleme
echo -e "${GREEN}Windows için derleniyor...${NC}"
GOOS=windows GOARCH=amd64 go build -o build/flatuncoin-windows.exe ./cmd/standalone

echo -e "${GREEN}Mac için derleniyor (Apple Silicon)...${NC}"
# Not: dmg paketi arm64; sidecar da native arm64 olmalı (PoW hızı için önemli).
# Intel Mac dağıtımı ileride ayrı artefakt olarak eklenecek (CI matrisi).
GOOS=darwin GOARCH=arm64 go build -o build/flatuncoin-mac ./cmd/standalone

echo -e "${GREEN}Linux için derleniyor...${NC}"
GOOS=linux GOARCH=amd64 go build -o build/flatuncoin-linux ./cmd/standalone

# Sunucu (droplet) binary'leri — linux/amd64
echo -e "${GREEN}Sunucu binary'leri derleniyor (linux/amd64)...${NC}"
mkdir -p build/server
GOOS=linux GOARCH=amd64 go build -o build/server/blockchain_server ./blockchain_server
GOOS=linux GOARCH=amd64 go build -o build/server/bootstrap_server ./cmd/bootstrap_server
GOOS=linux GOARCH=amd64 go build -o build/server/dns_updater ./cmd/dns_updater
GOOS=linux GOARCH=amd64 go build -o build/server/checkpoint_keygen ./cmd/checkpoint_keygen

echo -e "${GREEN}Derleme işlemi tamamlandı!${NC}"
echo "--------------------------------------"
echo -e "Uygulamayı çalıştırmak için: ${YELLOW}./build/flatuncoin-mac${NC} (veya windows/linux versiyonu)"
echo "Otomatik olarak tarayıcınız açılacak ve uygulama başlatılacaktır." 