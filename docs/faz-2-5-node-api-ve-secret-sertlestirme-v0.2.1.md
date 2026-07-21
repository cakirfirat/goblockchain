# Faz 2.5 — Secret Temizliği ve Node API Sertleştirmesi (v0.2.1)

Tarih: 20 Temmuz 2026
Kapsam: masaüstü (Electron) dağıtımının yayın öncesi sertleştirilmesi
Kaynak: v0.2.0 prod güvenlik denetimi (statik inceleme) + değerlendirme sonrası
seçilen "yüksek değer / düşük risk" iki madde. Konsensüs düzeltmeleri bilinçli
olarak bu faza ALINMADI (aşağıda Faz 3 planı).

## 1) SMS sırrı: `utils/sendsms.go` silindi

- `SendSms` hiçbir yerden çağrılmıyordu (ölü kod) ama içinde netgsm kullanıcı
  kodu + parolası sabitti. Fonksiyon env'e taşınmak yerine tamamen silindi:
  kullanılmayan koda secret altyapısı kurmak gereksiz yüzeydir; SMS özelliği
  bir gün gerekirse env/secret-manager ile sıfırdan yazılmalı.
- **KALAN İŞ (kod dışı, elle yapılacak):** netgsm panelinden parolayı DÖNDÜR
  (rotate). Bilgi git geçmişinde ve dağıtılmış kopyalarda kalıcı olarak sızmış
  kabul edilmeli — dosyayı silmek sızıntıyı geri almaz, yalnızca rotasyon alır.
  Rotasyon yapıldıktan sonra git geçmişi temizliği (filter-repo) opsiyoneldir;
  parola geçersizleştiğinde geçmişteki kopyanın değeri kalmaz.

## 2) Node API: masaüstünde loopback + mining token guard'ı

Denetim bulgusu: Electron sidecar'ının blockchain API'si (`5001`) varsayılan
`0.0.0.0`'da dinliyordu ve `/mine/start` token'sızdı — aynı LAN'daki biri
kullanıcının CPU'sunda madenciliği tetikleyebilir, peer enjekte edebilirdi.

### Değişiklikler

- **`desktop/main.js`**: sidecar spawn'ına `--node-bind=127.0.0.1` eklendi.
  Ayrıca her lansmanda cüzdan token'ından AYRI, rastgele bir `NODE_TOKEN`
  üretilir; sidecar'a `FLATUN_NODE_TOKEN` env'i ile geçer ve node API
  çağrılarına `X-Flatun-Node-Token` başlığı olarak eklenir.
- **`cmd/standalone/main.go`**: `FLATUN_NODE_TOKEN` doluysa mining uçları
  (`/mine`, `/mine/start`, `/mine/stop`, `/mine/status`) `nodeGuard` ile
  korunur (sabit-zamanlı karşılaştırma, yanlış/eksik token → 403). Token
  yoksa (sunucu/geliştirme modu) davranış değişmez.

### Neden iki ayrı token?

Cüzdan token'ı yalnızca 127.0.0.1'e bağlı cüzdan API'sinde yaşar. Node API'si
ise sunucu kurulumlarında dışa açılabilir; cüzdan token'ı o yüzeye sızmamalı.

### Neden loopback P2P'yi bozmaz?

Fork seçimi/senkronizasyon iki kanalla çalışır: periyodik ÇEKME döngüsü
(`ResolveConflicts` peer'lardan zincir çeker) ve mining sonrası İTME
(`POST /submit` ile peer'lara gönderir). İkisi de GİDEN bağlantıdır. Dışa
dinlememek, NAT arkasındaki node ile birebir aynı durumdur — ev kullanıcısı
zaten böyle tam ağ üyesiydi. Droplet (sunucu) node'ları gelen P2P bağlantıları
için `--node-bind=0.0.0.0` varsayılanıyla çalışmaya devam eder.

### Bilinçli kapsam sınırı

- `/chain`, `/transactions`, `/submit`, `/consensus`, `/peers` gibi P2P uçları
  token'sız kaldı: bunlar protokolün kendisi, peer'lar token bilemez. Masaüstünde
  loopback binding bu yüzeyi zaten dışarıya kapatıyor.
- Droplet'teki `blockchain_server` ve legacy `wallet_server` bu fazda
  değişmedi (Faz 3-4 kapsamı, aşağıda).

## Test kanıtları (20 Tem 2026)

- `go build ./...`, `go vet ./...`, `go test ./...`, `gofmt` temiz;
  `npm test` (15 walletstore testi) geçti.
- Standalone smoke (gerçek binary):
  - `FLATUN_NODE_TOKEN` + `--node-bind=127.0.0.1` ile: her iki port da yalnızca
    `127.0.0.1`'de dinliyor (lsof); token'sız `/mine/status` → 403, yanlış
    token'lı `/mine/start` → 403; doğru token'la start→status(mining:true)→stop
    tam döngü çalıştı; `/chain` token'sız 200 (P2P açık).
  - Env'siz (sunucu modu): mining uçları eskisi gibi açık, binding `0.0.0.0`
    varsayılanı korunuyor.
- Uçtan uca (gerçek Electron + yeni derlenen sidecar, CDP): onboarding e2e'nin
  tamamı geçti; node `127.0.0.1:15001`'de dinliyor; dışarıdan token'sız
  `/mine/start` → 403 alırken uygulama içi mining aç/kapa (IPC → token'lı
  istek) sorunsuz çalıştı.

## v0.2.1 yayın kontrol listesi

1. [ ] netgsm parolasını döndür (bu commit'ten bağımsız, hemen).
2. [x] SMS sırrı koddan silindi.
3. [x] Node API masaüstünde loopback + mining token guard'ı.
4. [ ] `./release.sh --bump patch` ile v0.2.1 (imza/notarization hâlâ
   Certum + Apple onayı bekliyor — bilinen durum, engel değil).

## Faz 3 planı — konsensüs bütünlüğü (ayrı, test-yoğun iş)

Denetimin doğruladığı, "gerçek değer taşıyan mainnet" öncesi çözülmesi gereken
maddeler. Bunlar davranış değiştiren konsensüs kuralları olduğundan ağ genelinde
eşgüdümlü sürüm (fork) gerektirir ve regresyon testleriyle birlikte yapılmalı:

| Öncelik | İş | Özet |
|---|---|---|
| P0 | Kanonik transaction ID + blok içi/zincir geneli dedup | txid'yi imza DIŞI yükten türet (SigningPayload zaten mevcut); `ValidChain` ve mempool'da benzersizlik zorunlu. Aynı imzalı işlemin bir blokta tekrar tahsilini kapatır. |
| P0 | Low-S (kanonik) imza zorunluluğu | Malleated imzanın farklı "kimlik" üretmesini kapatır; txid ayrımıyla birlikte replay korumasını tamamlar. |
| P1 | Beklenen zorluk kuralı | Her yükseklik için `ExpectedDifficulty` hesapla, `ValidChain`'de tam eşleşme iste. (Kümülatif-iş fork seçimi + checkpoint bunu bugün büyük ölçüde nötralize ediyor; yine de savunma katmanı.) |
| P1 | Timestamp kuralları | Timestamp'i PoW kapsamına al; median-time-past + gelecek-sapma sınırı. |
| P2 | PoW'u global mutex dışına çıkar | Snapshot'ı kilitsiz kaz, commit'te yeniden doğrula; mining sırasında API blokajı kalkar. |
| P2 | Droplet yüzeyi | `blockchain_server`'daki mempool DELETE/mining uçlarına auth + rate-limit; legacy `wallet_server`'ı kaldır veya loopback+token'a kilitle. |
| P3 | P2P kimliği | Bootstrap HTTPS/imzalı node identity; mnemonic aktarımını env'den tek kullanımlık pipe'a taşı; chain-file migrasyonu. |

Her P0/P1 maddesi için asgari test seti: duplicate/malleated işlem regresyonu,
düşük zorluklu zincir reddi, timestamp manipülasyon senaryosu, çok-node reorg.

## Sıradaki fazlar özeti

- **Şimdi (v0.2.1):** bu faz + netgsm rotasyonu → masaüstü testnet cüzdanı
  olarak dağıtım güvenli.
- **Faz 3 (konsensüs):** yukarıdaki tablo — "gerçek para" çıtası bunlara bağlı.
- **Bekleyen dış bağımlılıklar:** Apple Developer ID + Certum (imza/notarization),
  CI kalite kapısı (yalnızca test, secret'sız — surum-hatti dokümanındaki karar).
