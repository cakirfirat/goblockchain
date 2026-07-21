# Faz 2 — Cüzdan Şifreleme, Onboarding, Gönderim Onayı (v0.2.0)

Tarih: 17 Temmuz 2026
Kapsam: masaüstü (Electron) uygulaması + Go standalone sidecar
Önceki faz: Faz 1 (cüzdan API guard'ı, panic düzeltmesi, adres checksum, DoS sertleştirme)

## Kapatılan açık

**Mnemonic diskte düz metin duruyordu** (`standalone_wallet.json`). Diski okuyabilen
her süreç/yedek/kötü amaçlı yazılım cüzdanın tamamını ele geçirebiliyordu. Ayrıca
kullanıcı kelimelerini hiç görmüyor (yedekleyemiyor) ve gönderimler tek tıkla,
onaysız gidiyordu.

## Yeni mimari

### Saklama: `wallet.enc.json` (Electron profil dizininde)

```json
{
  "version": 1,
  "cipher": "safestorage",        // veya "none" (aşağıya bak)
  "data": "<base64 şifreli mnemonic>",
  "blockchain_address": "1...",   // adres herkese açık — düz durabilir
  "backup_done": true,
  "migrated_from_legacy": false,
  "created_at": "ISO-8601"
}
```

- Şifreleme **Electron safeStorage** ile: macOS Keychain / Windows DPAPI / Linux keyring.
- Linux'ta keyring yoksa (`basic_text` backend) şifrelemeye GÜVENİLMEZ: backend
  sonradan değişirse veri çözülemez hale gelebilir. Bu durumda `cipher: "none"`
  ile devam edilir ve arayüz kullanıcıyı uyarır; gerçek keyring geldiği ilk
  açılışta dosya sessizce şifreli biçime yükseltilir.
- Yazma her zaman atomiktir (tmp + rename), dosya izni 0600.

### Mnemonic'in Go sidecar'a taşınması

Electron açılışta dosyayı çözer ve mnemonic'i sidecar'a **`FLATUN_WALLET_MNEMONIC`
env değişkeniyle** geçirir (argv değil — `ps` görünürlüğü). Env doluyken Go tarafı
**diske düz metin yazmaz ve dosya okumaz**; env geçersizse (bozuk dosya) sessizce
yanlış cüzdan türetmek yerine açık hatayla çıkar.

Env yoksa (sunucu/geliştirme modu) eski davranış aynen sürer: `standalone_wallet.json`
okunur/oluşturulur. Droplet node'ları bu değişiklikten etkilenmez.

### Tek seferlik cüzdan aracı (`--wallet-tool`)

Onboarding'in kripto işleri Go'da kalır (JS'e ikinci BIP-39 implementasyonu girmez):

- `--wallet-tool=generate` → yeni 24 kelimelik mnemonic + adres (JSON, stdout)
- `--wallet-tool=inspect`  → env'deki mnemonic'i BIP-39 doğrular, adresini basar
  (çıkış kodları: 0 başarı, 2 kullanım hatası, 3 geçersiz mnemonic)

### BIP-39 doğrulama (`wallet` paketi)

`wallet.ValidateMnemonic` + `wallet.NormalizeMnemonic` eklendi. `bip39.NewSeed`
her metni kabul ettiğinden, içe aktarma sınırlarında (onboarding import, standalone
`/wallet/hd/import`, env yükleme) checksum doğrulaması artık zorunlu — yazım hatalı
mnemonic sessizce boş bir cüzdan açamaz. İmzalama yolları bilinçli olarak
doğrulamasız bırakıldı (eski BIP-39-dışı cüzdanı olan biri parasına erişmeye devam edebilmeli).

## Onboarding akışları

| Durum | Akış |
|---|---|
| Yeni kullanıcı | Oluştur → 24 kelime göster → 3 rastgele kelimeyle doğrula → şifreli kaydet → node başlar. Kelimeler doğrulanana kadar yalnızca ana süreç belleğinde. |
| Kelimeleri olan kullanıcı | İçe aktar → BIP-39 doğrulama (normalize: boşluk/büyük harf toleranslı) → şifreli kaydet → node başlar |
| Eski (düz metin) kullanıcı | İlk açılışta otomatik migrasyon + "kelimelerini yedekle" banner'ı |
| Çözülemeyen cüzdan dosyası | Dosya `wallet.enc.corrupt-<ts>.json` olarak karantinaya alınır → kurtarma ekranı (kelimelerle geri yükleme) |

### Migrasyon güvenlik sırası

oku → `inspect` ile doğrula → dosyadaki adresle karşılaştır → şifreli yaz →
**geri okuyup birebir doğrula**. Düz metin dosya bu aşamada silinmez: kullanıcı
kelimelerini henüz hiç yedeklemedi ve safeStorage anahtarı ortama bağlı. Silme,
kullanıcı yedekleme ekranında "yazdım, yedekledim" dediğinde — şifreli kopyanın
çözülebildiği bir kez daha doğrulandıktan sonra — yapılır. Migrasyonun herhangi
bir adımı başarısız olursa düz metne dokunulmaz, uygulama eski (legacy) modda
çalışır ve arayüz uyarı gösterir.

## Gönderim onayı

"Gönder" artık doğrudan göndermez: alıcı adresi, tutar, ağ ücreti ve mevcut
bakiyeyi gösteren onay modalı açılır; bakiye yetersizse uyarı eklenir. Gönderim
yalnızca "Onayla ve gönder" ile yapılır. Asıl doğrulama (adres checksum, tutar
çözümleme, bakiye) Faz 1'deki gibi Go tarafında kalır — modal insan-hatası katmanıdır.

## Üretim sertleştirmesi (prod incelemesi sonrası eklendi)

- **Tek kopya kilidi** (`requestSingleInstanceLock`): ikinci kopya, port
  çakışması + farklı token yüzünden sessizce bozuk çalışmak yerine mevcut
  pencereyi öne getirip kapanır.
- **Sidecar otomatik yeniden başlatma**: node çökerse en fazla 3 deneme,
  artan bekleme ile yeniden başlatılır (60 sn sağlıklı çalışma sayacı sıfırlar;
  port çakışması gibi kalıcı hatada sonsuz döngüye girilmez). Mnemonic yeniden
  başlatma anında diskten çözülür; çözülemezse sidecar env'siz BAŞLATILMAZ
  (Go tarafının yeni cüzdan üretmesine izin verilmez).
- **Bozuk şifreli dosya + henüz silinmemiş düz metin kopya**: kurtarma ekranına
  düşmeden önce düz metinden otomatik yeniden şifrelenir (kelime sorulmaz).
- **API timeout** (15 sn) tüm sidecar çağrılarında; asılı node IPC birikmesine
  yol açamaz.
- Renderer'daki tüm IPC çağrıları hata yakalar: gönderim/yedekleme/madencilik
  sırasında node düşerse modal/buton kilitli kalmaz.
- Onboarding kelimeleri iş bitince DOM'dan ve bellekten temizlenir;
  `onboarding:confirmNew` kayıt hatasında kelimeleri koruyup net hata döndürür;
  içe aktarmada çift tık koruması vardır.
- `FLATUN_HIDDEN` test kancası paketli uygulamada devre dışıdır.

## Test kanıtları

- `go build ./... && go vet ./... && go test ./...` temiz; gofmt temiz.
- `wallet` paketi: normalize/doğrulama/roundtrip birim testleri.
- `wallet-tool` shell roundtrip: generate → inspect adres eşleşmesi; bozuk/boş
  mnemonic doğru çıkış kodlarıyla reddedildi.
- Sidecar smoke: env-mnemonic modunda `/wallet/info` doğru adresi döndürdü ve
  diskte cüzdan dosyası OLUŞMADI; env'siz modda eski davranış korundu.
- `desktop/test/walletstore.test.js` (düz node, 15 test): saklama, migrasyon
  sıralaması, karantina, cipher sarmalayıcısı.
- Uçtan uca (gerçek Electron + gerçek safeStorage, CDP ile sürüldü):
  - `test/e2e-onboarding.js`: seçim → üretim → yanlış kelime reddi → doğrulama →
    ana ekran → geçersiz tutar reddi → onay modalı → vazgeç → onayla (bakiyesiz
    gönderim node tarafından reddedildi, hata kullanıcıya ulaştı).
  - `test/e2e-backup.js`: düz metin cüzdan migrasyonu → yedek banner'ı → modaldaki
    kelimeler eski mnemonic ile birebir aynı → onay sonrası düz metin silindi.
  - `test/e2e-recovery.js`: bozuk şifreli dosya → karantina → geçersiz kelime
    reddi → normalize edilerek içe aktarma → aynı adresle geri yükleme.
  - İkinci açılış kararlılığı: şifreli dosya değişmeden çözüldü, yeniden migrasyon yok.
  - Sertleştirme senaryoları (canlı): bozuk şifreli dosya + düz metin kopya →
    kelime sorulmadan otomatik yeniden şifreleme (adres korundu); sidecar
    SIGKILL → yeni PID ile otomatik yeniden doğuş, token guard'ı yeni süreçte
    de 403 veriyor; ikinci uygulama kopyası tek-kopya kilidiyle hemen kapandı.

Test için izolasyon kancaları: `FLATUN_USER_DATA` (ayrı profil), `FLATUN_HIDDEN`
(pencereyi gösterme), `FLATUN_WALLET_PORT` / `FLATUN_NODE_PORT` (port çakışmasız e2e).

## Dağıtım notu

`build/` ve `desktop/sidecar/` altındaki paketli binary'ler hâlâ Faz 1 öncesinden.
Faz 1+2'nin kullanıcıya ulaşması için: `./build.sh` → `cp build/flatuncoin-* desktop/sidecar/`
→ `cd desktop && npm run dist`. Desktop sürümü bu fazla `0.2.0`'a yükseltildi.

## Sıradaki fazlar (değişmedi)

- Faz 3-4: yedek checkpoint otoritesi, kanonik genesis, PoW/mutex iyileştirmeleri,
  legacy `wallet_server`'ın 0.0.0.0 binding'i.
