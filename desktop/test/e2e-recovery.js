// Uçtan uca kurtarma (çözülemeyen cüzdan → kelimelerle geri yükleme) testi.
//
// Ön koşul: profil dizininde çözülemeyen bir wallet.enc.json varken uygulama
// başlatılmış olmalı (initWallet dosyayı karantinaya alıp recovery'ye düşer):
//   mkdir -p /tmp/flatun-e2e-rec && (bozuk wallet.enc.json koy)
//   FLATUN_USER_DATA=/tmp/flatun-e2e-rec FLATUN_HIDDEN=1 FLATUN_WALLET_PORT=15080 \
//     FLATUN_NODE_PORT=15001 electron . --remote-debugging-port=9223 &
//   node test/e2e-recovery.js <geçerli mnemonic> <beklenen adres>
'use strict';

const assert = require('assert');
const { chromium } = require('playwright-core');

const CDP = process.env.FLATUN_E2E_CDP || 'http://127.0.0.1:9223';
const mnemonic = process.argv[2];
const expectedAddress = process.argv[3];
assert.ok(mnemonic && expectedAddress,
  'kullanım: node test/e2e-recovery.js "<mnemonic>" <beklenen-adres>');

(async () => {
  const browser = await chromium.connectOverCDP(CDP);
  try {
    const ctx = browser.contexts()[0];
    const app = ctx.pages().find((p) => p.url().includes('index.html'));
    assert.ok(app, 'renderer sayfası bulunamadı');

    // Testler Türkçe metin doğrular; dili tr'ye sabitle ve yeniden yükle
    await app.evaluate(() => localStorage.setItem('flatun-lang', 'tr'));
    await app.reload({ waitUntil: 'load' });

    // 1) Kurtarma ekranı: içe aktarma adımı, "Cüzdanı kurtar" başlığıyla açılmalı
    await app.waitForSelector('#ob-import:not(.hidden)', { timeout: 10000 });
    const title = await app.$eval('#ob-import-title', (el) => el.textContent);
    assert.strictEqual(title, 'Cüzdanı kurtar');
    console.log('  ok  kurtarma ekranı açıldı');

    // 2) Geçersiz kelimeler reddedilmeli (BIP-39 checksum)
    await app.fill('#ob-import-text', 'bunlar gecerli kelimeler degil hic degil');
    await app.click('#ob-import-go');
    await app.waitForSelector('#ob-import-error:not(.hidden)', { timeout: 10000 });
    console.log('  ok  geçersiz kelimeler reddedildi');

    // 3) Doğru kelimeler (bol boşluk/büyük harfle) kabul edilip ana ekrana geçilmeli
    await app.fill('#ob-import-text', '  ' + mnemonic.toUpperCase().split(' ').join('   ') + ' ');
    await app.click('#ob-import-go');
    await app.waitForSelector('#screen-main:not(.hidden)', { timeout: 15000 });
    console.log('  ok  kelimeler (normalize edilerek) kabul edildi, ana ekran açıldı');

    // 4) Node açıldığında adres beklenen adres olmalı — doğru cüzdan geri geldi
    await app.waitForFunction(
      () => document.getElementById('address').textContent.length > 20,
      { timeout: 30000 }
    );
    const shownAddress = await app.$eval('#address', (el) => el.textContent);
    assert.strictEqual(shownAddress, expectedAddress, 'geri yüklenen cüzdanın adresi farklı!');
    console.log('  ok  geri yüklenen cüzdan beklenen adresi taşıyor: ' + shownAddress);

    console.log('\nTüm e2e kurtarma testleri geçti.');
  } finally {
    await browser.close();
  }
})().catch((err) => {
  console.error('E2E BAŞARISIZ:', err.message);
  process.exit(1);
});
