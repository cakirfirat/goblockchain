# Sürüm Hattı (CI/CD) — v0.2.0

Tek komutla üç platformun (macOS/Windows/Linux) derlenip sunucuya yayınlanması
ve web sitesindeki indirme linklerinin kendiliğinden güncellenmesi.

## Karar: neden Jenkins da GitHub Actions da değil?

| Seçenek | Değerlendirme |
|---|---|
| **Jenkins (sunucuda)** | Java + plugin yığını sürekli güvenlik yaması ister; internete açık Jenkins başlı başına saldırı yüzeyidir. Üstelik Linux sunucu macOS paketi imzalayamaz/notarize edemez — yine Mac'e muhtaç kalınır. Solo geliştirici için bakım yükü, kazancından büyük. |
| **GitHub Actions** | İmza anahtarlarının (Apple kimliği, sunucu SSH anahtarı) bulut CI'a secret olarak konması gerekir; cüzdan projesi için haklı bir çekince. Ayrıca Certum sertifikan **fiziksel karta** gelecek — kart USB'de takılı olmadan bulutta Windows imzası zaten yapılamaz. |
| **Yerel sürüm hattı (seçilen)** | Tetikleyici sensin: Mac'te `./release.sh`. İmza anahtarları hiçbir üçüncü tarafa çıkmaz. macOS imzası da (Xcode/notarization) Windows kart imzası da zaten fiziksel olarak senin makinende olmak zorunda — hat, zorunluluğu avantaja çevirir. Sunucu yalnızca statik dosya servis eder; ele geçse bile imza anahtarı yoktur. |

Not: `go test` + `node test` her sürümde hattın ilk adımı olarak zaten koşuyor;
ileride istenirse GitHub Actions **yalnızca test için** (secret'sız, imzasız)
eklenebilir — yayın yetkisi yine sadece Mac'te kalır.

## Mimari

```
Mac (tetik: ./release.sh)
 ├─ go vet + go test + npm test          (kalite kapısı)
 ├─ build.sh → 3 platform Go sidecar
 ├─ electron-builder → dmg(arm64) + exe(x64) + AppImage(x64)
 ├─ SHA256SUMS.txt + latest.json          (manifest)
 └─ rsync → droplet:/var/www/flatun-downloads/releases/vX.Y.Z/
            sonra 'latest' symlink'leri + latest.json ATOMİK güncellenir
                         │
Droplet (nginx, downloads.yoxar.com)
 ├─ /releases/vX.Y.Z/…                    kalıcı sürüm arşivi
 ├─ /latest/FlatunChain-mac-arm64.dmg     sabit linkler (symlink)
 └─ /latest.json                          web sitesinin okuduğu manifest
                         │
Web sitesi: sabit /latest/ linkleri + latest.json'dan sürüm/boyut yazar
            → sitede elle link değiştirme yok
```

Yayın atomiktir: önce sürüm klasörü eksiksiz yüklenir, en son symlink'ler ve
`latest.json` değiştirilir. Kullanıcı hiçbir anda yarım dosya linki görmez.
Aynı sürüm numarası ikinci kez yayınlanmak istenirse hat durdurur (`--force`
ile bilinçli ezme mümkün).

## Tek seferlik kurulum

1. **DNS**: DigitalOcean panelinde A kaydı → `downloads.yoxar.com` →
   `159.89.31.131` (dns_updater yalnızca `seed` kaydına dokunduğu için çakışmaz).
2. **SSH anahtarı**: hattın şifresiz `ssh root@159.89.31.131` yapabilmesi
   gerekir: `ssh-copy-id root@159.89.31.131` (anahtar yoksa önce `ssh-keygen -t ed25519`).
3. **Sunucu**: `deploy/setup_downloads.sh` dosyasını droplet'e kopyalayıp
   çalıştır — nginx + Let's Encrypt + dizin düzenini kurar:

   ```bash
   scp deploy/setup_downloads.sh root@159.89.31.131:/root/
   ssh root@159.89.31.131 'CERTBOT_EMAIL=seninmail@ornek.com bash /root/setup_downloads.sh'
   ```

4. **Web sitesi**: `deploy/website-download-snippet.html` içeriğini indirme
   sayfasına bir kez ekle. Linkler sabit `/latest/` adreslerine bakar; sayfa
   `latest.json`'dan sürüm numarasını ve dosya boyutlarını kendisi yazar.

## Kullanım

```bash
./release.sh --bump patch     # 0.2.0 → 0.2.1: test + derle + yayınla
./release.sh                  # sürümü artırmadan (package.json'daki sürümle)
./release.sh --no-upload      # sadece derle (yayın yok); sonra --publish-only
./release.sh --target /tmp/x  # sahte hedefe prova yayını
```

Sürümün kaynağı `desktop/package.json` içindeki `version` alanıdır.
Yayın sonrası önerilen: `git tag vX.Y.Z && git push origin vX.Y.Z`.

Yapılandırma (varsayılanları ezmek için repo köküne `release.env`, gitignore'da):

```bash
DEPLOY_TARGET=root@159.89.31.131
REMOTE_DIR=/var/www/flatun-downloads
BASE_URL=https://downloads.yoxar.com
```

## Sertifikalar gelince

- **macOS**: Apple Developer onayı + "Developer ID Application" sertifikası
  Keychain'e kurulunca `release.env`'e şu üç satır eklenir; hat imza +
  notarization'ı kendiliğinden yapar:

  ```bash
  APPLE_ID=appleid@ornek.com
  APPLE_APP_SPECIFIC_PASSWORD=xxxx-xxxx-xxxx-xxxx   # appleid.apple.com'dan
  APPLE_TEAM_ID=XXXXXXXXXX
  ```

- **Windows (Certum kart)**: kart fiziksel USB'de olduğundan imza, kartın
  takılı olduğu makinede atılır (`signtool`/proCertum ya da macOS'te
  jsign+PKCS#11). Kart gelince `release.sh` içindeki "Windows imzası"
  bölümüne tek adım eklenecek; o güne dek exe imzasız (SmartScreen uyarısı
  normal, SHA-256 ile doğrulanabilir).

## Bilinen sınır: imzasız macOS paketi ve "hasarlı" uyarısı

Developer ID olmadan paketlenen uygulamada yalnızca linker imzası kalıyordu;
tarayıcıdan indirilen (karantinalı) kopyada Gatekeeper bunu **"FlatunChain
hasarlı, çöpe taşıyın"** diye gösterip hiçbir açma yolu sunmuyordu.

Çözüm (18 Tem 2026): `desktop/scripts/adhoc-sign.js` afterPack kancası,
Apple kimlik bilgileri tanımlı değilken uygulamayı geçerli bir **ad-hoc**
imzayla mühürlüyor (`codesign --verify --deep --strict` geçiyor, DMG
içindeki kopyada doğrulandı). Sonuç: "hasarlı" çıkmazı yerine "geliştirici
doğrulanamadı" uyarısı + Sistem Ayarları → Gizlilik ve Güvenlik →
**"Yine de Aç"** yolu. Terminal bilen testçiler için kestirme:

```bash
xattr -d com.apple.quarantine ~/Downloads/FlatunChain-mac-arm64.dmg  # açmadan önce
# veya kopyalanmış uygulama için:
xattr -d com.apple.quarantine /Applications/FlatunChain.app
```

Apple onayı gelip `release.env` dolunca kanca kendini devre dışı bırakır,
gerçek imza + notarization uyarıların tamamını kaldırır.

## Geri alma (rollback)

Eski sürümler `/releases/` altında kalıcı durur; geri almak symlink'leri
eski sürüme çevirmekten ibarettir:

```bash
ssh root@159.89.31.131
cd /var/www/flatun-downloads
ln -sfn ../releases/v0.2.0/FlatunChain-0.2.0-arm64.dmg      latest/FlatunChain-mac-arm64.dmg
ln -sfn ../releases/v0.2.0/FlatunChain-Setup-0.2.0-x64.exe  latest/FlatunChain-windows-x64.exe
ln -sfn ../releases/v0.2.0/FlatunChain-0.2.0-x86_64.AppImage latest/FlatunChain-linux-x86_64.AppImage
cp releases/v0.2.0/latest.json latest.json
```

## Test kanıtı (18 Tem 2026)

- `./release.sh --target /tmp/flatun-fake-server` uçtan uca ÇALIŞTI (46 sn):
  go vet/test + npm test geçti; `FlatunChain-0.2.0-arm64.dmg` (131 MB),
  `FlatunChain-Setup-0.2.0-x64.exe` (109 MB), `FlatunChain-0.2.0-x86_64.AppImage`
  (138 MB) üretildi; SHA256SUMS doğrulandı (3/3 OK); symlink'ler doğru sürüme
  çözüldü; `latest.json` üç platformu doğru URL ve sha256 ile içeriyor.
- Aynı sürümü tekrar yayınlama denemesi hat tarafından reddedildi;
  `--force` ile bilinçli ezme çalıştı.

## Canlı yayın kanıtı (18 Tem 2026, v0.2.0)

- DNS: `downloads.yoxar.com` → 159.89.31.131 (DO panelinden eklendi).
- Droplet kurulumu `setup_downloads.sh` ile yapıldı. İlk denemede Let's
  Encrypt doğrulaması ufw yüzünden düştü (80/443 kapalıydı); portlar açılıp
  sertifika alındı, script'e ufw adımı kalıcı olarak eklendi. Sertifika
  16 Eki 2026'ya kadar geçerli, certbot otomatik yeniliyor.
- `./release.sh --publish-only` ile v0.2.0 gerçek sunucuya yayınlandı
  (379 MB, ~20 sn @ 23 MB/s).
- İnternetten doğrulandı: `latest.json` 200 + `Access-Control-Allow-Origin: *`
  + `Cache-Control: max-age=300`; üç sabit link de 200 ve doğru boyutta;
  HTTP→HTTPS 301 yönlendirmesi aktif; sunucudaki SHA256SUMS yereldekiyle
  birebir aynı; dmg'nin ilk 1 MB'ı gerçekten indirildi.
- Droplet'teki mevcut servisler (flatun-node, flatun-bootstrap,
  flatun-dns-updater) kurulumdan etkilenmedi, hepsi `active`.
