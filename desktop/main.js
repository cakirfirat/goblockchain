// FlatunChain Desktop — Electron ana süreci
//
// Mimari: Go "standalone" binary'si yan süreç (sidecar) olarak çalışır;
// cüzdan VE node API'leri YALNIZCA 127.0.0.1'e bağlıdır, anahtarlar/mnemonic
// renderer'a asla ulaşmaz (tek istisna: kullanıcının açıkça istediği
// yedekleme/onboarding ekranlarında kelimelerin GÖSTERİLMESİ). Renderer izole
// bir web sayfasıdır ve node/cüzdanla yalnızca preload köprüsündeki dar API
// üzerinden (IPC) konuşur. Mining uçları lansman başına üretilen ayrı bir
// token'la (FLATUN_NODE_TOKEN) kilitlidir.
//
// Cüzdan saklama (Faz 2): mnemonic diskte Electron safeStorage ile şifreli
// durur (macOS Keychain / Windows DPAPI / Linux keyring). Go sidecar'ı düz
// metin dosya OKUMAZ/YAZMAZ; mnemonic her açılışta çözülüp env ile geçirilir.
const { app, BrowserWindow, ipcMain, safeStorage } = require('electron');
const { spawn, execFile } = require('child_process');
const crypto = require('crypto');
const path = require('path');
const fs = require('fs');

const store = require('./walletstore');

// Test/geliştirme izolasyonu: gerçek cüzdana dokunmadan onboarding'i denemek
// için FLATUN_USER_DATA=<dizin> ile ayrı bir profil kullanılabilir.
if (process.env.FLATUN_USER_DATA) {
  app.setPath('userData', process.env.FLATUN_USER_DATA);
}

// Tek kopya kilidi: ikinci kopya port çakışması + farklı token yüzünden
// sessizce bozuk çalışırdı (sidecar'ı açılamaz, cüzdan istekleri 403 yerdi).
// İkinci kopya açılırsa mevcut pencereyi öne getirip çıkıyoruz.
if (!app.requestSingleInstanceLock()) {
  app.quit();
} else {
  app.on('second-instance', () => {
    if (mainWindow && !mainWindow.isDestroyed()) {
      if (mainWindow.isMinimized()) mainWindow.restore();
      mainWindow.show();
      mainWindow.focus();
    }
  });
}

// ---- Yapılandırma ----
// Portlar test/otomasyon için env ile değiştirilebilir (varsayılanlar üretim)
const WALLET_PORT = Number(process.env.FLATUN_WALLET_PORT) || 18080; // cüzdan API (yalnızca 127.0.0.1)
const NODE_PORT = Number(process.env.FLATUN_NODE_PORT) || 5001;      // blockchain API (yalnızca 127.0.0.1)
const WALLET_BASE = `http://127.0.0.1:${WALLET_PORT}`;
const NODE_BASE = `http://127.0.0.1:${NODE_PORT}`;

// Her lansmanda rastgele üretilen gizli token'lar. Sidecar'a env ile geçirilir
// ve ilgili API çağrılarına başlık olarak eklenir. Kullanıcının tarayıcısında
// açılan kötü niyetli bir sayfa 127.0.0.1'e istek atabilir ama token'ları
// bilemez — böylece "localhost cüzdan boşaltma" ve "gizli madencilik
// aç/kapa" saldırıları engellenir. İki token bilinçli olarak ayrıdır: node
// API'si sunucu kurulumlarında dışa açılabilir, cüzdan token'ı o yüzeye sızmamalı.
const WALLET_TOKEN = crypto.randomBytes(32).toString('hex');
const NODE_TOKEN = crypto.randomBytes(32).toString('hex');

let mainWindow = null;
let sidecar = null;
let sidecarLog = [];
let cipher = null;
let quitting = false;

// Çökme sonrası otomatik yeniden başlatma sayacı (oturum başına).
// 60 sn'den uzun sağlıklı çalışma sayacı sıfırlar; art arda 3 çökmede durur
// (ör. port çakışması gibi kalıcı bir sorunda sonsuz döngüye girilmez).
let sidecarRestarts = 0;
let sidecarStartedAt = 0;

// Uygulama durumu — renderer hangi ekranı göstereceğini buradan öğrenir.
//   phase: 'onboarding' — cüzdan yok, oluştur/içe aktar ekranı
//          'recovery'   — şifreli cüzdan çözülemedi, kelimelerle kurtarma
//          'ready'      — cüzdan hazır, ana ekran
//   walletMode: 'encrypted' (env ile sidecar'a geçer) | 'legacy' (sidecar düz
//               metin dosyayı kendi okur — keyring yoksa / migrasyon başarısızsa)
//   warning: 'keyring-missing' | 'legacy-kept' | null
let appState = {
  phase: 'onboarding',
  walletMode: 'encrypted',
  warning: null,
  backupPending: false,
  address: null,
};

// Onboarding sırasında üretilen ve henüz diske yazılmamış cüzdan.
// Kullanıcı kelimeleri doğrulayana kadar yalnızca ana süreç belleğinde durur.
let pendingWallet = null;
let importInFlight = false; // çifte içe aktarma (çift tık) koruması

// Sidecar binary yolu: paketli uygulamada resources/sidecar/, geliştirmede build/
function sidecarPath() {
  const names = {
    darwin: 'flatuncoin-mac',
    win32: 'flatuncoin-windows.exe',
    linux: 'flatuncoin-linux',
  };
  const name = names[process.platform];
  if (app.isPackaged) {
    return path.join(process.resourcesPath, 'sidecar', name);
  }
  return path.join(__dirname, '..', 'build', name);
}

// ---- Tek seferlik cüzdan aracı (sidecar binary'si, sunucu başlatmadan) ----
function runWalletTool(mode, mnemonic = null) {
  return new Promise((resolve, reject) => {
    const env = { ...process.env };
    // Mnemonic argv yerine env ile geçirilir (argv'ler `ps` ile görünebilir)
    if (mnemonic !== null) env.FLATUN_WALLET_MNEMONIC = mnemonic;
    execFile(sidecarPath(), [`--wallet-tool=${mode}`],
      { env, timeout: 15000 },
      (err, stdout) => {
        if (err) { reject(err); return; }
        try { resolve(JSON.parse(stdout)); }
        catch { reject(new Error('wallet-tool çıktısı çözümlenemedi')); }
      });
  });
}

// inspect: mnemonic geçerliyse adresini döndürür, değilse null
async function inspectMnemonic(mnemonic) {
  try {
    const out = await runWalletTool('inspect', mnemonic);
    return out.blockchain_address || null;
  } catch (err) {
    if (err && err.code === 3) return null; // geçersiz mnemonic (BIP-39)
    throw err; // binary yok / başka hata — çağıran ayrı ele alır
  }
}

// ---- Cüzdan başlatma: yükle / taşı / onboarding'e yönlendir ----
// Dönüş: sidecar'a env ile geçirilecek mnemonic (legacy modda null).
async function initWallet() {
  const dir = app.getPath('userData');
  fs.mkdirSync(dir, { recursive: true });
  cipher = store.makeCipher(safeStorage);
  const baseWarning = cipher.trustworthy ? null : 'keyring-missing';

  // 1) Şifreli cüzdan dosyası
  const loaded = store.load(dir, cipher);
  if (loaded.status === 'ok') {
    // Fırsatçı yükseltme: dosya 'none' ile yazılmış ama artık gerçek
    // şifreleme mevcutsa sessizce şifrele.
    if (loaded.record.cipher === 'none' && cipher.scheme === 'safestorage') {
      store.save(dir, cipher, loaded.mnemonic, {
        address: loaded.record.blockchain_address,
        backupDone: loaded.record.backup_done,
        migratedFromLegacy: loaded.record.migrated_from_legacy,
        createdAt: loaded.record.created_at,
      });
      sidecarLog.push('Cüzdan dosyası sistem anahtarlığıyla yeniden şifrelendi');
    }
    appState = {
      phase: 'ready',
      walletMode: 'encrypted',
      warning: baseWarning,
      backupPending: !loaded.record.backup_done,
      address: loaded.record.blockchain_address || null,
    };
    return loaded.mnemonic;
  }

  if (loaded.status === 'corrupt') {
    // Dosya var ama çözülemiyor (ör. Keychain sıfırlandı, veri başka
    // makineden kopyalandı). Sil(ME), kenara taşı. Kurtarma ekranına düşmeden
    // önce aşağıda düz metin kopya denenir: migrasyon, kullanıcı kelimelerini
    // yedekleyene kadar düz metni sakladığından, bu pencerede keychain kaybı
    // kelime sormadan kendiliğinden onarılabilir.
    const quarantined = store.quarantineCorrupt(dir);
    sidecarLog.push(`UYARI: cüzdan dosyası çözülemedi (${loaded.reason}); ` +
      `${quarantined ? path.basename(quarantined) + ' olarak saklandı' : ''}`);
    if (!store.hasLegacy(dir)) {
      appState = {
        phase: 'recovery',
        walletMode: 'encrypted',
        warning: baseWarning,
        backupPending: false,
        address: (loaded.record && loaded.record.blockchain_address) || null,
      };
      return null;
    }
    // düz metin kopya var — aşağıdaki legacy dalı yeniden şifreleyecek
  }

  // 2) Eski düz metin cüzdan (v1 kurulumları / yedeklenmemiş migrasyon kopyası)
  if (store.hasLegacy(dir)) {
    if (cipher.trustworthy) {
      try {
        const mig = await store.migrateLegacy(dir, cipher, inspectMnemonic);
        if (mig.status === 'migrated') {
          sidecarLog.push('Cüzdan şifreli depoya taşındı; kurtarma kelimeleri yedeklendiğinde düz metin kopya silinecek');
          appState = {
            phase: 'ready',
            walletMode: 'encrypted',
            warning: null,
            backupPending: true, // eski kullanıcı kelimelerini hiç yedeklemedi
            address: mig.record.blockchain_address || null,
          };
          return mig.mnemonic;
        }
        sidecarLog.push(`UYARI: cüzdan taşınamadı (${mig.status}); düz metin modunda devam ediliyor`);
      } catch (err) {
        // inspect çalıştırılamadı (ör. sidecar binary yok) — dokunma
        sidecarLog.push(`UYARI: cüzdan taşıma denenemedi: ${err.message}`);
      }
    }
    // Taşınamadı veya şifreleme güvenilir değil: sidecar düz metin dosyayı
    // bugüne kadarki gibi kendisi okur. Cüzdan çalışmaya devam eder.
    appState = {
      phase: 'ready',
      walletMode: 'legacy',
      warning: cipher.trustworthy ? 'legacy-kept' : 'keyring-missing',
      backupPending: true,
      address: null, // adres sidecar açılınca /wallet/info'dan gelir
    };
    return null;
  }

  // 3) Hiç cüzdan yok — onboarding
  appState = {
    phase: 'onboarding',
    walletMode: 'encrypted',
    warning: baseWarning,
    backupPending: false,
    address: null,
  };
  return null;
}

function startSidecar(mnemonic) {
  if (sidecar) return; // çifte spawn koruması
  const bin = sidecarPath();
  if (!fs.existsSync(bin)) {
    sidecarLog.push(`HATA: sidecar bulunamadı: ${bin} — önce ./build.sh çalıştırın`);
    return;
  }

  // Not: standalone'ın kendi varsayılanıyla aynı konum — kullanıcı uygulamayı
  // Electron'dan da tarayıcı modundan da açsa AYNI veri dizinini görür
  const dataDir = app.getPath('userData');
  fs.mkdirSync(dataDir, { recursive: true });

  const env = {
    ...process.env,
    FLATUN_WALLET_TOKEN: WALLET_TOKEN,
    FLATUN_NODE_TOKEN: NODE_TOKEN, // mining uçları yalnızca bu uygulamadan yönetilebilir
  };
  // Şifreli moddaki cüzdan sidecar'a env ile geçer (argv değil: `ps` görünürlüğü).
  // Legacy modda env verilmez; sidecar düz metin dosyayı kendisi okur.
  if (mnemonic) env.FLATUN_WALLET_MNEMONIC = mnemonic;

  sidecar = spawn(bin, [
    `--wallet-port=${WALLET_PORT}`,
    `--blockchain-port=${NODE_PORT}`,
    `--bind=127.0.0.1`,
    // Node API de yalnızca loopback dinler: LAN'daki biri madenciliği tetikleyemez
    // veya peer enjekte edemez. Dışa dinlememek ağ üyeliğini bozmaz — NAT
    // arkasındaki node gibi çekme döngüsü + /submit itmesiyle senkron kalır.
    `--node-bind=127.0.0.1`,
    `--data-dir=${dataDir}`,
    `--open=false`,
    `--miner=false`,
  ], {
    stdio: ['ignore', 'pipe', 'pipe'],
    env,
  });

  const capture = (buf) => {
    const line = buf.toString().trim();
    if (!line) return;
    sidecarLog.push(line);
    if (sidecarLog.length > 200) sidecarLog.shift();
    if (mainWindow && !mainWindow.isDestroyed()) {
      mainWindow.webContents.send('sidecar-log', line);
    }
  };
  sidecar.stdout.on('data', capture);
  sidecar.stderr.on('data', capture);
  sidecarStartedAt = Date.now();
  sidecar.on('exit', (code) => {
    sidecarLog.push(`Sidecar kapandı (kod: ${code})`);
    sidecar = null;
    if (quitting || appState.phase !== 'ready' || code === 0) return;

    // Çökme: sınırlı sayıda otomatik yeniden başlat. Uzun süre sağlıklı
    // çalıştıysa sayaç sıfırlanır (günde bir kez çöken node'u öldürmeyelim);
    // açılışta art arda çöküyorsa (ör. port çakışması) 3 denemede vazgeç.
    if (Date.now() - sidecarStartedAt > 60_000) sidecarRestarts = 0;
    if (sidecarRestarts >= 3) {
      sidecarLog.push('HATA: node art arda çöktü; otomatik yeniden başlatma durduruldu. Uygulamayı yeniden başlatın.');
      return;
    }
    sidecarRestarts++;
    const delay = 2000 * sidecarRestarts;
    sidecarLog.push(`Node ${delay / 1000} sn içinde yeniden başlatılacak (deneme ${sidecarRestarts}/3)…`);
    setTimeout(() => {
      if (quitting || sidecar) return;
      const res = resolveMnemonicForSidecar();
      // Şifreli modda cüzdan o an çözülemiyorsa sidecar'ı env'siz başlatMAyız:
      // Go tarafı düz metin dosya da bulamayınca YENİ cüzdan üretirdi —
      // kullanıcı bir anda boş/yabancı cüzdan görürdü. Hiç başlatmamak doğru.
      if (res.ok) startSidecar(res.mnemonic);
      else sidecarLog.push('HATA: yeniden başlatmada cüzdan çözülemedi; node durduruldu. Uygulamayı yeniden başlatın.');
    }, delay);
  });
}

// Yeniden başlatma için mnemonic'i o an diskten çözer (bellekte tutulmaz).
// Legacy modda env gerekmez (sidecar düz metin dosyayı kendisi okur).
function resolveMnemonicForSidecar() {
  if (appState.walletMode !== 'encrypted') return { ok: true, mnemonic: null };
  const loaded = store.load(app.getPath('userData'), cipher);
  if (loaded.status !== 'ok') return { ok: false, mnemonic: null };
  return { ok: true, mnemonic: loaded.mnemonic };
}

function stopSidecar() {
  if (sidecar) {
    sidecar.kill('SIGTERM');
    sidecar = null;
  }
}

// ---- Yardımcı: sidecar API çağrısı ----
async function api(base, pathname, options = {}) {
  const headers = { ...(options.headers || {}) };
  // İlgili API'ye yapılan her çağrıya kendi gizli token'ını ekle
  if (base === WALLET_BASE) headers['X-Flatun-Token'] = WALLET_TOKEN;
  if (base === NODE_BASE) headers['X-Flatun-Node-Token'] = NODE_TOKEN;
  // Timeout şart: sidecar askıda kalırsa IPC promise'leri sonsuza dek
  // birikir ve renderer'daki periyodik yenilemeler üst üste yığılırdı.
  const res = await fetch(base + pathname, {
    ...options,
    headers,
    signal: AbortSignal.timeout(15_000),
  });
  const text = await res.text();
  let json = null;
  try { json = JSON.parse(text); } catch { /* düz metin yanıt */ }
  return { ok: res.ok, status: res.status, json, text };
}

// Onboarding tamamlandığında ortak bitiş: kaydet, sidecar'ı başlat, duruma geç
function finishOnboarding(mnemonic, address, { migratedFromLegacy = false } = {}) {
  const dir = app.getPath('userData');
  store.save(dir, cipher, mnemonic, {
    address,
    backupDone: true, // kullanıcı kelimeleri ya doğruladı ya da kendisi girdi
    migratedFromLegacy,
  });
  appState = {
    phase: 'ready',
    walletMode: 'encrypted',
    warning: cipher.trustworthy ? null : 'keyring-missing',
    backupPending: false,
    address,
  };
  startSidecar(mnemonic);
}

// ---- IPC köprüsü (renderer'ın görebildiği TEK yüzey) ----
ipcMain.handle('app:state', () => appState);

// Onboarding: yeni cüzdan üret. Kelimeler doğrulanana kadar YALNIZCA ana
// süreç belleğinde (pendingWallet) tutulur; diske hiçbir şey yazılmaz.
// Hata dönüşlerindeki `code` alanı renderer'da yerelleştirilir (i18n.js /
// ERR_CODE_KEYS); `message` eski Türkçe metin olarak yedek amaçlı kalır.
ipcMain.handle('onboarding:generate', async () => {
  if (appState.phase !== 'onboarding') return { ok: false, code: 'bad-state', message: 'geçersiz durum' };
  try {
    const out = await runWalletTool('generate');
    pendingWallet = { mnemonic: out.mnemonic, address: out.blockchain_address };
    return { ok: true, words: out.mnemonic.split(' '), address: out.blockchain_address };
  } catch (err) {
    if (err && err.code === 'ENOENT') {
      return { ok: false, code: 'no-core', message: 'Node çekirdeği bulunamadı (geliştirme: önce ./build.sh çalıştırın)' };
    }
    const detail = err.message || '';
    return { ok: false, code: 'generate-failed', detail, message: 'Cüzdan üretilemedi: ' + (detail || 'bilinmeyen hata') };
  }
});

// Onboarding: kullanıcı kelimeleri yazdığını doğruladı — artık kalıcılaştır
ipcMain.handle('onboarding:confirmNew', () => {
  if (appState.phase !== 'onboarding' || !pendingWallet) {
    return { ok: false, code: 'bad-state', message: 'geçersiz durum' };
  }
  const { mnemonic, address } = pendingWallet;
  try {
    finishOnboarding(mnemonic, address);
  } catch (err) {
    // Kayıt başarısız (ör. Keychain o an kilitli/erişilemez): pendingWallet'ı
    // KORU ki kullanıcı aynı kelimelerle tekrar deneyebilsin.
    const detail = err.message || '';
    return { ok: false, code: 'save-failed', detail, message: 'Cüzdan kaydedilemedi: ' + (detail || 'bilinmeyen hata') };
  }
  pendingWallet = null;
  return { ok: true, address };
});

// Onboarding/kurtarma: mevcut kelimelerle içe aktar
ipcMain.handle('onboarding:import', async (_e, rawMnemonic) => {
  if (appState.phase !== 'onboarding' && appState.phase !== 'recovery') {
    return { ok: false, code: 'bad-state', message: 'geçersiz durum' };
  }
  if (typeof rawMnemonic !== 'string') return { ok: false, code: 'bad-input', message: 'geçersiz girdi' };
  if (importInFlight) return { ok: false, code: 'busy', message: 'işlem sürüyor' };

  // Go tarafı da normalize eder; burada erken yapmak hata mesajını netleştirir
  const mnemonic = rawMnemonic.trim().toLowerCase().split(/\s+/).join(' ');
  if (!mnemonic) return { ok: false, code: 'empty-mnemonic', message: 'Kurtarma kelimelerini girin.' };

  importInFlight = true;
  try {
    const address = await inspectMnemonic(mnemonic);
    if (!address) {
      return { ok: false, code: 'invalid-mnemonic', message: 'Kelimeler geçersiz (BIP-39 doğrulaması başarısız). Sırayı ve yazımı kontrol edin.' };
    }
    finishOnboarding(mnemonic, address);
    return { ok: true, address };
  } catch (err) {
    if (err && err.code === 'ENOENT') {
      return { ok: false, code: 'no-core', message: 'Node çekirdeği bulunamadı (geliştirme: önce ./build.sh çalıştırın)' };
    }
    const detail = err.message || '';
    return { ok: false, code: 'import-failed', detail, message: 'İçe aktarma başarısız: ' + (detail || 'bilinmeyen hata') };
  } finally {
    importInFlight = false;
  }
});

// Yedekleme: kelimeleri göster. Bellekte tutulmaz — her seferinde diskten
// okunup çözülür ki mnemonic ana süreçte kalıcı yaşamasın.
ipcMain.handle('wallet:reveal', () => {
  if (appState.phase !== 'ready') return { ok: false, code: 'not-ready', message: 'cüzdan hazır değil' };
  const dir = app.getPath('userData');

  if (appState.walletMode === 'legacy') {
    try {
      const parsed = JSON.parse(fs.readFileSync(store.legacyPath(dir), 'utf8'));
      if (parsed && typeof parsed.mnemonic === 'string' && parsed.mnemonic) {
        return { ok: true, words: parsed.mnemonic.trim().split(/\s+/) };
      }
    } catch { /* aşağıda genel hata */ }
    return { ok: false, code: 'read-failed', message: 'cüzdan dosyası okunamadı' };
  }

  const loaded = store.load(dir, cipher);
  if (loaded.status !== 'ok') return { ok: false, code: 'decrypt-failed', message: 'cüzdan çözülemedi' };
  return { ok: true, words: loaded.mnemonic.split(' ') };
});

ipcMain.handle('wallet:backupDone', () => {
  if (appState.phase !== 'ready') return { ok: false };
  if (appState.walletMode === 'encrypted') {
    // Şifreli kopyanın çözülebildiği son kez doğrulanır; ancak o zaman
    // migrasyondan kalan düz metin dosya silinir.
    try { store.markBackupDone(app.getPath('userData'), cipher); }
    catch (err) {
      sidecarLog.push(`UYARI: yedek onayı kaydedilemedi: ${err.message}`);
      return { ok: false };
    }
  }
  // legacy modda kalıcı meta alanımız yok; bu oturum için uyarıyı kapat
  appState.backupPending = false;
  return { ok: true };
});

ipcMain.handle('wallet:info', () => api(WALLET_BASE, '/wallet/info'));
ipcMain.handle('wallet:balance', async () => {
  const info = await api(WALLET_BASE, '/wallet/info');
  if (!info.ok || !info.json) return { ok: false };
  return api(WALLET_BASE, `/wallet/amount?blockchain_address=${info.json.blockchain_address}`);
});
ipcMain.handle('wallet:send', (_e, { recipient, value }) => {
  // Girdi doğrulaması main tarafında da yapılır — renderer'a güvenilmez
  if (typeof recipient !== 'string' || typeof value !== 'string' ||
      recipient.length < 26 || recipient.length > 40) {
    return { ok: false, status: 400, json: { message: 'invalid input' } };
  }
  return api(WALLET_BASE, '/wallet/send', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ recipient, value }),
  });
});
ipcMain.handle('mine:start', () => api(NODE_BASE, '/mine/start'));
ipcMain.handle('mine:stop', () => api(NODE_BASE, '/mine/stop'));
ipcMain.handle('mine:status', () => api(NODE_BASE, '/mine/status'));
ipcMain.handle('node:log', () => sidecarLog.slice(-50));

function createWindow() {
  mainWindow = new BrowserWindow({
    width: 960,
    height: 680,
    title: 'FlatunChain',
    // Linux/Windows pencere ikonu; macOS paketli sürümde ikon bundle'dan gelir
    icon: path.join(__dirname, 'assets', 'icon.png'),
    // FLATUN_HIDDEN: otomasyon/e2e testleri pencereyi göstermeden çalıştırır.
    // Yalnızca geliştirme modunda etkilidir — paketli uygulamada ortamdan
    // gelen bir değişken pencereyi gizleyip "uygulama açılmıyor" izlenimi verememeli.
    show: app.isPackaged || !process.env.FLATUN_HIDDEN,
    webPreferences: {
      preload: path.join(__dirname, 'preload.js'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: true,
      // Pencere arka planda/simge durumundayken de node durumu güncel kalsın
      backgroundThrottling: false,
    },
  });
  mainWindow.loadFile(path.join(__dirname, 'renderer', 'index.html'));
}

app.whenReady().then(async () => {
  // safeStorage (Linux'ta) ancak ready sonrası güvenilir cevap verir
  let mnemonic = null;
  try {
    mnemonic = await initWallet();
  } catch (err) {
    sidecarLog.push(`HATA: cüzdan başlatılamadı: ${err.message}`);
  }
  if (appState.phase === 'ready') startSidecar(mnemonic);
  mnemonic = null;

  createWindow();
  app.on('activate', () => {
    if (BrowserWindow.getAllWindows().length === 0) createWindow();
  });
});

app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') app.quit();
});
app.on('before-quit', () => {
  quitting = true;
  stopSidecar();
});
