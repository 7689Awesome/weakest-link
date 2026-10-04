// Phone views. Contestants and the chaser share a shell but see different
// controls: the chaser makes offers and plays the head-to-head from the other
// side; contestants choose offers, answer, vote for a set and buzz.
import { html, money } from '../core/dom.js';
import {
  boardHTML, trackHTML, optionsHTML, rosterChips, clockEl, frozenEl, resultHTML,
  nameOf, chaserOf, currentOf, stepsToHome,
} from './components.js';

export const offerDraft = { lower: null, higher: null, forKey: '' };

const wait = (title, sub = '') => html`<div class="wait"><h2>${title}</h2>${sub ? html`<p>${sub}</p>` : ''}</div>`;

function offerChoice(s) {
  const o = s.offers;
  const btn = (key, label, amount) => html`
    <button class="offer-btn ${key}" data-send="act" data-action="chooseOffer" data-arg='${JSON.stringify({ choice: key })}'>
      <span class="l">${label}</span><b>${money(amount)}</b><small>${stepsToHome(s, key)} correct to get home</small>
    </button>`;
  return html`<div class="offer-stack">
    ${btn('higher', 'Higher offer · closer to the chaser', o.higher)}
    ${btn('middle', 'Your cash builder', o.middle)}
    ${btn('lower', 'Lower offer · further away', o.lower)}
  </div>`;
}

function offerForm(s) {
  const key = `${s.currentId}`;
  if (offerDraft.forKey !== key) {
    offerDraft.forKey = key;
    offerDraft.lower = s.offers.suggestLower;
    offerDraft.higher = s.offers.suggestHigher;
  }
  return html`
    <div class="offer-form">
      <label>Higher offer<input type="number" inputmode="numeric" step="1000" data-draft="higher" value="${offerDraft.higher}"></label>
      <div class="mid">${nameOf(s, s.currentId)}’s cash builder <b>${money(s.offers.middle)}</b></div>
      <label>Lower offer<input type="number" inputmode="numeric" step="1000" data-draft="lower" value="${offerDraft.lower}"></label>
      <button class="btn primary block" data-offers="act">Make the offers</button>
    </div>`;
}

function h2hSide(s, me) {
  const h = s.h2h;
  const p = currentOf(s);
  const c = chaserOf(s);
  const playing = me.isCurrent || me.isChaser;
  switch (s.phase) {
    case 'h2h_ready':
      return html`${boardHTML(s)}${wait('Get ready', me.isCurrent ? `You’re playing for ${money(s.offers.amount)}.` : me.isChaser ? `${p?.name} plays for ${money(s.offers.amount)}.` : `Help ${p?.name} bring it home.`)}`;
    case 'h2h_question': {
      const locked = h.myPick != null;
      const otherLocked = me.isChaser ? h.playerLocked : h.chaserLocked;
      return html`
        ${boardHTML(s)}
        <h2 class="q-text">${h.question}</h2>
        ${optionsHTML(s, { buttons: playing, mine: h.myPick, locked })}
        <p class="status">${!playing ? `Advise ${p?.name}…`
          : locked ? (otherLocked ? 'Locked in. Revealing…' : 'Locked in. Waiting for the other side…')
          : otherLocked ? html`Other side is in. <span class="clock" data-ends="${h.deadlineAt}" data-fmt="sec" data-warn="no">5</span>s left` : 'Choose your answer'}</p>`;
    }
    case 'h2h_reveal':
      return html`${boardHTML(s)}<h2 class="q-text">${h.question}</h2>${optionsHTML(s)}
        <p class="status">${h.reveal.playerRight ? `${p?.name} was right.` : `${p?.name} missed.`} ${h.reveal.chaserRight ? `${c?.name} was right.` : `${c?.name} missed.`}</p>`;
    case 'h2h_over':
      return html`${boardHTML(s)}${h.outcome === 'home'
        ? html`<div class="banner home"><h2>Home!</h2><p>${money(s.offers.amount)} to the team</p></div>`
        : html`<div class="banner caught"><h2>Caught</h2><p>${p?.name} is out</p></div>`}`;
    default:
      return '';
  }
}

function finalSide(s, me) {
  const f = s.final;
  const c = chaserOf(s);
  switch (s.phase) {
    case 'final_pick':
      if (me.isChaser) return wait('The team is choosing', 'You’ll get the other set.');
      if (!me.finalist) return wait('You were caught', 'Cheer on your team.');
      return html`<div class="wait"><h2>Pick a question set</h2>
        <div class="set-pick">
          ${['A', 'B'].map((x) => html`<button class="set-btn ${me.vote === x ? 'chosen' : ''}" data-send="act" data-action="pickSet" data-arg='${JSON.stringify({ set: x })}'>Set ${x}</button>`)}
        </div><p>${me.vote ? `You voted for set ${me.vote}.` : 'Majority wins.'}</p></div>`;
    case 'final_team_ready':
      return wait(me.isChaser ? 'The team is about to play' : `Team plays set ${f.teamSet}`, me.isChaser ? `Target will be ${f.target}.` : 'Buzz in when you know it.');
    case 'final_team': {
      const mine = f.buzzedBy === me.id;
      const other = f.buzzedBy && !mine;
      return html`
        <div class="statbar">${clockEl(f.endsAt, { cls: 'lg' })}<span><b>${f.teamCorrect}</b> correct</span></div>
        <h2 class="q-text">${f.question}</h2>
        ${me.finalist ? html`<button class="buzzer ${mine ? 'won' : other ? 'lost' : ''}" data-send="act" data-action="buzz" ${f.buzzedBy ? 'disabled' : ''}>
            ${mine ? 'You’re in: answer now' : other ? `${nameOf(s, f.buzzedBy)} is answering` : 'BUZZ'}</button>`
          : html`<p class="status">${f.buzzedBy ? `${nameOf(s, f.buzzedBy)} buzzed in` : 'Buzzers are live'}</p>`}
        ${trackHTML(f)}`;
    }
    case 'final_team_done':
      return html`${wait(`${f.teamCorrect} + ${f.head} head start`, `${c?.name} needs ${f.target}.`)}${trackHTML(f)}`;
    case 'final_chaser':
    case 'final_push': {
      const push = s.phase === 'final_push';
      const mine = f.buzzedBy === me.id;
      const other = f.buzzedBy && !mine;
      return html`
        <div class="statbar ${push ? 'stopped' : ''}">${push ? frozenEl(f.remainingMs) : clockEl(f.endsAt, { cls: 'lg' })}<span><b>${f.chaserCorrect}</b> of ${f.target}</span></div>
        <h2 class="q-text">${f.question}</h2>
        ${push && me.finalist
          ? html`<button class="buzzer ${mine ? 'won' : other ? 'lost' : ''}" data-send="act" data-action="buzz" ${f.buzzedBy ? 'disabled' : ''}>
              ${mine ? 'You’re in: answer for the team' : other ? `${nameOf(s, f.buzzedBy)} is answering` : 'BUZZ to push back'}</button>`
          : html`<p class="status">${push ? 'Clock stopped. The team can push the chaser back.' : me.isChaser ? 'Say your answer out loud.' : `${c?.name} is answering…`}</p>`}
        ${trackHTML(f)}`;
    }
    case 'gameover':
      return html`${resultHTML(s)}${trackHTML(f)}`;
    default:
      return '';
  }
}

function body(s) {
  const me = s.me;
  const p = currentOf(s);
  const c = chaserOf(s);
  if (!me) return wait('Reconnecting…');

  if (s.phase.startsWith('h2h_')) return h2hSide(s, me);
  if (s.phase.startsWith('final_') || s.phase === 'gameover') return finalSide(s, me);

  switch (s.phase) {
    case 'lobby':
      return html`${wait(me.isChaser ? 'You’re the Chaser' : 'You’re in', 'Waiting for the quizmaster to start.')}${rosterChips(s)}`;
    case 'between':
      return html`${wait(me.isChaser ? 'Next contestant coming up' : 'Who’s next?')}${rosterChips(s)}`;
    case 'cb_ready':
      return wait(me.isCurrent ? 'You’re up' : `${p?.name} is up`, me.isCurrent ? `Cash builder: ${s.cfg.cbSeconds} seconds. Answer out loud.` : 'Cash builder is about to start.');
    case 'cb_playing':
      return html`<div class="statbar">${clockEl(s.cb.endsAt, { fmt: 'sec', cls: 'lg' })}<span><b>${s.cb.correct}</b> correct · ${money(s.cb.total)}</span></div>
        <h2 class="q-text">${me.isCurrent || me.isChaser ? '' : s.cb.question}</h2>
        ${me.isCurrent ? wait('Answer out loud', 'The quizmaster is marking you.') : ''}`;
    case 'cb_done':
      return wait(`${p?.name} built ${money(s.cb.total)}`, me.isChaser ? 'Time to think about your offers.' : '');
    case 'offers_set':
      return me.isChaser ? html`<h2 class="pad-title">Make your offers</h2>${offerForm(s)}`
        : wait(me.isCurrent ? `${c?.name} is deciding…` : `${c?.name} is making an offer`, `${p?.name} has ${money(s.offers.middle)}.`);
    case 'offers_choose':
      return me.isCurrent ? html`<h2 class="pad-title">Your offers</h2>${offerChoice(s)}`
        : html`${wait(me.isChaser ? `${p?.name} is choosing` : `Advise ${p?.name}`, 'The offers are on the big screen.')}
            <p class="status">${money(s.offers.higher)} · ${money(s.offers.middle)} · ${money(s.offers.lower)}</p>`;
    default:
      return '';
  }
}

export function playerView(s, ctx) {
  const me = s.me;
  const role = !me ? '' : me.isChaser ? 'chaser' : me.isCurrent ? 'hotseat' : 'contestant';
  return html`
    <div class="phone ${role}">
      <header class="bar">
        <b>${me?.name ?? ''}</b>
        <span class="pill ${role}">${me?.isChaser ? 'Chaser' : me?.isCurrent ? 'In the hot seat' : me?.finalist ? 'Finalist' : 'Contestant'}</span>
        <span class="code">${s.code}</span>
      </header>
      ${ctx.error ? html`<div class="toast" role="alert">${ctx.error}</div>` : ''}
      ${ctx.offline ? html`<div class="toast" role="status">Reconnecting…</div>` : ''}
      <main>${body(s)}</main>
    </div>`;
}
