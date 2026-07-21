// Cüzdan dosyası yönetimi — mnemonic'in diskte ŞİFRELİ saklanması.
//
// Şifreleme Electron safeStorage ile yapılır (macOS Keychain / Windows DPAPI /
// Linux keyring). Bu modül safeStorage'a doğrudan bağımlı DEĞİLDİR: main.js
// bir "cipher" nesnesi enjekte eder; testler sahte cipher ile çalışır.
//
// cipher arayüzü:
//   scheme:  'safestorage' | 'none'   (yeni kayıtlar bu şemayla yazılır)
//   encrypt(str)          -> base64 string
//   decrypt(scheme, b64)  -> str (bilinmeyen şema / bozuk veri => throw)
//
// Dosya formatı (wallet.enc.json):
//   { version, cipher, data(base64), blockchain_address, backup_done,
//     migrated_from_legacy, created_at }
// blockchain_address düz metin durur — adres zaten herkese açık; mnemonic'i
// çözmeye gerek kalmadan arayüzde gösterilebilmesini sağlar.
'use strict';

const fs = require('fs');
const path = require('path');

const WALLET_FILE = 'wallet.enc.json';
const LEGACY_FILE = 'standalone_wallet.json'; // Go sidecar'ın eski düz metin dosyası

function walletPath(dir) { return path.join(dir, WALLET_FILE); }
function legacyPath(dir) { return path.join(dir, LEGACY_FILE); }

// Yarım yazılmış cüzdan dosyası = kayıp cüzdan. Önce .tmp'ye yaz, sonra
// atomik rename — elektrik kesilse bile eski dosya bozulmaz.
function atomicWrite(file, contents) {
  const tmp = file + '.tmp';
  fs.writeFileSync(tmp, contents, { mode: 0o600 });
  fs.renameSync(tmp, file);
}

function save(dir, cipher, mnemonic, meta = {}) {
  const record = {
    version: 1,
    cipher: cipher.scheme,
    data: cipher.encrypt(mnemonic),
    blockchain_address: meta.address || '',
    backup_done: !!meta.backupDone,
    migrated_from_legacy: !!meta.migratedFromLegacy,
    created_at: meta.createdAt || new Date().toISOString(),
  };
  atomicWrite(walletPath(dir), JSON.stringify(record, null, 2));
  return record;
}

// load sonuçları:
//   { status: 'empty' }                       — dosya yok
//   { status: 'ok', mnemonic, record }        — çözüldü
//   { status: 'corrupt', reason, record? }    — okunamadı/çözülemedi
function load(dir, cipher) {
  const file = walletPath(dir);
  if (!fs.existsSync(file)) return { status: 'empty' };

  let record;
  try {
    record = JSON.parse(fs.readFileSync(file, 'utf8'));
  } catch {
    return { status: 'corrupt', reason: 'json' };
  }
  if (!record || typeof record.data !== 'string' || record.data === '' ||
      typeof record.cipher !== 'string') {
    return { status: 'corrupt', reason: 'format', record };
  }

  let mnemonic;
  try {
    mnemonic = cipher.decrypt(record.cipher, record.data);
  } catch {
    return { status: 'corrupt', reason: 'decrypt', record };
  }
  if (!mnemonic) return { status: 'corrupt', reason: 'decrypt', record };
  return { status: 'ok', mnemonic, record };
}

// Eski düz metin cüzdanı şifreli formata taşır. Güvenlik sırası:
//   oku -> doğrula (inspect: BIP-39 + adres türet) -> adres tutarlılık kontrolü
//   -> şifreli yaz -> geri okuyup birebir doğrula.
//
// Düz metin dosya bu aşamada SİLİNMEZ. Nedeni: kullanıcı kelimelerini henüz
// hiç yedeklemedi ve safeStorage anahtarı ortama bağlıdır (ör. macOS'ta
// geliştirme ve paketli uygulamanın Keychain servisi farklı olabilir; keyring
// sıfırlanabilir). Şifreli kopya çözülemez hale gelirse düz metin tek kurtuluş
// yoludur. Silme, kullanıcı "kelimeleri yedekledim" diye onayladığında
// markBackupDone içinde yapılır.
//
// Herhangi bir adım başarısız olursa düz metin dosyaya DOKUNULMAZ; uygulama
// eski davranışla (legacy mod) çalışmaya devam eder — cüzdan asla riske girmez.
//
// inspect: async (mnemonic) => address | null  (main.js sidecar binary'sini çağırır)
async function migrateLegacy(dir, cipher, inspect) {
  const legacy = legacyPath(dir);
  if (!fs.existsSync(legacy)) return { status: 'none' };

  let parsed;
  try {
    parsed = JSON.parse(fs.readFileSync(legacy, 'utf8'));
  } catch {
    return { status: 'unreadable' };
  }
  const mnemonic = (parsed && typeof parsed.mnemonic === 'string')
    ? parsed.mnemonic.trim() : '';
  if (!mnemonic) return { status: 'unreadable' };

  const address = await inspect(mnemonic);
  if (!address) return { status: 'invalid' };
  if (parsed.blockchain_address && parsed.blockchain_address !== address) {
    return { status: 'mismatch' };
  }

  // Eski kullanıcı onboarding görmedi, kelimelerini hiç yedeklememiş olabilir
  // — backup_done=false ile işaretle ki arayüz yedekleme uyarısı göstersin.
  save(dir, cipher, mnemonic, {
    address,
    backupDone: false,
    migratedFromLegacy: true,
  });

  const check = load(dir, cipher);
  if (check.status !== 'ok' || check.mnemonic !== mnemonic) {
    try { fs.unlinkSync(walletPath(dir)); } catch { /* en kötü artık dosya kalır */ }
    return { status: 'verify-failed' };
  }

  return { status: 'migrated', mnemonic, record: check.record };
}

// Kullanıcı kelimelerini yedeklediğini onayladı.
// Migrasyondan kalan düz metin dosya ancak burada — şifreli kopyanın hâlâ
// çözülebildiği bir kez daha doğrulandıktan sonra — silinir.
// Doğrulama başarısızsa Error fırlatır ve hiçbir dosyaya dokunmaz.
function markBackupDone(dir, cipher) {
  const check = load(dir, cipher);
  if (check.status !== 'ok') {
    throw new Error('cüzdan dosyası doğrulanamadı: ' + (check.reason || check.status));
  }

  const record = check.record;
  record.backup_done = true;
  atomicWrite(walletPath(dir), JSON.stringify(record, null, 2));

  if (record.migrated_from_legacy && fs.existsSync(legacyPath(dir))) {
    fs.unlinkSync(legacyPath(dir));
  }
  return record;
}

// Çözülemeyen cüzdan dosyasını silmek yerine kenara taşı: içinde hâlâ
// kurtarılabilir veri olabilir (ör. keychain geri gelirse).
function quarantineCorrupt(dir) {
  const file = walletPath(dir);
  if (!fs.existsSync(file)) return null;
  const dest = path.join(dir, `wallet.enc.corrupt-${Date.now()}.json`);
  fs.renameSync(file, dest);
  return dest;
}

function hasLegacy(dir) { return fs.existsSync(legacyPath(dir)); }

// safeStorage'ı walletstore'un beklediği cipher arayüzüne sarar.
// Linux'ta keyring yoksa Electron 'basic_text' backend'ine düşer: anahtar
// diskte düz metin durur VE ileride gerçek keyring kurulursa backend değişip
// eski veri ÇÖZÜLEMEZ hale gelebilir. Bu yüzden basic_text'e güvenmiyoruz —
// o durumda 'none' şemasında kalıp kullanıcıyı arayüzden uyarıyoruz.
function makeCipher(safeStorage, platform = process.platform) {
  let trustworthy = safeStorage.isEncryptionAvailable();
  if (trustworthy && platform === 'linux' &&
      typeof safeStorage.getSelectedStorageBackend === 'function' &&
      safeStorage.getSelectedStorageBackend() === 'basic_text') {
    trustworthy = false;
  }
  return {
    scheme: trustworthy ? 'safestorage' : 'none',
    trustworthy,
    encrypt: (s) => trustworthy
      ? safeStorage.encryptString(s).toString('base64')
      : Buffer.from(s, 'utf8').toString('base64'),
    decrypt: (scheme, b64) => {
      if (scheme === 'safestorage') {
        return safeStorage.decryptString(Buffer.from(b64, 'base64'));
      }
      if (scheme === 'none') {
        return Buffer.from(b64, 'base64').toString('utf8');
      }
      throw new Error('bilinmeyen cipher: ' + scheme);
    },
  };
}

module.exports = {
  WALLET_FILE,
  LEGACY_FILE,
  walletPath,
  legacyPath,
  hasLegacy,
  save,
  load,
  migrateLegacy,
  markBackupDone,
  quarantineCorrupt,
  makeCipher,
};
