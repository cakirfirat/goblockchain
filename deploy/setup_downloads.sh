#!/usr/bin/env bash
set -euo pipefail

# ============================================================================
# FlatunChain indirme sunucusu kurulumu (droplet'te BİR KEZ çalıştırılır)
#
# Ne yapar:
#   - nginx + certbot kurar
#   - /var/www/flatun-downloads dizinini oluşturur
#   - downloads.yoxar.com için nginx site tanımı + Let's Encrypt sertifikası
#
# Ön koşul: DigitalOcean DNS'te A kaydı ekleyin:
#   downloads.yoxar.com -> bu droplet'in IP'si (159.89.31.131)
#
# Kullanım (droplet'te root olarak):
#   bash setup_downloads.sh
# ============================================================================

DOMAIN="${DOMAIN:-downloads.yoxar.com}"
WEBROOT="/var/www/flatun-downloads"
EMAIL="${CERTBOT_EMAIL:-}"   # boşsa certbot interaktif sorar

[ "$(id -u)" = 0 ] || { echo "root olarak çalıştırın"; exit 1; }

echo "== Paketler kuruluyor (nginx, certbot)..."
apt-get update -qq
DEBIAN_FRONTEND=noninteractive apt-get install -y -qq nginx certbot python3-certbot-nginx >/dev/null

echo "== İndirme dizini hazırlanıyor: $WEBROOT"
mkdir -p "$WEBROOT/releases" "$WEBROOT/latest"
chown -R www-data:www-data "$WEBROOT"

# ufw aktifse HTTP/HTTPS'i aç (Let's Encrypt doğrulaması 80'e ulaşamazsa başarısız olur)
if command -v ufw >/dev/null && ufw status | grep -q "Status: active"; then
  echo "== ufw: 80/443 portları açılıyor..."
  ufw allow 80/tcp comment 'HTTP (indirme + LE)'
  ufw allow 443/tcp comment 'HTTPS (indirme)'
fi

echo "== nginx site tanımı yazılıyor..."
cat > /etc/nginx/sites-available/flatun-downloads <<NGINX
server {
    listen 80;
    listen [::]:80;
    server_name $DOMAIN;

    root $WEBROOT;

    # İndirme dosyaları büyük; sendfile ile verimli servis
    sendfile on;
    tcp_nopush on;

    # Sürüm manifesti: web sitesi başka origin'den okuyacağı için CORS açık.
    # Cache kısa tutulur ki yeni sürüm linki hızla görünsün.
    location = /latest.json {
        add_header Access-Control-Allow-Origin "*" always;
        add_header Cache-Control "public, max-age=300" always;
        default_type application/json;
    }

    # Sabit "en güncel sürüm" linkleri (symlink'ler release.sh ile güncellenir)
    location /latest/ {
        add_header Cache-Control "public, max-age=300" always;
    }

    # Eski sürümler kalıcı URL'lerde; agresif cache'lenebilir
    location /releases/ {
        autoindex on;
        add_header Cache-Control "public, max-age=31536000, immutable" always;
    }

    location = / {
        return 302 /releases/;
    }
}
NGINX

ln -sfn /etc/nginx/sites-available/flatun-downloads /etc/nginx/sites-enabled/flatun-downloads
nginx -t
systemctl reload nginx

echo "== Let's Encrypt sertifikası alınıyor ($DOMAIN)..."
if [ -n "$EMAIL" ]; then
  certbot --nginx -d "$DOMAIN" --non-interactive --agree-tos -m "$EMAIL" --redirect
else
  certbot --nginx -d "$DOMAIN" --redirect
fi

echo
echo "== KURULUM TAMAM =="
echo "İndirme kökü : $WEBROOT"
echo "Adres        : https://$DOMAIN"
echo "Sonraki adım : geliştirme makinesinde ./release.sh çalıştırın"
