#!/bin/bash
# FlatunChain droplet kurulum scripti
#
# DROPLET ÜZERİNDE root olarak çalıştırılır. Öncesinde yerelden şu dosyalar
# kopyalanmış olmalı (bkz. deploy/README.md):
#   build/server/*   -> droplet:/root/flatun-deploy/bin/
#   deploy/systemd/* -> droplet:/root/flatun-deploy/systemd/
#   deploy/setup_droplet.sh -> droplet:/root/flatun-deploy/
#
# Script: kullanıcı + dizinleri oluşturur, binary'leri ve servisleri kurar,
# checkpoint anahtarı üretir (yoksa), firewall'u açar, servisleri başlatır.
set -euo pipefail

DEPLOY_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
INSTALL_DIR=/opt/flatunchain
ENV_FILE=/etc/flatunchain/env

echo "==> FlatunChain kurulumu başlıyor"

# 1. Servis kullanıcısı
if ! id flatun &>/dev/null; then
    useradd --system --home-dir $INSTALL_DIR --shell /usr/sbin/nologin flatun
    echo "  flatun servis kullanıcısı oluşturuldu"
fi

# 2. Dizinler ve binary'ler
mkdir -p $INSTALL_DIR/bin $INSTALL_DIR/data /etc/flatunchain
cp "$DEPLOY_DIR"/bin/* $INSTALL_DIR/bin/
chmod +x $INSTALL_DIR/bin/*
echo "  Binary'ler kuruldu: $INSTALL_DIR/bin"

# 3. Ortam dosyası + checkpoint anahtarı
if [ ! -f $ENV_FILE ]; then
    echo "  Checkpoint otorite anahtarı üretiliyor..."
    $INSTALL_DIR/bin/checkpoint_keygen --machine > $ENV_FILE
    echo "DO_API_TOKEN=BURAYA_TOKEN_YAZIN" >> $ENV_FILE
    echo "  $ENV_FILE oluşturuldu"
    echo ""
    echo "  ÖNEMLİ: Diğer node'lara dağıtılacak AÇIK anahtar:"
    grep CHECKPOINT_PUBKEY $ENV_FILE
    echo ""
    echo "  ÖNEMLİ: $ENV_FILE içindeki DO_API_TOKEN satırını düzenleyin!"
else
    echo "  $ENV_FILE zaten var, dokunulmadı"
fi
chmod 600 $ENV_FILE
chown root:root $ENV_FILE

# 4. Veri dizini sahipliği
chown -R flatun:flatun $INSTALL_DIR

# 5. systemd servisleri
cp "$DEPLOY_DIR"/systemd/*.service /etc/systemd/system/
systemctl daemon-reload
systemctl enable flatun-bootstrap flatun-node flatun-dns-updater
echo "  systemd servisleri kuruldu"

# 6. Firewall (ufw varsa)
if command -v ufw &>/dev/null; then
    ufw allow 22/tcp   comment 'SSH'
    ufw allow 5001/tcp comment 'FlatunChain node'
    ufw allow 8000/tcp comment 'FlatunChain bootstrap'
    ufw --force enable
    echo "  Firewall: 22, 5001, 8000 açık"
fi

# 7. Servisleri başlat
systemctl restart flatun-bootstrap
sleep 2
systemctl restart flatun-node flatun-dns-updater

echo ""
echo "==> Kurulum tamamlandı. Kontrol:"
echo "    systemctl status flatun-bootstrap flatun-node flatun-dns-updater"
echo "    journalctl -u flatun-node -f"
echo ""
echo "    DİKKAT: DO_API_TOKEN'ı henüz girmediyseniz:"
echo "      nano $ENV_FILE   (DO_API_TOKEN satırını düzenleyin)"
echo "      systemctl restart flatun-dns-updater"
