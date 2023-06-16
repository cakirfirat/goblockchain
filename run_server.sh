#!/bin/bash

# Çıktı dosyasını belirleyin
output_file="output.txt"

# go run komutunu çalıştırırken çıktıları output_file'a yönlendirin
go run . > "$output_file" 2>&1 &

# wallet_server klasörüne girin ve go run . komutunu çalıştırın
(cd wallet_server && go run .) &

# blockchain_server klasörüne girin ve go run . komutunu çalıştırın
(cd blockchain_server && go run .)