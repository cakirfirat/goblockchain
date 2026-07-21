# Arayüz Yenileme — "Mining Core" (v0.3.0)

Tarih: 21 Temmuz 2026
Kapsam: masaüstü (Electron) renderer + Go telemetri desteği
Hedef: kullanıcı madenciliğe başladığında bunu arayüzden HİSSETSİN —
teknoloji odaklı, ölçülü "matrixvari" bir node terminali görünümü.

## Tasarım dili

- Koyu "node terminali" teması: derin lacivert-siyah zemin, çok hafif teknik
  ızgara, neon yeşil (madencilik) + cyan (ağ) vurgular. Veri alanları monospace.
- Onboarding, gönderim onayı, yedekleme modalı dahil TÜM ekranlar aynı dile
  taşındı; marka bloğu (⛓ FLATUNCHAIN · testnet rozeti) eklendi.
- `prefers-reduced-motion` desteklenir: dekoratif animasyonlar kapanır.

## Mining Core paneli

| Öğe | Kapalıyken | Kazarken |
|---|---|---|
| Hex yağmuru (canvas) | seyrek, loş mavi-gri "nefes" | yoğun, hızlı, neon yeşil |
| LED + durum | gri · `BEKLEMEDE` | yanıp sönen yeşil · `KAZIYOR` |
| Tarama çizgisi | kapalı | 3.2 sn'de bir yukarıdan aşağı |
| Hash şeridi | `son blok ▸ <gerçek hash>…` | 140 ms'de bir değişen "deneme ▸ …" adayı |
| Blok bulununca | — | `◆ BLOK #N KAZILDI` flaşı + bakiye yenileme |

Canlı istatistik ızgarası (gerçek node verisi): blok yüksekliği, zorluk hedefi
(`00000…`), hash denemesi (PoW sırasında `deneme/sn` hızına döner), bekleyen
işlem, **kazdığın blok** ve **madencilik kazancın** (neon vurgulu). Altta ~60
sn'lik blok döngüsü ilerleme çubuğu ve oturum satırı (`oturum ▸ X dk · Y blok`).

Animasyonlar tamamen yereldir (canvas ~30 fps, pencere gizliyken Electron rAF'ı
durdurur); node'a ek yük binmez. Gerçek veri 2 sn'de bir `/mine/status`'tan gelir.

## Go tarafı: kilitsiz telemetri

`/mine/status` zenginleştirildi (token guard'ı Faz 2.5'ten aynen sürüyor):

```json
{"mining":true,"height":6339,"difficulty":5,"pool":0,
 "last_block_time":…,"last_block_hash":"93d…","last_mining_ms":508,
 "hash_attempts":205326,"blocks_by_me":1,"reward_by_me":5000000000,"busy":false}
```

- **`hashAttempts` (atomic.Int64)**: PoW döngüsü her nonce denemesinde artırır —
  arayüzdeki sayaç gerçek çalışmayı gösterir, uydurma değildir.
- **`StatsForUI` hiçbir zaman bloklamaz**: `TryLock` alabilirse taze anlık
  görüntü döner ve önbelleğe yazar; PoW global kilidi tutuyorsa son görüntü +
  CANLI hash sayacı + `busy:true` döner. Böylece node blok kazarken arayüz donmaz.
- **`miningActive` atomik oldu** (mutex'ten çıkarıldı): PoW kilidi tutulurken
  bile `IsMining`/`StopMining` anında yanıt verir — "Durdur" butonu takılmaz.
  (Bu ayrıca Faz 3'teki "PoW'u kilit dışına çıkar" işinin ilk küçük adımı.)
- `blocks_by_me`/`reward_by_me` zincirdeki coinbase işlemlerinden hesaplanır
  (`MINING_SENDER` → node adresi). Konsensüse hiçbir etkisi yoktur.

## Korunanlar

- Tüm element ID'leri ve `.hidden` akışı aynı — üç e2e senaryosu değişmeden geçer.
- `window.flatun` köprüsü ve IPC yüzeyi değişmedi; CSP (`default-src 'self'`) aynı.
- Gönderim onay modalı, yedekleme ve onboarding akış mantığına dokunulmadı.

## Test kanıtları (21 Tem 2026)

- `go vet`, `go test ./...` ve `go test -race ./block/` temiz; `npm test` geçti.
- e2e-onboarding (gerçek Electron + yeni sidecar): 8/8 adım geçti.
- Canlı doğrulama (CDP): madencilik başlatıldı → `KAZIYOR`, hash sayacı 205 bin
  denemeye tırmandı, gerçek testnet zincirinde blok kazıldı (`blocks_by_me` 0→1,
  bakiye +50 FLATUN), flaş gösterildi; durduruldu → `BEKLEMEDE`, buton anında
  yanıt verdi. Ekran görüntüleriyle idle/mining/stop durumları doğrulandı.

## Yayın

v0.3.0 (minor: kullanıcıya görünür yeni özellik). Zincir/cüzdan verisine ve
protokole dokunulmadı — v0.2.x kullanıcıları üzerine kurup aynen devam eder.
