// FlatunChain Desktop — Electron ana süreci
//
// Mimari: Go "standalone" binary'si yan süreç (sidecar) olarak çalışır;
// cüzdan API'si YALNIZCA 127.0.0.1'e bağlıdır, anahtarlar/mnemonic renderer'a
// asla ulaşmaz. Renderer izole bir web sayfasıdır ve node/cüzdanla yalnızca
// preload köprüsündeki dar API üzerinden (IPC) konuşur.
const { app, BrowserWindow, ipcMain } = require('electron');
const { spawn } = require('child_process');
const path = require('path');
const fs = require('fs');

// ---- Yapılandırma ----
const WALLET_PORT = 18080; // cüzdan API (yalnızca 127.0.0.1)
const NODE_PORT = 5001;    // blockchain API (P2P için dışa açık kalır)
const WALLET_BASE = `http://127.0.0.1:${WALLET_PORT}`;
const NODE_BASE = `http://127.0.0.1:${NODE_PORT}`;

let mainWindow = null;
let sidecar = null;
let sidecarLog = [];

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

function startSidecar() {
  const bin = sidecarPath();
  if (!fs.existsSync(bin)) {
    sidecarLog.push(`HATA: sidecar bulunamadı: ${bin} — önce ./build.sh çalıştırın`);
    return;
  }

  const dataDir = path.join(app.getPath('userData'), 'chain');
  fs.mkdirSync(dataDir, { recursive: true });

  sidecar = spawn(bin, [
    `--wallet-port=${WALLET_PORT}`,
    `--blockchain-port=${NODE_PORT}`,
    `--bind=127.0.0.1`,
    `--data-dir=${dataDir}`,
    `--open=false`,
    `--miner=false`,
  ], { stdio: ['ignore', 'pipe', 'pipe'] });

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
  sidecar.on('exit', (code) => {
    sidecarLog.push(`Sidecar kapandı (kod: ${code})`);
    sidecar = null;
  });
}

function stopSidecar() {
  if (sidecar) {
    sidecar.kill('SIGTERM');
    sidecar = null;
  }
}

// ---- Yardımcı: sidecar API çağrısı ----
async function api(base, pathname, options = {}) {
  const res = await fetch(base + pathname, options);
  const text = await res.text();
  let json = null;
  try { json = JSON.parse(text); } catch { /* düz metin yanıt */ }
  return { ok: res.ok, status: res.status, json, text };
}

// ---- IPC köprüsü (renderer'ın görebildiği TEK yüzey) ----
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
    webPreferences: {
      preload: path.join(__dirname, 'preload.js'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: true,
    },
  });
  mainWindow.loadFile(path.join(__dirname, 'renderer', 'index.html'));
}

app.whenReady().then(() => {
  startSidecar();
  createWindow();
  app.on('activate', () => {
    if (BrowserWindow.getAllWindows().length === 0) createWindow();
  });
});

app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') app.quit();
});
app.on('before-quit', stopSidecar);
