# FlatunChain Omurga Kurulumu (yoxar.com)

Omurga sunucusu üç servisi birlikte çalıştırır:

| Servis | Port | Görev |
|---|---|---|
| `flatun-node` | 5001 | Blockchain node'u + **checkpoint otoritesi** |
| `flatun-bootstrap` | 8000 | Yeni node'ların kayıt olduğu rehber sunucusu |
| `flatun-dns-updater` | - | Aktif node'ları `seed.yoxar.com` A kayıtlarına yansıtır |

Akış: yeni node → `seed.yoxar.com` çözer → omurga IP'sini bulur → bağlanır →
bootstrap'a kaydolur → dns_updater onu da (public IP'liyse) DNS'e ekler.

## 1. Yerelde derle

```bash
./build.sh          # build/server/ altına linux/amd64 binary'leri koyar
```

## 2. Dosyaları droplet'e kopyala

```bash
scp -r build/server root@159.89.31.131:/root/flatun-deploy/bin
scp -r deploy/systemd root@159.89.31.131:/root/flatun-deploy/systemd
scp deploy/setup_droplet.sh root@159.89.31.131:/root/flatun-deploy/
```

## 3. Droplet'te kurulumu çalıştır

```bash
ssh root@159.89.31.131
bash /root/flatun-deploy/setup_droplet.sh
```

Script ilk çalıştırmada checkpoint otorite anahtarını üretir ve AÇIK anahtarı
ekrana yazar — **bu açık anahtarı not edin**, ağa katılacak diğer tüm node'lar
`--checkpoint-pubkey=<AÇIK>` ile başlatılmalı.

## 4. DigitalOcean API token'ını gir

Token'ı yalnızca droplet üzerindeki env dosyasına yazın (repo'ya/chat'e değil):

```bash
nano /etc/flatunchain/env     # DO_API_TOKEN satırını doldurun
systemctl restart flatun-dns-updater
```

## 5. Doğrulama

```bash
# Servisler ayakta mı?
systemctl status flatun-bootstrap flatun-node flatun-dns-updater

# Node yanıt veriyor mu?
curl -s http://127.0.0.1:5001/p2p/status
curl -s http://127.0.0.1:5001/checkpoint

# DNS kaydı oluştu mu? (updater ilk turda ekler; node public IP'yle
# bootstrap'a kayıtlı olmalı)
dig +short seed.yoxar.com A     # -> 159.89.31.131 görünmeli
```

## Yeni node'ların ağa katılması

Herhangi bir makinede:

```bash
./blockchain_server --port=5001 --dns=true --dns-seeds=seed.yoxar.com \
    --checkpoint-pubkey=<OMURGANIN_AÇIK_ANAHTARI>
```

## Notlar

- `dns_updater` yalnızca `seed` isimli A kayıtlarına dokunur; zone'daki diğer
  kayıtlar (api, objectchain, sync...) güvendedir.
- Private/loopback IP'ler DNS'e asla yazılmaz.
- Checkpoint ÖZEL anahtarı yalnızca `/etc/flatunchain/env` içinde durur
  (root, 600). Yedeğini güvenli bir yerde saklayın — kaybolursa yeni anahtar
  üretilip tüm node'lara yeni açık anahtar dağıtılması gerekir.
- ⚠️ yoxar.com domaininin süresi 31 Ağustos 2026'da doluyor — yenilemeyi unutmayın.
