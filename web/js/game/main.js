// Entry point. The URL decides the role:
//   /                      landing (host / join / big screen)
//   /?join=CODE            join form for a room
//   /?play=CODE            a player's phone (seat is remembered in localStorage)
//   /?host=CODE[&key=K]    the quizmaster's controller
//   /?screen=CODE          the shared big screen
//   /host   (host.html)    bookmarkable quizmaster entry: new game or resume
//   /screen (screen.html)  bookmarkable TV entry: asks for the code
import { html, $ } from '../core/dom.js';
import { api, connect } from '../core/net.js';
import { syncClock, now, tickClocks } from '../core/clock.js';
import { unlock, play } from '../core/sound.js';
import { screenView, screenSfx } from './screen.js';
import { hostView, hostKey, draft as hostDraft } from './host.js';
import { playerView, offerDraft } from './player.js';

const app = $('#app');
const params = new URLSearchParams(location.search);
const store = {
  get: (k) => { try { return JSON.parse(localStorage.getItem(k)); } catch { return null; } },
  set: (k, v) => { try { localStorage.setItem(k, JSON.stringify(v)); } catch { /* private mode */ } },
  del: (k) => { try { localStorage.removeItem(k); } catch { /* ignore */ } },
};
const clean = (c) => String(c || '').toUpperCase().replace(/[^A-Z]/g, '').slice(0, 4);

let role = null;
let conn = null;
let state = null;
let prev = null;
let lastHTML = '';
let soundOn = false;
const ctx = { error: '', offline: false };
let errTimer = null;

// ---------------------------------------------------------------- landing

function landing(message = '', prefillJoin = '') {
  const lastSeat = Object.keys(localStorage).find((k) => k.startsWith('chase.seat.'));
  const seatCode = lastSeat ? lastSeat.replace('chase.seat.', '') : '';
  app.innerHTML = String(html`
    <div class="landing">
      <header>
        <h1>Chase Night</h1>
        <p>A quiz-show party game. One chaser, up to four players, and a TV.</p>
      </header>
      ${message ? html`<div class="toast" role="alert">${message}</div>` : ''}
      <div class="cards">
        <section class="card">
          <h2>Host a game</h2>
          <p>You’re the quizmaster. You’ll get the controller on this device, and a code for the TV and phones.</p>
          <button class="btn primary block" data-local="create">Create a room</button>
        </section>
        <section class="card">
          <h2>Join a game</h2>
          <form data-form="join">
            <label>Room code<input name="code" value="${prefillJoin}" maxlength="4" autocapitalize="characters" autocomplete="off" required placeholder="ABCD"></label>
            <label>Your name<input name="name" maxlength="16" autocomplete="nickname" required></label>
            <button class="btn primary block">Join</button>
          </form>
          ${seatCode ? html`<p class="hint"><a href="/?play=${seatCode}">Rejoin room ${seatCode}</a></p>` : ''}
        </section>
        <section class="card">
          <h2>Show the big screen</h2>
          <p>Open this on the TV and enter the room code.</p>
          <form data-form="screen">
            <label>Room code<input name="code" maxlength="4" autocapitalize="characters" autocomplete="off" required placeholder="ABCD"></label>
            <button class="btn ghost block">Show on this screen</button>
          </form>
        </section>
      </div>
    </div>`);
}

// Bookmarkable /host page: start a new game, or resume one this device was running.
async function hostLanding(message = '') {
  app.innerHTML = String(html`<div class="landing"><header><h1>Quizmaster</h1><p>Run the game from this device.</p></header>
    ${message ? html`<div class="toast" role="alert">${message}</div>` : ''}
    <div class="cards"><section class="card"><h2>New game</h2>
      <p>Creates a room code for the TV and phones.</p>
      <button class="btn primary block" data-local="create">Start a new game</button></section>
      <section class="card" id="resume" hidden><h2>Resume</h2><ul class="people" id="resume-list"></ul></section></div></div>`);
  const codes = Object.keys(localStorage).filter((k) => k.startsWith('chase.host.')).map((k) => k.slice(11));
  const alive = [];
  for (const code of codes) {
    try {
      const info = await api('GET', `/api/rooms/${code}`);
      if (info.exists) alive.push({ code, phase: info.phase, n: info.playerCount });
      else store.del(`chase.host.${code}`);
    } catch { /* offline: skip */ }
  }
  if (!alive.length) return;
  $('#resume').hidden = false;
  $('#resume-list').innerHTML = alive.map((r) =>
    `<li><b class="code">${r.code}</b><span class="grow"></span><span class="hint">${r.n} player${r.n === 1 ? '' : 's'}</span> <a class="btn small primary" href="/?host=${r.code}">Resume</a></li>`).join('');
}

// Bookmarkable /screen page for the TV: just ask for the code.
function screenLanding() {
  app.innerHTML = String(html`<div class="landing"><header><h1>Big screen</h1><p>Enter the room code shown on the quizmaster’s device.</p></header>
    <section class="card"><form data-form="screen"><label>Room code<input name="code" maxlength="4" autocapitalize="characters" autocomplete="off" required autofocus placeholder="ABCD"></label>
    <button class="btn primary block">Show on this screen</button></form></section></div>`);
}

async function createRoom() {
  try {
    const r = await api('POST', '/api/rooms');
    store.set(`chase.host.${r.roomCode}`, r.hostKey);
    location.assign(`/?host=${r.roomCode}`);
  } catch (e) { (document.body.dataset.entry === 'host' ? hostLanding : landing)(e.message); }
}

async function joinRoom(code, name) {
  code = clean(code);
  try {
    const info = await api('GET', `/api/rooms/${code}`);
    if (!info.exists) throw new Error('That room does not exist. Check the code.');
    const seat = store.get(`chase.seat.${code}`);
    const body = { name };
    if (seat) { body.rejoinId = seat.id; body.rejoinToken = seat.token; }
    const r = await api('POST', `/api/rooms/${code}/players`, body);
    store.set(`chase.seat.${code}`, { id: r.playerId, token: r.playerToken, name });
    location.assign(`/?play=${code}`);
  } catch (e) { landing(e.message, code); }
}

// ---------------------------------------------------------------- game shell

function fatal(msg, withJoinLink = '') {
  conn?.close();
  app.innerHTML = String(html`<div class="landing"><header><h1>Chase Night</h1></header>
    <div class="card center"><h2>${msg}</h2>
    ${withJoinLink ? html`<a class="btn primary" href="/?join=${withJoinLink}">Join again</a>` : html`<a class="btn primary" href="/">Back to start</a>`}</div></div>`);
}

function render() {
  if (!state) return;
  const view = role === 'screen' ? screenView(state)
    : role === 'host' ? hostView(state, ctx)
    : playerView(state, ctx);
  const out = String(view);
  if (out !== lastHTML) {
    const active = document.activeElement;
    const key = active?.dataset?.draft;
    app.innerHTML = out;
    lastHTML = out;
    if (role === 'screen' && !soundOn) {
      app.insertAdjacentHTML('beforeend', '<button class="sound-hint" data-local="unlock">Click to enable sound</button>');
    }
    if (key) app.querySelector(`[data-draft="${key}"]`)?.focus();
  }
  tickClocks(app);
}

function showError(msg) {
  ctx.error = msg;
  render();
  clearTimeout(errTimer);
  errTimer = setTimeout(() => { ctx.error = ''; render(); }, 3500);
}

function start(r, code, params2) {
  role = r;
  document.body.dataset.role = r;
  conn = connect(params2, {
    onState: (s) => {
      syncClock(s.now);
      prev = state;
      state = s;
      if (role === 'screen' && soundOn) screenSfx(prev, s);
      render();
    },
    onError: showError,
    onStatus: (st) => { ctx.offline = st === 'offline'; render(); },
    onFatal: ({ code: c, reason }) => {
      if (role === 'player') {
        if (c === 4001) return fatal('This seat is open on another device.');
        store.del(`chase.seat.${code}`);
        return fatal(reason || 'This room has ended.', c === 4004 ? '' : code);
      }
      return fatal(reason || 'This room has ended.');
    },
  });
}

// ---------------------------------------------------------------- events

const draftFor = () => (role === 'host' ? hostDraft : offerDraft);

document.addEventListener('click', (e) => {
  if (!soundOn && role === 'screen') { unlock(); soundOn = true; $('.sound-hint')?.remove(); }
  const el = e.target.closest('[data-send],[data-local],[data-offers]');
  if (!el || el.disabled) return;

  if (el.dataset.send) {
    let arg = {};
    try { arg = JSON.parse(el.dataset.arg || '{}'); } catch { /* ignore */ }
    conn?.send(el.dataset.send, el.dataset.action, arg);
    if (el.dataset.action === 'lock' || el.dataset.action === 'buzz') navigator.vibrate?.(30);
  } else if (el.dataset.offers) {
    const d = draftFor();
    conn?.send(el.dataset.offers, 'setOffers', { lower: Number(d.lower), higher: Number(d.higher) });
  } else if (el.dataset.local === 'create') createRoom();
  else if (el.dataset.local === 'unlock') { unlock(); soundOn = true; el.remove(); play('start'); }
  else if (el.dataset.local === 'copyJoin') copy(`${location.origin}/?join=${state.code}`, 'Join link copied');
  else if (el.dataset.local === 'copyHost') copy(`${location.origin}/?host=${state.code}&key=${store.get(`chase.host.${state.code}`)}`, 'Controller link copied: keep it private');
});

function copy(text, msg) {
  navigator.clipboard?.writeText(text).then(() => showError(msg), () => showError(text));
}

document.addEventListener('input', (e) => {
  const k = e.target.dataset?.draft;
  if (k) draftFor()[k] = e.target.value;
  if (e.target.name === 'code') e.target.value = clean(e.target.value);
});

document.addEventListener('submit', (e) => {
  const form = e.target.closest('[data-form]');
  if (!form) return;
  e.preventDefault();
  const data = new FormData(form);
  if (form.dataset.form === 'join') joinRoom(data.get('code'), String(data.get('name')).trim());
  if (form.dataset.form === 'screen') location.assign(`/?screen=${clean(data.get('code'))}`);
});

document.addEventListener('keydown', (e) => {
  if (role !== 'host' || !state || e.target.matches('input, textarea') || e.repeat) return;
  const m = hostKey(state, e.key);
  if (m) { e.preventDefault(); conn?.send('control', m.action, m.arg); }
});

// Live clocks + the last-ten-seconds tick on the big screen.
let lastSec = -1;
setInterval(() => {
  tickClocks(app);
  if (role !== 'screen' || !soundOn || !state) return;
  const end = state.cb?.endsAt && state.phase === 'cb_playing' ? state.cb.endsAt
    : state.final?.running ? state.final.endsAt : 0;
  if (!end) { lastSec = -1; return; }
  const sec = Math.ceil((end - now()) / 1000);
  if (sec !== lastSec && sec > 0 && sec <= 10) play('tick');
  lastSec = sec;
}, 200);

// ---------------------------------------------------------------- boot

(function boot() {
  const host = clean(params.get('host'));
  const screen = clean(params.get('screen'));
  const playCode = clean(params.get('play'));
  const join = clean(params.get('join'));

  if (host) {
    const key = params.get('key') || store.get(`chase.host.${host}`);
    if (!key) return fatal('This device doesn’t have the controller key for that room.');
    store.set(`chase.host.${host}`, key);
    if (params.get('key')) history.replaceState(null, '', `/?host=${host}`);
    return start('host', host, { role: 'host', code: host, key });
  }
  if (screen) return start('screen', screen, { role: 'screen', code: screen });
  if (playCode) {
    const seat = store.get(`chase.seat.${playCode}`);
    if (!seat) return location.replace(`/?join=${playCode}`);
    return start('player', playCode, { role: 'player', code: playCode, playerId: seat.id, token: seat.token });
  }
  if (document.body.dataset.entry === 'host') return hostLanding();
  if (document.body.dataset.entry === 'screen') return screenLanding();
  return landing('', join);
}());


