// REST helpers and a self-healing WebSocket.

export async function api(method, path, body) {
  const res = await fetch(path, {
    method,
    headers: body ? { 'Content-Type': 'application/json' } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(data.error || `Request failed (${res.status})`);
  return data;
}

// Close codes the server uses to say "do not retry".
const FATAL = new Set([4001, 4003, 4004]);

export function connect(params, { onState, onError, onStatus, onFatal }) {
  const proto = location.protocol === 'https:' ? 'wss' : 'ws';
  const url = `${proto}://${location.host}/ws?${new URLSearchParams(params)}`;
  let ws = null;
  let attempt = 0;
  let closed = false;
  const queue = [];

  function open() {
    ws = new WebSocket(url);
    ws.onopen = () => {
      attempt = 0;
      onStatus?.('online');
      while (queue.length) ws.send(queue.shift());
    };
    ws.onmessage = (ev) => {
      let msg;
      try { msg = JSON.parse(ev.data); } catch { return; }
      if (msg.type === 'state') onState(msg.state);
      else if (msg.type === 'error') onError?.(msg.message);
    };
    ws.onclose = (ev) => {
      if (closed) return;
      if (FATAL.has(ev.code)) {
        closed = true;
        onFatal?.({ code: ev.code, reason: ev.reason });
        return;
      }
      onStatus?.('offline');
      attempt += 1;
      setTimeout(open, Math.min(500 * 2 ** attempt, 8000));
    };
  }
  open();

  return {
    send(type, action, arg = {}) {
      const data = JSON.stringify({ type, action, arg });
      if (ws && ws.readyState === WebSocket.OPEN) ws.send(data);
      else queue.push(data);
    },
    close() { closed = true; ws?.close(); },
  };
}
