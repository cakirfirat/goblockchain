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

# Gerekli klasörleri kopyala
echo -e "${GREEN}Statik dosyalar kopyalanıyor...${NC}"
mkdir -p build/templates
cp -r wallet_server/templates/* build/templates/

# Her sistem için derleme
echo -e "${GREEN}Windows için derleniyor...${NC}"
GOOS=windows GOARCH=amd64 go build -o build/flatuncoin-windows.exe cmd/standalone/main.go

echo -e "${GREEN}Mac için derleniyor...${NC}"
GOOS=darwin GOARCH=amd64 go build -o build/flatuncoin-mac cmd/standalone/main.go

echo -e "${GREEN}Linux için derleniyor...${NC}"
GOOS=linux GOARCH=amd64 go build -o build/flatuncoin-linux cmd/standalone/main.go

echo -e "${GREEN}Derleme işlemi tamamlandı!${NC}"
echo "--------------------------------------"
echo -e "Uygulamayı çalıştırmak için: ${YELLOW}./build/flatuncoin-mac${NC} (veya windows/linux versiyonu)"
echo "Otomatik olarak tarayıcınız açılacak ve uygulama başlatılacaktır." 