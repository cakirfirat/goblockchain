// FlatunChain renderer — yalnızca window.flatun köprüsünü kullanır.
// Ekran akışı: onboarding (oluştur/doğrula/içe aktar) -> ana ekran.
// Para gönderimi her zaman onay modalından geçer.
//
// Mining Core: madencilik başladığında hex-yağmuru canvas'ı, tarama çizgisi
// ve canlı sayaçlar devreye girer; blok bulununca kısa bir "BLOK KAZILDI"
// flaşı gösterilir. Tüm animasyonlar yereldir ve node'a yük bindirmez —
// gerçek veri /mine/status'tan gelir (hash denemesi, zorluk, yükseklik...).
const $ = (id) => document.getElementById(id);
const show = (el) => el.classList.remove('hidden');
const hide = (el) => el.classList.add('hidden');

let mining = false;
let pollersStarted = false;
let lastBalanceText = null;
let lastBalanceUnits = null;
let lastAppState = null;   // dil değişince banner'ı yeniden çizebilmek için
let recoveryMode = false;  // kurtarma modunda içe aktarma metinleri farklı

// Onboarding geçici durumu (yalnızca bu ekranlar açıkken dolu)
let obWords = null;
let quizIndices = [];

// ---------- Dil (i18n) ----------

// Sayı biçimlendirici dile göre yeniden kurulur
let fmt = new Intl.NumberFormat(i18nLocale());

function populateLangSelects() {
  for (const sel of [$('lang-select'), $('ob-lang-select')]) {
    sel.textContent = '';
    for (const m of LANG_META) {
      const opt = document.createElement('option');
      opt.value = m.code;
      opt.textContent = m.name;
      sel.appendChild(opt);
    }
    sel.value = getLang();
    sel.addEventListener('change', () => switchLang(sel.value));
  }
}

// Dili anında uygular: statik metinler hemen, dinamik alanlar bir sonraki
// yoklama turunda (2 sn) tazelenir. Ekran durumu (onboarding adımı, modal,
// bekleyen cüzdan) korunur — yeniden yükleme YOK.
function switchLang(code) {
  setLang(code);
  fmt = new Intl.NumberFormat(i18nLocale());
  $('lang-select').value = code;
  $('ob-lang-select').value = code;
  applyStaticI18n();
  if (recoveryMode) applyRecoveryTexts();
  // Quiz etiketleri ("N. kelime") statik değil — açıksa yeniden yaz
  for (const label of $('ob-quiz').querySelectorAll('label')) {
    const idx = Number(label.dataset.idx);
    if (Number.isFinite(idx)) label.textContent = t('ob.quizWord', { n: idx + 1 });
  }
  if (lastAppState) updateBanner(lastAppState);
  // Madencilik butonu/durumu anında güncellensin
  const btn = $('mine-toggle');
  if (!btn.disabled) btn.textContent = mining ? t('mine.stop') : t('mine.start');
  setMineUIState(mining);
  $('mine-info').textContent = mining ? t('mine.info') : '';
}

function applyRecoveryTexts() {
  $('ob-import-title').textContent = t('ob.recoverTitle');
  $('ob-import-note').textContent = t('ob.recoverNote');
}

// ---------- Ekran yönlendirme ----------

function showObStep(stepId) {
  for (const s of document.querySelectorAll('.ob-step')) hide(s);
  show($(stepId));
}

async function boot() {
  const st = await window.flatun.appState();
  lastAppState = st;

  if (st.phase === 'onboarding') {
    show($('screen-onboarding'));
    if (st.warning === 'keyring-missing') show($('ob-keyring-warn'));
    showObStep('ob-choice');
    return;
  }

  if (st.phase === 'recovery') {
    // Şifreli cüzdan çözülemedi — tek çıkış yolu kelimelerle geri yükleme
    show($('screen-onboarding'));
    recoveryMode = true;
    applyRecoveryTexts();
    hide($('ob-import-back')); // geri dönülecek ekran yok
    showObStep('ob-import');
    return;
  }

  recoveryMode = false;
  enterMain(st);
}

function enterMain(st) {
  hide($('screen-onboarding'));
  show($('screen-main'));
  // Adres cüzdan meta'sında hazır — node açılmasını beklemeden göster
  if (st.address) $('address').textContent = st.address;
  updateBanner(st);
  initMiningVisual();
  if (!pollersStarted) {
    pollersStarted = true;
    setInterval(refreshStatus, 2000);
    setInterval(refreshWallet, 5000);
  }
  refreshStatus();
  refreshWallet();
}

function updateBanner(st) {
  const banner = $('banner');
  const text = $('banner-text');
  const action = $('banner-action');
  hide(banner); hide(action);

  if (st.backupPending) {
    text.textContent = t('banner.backup');
    action.textContent = t('banner.backupBtn');
    show(action);
    show(banner);
  } else if (st.warning === 'keyring-missing') {
    text.textContent = t('banner.keyring');
    show(banner);
  } else if (st.warning === 'legacy-kept') {
    text.textContent = t('banner.legacy');
    show(banner);
  }
}

// ---------- Onboarding: yeni cüzdan ----------

function renderWords(listEl, words) {
  listEl.textContent = '';
  words.forEach((w) => {
    const li = document.createElement('li');
    li.textContent = w;
    listEl.appendChild(li);
  });
}

$('ob-create-btn').addEventListener('click', async () => {
  $('ob-create-btn').disabled = true;
  hide($('ob-choice-error'));
  show($('ob-busy'));
  const res = await window.flatun.onboardingGenerate();
  hide($('ob-busy'));
  $('ob-create-btn').disabled = false;
  if (!res.ok) {
    $('ob-choice-error').textContent = errText(res, 'err.createFail');
    show($('ob-choice-error'));
    return;
  }
  obWords = res.words;
  renderWords($('ob-words-list'), obWords);
  showObStep('ob-words');
});

$('ob-words-back').addEventListener('click', () => {
  obWords = null;
  showObStep('ob-choice');
});

$('ob-words-next').addEventListener('click', () => {
  buildQuiz();
  showObStep('ob-verify');
});

function buildQuiz() {
  // 3 farklı rastgele kelime sorulur — kâğıda gerçekten yazıldığının testi
  const n = obWords.length;
  const picks = new Set();
  while (picks.size < 3) picks.add(Math.floor(Math.random() * n));
  quizIndices = [...picks].sort((a, b) => a - b);

  const quiz = $('ob-quiz');
  quiz.textContent = '';
  hide($('ob-verify-error'));
  quizIndices.forEach((idx) => {
    const label = document.createElement('label');
    label.textContent = t('ob.quizWord', { n: idx + 1 });
    label.dataset.idx = String(idx); // dil değişiminde yeniden çevrilebilsin
    const input = document.createElement('input');
    input.type = 'text';
    input.autocomplete = 'off';
    input.spellcheck = false;
    input.dataset.idx = String(idx);
    quiz.appendChild(label);
    quiz.appendChild(input);
  });
}

$('ob-verify-back').addEventListener('click', () => showObStep('ob-words'));

$('ob-verify-btn').addEventListener('click', async () => {
  const inputs = $('ob-quiz').querySelectorAll('input');
  for (const input of inputs) {
    const idx = Number(input.dataset.idx);
    if (input.value.trim().toLowerCase() !== obWords[idx]) {
      $('ob-verify-error').textContent = t('ob.mismatch');
      show($('ob-verify-error'));
      return;
    }
  }
  $('ob-verify-btn').disabled = true;
  let res;
  try {
    res = await window.flatun.onboardingConfirmNew();
  } catch {
    res = { ok: false };
  }
  $('ob-verify-btn').disabled = false;
  if (!res.ok) {
    $('ob-verify-error').textContent = errText(res, 'err.saveFailed');
    show($('ob-verify-error'));
    return;
  }
  // Kelimeleri bellekten ve DOM'dan temizle — arka planda gizli de olsa kalmasın
  obWords = null;
  $('ob-words-list').textContent = '';
  $('ob-quiz').textContent = '';
  boot();
});

// ---------- Onboarding: içe aktar / kurtar ----------

$('ob-import-btn').addEventListener('click', () => {
  hide($('ob-import-error'));
  showObStep('ob-import');
});

$('ob-import-back').addEventListener('click', () => showObStep('ob-choice'));

$('ob-import-go').addEventListener('click', async () => {
  const text = $('ob-import-text').value;
  hide($('ob-import-error'));
  $('ob-import-go').disabled = true;
  const res = await window.flatun.onboardingImport(text);
  $('ob-import-go').disabled = false;
  if (!res.ok) {
    $('ob-import-error').textContent = errText(res, 'err.importFailed');
    show($('ob-import-error'));
    return;
  }
  $('ob-import-text').value = '';
  boot();
});

// ============================================================
// MINING CORE — görsel katman
// ============================================================
//
// hex-yağmuru: her sütunda aşağı akan rastgele hex karakterleri.
// Madencilik kapalıyken çok seyrek ve soluk (node "nefes alıyor"),
// açıkken yoğun ve parlak. requestAnimationFrame ~30fps'e kısılır;
// pencere gizliyken Electron zaten rAF'ı duraklatır.
const HEX_CHARS = '0123456789abcdef';
const rain = {
  canvas: null, ctx: null, cols: [], fontSize: 13,
  running: false, lastFrame: 0, dpr: 1,
};

function initMiningVisual() {
  if (rain.canvas) return; // bir kez kur
  rain.canvas = $('mine-canvas');
  rain.ctx = rain.canvas.getContext('2d');
  resizeRain();
  window.addEventListener('resize', resizeRain);
  rain.running = true;
  requestAnimationFrame(rainFrame);
}

function resizeRain() {
  if (!rain.canvas) return;
  const rect = rain.canvas.parentElement.getBoundingClientRect();
  rain.dpr = Math.min(window.devicePixelRatio || 1, 2);
  rain.canvas.width = Math.max(1, Math.floor(rect.width * rain.dpr));
  rain.canvas.height = Math.max(1, Math.floor(rect.height * rain.dpr));
  const colCount = Math.floor(rect.width / (rain.fontSize * 0.9));
  rain.cols = Array.from({ length: colCount }, () => ({
    y: Math.random() * rect.height,
    speed: 0.6 + Math.random() * 1.8,
    glow: Math.random() < 0.12,
  }));
}

function rainFrame(ts) {
  if (!rain.running) return;
  requestAnimationFrame(rainFrame);
  if (ts - rain.lastFrame < 33) return; // ~30 fps yeter
  rain.lastFrame = ts;

  const { ctx, canvas, dpr, fontSize } = rain;
  const w = canvas.width / dpr, h = canvas.height / dpr;
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0);

  // İz bırakan karartma — akış hissinin sırrı
  ctx.fillStyle = mining ? 'rgba(3, 6, 9, 0.22)' : 'rgba(3, 6, 9, 0.14)';
  ctx.fillRect(0, 0, w, h);

  ctx.font = `${fontSize}px ui-monospace, Menlo, monospace`;
  const step = fontSize * 0.9;

  for (let i = 0; i < rain.cols.length; i++) {
    const col = rain.cols[i];
    // Kapalıyken sütunların çoğu uyur — seyrek, sakin bir akış
    if (!mining && i % 4 !== 0) continue;

    const ch = HEX_CHARS[(Math.random() * 16) | 0];
    const x = i * step;

    if (mining && col.glow) {
      ctx.fillStyle = 'rgba(46, 242, 154, 0.85)';
    } else if (mining) {
      ctx.fillStyle = 'rgba(46, 242, 154, 0.32)';
    } else {
      ctx.fillStyle = 'rgba(56, 100, 130, 0.18)';
    }
    ctx.fillText(ch, x, col.y);

    col.y += col.speed * (mining ? 4.2 : 1.2);
    if (col.y > h + 20) {
      col.y = -10 - Math.random() * 40;
      col.speed = 0.6 + Math.random() * 1.8;
      col.glow = Math.random() < (mining ? 0.16 : 0.06);
    }
  }
}

// Sahte-gerçek hash şeridi: gerçek hash'i her saniye rastgele hex ile
// harmanlayarak "şu an denenen aday" hissi verir. Gerçek son blok hash'i
// geldiğinde onun etrafında döner.
let tickerTimer = null;
let lastKnownHash = '';

function startTicker() {
  if (tickerTimer) return;
  tickerTimer = setInterval(() => {
    const el = $('hash-ticker');
    if (!mining) return;
    let head = '';
    for (let i = 0; i < 10; i++) head += HEX_CHARS[(Math.random() * 16) | 0];
    const base = lastKnownHash || ''.padEnd(24, '0');
    el.textContent = `${t('mine.attempt')} ▸ ${head}${base.slice(10, 54)}…`;
  }, 140);
}

function stopTickerText() {
  const el = $('hash-ticker');
  el.classList.remove('on');
  el.textContent = lastKnownHash
    ? `${t('mine.lastBlock')} ▸ ${lastKnownHash.slice(0, 56)}…`
    : t('mine.readyTicker');
}

// Blok bulma flaşı
let flashTimer = null;
function blockFound(height) {
  const flash = $('block-flash');
  $('block-flash-title').textContent = t('mine.found', { h: height });
  $('block-flash-sub').textContent = t('mine.reward');
  // Animasyonu yeniden tetiklemek için elementi tazele
  flash.classList.remove('hidden');
  flash.style.animation = 'none';
  void flash.offsetWidth; // reflow — animasyon sıfırlansın
  flash.style.animation = '';
  clearTimeout(flashTimer);
  flashTimer = setTimeout(() => hide(flash), 2600);
}

// ---------- Ana ekran: durum ve bakiye ----------

let prevHeight = null;
let prevMyBlocks = null;
let prevAttempts = null;
let prevAttemptsAt = 0;
let sessionStart = null;
let sessionBlocks = 0;
let lastBlockAt = null; // son blok zamanı (ms, yerel saat değil node saati)

function setMineUIState(on) {
  $('mine-led').className = on ? 'led on' : 'led';
  const stateEl = $('mine-state');
  stateEl.textContent = on ? t('mine.mining') : t('mine.idle');
  stateEl.className = on ? 'mine-state mono on' : 'mine-state mono';
  $('scanline').className = on ? 'scanline on' : 'scanline';
  $('hash-ticker').classList.toggle('on', on);
  if (on) startTicker(); else stopTickerText();
}

async function refreshStatus() {
  try {
    const st = await window.flatun.mineStatus();
    if (!st.ok || !st.json) throw new Error('node yanıt vermedi');
    const j = st.json;

    $('node-status').textContent = t('status.sync', { h: fmt.format(j.height) });
    $('node-status').className = 'badge ok';

    const wasMining = mining;
    mining = !!j.mining;
    const btn = $('mine-toggle');
    btn.disabled = false;
    btn.textContent = mining ? t('mine.stop') : t('mine.start');
    btn.className = mining ? 'primary stop' : 'primary';
    $('mine-info').textContent = mining ? t('mine.info') : '';
    if (wasMining !== mining) setMineUIState(mining);

    // --- İstatistikler ---
    $('st-height').textContent = fmt.format(j.height);
    $('st-difficulty').textContent = '0'.repeat(Math.max(0, j.difficulty)) + '…';
    $('st-pool').textContent = fmt.format(j.pool);
    $('st-myblocks').textContent = fmt.format(j.blocks_by_me ?? 0);
    const rewardFlatun = (j.reward_by_me ?? 0) / 1e8;
    $('st-myreward').textContent = fmt.format(rewardFlatun);

    // Hash hızı: iki yoklama arasındaki deneme farkından hesaplanır
    const now = Date.now();
    if (prevAttempts !== null && j.hash_attempts >= prevAttempts && now > prevAttemptsAt) {
      const rate = ((j.hash_attempts - prevAttempts) * 1000) / (now - prevAttemptsAt);
      $('st-attempts').textContent = rate > 0.5
        ? `${fmt.format(Math.round(rate))}${t('mine.perSec')}`
        : fmt.format(j.hash_attempts);
    } else {
      $('st-attempts').textContent = fmt.format(j.hash_attempts ?? 0);
    }
    prevAttempts = j.hash_attempts ?? 0;
    prevAttemptsAt = now;

    if (j.last_block_hash) lastKnownHash = j.last_block_hash;
    if (!mining) stopTickerText();

    // Blok döngüsü ilerlemesi (~60 sn hedef); node saati ns cinsinden
    if (j.last_block_time) {
      lastBlockAt = j.last_block_time / 1e6;
      const elapsed = Math.max(0, (Date.now() - lastBlockAt) / 1000);
      const pct = Math.min(100, (elapsed / 60) * 100);
      $('cycle-fill').style.width = pct + '%';
      $('cycle-label').textContent = mining
        ? t('mine.cycle', { s: Math.min(60, Math.round(elapsed)) })
        : t('mine.lastAgo', { ago: formatAgo(elapsed) });
    }

    // Yeni blok tespiti + "kazdığın blok" artışı
    if (prevHeight !== null && j.height > prevHeight && mining) {
      const mineIncreased = prevMyBlocks !== null && (j.blocks_by_me ?? 0) > prevMyBlocks;
      if (mineIncreased) {
        sessionBlocks += (j.blocks_by_me - prevMyBlocks);
        blockFound(j.height);
        refreshWallet(); // ödül bakiyeye yansısın
      }
    }
    prevHeight = j.height;
    prevMyBlocks = j.blocks_by_me ?? 0;

    // Oturum satırı
    if (mining && sessionStart === null) { sessionStart = Date.now(); sessionBlocks = 0; }
    if (!mining) sessionStart = null;
    if (mining && sessionStart) {
      const mins = Math.floor((Date.now() - sessionStart) / 60000);
      $('mine-session').textContent =
        t('mine.session', { m: mins, b: sessionBlocks }) +
        (j.busy ? t('mine.busyCore') : '');
    } else {
      $('mine-session').textContent = '';
    }
  } catch {
    $('node-status').textContent = t('status.starting');
    $('node-status').className = 'badge';
    $('hash-ticker').textContent = t('mine.waitingNode');
  }
}

function formatAgo(sec) {
  if (sec < 90) return t('mine.agoSec', { n: Math.round(sec) });
  if (sec < 5400) return t('mine.agoMin', { n: Math.round(sec / 60) });
  return t('mine.agoHour', { n: Math.round(sec / 3600) });
}

async function refreshWallet() {
  try {
    const info = await window.flatun.walletInfo();
    if (info.ok && info.json) $('address').textContent = info.json.blockchain_address;

    const bal = await window.flatun.balance();
    if (bal.ok && bal.json) {
      lastBalanceText = bal.json.amount ?? '0';
      lastBalanceUnits = bal.json.amount_units ?? null;
      $('balance').textContent = lastBalanceText;
    }
  } catch { /* node henüz hazır değil */ }
}

$('mine-toggle').addEventListener('click', async () => {
  $('mine-toggle').disabled = true;
  try {
    if (mining) await window.flatun.mineStop();
    else await window.flatun.mineStart();
  } catch { /* node yanıt vermedi — durum yenilemesi butonu düzeltir */ }
  await refreshStatus();
});

$('copy-addr').addEventListener('click', async () => {
  const addr = $('address').textContent;
  if (!addr || addr === '—') return;
  try {
    await navigator.clipboard.writeText(addr);
    $('copy-addr').textContent = t('main.copied');
    setTimeout(() => { $('copy-addr').textContent = t('main.copy'); }, 1500);
  } catch { /* pano erişimi yoksa sessiz geç */ }
});

// ---------- Para gönderimi: onay modalı ----------

let pendingSend = null; // { to, amount }

$('send-btn').addEventListener('click', () => {
  const to = $('send-to').value.trim();
  const amount = $('send-amount').value.trim();
  const out = $('send-result');
  out.textContent = '';

  if (!to || !amount) { out.textContent = t('send.needBoth'); return; }
  const parsed = Number(amount.replace(',', '.'));
  if (!Number.isFinite(parsed) || parsed <= 0) {
    out.textContent = t('send.badAmount');
    return;
  }

  // Onay modalını doldur (asıl doğrulama Go tarafında: adres checksum,
  // tutar çözümleme, bakiye kontrolü — burası son bir insan kontrolü)
  pendingSend = { to, amount: amount.replace(',', '.') };
  $('cf-to').textContent = to;
  $('cf-amount').textContent = `${pendingSend.amount} FLATUN`;
  $('cf-balance').textContent = lastBalanceText !== null ? `${lastBalanceText} FLATUN` : '—';

  const insufficient = lastBalanceUnits !== null &&
    Math.round(parsed * 1e8) > lastBalanceUnits;
  if (insufficient) show($('cf-warn')); else hide($('cf-warn'));

  $('cf-send').disabled = false;
  show($('confirm-modal'));
});

$('cf-cancel').addEventListener('click', () => {
  pendingSend = null;
  hide($('confirm-modal'));
});

$('cf-send').addEventListener('click', async () => {
  if (!pendingSend) return;
  const { to, amount } = pendingSend;
  const out = $('send-result');

  $('cf-send').disabled = true;
  out.textContent = t('send.sending');
  let res;
  try {
    res = await window.flatun.send(to, amount);
  } catch {
    // node yanıt vermedi/zaman aşımı — modal kilitli kalmasın
    res = { ok: false, json: { message: t('send.noNode') } };
  }
  $('cf-send').disabled = false;
  pendingSend = null;
  hide($('confirm-modal'));

  if (res.ok) {
    out.textContent = t('send.sent');
    $('send-to').value = ''; $('send-amount').value = '';
  } else {
    const msg = res.json?.message;
    const nice = msg === 'invalid recipient'
      ? t('send.badRecipient')
      : msg === 'invalid amount' ? t('send.badAmountShort') : (msg || t('send.unknown'));
    out.textContent = t('send.failPrefix') + nice;
  }
  refreshWallet();
});

// ---------- Yedekleme modalı ----------

$('banner-action').addEventListener('click', async () => {
  hide($('backup-error'));
  let res;
  try {
    res = await window.flatun.revealMnemonic();
  } catch {
    res = { ok: false, message: t('bk.readFailRetry') };
  }
  if (!res.ok) {
    $('backup-error').textContent = errText(res, 'bk.readFail');
    show($('backup-error'));
    $('backup-words').textContent = '';
    show($('backup-modal'));
    return;
  }
  renderWords($('backup-words'), res.words);
  show($('backup-modal'));
});

$('backup-close').addEventListener('click', () => {
  $('backup-words').textContent = ''; // kelimeleri DOM'da bırakma
  hide($('backup-modal'));
});

$('backup-done').addEventListener('click', async () => {
  let res;
  try {
    res = await window.flatun.backupDone();
  } catch {
    res = { ok: false };
  }
  $('backup-words').textContent = '';
  hide($('backup-modal'));
  if (res.ok) {
    const st = await window.flatun.appState();
    lastAppState = st;
    updateBanner(st);
  }
});

// ---------- Günlük ----------

window.flatun.onLog((line) => {
  const log = $('log');
  log.textContent += line + '\n';
  log.scrollTop = log.scrollHeight;
});

populateLangSelects();
applyStaticI18n();
boot();
