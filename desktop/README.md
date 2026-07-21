# FlatunChain Desktop (Electron)

Masaüstü cüzdan + madenci. Go node'u ("standalone" binary) yan süreç olarak
çalışır; cüzdan VE node API'leri yalnızca 127.0.0.1'e bağlıdır ve her açılışta
üretilen gizli token'ları zorunlu kılar. Anahtarlar/mnemonic renderer'a ulaşmaz
(tek istisna: kullanıcının açıkça istediği onboarding/yedekleme ekranlarında
kelimelerin gösterilmesi).

## Geliştirme

```bash
# 1. Sidecar binary'sini derle (proje kökünde)
./build.sh          # veya hızlı: go build -o build/flatuncoin-mac ./cmd/standalone

# 2. Uygulamayı başlat
cd desktop
npm install
npm start

# Testler
npm test                                   # walletstore birim testleri (düz node)
# e2e testleri için test/e2e-*.js başlıklarındaki çalıştırma notlarına bak
```

Geliştirme kancaları (env): `FLATUN_USER_DATA` (gerçek cüzdana dokunmadan ayrı
profil), `FLATUN_HIDDEN` (pencereyi gösterme), `FLATUN_WALLET_PORT` /
`FLATUN_NODE_PORT` (port çakışmasız test).

## Cüzdan saklama (v0.2.0)

- Mnemonic, profil dizininde `wallet.enc.json` içinde **Electron safeStorage**
  ile şifreli durur (macOS Keychain / Windows DPAPI / Linux keyring).
- Açılışta Electron dosyayı çözer ve mnemonic'i sidecar'a `FLATUN_WALLET_MNEMONIC`
  env'i ile geçirir; Go tarafı diske düz metin yazmaz.
- Eski `standalone_wallet.json` (v0.1 düz metin) ilk açılışta otomatik taşınır;
  düz metin kopya, kullanıcı kurtarma kelimelerini yedeklediğini onaylayana
  kadar güvenlik ağı olarak tutulur.
- İlk kurulumda onboarding: yeni cüzdan (24 kelime + doğrulama) veya kelimelerle
  içe aktarma. Para gönderimi her zaman onay ekranından geçer.
- Ayrıntılar: `docs/faz-2-cuzdan-guvenligi-v0.2.0.md`.

## Arayüz dili (v0.4.0)

- 6 dil: Türkçe, English, 中文, हिन्दी, Español, Français. İlk açılışta sistem
  dili algılanır; kullanıcı seçimi `localStorage`'da kalıcıdır.
- Tüm metinler `renderer/i18n.js` sözlüğünden gelir; statik alanlar `data-i18n`
  öznitelikleriyle, dinamik alanlar `t()` ile çevrilir. Ağ erişimi yoktur.
- E2E testleri Türkçe metin doğruladığı için başta dili `tr`'ye sabitler.
- Ayrıntılar: `docs/coklu-dil-destegi-v0.4.0.md`.

## Mimari güvenlik kuralları

- Renderer: `contextIsolation: true`, `nodeIntegration: false`, `sandbox: true`;
  dış dünyayla teması yalnızca `preload.js`'teki dar `window.flatun` köprüsü.
- Sidecar cüzdan API'si `--bind=127.0.0.1` + `X-Flatun-Token` guard'ı ile açılır;
  LAN'dan ve tarayıcıdaki çapraz-köken sayfalardan erişilemez.
- Node API'si de `--node-bind=127.0.0.1` ile yalnızca loopback dinler; mining
  uçları (`/mine*`) lansman başına üretilen ayrı `X-Flatun-Node-Token` guard'ını
  zorunlu kılar. Dışa dinlememek ağ üyeliğini bozmaz: NAT arkasındaki node gibi
  çekme döngüsü + `/submit` itmesiyle senkron kalır.
- `/wallet/info` mnemonic DÖNDÜRMEZ; imzalama Go tarafında yapılır.
- Gizli değerler (token'lar, mnemonic) sidecar'a argv ile değil env ile geçer.

## Paketleme

```bash
# Sidecar binary'lerini paketleme dizinine koy
mkdir -p desktop/sidecar && cp build/flatuncoin-* desktop/sidecar/
cd desktop && npm run dist     # dist/ altına dmg/nsis/AppImage üretir
```

## Bilinen eksikler (yol haritası)

- Kazanç grafikleri, otomatik güncelleme.
- Gönderim ücretleri şu an 0; ücret piyasası gelince onay ekranı gerçek ücreti
  gösterecek şekilde hazır.
