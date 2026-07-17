// FlatunChain renderer — yalnızca window.flatun köprüsünü kullanır
const $ = (id) => document.getElementById(id);

let mining = false;

async function refreshStatus() {
  try {
    const st = await window.flatun.mineStatus();
    if (st.ok && st.json) {
      $('node-status').textContent = `çalışıyor · blok #${st.json.height}`;
      $('node-status').className = 'badge ok';
      mining = st.json.mining;
      const btn = $('mine-toggle');
      btn.disabled = false;
      btn.textContent = mining ? 'Madenciliği Durdur' : 'Madenciliği Başlat';
      btn.className = mining ? 'primary stop' : 'primary';
      $('mine-info').textContent = mining ? 'Kazıyor… her blok ödül getirir.' : '';
    } else {
      throw new Error('node yanıt vermedi');
    }
  } catch {
    $('node-status').textContent = 'node başlatılıyor…';
    $('node-status').className = 'badge';
  }
}

async function refreshWallet() {
  try {
    const info = await window.flatun.walletInfo();
    if (info.ok && info.json) $('address').textContent = info.json.blockchain_address;

    const bal = await window.flatun.balance();
    if (bal.ok && bal.json) $('balance').textContent = bal.json.amount ?? '0';
  } catch { /* node henüz hazır değil */ }
}

$('mine-toggle').addEventListener('click', async () => {
  $('mine-toggle').disabled = true;
  if (mining) await window.flatun.mineStop();
  else await window.flatun.mineStart();
  await refreshStatus();
});

$('send-btn').addEventListener('click', async () => {
  const to = $('send-to').value.trim();
  const amount = $('send-amount').value.trim();
  const out = $('send-result');
  if (!to || !amount) { out.textContent = 'Alıcı ve miktar gerekli.'; return; }

  $('send-btn').disabled = true;
  out.textContent = 'Gönderiliyor…';
  const res = await window.flatun.send(to, amount);
  $('send-btn').disabled = false;
  if (res.ok) {
    out.textContent = '✓ Gönderildi — bir sonraki blokta işlenecek.';
    $('send-to').value = ''; $('send-amount').value = '';
  } else {
    out.textContent = '✗ Gönderilemedi: ' + (res.json?.message || 'bilinmeyen hata');
  }
  refreshWallet();
});

window.flatun.onLog((line) => {
  const log = $('log');
  log.textContent += line + '\n';
  log.scrollTop = log.scrollHeight;
});

setInterval(refreshStatus, 3000);
setInterval(refreshWallet, 5000);
refreshStatus();
refreshWallet();
