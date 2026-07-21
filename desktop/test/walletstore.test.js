// walletstore birim testleri — Electron gerektirmez, düz node ile çalışır:
//   node test/walletstore.test.js
// safeStorage yerine sahte cipher kullanılır; amaç dosya yönetimi ve
// migrasyon mantığının (özellikle "düz metni ancak doğrulama sonrası sil"
// kuralının) kilitlenmesidir.
'use strict';

const assert = require('assert');
const fs = require('fs');
const os = require('os');
const path = require('path');

const store = require('../walletstore');

// Sahte "şifreleme": ters çevrilebilir ama gerçek şifreleme değil — testler
// yalnızca depo mantığını sınar. Prefix, decrypt'in doğru şemayı seçtiğini
// doğrulamamızı sağlar.
function fakeCipher(scheme = 'safestorage') {
  return {
    scheme,
    encrypt: (s) => Buffer.from('FAKE:' + s, 'utf8').toString('base64'),
    decrypt: (recScheme, b64) => {
      if (recScheme === 'none') return Buffer.from(b64, 'base64').toString('utf8');
      const raw = Buffer.from(b64, 'base64').toString('utf8');
      if (!raw.startsWith('FAKE:')) throw new Error('çözülemedi');
      return raw.slice(5);
    },
  };
}

const brokenCipher = {
  scheme: 'safestorage',
  encrypt: () => { throw new Error('şifreleme yok'); },
  decrypt: () => { throw new Error('çözme yok'); },
};

const MNEMONIC = 'oak cheap payment core welcome state fantasy machine walnut jelly';
const ADDRESS = '1MmFnxeg23MEZzgLPWUUoB6WkjHfpWqJfx';

let failures = 0;
let dirsToClean = [];

function tmpdir() {
  const d = fs.mkdtempSync(path.join(os.tmpdir(), 'flatun-store-test-'));
  dirsToClean.push(d);
  return d;
}

async function test(name, fn) {
  try {
    await fn();
    console.log('  ok  ' + name);
  } catch (e) {
    failures++;
    console.error('FAIL  ' + name + '\n      ' + (e && e.message));
  }
}

function writeLegacy(dir, obj) {
  fs.writeFileSync(store.legacyPath(dir), JSON.stringify(obj));
}

(async () => {
  await test('save/load roundtrip', () => {
    const dir = tmpdir();
    const cipher = fakeCipher();
    store.save(dir, cipher, MNEMONIC, { address: ADDRESS, backupDone: true });
    const res = store.load(dir, cipher);
    assert.strictEqual(res.status, 'ok');
    assert.strictEqual(res.mnemonic, MNEMONIC);
    assert.strictEqual(res.record.blockchain_address, ADDRESS);
    assert.strictEqual(res.record.backup_done, true);
    assert.strictEqual(res.record.cipher, 'safestorage');
    // Diskte mnemonic düz metin olarak BULUNMAMALI
    const onDisk = fs.readFileSync(store.walletPath(dir), 'utf8');
    assert.ok(!onDisk.includes('oak cheap'), 'mnemonic diskte düz metin!');
  });

  await test('cipher none ile kayıt (keyring yoksa) yine çalışır', () => {
    const dir = tmpdir();
    const cipher = fakeCipher('none');
    // scheme none: encrypt base64 düz koyar — decrypt none yolunu kullanmalı
    cipher.encrypt = (s) => Buffer.from(s, 'utf8').toString('base64');
    store.save(dir, cipher, MNEMONIC, { address: ADDRESS });
    const res = store.load(dir, cipher);
    assert.strictEqual(res.status, 'ok');
    assert.strictEqual(res.mnemonic, MNEMONIC);
    assert.strictEqual(res.record.cipher, 'none');
  });

  await test('load: dosya yoksa empty', () => {
    const res = store.load(tmpdir(), fakeCipher());
    assert.strictEqual(res.status, 'empty');
  });

  await test('load: bozuk JSON => corrupt/json', () => {
    const dir = tmpdir();
    fs.writeFileSync(store.walletPath(dir), '{bozuk');
    const res = store.load(dir, fakeCipher());
    assert.strictEqual(res.status, 'corrupt');
    assert.strictEqual(res.reason, 'json');
  });

  await test('load: çözülemeyen veri => corrupt/decrypt', () => {
    const dir = tmpdir();
    store.save(dir, fakeCipher(), MNEMONIC, { address: ADDRESS });
    const res = store.load(dir, brokenCipher);
    assert.strictEqual(res.status, 'corrupt');
    assert.strictEqual(res.reason, 'decrypt');
  });

  await test('migrateLegacy: mutlu yol — şifreler, doğrular; düz metin YEDEK ONAYINA KADAR kalır', async () => {
    const dir = tmpdir();
    const cipher = fakeCipher();
    writeLegacy(dir, { mnemonic: MNEMONIC, blockchain_address: ADDRESS });
    const res = await store.migrateLegacy(dir, cipher, async (m) => {
      assert.strictEqual(m, MNEMONIC);
      return ADDRESS;
    });
    assert.strictEqual(res.status, 'migrated');
    // Kullanıcı kelimeleri henüz yedeklemedi — düz metin güvenlik ağı olarak durmalı
    assert.ok(fs.existsSync(store.legacyPath(dir)), 'düz metin yedek onayından önce silinmemeli');
    const loaded = store.load(dir, cipher);
    assert.strictEqual(loaded.mnemonic, MNEMONIC);
    assert.strictEqual(loaded.record.backup_done, false, 'eski kullanıcı yedek uyarısı görmeli');
    assert.strictEqual(loaded.record.migrated_from_legacy, true);

    // Yedek onayı: şifreli kopya doğrulanır ve düz metin ancak şimdi silinir
    store.markBackupDone(dir, cipher);
    assert.ok(!fs.existsSync(store.legacyPath(dir)), 'yedek onayından sonra düz metin silinmeli');
    const after = store.load(dir, cipher);
    assert.strictEqual(after.record.backup_done, true);
    assert.strictEqual(after.mnemonic, MNEMONIC);
  });

  await test('migrateLegacy: inspect reddederse düz metne dokunulmaz', async () => {
    const dir = tmpdir();
    writeLegacy(dir, { mnemonic: 'gecersiz kelimeler', blockchain_address: ADDRESS });
    const res = await store.migrateLegacy(dir, fakeCipher(), async () => null);
    assert.strictEqual(res.status, 'invalid');
    assert.ok(fs.existsSync(store.legacyPath(dir)), 'düz metin korunmalı');
    assert.ok(!fs.existsSync(store.walletPath(dir)), 'yarım şifreli dosya olmamalı');
  });

  await test('migrateLegacy: adres uyuşmazsa taşınmaz', async () => {
    const dir = tmpdir();
    writeLegacy(dir, { mnemonic: MNEMONIC, blockchain_address: 'FARKLI_ADRES' });
    const res = await store.migrateLegacy(dir, fakeCipher(), async () => ADDRESS);
    assert.strictEqual(res.status, 'mismatch');
    assert.ok(fs.existsSync(store.legacyPath(dir)));
  });

  await test('migrateLegacy: geri-okuma doğrulaması patlarsa düz metin kalır', async () => {
    const dir = tmpdir();
    writeLegacy(dir, { mnemonic: MNEMONIC, blockchain_address: ADDRESS });
    // encrypt çalışır ama decrypt hep patlar => verify-failed beklenir
    const oneWay = {
      scheme: 'safestorage',
      encrypt: (s) => Buffer.from('FAKE:' + s).toString('base64'),
      decrypt: () => { throw new Error('keychain gitti'); },
    };
    const res = await store.migrateLegacy(dir, oneWay, async () => ADDRESS);
    assert.strictEqual(res.status, 'verify-failed');
    assert.ok(fs.existsSync(store.legacyPath(dir)), 'düz metin korunmalı');
    assert.ok(!fs.existsSync(store.walletPath(dir)), 'doğrulanamayan şifreli dosya bırakılmamalı');
  });

  await test('makeCipher: safeStorage sarmalayıcısı roundtrip + basic_text reddi', () => {
    // Sahte safeStorage: XOR benzeri basit ters çevrilebilir dönüşüm
    const fakeSafeStorage = {
      isEncryptionAvailable: () => true,
      encryptString: (s) => Buffer.from('SS:' + s, 'utf8'),
      decryptString: (buf) => {
        const raw = buf.toString('utf8');
        if (!raw.startsWith('SS:')) throw new Error('bozuk');
        return raw.slice(3);
      },
      getSelectedStorageBackend: () => 'kwallet',
    };
    const c = store.makeCipher(fakeSafeStorage, 'darwin');
    assert.strictEqual(c.scheme, 'safestorage');
    assert.strictEqual(c.decrypt('safestorage', c.encrypt(MNEMONIC)), MNEMONIC);
    // 'none' şemalı eski kayıt da okunabilmeli
    const b64 = Buffer.from(MNEMONIC, 'utf8').toString('base64');
    assert.strictEqual(c.decrypt('none', b64), MNEMONIC);
    // bilinmeyen şema => throw
    assert.throws(() => c.decrypt('acayip', b64));

    // Linux + basic_text => güvenilmez, 'none' şemasına düşmeli
    fakeSafeStorage.getSelectedStorageBackend = () => 'basic_text';
    const cLinux = store.makeCipher(fakeSafeStorage, 'linux');
    assert.strictEqual(cLinux.scheme, 'none');
    assert.strictEqual(cLinux.trustworthy, false);
    // macOS'ta backend kontrolü yok — güvenilir kalmalı
    const cMac = store.makeCipher(fakeSafeStorage, 'darwin');
    assert.strictEqual(cMac.trustworthy, true);
    // Şifreleme hiç yoksa 'none'
    const cNone = store.makeCipher({ isEncryptionAvailable: () => false }, 'win32');
    assert.strictEqual(cNone.scheme, 'none');
  });

  await test('migrateLegacy: legacy dosya yoksa none', async () => {
    const res = await store.migrateLegacy(tmpdir(), fakeCipher(), async () => ADDRESS);
    assert.strictEqual(res.status, 'none');
  });

  await test('markBackupDone meta günceller, veriye dokunmaz', () => {
    const dir = tmpdir();
    const cipher = fakeCipher();
    store.save(dir, cipher, MNEMONIC, { address: ADDRESS, backupDone: false });
    const before = JSON.parse(fs.readFileSync(store.walletPath(dir), 'utf8'));
    store.markBackupDone(dir, cipher);
    const after = JSON.parse(fs.readFileSync(store.walletPath(dir), 'utf8'));
    assert.strictEqual(after.backup_done, true);
    assert.strictEqual(after.data, before.data, 'şifreli veri değişmemeli');
    assert.strictEqual(store.load(dir, cipher).mnemonic, MNEMONIC);
  });

  await test('markBackupDone: çözülemeyen cüzdanda hata fırlatır, dosyalara dokunmaz', () => {
    const dir = tmpdir();
    store.save(dir, fakeCipher(), MNEMONIC, { address: ADDRESS, migratedFromLegacy: true });
    writeLegacy(dir, { mnemonic: MNEMONIC, blockchain_address: ADDRESS });
    assert.throws(() => store.markBackupDone(dir, brokenCipher));
    assert.ok(fs.existsSync(store.legacyPath(dir)), 'düz metin korunmalı');
    const rec = JSON.parse(fs.readFileSync(store.walletPath(dir), 'utf8'));
    assert.strictEqual(rec.backup_done, false, 'backup_done işaretlenmemeli');
  });

  await test('markBackupDone: migre edilmemiş cüzdanda legacy dosyaya dokunmaz', () => {
    const dir = tmpdir();
    const cipher = fakeCipher();
    // İçe aktarılmış (migrasyon DIŞI) cüzdan + diskte alakasız bir legacy dosya
    store.save(dir, cipher, MNEMONIC, { address: ADDRESS, backupDone: false });
    writeLegacy(dir, { mnemonic: 'baska bir cuzdanin kelimeleri', blockchain_address: 'X' });
    store.markBackupDone(dir, cipher);
    assert.ok(fs.existsSync(store.legacyPath(dir)), 'alakasız legacy dosya silinmemeli');
  });

  await test('quarantineCorrupt dosyayı silmez, kenara taşır', () => {
    const dir = tmpdir();
    store.save(dir, fakeCipher(), MNEMONIC, { address: ADDRESS });
    const dest = store.quarantineCorrupt(dir);
    assert.ok(dest && fs.existsSync(dest));
    assert.ok(!fs.existsSync(store.walletPath(dir)));
    assert.strictEqual(store.load(dir, fakeCipher()).status, 'empty');
  });

  for (const d of dirsToClean) fs.rmSync(d, { recursive: true, force: true });

  if (failures > 0) {
    console.error(`\n${failures} test BAŞARISIZ`);
    process.exit(1);
  }
  console.log('\nTüm walletstore testleri geçti.');
})();
