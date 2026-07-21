// Uçtan uca migrasyon + yedekleme testi.
//
// Ön koşul: profil dizininde eski biçim (düz metin) standalone_wallet.json
// varken uygulama başlatılmış olmalı — ilk açılışta şifreli depoya taşınır:
//   mkdir -p /tmp/flatun-e2e-mig && (standalone_wallet.json koy)
//   FLATUN_USER_DATA=/tmp/flatun-e2e-mig FLATUN_HIDDEN=1 FLATUN_WALLET_PORT=15080 \
//     FLATUN_NODE_PORT=15001 electron . --remote-debugging-port=9223 &
//   node test/e2e-backup.js /tmp/flatun-e2e-mig
//
// Doğrulananlar: yedek uyarı banner'ı görünür; kelimeler modalda eski
// mnemonic ile birebir aynıdır; "yedekledim" sonrası banner kapanır, düz
// metin dosya silinir ve backup_done kalıcı olarak işaretlenir.
'use strict';

const assert = require('assert');
const fs = require('fs');
const path = require('path');
const { chromium } = require('playwright-core');

const CDP = process.env.FLATUN_E2E_CDP || 'http://127.0.0.1:9223';
const profile = process.argv[2];
assert.ok(profile, 'kullanım: node test/e2e-backup.js <profil-dizini>');

(async () => {
  const legacyFile = path.join(profile, 'standalone_wallet.json');
  const encFile = path.join(profile, 'wallet.enc.json');
  assert.ok(fs.existsSync(legacyFile), 'ön koşul: düz metin cüzdan dosyası yok');
  const expectedWords = JSON.parse(fs.readFileSync(legacyFile, 'utf8')).mnemonic.trim().split(/\s+/);

  const browser = await chromium.connectOverCDP(CDP);
  try {
    const ctx = browser.contexts()[0];
    const app = ctx.pages().find((p) => p.url().includes('index.html'));
    assert.ok(app, 'renderer sayfası bulunamadı');

    // Testler Türkçe metin doğrular; dili tr'ye sabitle ve yeniden yükle
    await app.evaluate(() => localStorage.setItem('flatun-lang', 'tr'));
    await app.reload({ waitUntil: 'load' });

    // 1) Migre edilen kullanıcı DOĞRUDAN ana ekrana düşer (onboarding yok)
    await app.waitForSelector('#screen-main:not(.hidden)', { timeout: 10000 });
    console.log('  ok  migre edilen kullanıcı ana ekranda');

    // 2) Yedek uyarı banner'ı görünür olmalı
    await app.waitForSelector('#banner:not(.hidden)', { timeout: 5000 });
    const bannerText = await app.$eval('#banner-text', (el) => el.textContent);
    assert.ok(bannerText.includes('yedeklenmedi'), 'banner metni beklenmedik: ' + bannerText);
    console.log('  ok  yedekleme uyarısı görünüyor');

    // 3) "Şimdi yedekle" → modalda kelimeler eski mnemonic ile birebir aynı
    await app.click('#banner-action');
    await app.waitForSelector('#backup-modal:not(.hidden)', { timeout: 5000 });
    const words = await app.$$eval('#backup-words li', (els) => els.map((e) => e.textContent.trim()));
    assert.deepStrictEqual(words, expectedWords, 'modaldaki kelimeler eski cüzdanla aynı değil!');
    console.log('  ok  modal, taşınan cüzdanın kelimelerini doğru gösteriyor');

    // 4) "Yazdım, yedekledim" → banner kapanır, düz metin silinir, meta işaretlenir
    await app.click('#backup-done');
    await app.waitForFunction(
      () => document.getElementById('banner').classList.contains('hidden'),
      { timeout: 5000 }
    );
    assert.ok(!fs.existsSync(legacyFile), 'düz metin dosya yedek onayı sonrası silinmeliydi');
    const rec = JSON.parse(fs.readFileSync(encFile, 'utf8'));
    assert.strictEqual(rec.backup_done, true, 'backup_done işaretlenmedi');
    // Kelimeler modal kapanınca DOM'dan temizlenmeli
    const domWords = await app.$eval('#backup-words', (el) => el.textContent);
    assert.strictEqual(domWords, '', 'kelimeler DOM\'da kaldı');
    console.log('  ok  yedek onayı: banner kapandı, düz metin silindi, meta kalıcı');

    console.log('\nTüm e2e migrasyon/yedekleme testleri geçti.');
  } finally {
    await browser.close();
  }
})().catch((err) => {
  console.error('E2E BAŞARISIZ:', err.message);
  process.exit(1);
});
