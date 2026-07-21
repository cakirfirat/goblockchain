// Preload köprüsü — renderer'a açılan TEK API yüzeyi.
// Renderer hiçbir zaman node API'sine, dosya sistemine veya anahtarlara
// doğrudan erişemez; yalnızca bu dar fonksiyon kümesini görür.
const { contextBridge, ipcRenderer } = require('electron');

contextBridge.exposeInMainWorld('flatun', {
  // Uygulama durumu (onboarding mi, ana ekran mı)
  appState: () => ipcRenderer.invoke('app:state'),

  // Onboarding / kurtarma
  onboardingGenerate: () => ipcRenderer.invoke('onboarding:generate'),
  onboardingConfirmNew: () => ipcRenderer.invoke('onboarding:confirmNew'),
  onboardingImport: (mnemonic) => ipcRenderer.invoke('onboarding:import', mnemonic),

  // Yedekleme (kelimeleri göster / yedeklendi işaretle)
  revealMnemonic: () => ipcRenderer.invoke('wallet:reveal'),
  backupDone: () => ipcRenderer.invoke('wallet:backupDone'),

  // Cüzdan / madencilik
  walletInfo: () => ipcRenderer.invoke('wallet:info'),
  balance: () => ipcRenderer.invoke('wallet:balance'),
  send: (recipient, value) => ipcRenderer.invoke('wallet:send', { recipient, value }),
  mineStart: () => ipcRenderer.invoke('mine:start'),
  mineStop: () => ipcRenderer.invoke('mine:stop'),
  mineStatus: () => ipcRenderer.invoke('mine:status'),
  nodeLog: () => ipcRenderer.invoke('node:log'),
  onLog: (cb) => ipcRenderer.on('sidecar-log', (_e, line) => cb(line)),
});
