// Clients count down locally against absolute timestamps from the server, so
// the server never has to send ticks. offset corrects for clock skew between
// this device and the server.

let offset = 0;
export const syncClock = (serverNow) => { offset = serverNow - Date.now(); };
export const now = () => Date.now() + offset;

function fmt(ms, mode) {
  ms = Math.max(0, ms);
  if (mode === 'sec') return String(Math.ceil(ms / 1000));
  const total = Math.ceil(ms / 1000);
  const m = Math.floor(total / 60);
  const s = String(total % 60).padStart(2, '0');
  return `${m}:${s}`;
}

// Elements with data-ends="<epoch ms>" count down live; data-frozen="<ms>" is static.
export function tickClocks(root = document) {
  root.querySelectorAll('[data-ends]').forEach((el) => {
    const ms = Number(el.dataset.ends) - now();
    el.textContent = fmt(ms, el.dataset.fmt);
    el.classList.toggle('low', ms < 10000 && el.dataset.warn !== 'no');
  });
  root.querySelectorAll('[data-frozen]').forEach((el) => {
    el.textContent = fmt(Number(el.dataset.frozen), el.dataset.fmt);
  });
}
