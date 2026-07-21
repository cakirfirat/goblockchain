# Çoklu Dil Desteği — 6 Dil (v0.4.0)

Tarih: 21 Temmuz 2026
Kapsam: masaüstü (Electron) renderer — Go tarafına dokunulmadı
Hedef: uygulamanın dünyada en çok kullanılan dillerde, Türkçe dahil en fazla
6 dilde kullanılabilmesi.

## Dil seti

| Kod | Dil | Neden |
|---|---|---|
| tr | Türkçe | ana dil (kaynak metinler) |
| en | English | küresel varsayılan |
| zh | 中文 (Basitleştirilmiş Çince) | en çok konuşulan 2. dil |
| hi | हिन्दी (Hintçe) | en çok konuşulan 3. dil |
| es | Español | en çok konuşulan 4. dil |
| fr | Français | en yaygın diller arasında, LTR |

Arapça bilinçli olarak alınmadı: RTL (sağdan sola) düzen, terminal/monospace
ağırlıklı arayüzde ayrı bir tasarım işi gerektirir; yarım RTL desteği kötü
deneyimdir. İleride istenirse ayrı iş olarak ele alınmalı.

Marka/teknik terimler çevrilmez: FlatunChain, FLATUN, Mining Core, testnet,
PoW, BIP-39, CPU. Node günlüğü paneli teknik log olduğundan olduğu gibi kalır.

## Mimari

- **`desktop/renderer/i18n.js` (yeni)**: 6 dilin tam sözlüğü (~95 anahtar/dil)
  + `t(key, vars)`, `setLang`, `applyStaticI18n`, `errText` yardımcıları.
  Modül sistemi yok; CSP (`default-src 'self'`) altında ikinci yerel `<script>`
  olarak yüklenir, ağdan hiçbir şey çekilmez.
- **Statik metinler**: `index.html`'deki tüm sabit metinler `data-i18n`
  (içerik), `data-i18n-placeholder` (girdi ipuçları) ve `data-i18n-title`
  (araç ipuçları) öznitelikleriyle işaretlendi; açılışta ve dil değişiminde
  toplu çevrilir.
- **Dinamik metinler**: `renderer.js`'teki tüm kullanıcı metinleri `t()`
  üzerinden üretilir (mining durumu, hash şeridi, oturum satırı, hata/başarı
  mesajları, onay modalı, "N. kelime" quiz etiketleri, "X dk önce" zaman
  biçimleri...). Sayı biçimlendirme dile göre `Intl.NumberFormat` ile yapılır
  (ör. tr `6.358`, en `6,358`, fr `6 358`).
- **IPC hataları kod tabanlı**: `main.js` artık hata dönüşlerine `code` alanı
  ekler (`bad-state`, `no-core`, `invalid-mnemonic`, `save-failed`...);
  renderer kodu mevcut dile çevirir. Eski Türkçe `message` alanı geriye dönük
  yedek olarak duruyor — davranış değişikliği yok.

## Dil seçimi ve kalıcılık

- İlk açılışta sistem dili algılanır (`navigator.language`); desteklenen 6
  dilden biriyle eşleşmezse İngilizce kullanılır.
- Kullanıcı seçimi `localStorage`'a (`flatun-lang`) yazılır ve kalıcıdır.
- Dil seçici iki yerde: ana ekran başlığında ve onboarding ekranının sağ üst
  köşesinde. Değişim **anında ve sayfa yenilemeden** uygulanır — onboarding
  adımı, üretilmiş kelimeler, açık modal hiçbir durum kaybolmaz (quiz
  etiketleri bile yerinde yeniden çevrilir). O an ekranda duran birkaç dinamik
  değer (node durumu gibi) en geç 2 sn'lik yoklama turunda tazelenir.
- `<html lang>` seçilen dile göre güncellenir.

## E2E testleri

Üç e2e senaryosu Türkçe metin doğruladığı için testler başta dili `tr`'ye
sabitler (`localStorage.flatun-lang = 'tr'` + sayfa yenileme). Böylece testler
hangi sistem dilinde çalışırsa çalışsın belirleyici (deterministic) kalır.

## Test kanıtları (21 Tem 2026)

- `npm test` (15 walletstore testi) geçti.
- Üç e2e senaryosu gerçek Electron + gerçek sidecar ile geçti:
  onboarding (8 adım), migrasyon/yedekleme (4 adım), kurtarma (4 adım).
- Canlı doğrulama (CDP): 6 dilin tamamı çalışan uygulamada sırayla seçildi;
  buton/etiket/durum metinleri ve sayı biçimleri her dilde doğrulandı, ekran
  görüntüleri alındı (zh/hi dahil — sistem fontları sorunsuz). Onboarding
  seçicisi ve quiz-açıkken dil değişimi ayrıca doğrulandı.

## Yayın

v0.4.0 (minor: kullanıcıya görünür yeni özellik). Zincir/cüzdan verisine,
protokole ve Go tarafına dokunulmadı — önceki sürüm kullanıcıları üzerine
kurup aynen devam eder; mevcut hesap ve kazılmış bloklar etkilenmez.
