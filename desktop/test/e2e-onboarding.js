// Uçtan uca onboarding + gönderim onayı testi.
//
// Çalıştırma (uygulama önceden başlatılmış olmalı):
//   FLATUN_USER_DATA=<boş-dizin> FLATUN_HIDDEN=1 FLATUN_WALLET_PORT=15080 \
//     FLATUN_NODE_PORT=15001 electron . --remote-debugging-port=9223 &
//   node test/e2e-onboarding.js
//
// CDP üzerinden gerçek pencereye bağlanır; onboarding'i (üret -> yanlış
// kelime reddi -> doğru kelimelerle bitir), ana ekrana geçişi ve gönderim
// onay modalını kullanıcı gibi tıklayarak doğrular.
'use strict';

const assert = require('assert');
const { chromium } = require('playwright-core');

const CDP = process.env.FLATUN_E2E_CDP || 'http://127.0.0.1:9223';

(async () => {
  const browser = await chromium.connectOverCDP(CDP);
  try {
    const ctx = browser.contexts()[0];
    const app = ctx.pages().find((p) => p.url().includes('index.html'));
    assert.ok(app, 'renderer sayfası bulunamadı: ' + ctx.pages().map((p) => p.url()).join(', '));

    // Testler Türkçe metin doğrular; i18n varsayılanı sistem diline baktığı
    // için dili tr'ye sabitleyip sayfayı yeniden yükle
    await app.evaluate(() => localStorage.setItem('flatun-lang', 'tr'));
    await app.reload({ waitUntil: 'load' });

    // 1) Onboarding seçim ekranı açık
    await app.waitForSelector('#ob-choice:not(.hidden)', { timeout: 5000 });
    console.log('  ok  onboarding seçim ekranı görünüyor');

    // 2) Yeni cüzdan üret — 24 kelime listelenmeli
    await app.click('#ob-create-btn');
    await app.waitForSelector('#ob-words:not(.hidden)', { timeout: 15000 });
    const words = await app.$$eval('#ob-words-list li', (els) => els.map((e) => e.textContent.trim()));
    assert.strictEqual(words.length, 24, '24 kelime bekleniyordu, gelen: ' + words.length);
    console.log('  ok  24 kelimelik mnemonic üretildi ve gösterildi');

    // 3) Doğrulama: önce yanlış kelimeler REDDEDİLMELİ
    await app.click('#ob-words-next');
    await app.waitForSelector('#ob-verify:not(.hidden)', { timeout: 5000 });
    const inputs = await app.$$('#ob-quiz input');
    assert.strictEqual(inputs.length, 3, '3 soru bekleniyordu');
    for (const inp of inputs) await inp.fill('yanlis');
    await app.click('#ob-verify-btn');
    await app.waitForSelector('#ob-verify-error:not(.hidden)', { timeout: 5000 });
    const stillOnboarding = await app.$eval('#screen-main', (el) => el.classList.contains('hidden'));
    assert.ok(stillOnboarding, 'yanlış kelimelere rağmen ana ekrana geçildi!');
    console.log('  ok  yanlış kelimeler reddedildi');

    // 4) Doğru kelimelerle bitir — ana ekran açılmalı, banner (yedek uyarısı) OLMAMALI
    for (const inp of inputs) {
      const idx = Number(await inp.getAttribute('data-idx'));
      await inp.fill(words[idx]);
    }
    await app.click('#ob-verify-btn');
    await app.waitForSelector('#screen-main:not(.hidden)', { timeout: 15000 });
    const bannerHidden = await app.$eval('#banner', (el) => el.classList.contains('hidden'));
    assert.ok(bannerHidden, 'yeni cüzdanda yedek uyarısı görünmemeli (kelimeler az önce doğrulandı)');
    console.log('  ok  onboarding tamamlandı, ana ekran açıldı (yedek uyarısı yok)');

    // 5) Gönderim: geçersiz miktar modal AÇMAMALI
    await app.fill('#send-to', '1MmFnxeg23MEZzgLPWUUoB6WkjHfpWqJfx');
    await app.fill('#send-amount', 'abc');
    await app.click('#send-btn');
    const modalHidden = await app.$eval('#confirm-modal', (el) => el.classList.contains('hidden'));
    assert.ok(modalHidden, 'geçersiz miktarla onay modalı açıldı!');
    const errMsg = await app.$eval('#send-result', (el) => el.textContent);
    assert.ok(errMsg.includes('geçersiz'), 'miktar hatası gösterilmedi: ' + errMsg);
    console.log('  ok  geçersiz miktar onay modalına ulaşamadı');

    // 6) Geçerli girdiyle onay modalı: özet doğru, bakiye yetersiz uyarısı var
    await app.fill('#send-amount', '1.5');
    await app.click('#send-btn');
    await app.waitForSelector('#confirm-modal:not(.hidden)', { timeout: 5000 });
    const cfTo = await app.$eval('#cf-to', (el) => el.textContent);
    const cfAmount = await app.$eval('#cf-amount', (el) => el.textContent);
    assert.strictEqual(cfTo, '1MmFnxeg23MEZzgLPWUUoB6WkjHfpWqJfx');
    assert.strictEqual(cfAmount, '1.5 FLATUN');
    console.log('  ok  onay modalı alıcı ve miktarı doğru gösteriyor');

    // 7) Vazgeç: modal kapanmalı, hiçbir şey gönderilmemeli
    await app.click('#cf-cancel');
    const closed = await app.$eval('#confirm-modal', (el) => el.classList.contains('hidden'));
    assert.ok(closed, 'vazgeç modalı kapatmadı');
    const resultAfterCancel = await app.$eval('#send-result', (el) => el.textContent);
    assert.ok(!resultAfterCancel.includes('Gönderildi'), 'vazgeçilen işlem gönderilmiş görünüyor');
    console.log('  ok  vazgeç işlemi iptal etti');

    // 8) Onayla ve gönder: node bu cüzdanda bakiye olmadığı için REDDETMELİ —
    //    modalın gerçek send yoluna bağlı olduğunu ve hatanın kullanıcıya
    //    ulaştığını kanıtlar
    await app.click('#send-btn');
    await app.waitForSelector('#confirm-modal:not(.hidden)', { timeout: 5000 });
    await app.click('#cf-send');
    await app.waitForFunction(
      () => document.getElementById('send-result').textContent.includes('✗') ||
            document.getElementById('send-result').textContent.includes('✓'),
      { timeout: 15000 }
    );
    const sendResult = await app.$eval('#send-result', (el) => el.textContent);
    assert.ok(sendResult.includes('✗'), 'bakiyesiz gönderim başarılı görünmemeli: ' + sendResult);
    console.log('  ok  onaylanan gönderim node\'a ulaştı ve bakiyesizlik nedeniyle reddedildi');

    console.log('\nTüm e2e onboarding/gönderim testleri geçti.');
  } finally {
    await browser.close();
  }
})().catch((err) => {
  console.error('E2E BAŞARISIZ:', err.message);
  process.exit(1);
});
