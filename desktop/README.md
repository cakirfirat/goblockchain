# FlatunChain Desktop (Electron)

Masaüstü cüzdan + madenci. Go node'u ("standalone" binary) yan süreç olarak
çalışır; cüzdan API'si yalnızca 127.0.0.1'e bağlıdır, anahtarlar renderer'a
asla ulaşmaz.

## Geliştirme

```bash
# 1. Sidecar binary'sini derle (proje kökünde)
./build.sh          # veya hızlı: go build -o build/flatuncoin-mac ./cmd/standalone

# 2. Uygulamayı başlat
cd desktop
npm install
npm start
```

Zincir verisi ve cüzdan: `~/Library/Application Support/flatunchain-desktop/chain/`
(cüzdan kurtarma mnemonic'i `standalone_wallet.json` içinde — v1'de düz metin,
şifreleme yol haritasında).

## Mimari güvenlik kuralları

- Renderer: `contextIsolation: true`, `nodeIntegration: false`, `sandbox: true`;
  dış dünyayla teması yalnızca `preload.js`'teki dar `window.flatun` köprüsü.
- Sidecar cüzdan API'si `--bind=127.0.0.1` ile açılır; LAN'dan erişilemez.
- `/wallet/info` mnemonic DÖNDÜRMEZ; imzalama Go tarafında yapılır.

## Paketleme

```bash
# Sidecar binary'lerini paketleme dizinine koy
mkdir -p desktop/sidecar && cp build/flatuncoin-* desktop/sidecar/
cd desktop && npm run dist     # dist/ altına dmg/nsis/AppImage üretir
```

## Bilinen eksikler (yol haritası)

- Sidecar henüz P2P ağına katılmıyor (yerel zincir); ağ entegrasyonu sırada.
- Mnemonic diski düz metin — Electron safeStorage ile şifrelenecek.
- Onboarding (cüzdan oluştur/içe aktar ekranı), kazanç grafikleri, otomatik güncelleme.
