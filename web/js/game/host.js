// The quizmaster's controller: the only role that sees answers and can move the game.
import { html, money, jarg } from '../core/dom.js';
import {
  boardHTML, trackHTML, optionsHTML, rosterChips, clockEl, frozenEl, resultHTML,
  nameOf, chaserOf, currentOf, contestants, stepsToHome,
} from './components.js';

// Offer drafts survive re-renders so typing isn't wiped by an incoming state.
export const draft = { lower: null, higher: null, forKey: '' };

const ctl = (action, label, { cls = 'primary', arg = {}, disabled = false, key = '' } = {}) =>
  html`<button class="btn ${cls}" data-send="control" data-action="${action}" data-arg="${jarg(arg)}" ${disabled ? 'disabled' : ''}>${label}${key ? html`<kbd>${key}</kbd>` : ''}</button>`;

const markButtons = (action, rightLabel, wrongLabel) => html`
  <div class="mark">
    ${ctl(action, rightLabel, { cls: 'right', arg: { correct: true }, key: 'Y' })}
    ${ctl(action, wrongLabel, { cls: 'wrong', arg: { correct: false }, key: 'N' })}
  </div>`;

const qCard = (label, q, a) => html`
  <section class="qcard">
    <p class="hint">${label}</p>
    <h2 class="q-text">${q}</h2>
    <p class="answer"><span>Answer</span> <b>${a}</b></p>
  </section>`;

function offerForm(s) {
  const key = `${s.currentId}`;
  if (draft.forKey !== key) {
    draft.forKey = key;
    draft.lower = s.offers.suggestLower;
    draft.higher = s.offers.suggestHigher;
  }
  return html`
    <div class="offer-form">
      <label>Higher offer<input type="number" inputmode="numeric" step="1000" data-draft="higher" value="${draft.higher}"></label>
      <div class="mid">Cash builder total <b>${money(s.offers.middle)}</b></div>
      <label>Lower offer<input type="number" inputmode="numeric" step="1000" data-draft="lower" value="${draft.lower}"></label>
      <button class="btn primary" data-offers="control">Send offers</button>
    </div>`;
}

function body(s) {
  const p = currentOf(s);
  const c = chaserOf(s);
  switch (s.phase) {
    case 'lobby': {
      const n = contestants(s).length;
      return html`
        <section class="card">
          <h2>Players</h2>
          ${s.players.length ? html`<ul class="people">${s.players.map((x) => html`
            <li class="${x.isChaser ? 'is-chaser' : ''}">
              <span class="dot ${x.connected ? 'on' : ''}"></span><b>${x.name}</b>
              <span class="grow"></span>
              <button class="btn small ${x.isChaser ? 'wrong' : ''}" data-send="control" data-action="setChaser" data-arg="${jarg({ id: x.id })}">${x.isChaser ? 'Chaser ✓' : 'Make chaser'}</button>
              <button class="btn small ghost" data-send="control" data-action="kick" data-arg="${jarg({ id: x.id })}" aria-label="Remove ${x.name}">Remove</button>
            </li>`)}</ul>` : html`<p class="hint">Waiting for players. Share the code or link above.</p>`}
        </section>
        ${ctl('startGame', c ? `Start game (${n} contestant${n === 1 ? '' : 's'})` : 'Pick a chaser to start', { disabled: !c || n < 1 })}`;
    }

    case 'between': {
      const queue = s.players.filter((x) => x.queued);
      return html`
        <section class="card"><h2>${queue.length ? 'Who plays next?' : ''}</h2>
          ${queue.map((x) => ctl('startPlayer', `${x.name}`, { arg: { id: x.id }, cls: 'primary block' }))}
        </section>
        <section class="card"><h2>So far</h2>${rosterChips(s)}</section>`;
    }

    case 'cb_ready':
      return html`<section class="card center"><h2>${p?.name}</h2><p>Cash builder. ${s.cfg.cbSeconds} seconds.</p>
        ${ctl('startCashBuilder', 'Start the clock')}</section>`;

    case 'cb_playing':
      return html`
        <div class="statbar">${clockEl(s.cb.endsAt, { fmt: 'sec', cls: 'lg' })}<span><b>${s.cb.correct}</b> correct · ${money(s.cb.total)}</span></div>
        ${qCard(p?.name ?? '', s.cb.question, s.cb.answer)}
        ${markButtons('markCB', 'Correct', 'Wrong / pass')}`;

    case 'cb_done':
      return html`<section class="card center"><h2>${p?.name} built ${money(s.cb.total)}</h2><p>${s.cb.correct} correct</p>
        ${ctl('toOffers', `Hand over to ${c?.name} for offers`)}</section>`;

    case 'offers_set':
      return html`<section class="card"><h2>Offers for ${p?.name}</h2>
        <p class="hint">${c?.name} can set these from their phone. You can also send them from here.</p>${offerForm(s)}</section>`;

    case 'offers_choose':
      return html`<section class="card"><h2>${p?.name} is choosing</h2>
        <p class="hint">Or choose for them:</p>
        <div class="mark3">
          ${ctl('chooseOffer', `Higher ${money(s.offers.higher)}`, { arg: { choice: 'higher' }, cls: 'ghost' })}
          ${ctl('chooseOffer', `Cash builder ${money(s.offers.middle)}`, { arg: { choice: 'middle' }, cls: 'ghost' })}
          ${ctl('chooseOffer', `Lower ${money(s.offers.lower)}`, { arg: { choice: 'lower' }, cls: 'ghost' })}
        </div></section>`;

    case 'h2h_ready':
      return html`<section class="card center"><h2>${p?.name} plays for ${money(s.offers.amount)}</h2>
        <p>${stepsToHome(s, s.offers.choice)} correct answers to get home.</p>
        ${ctl('startH2H', 'Start head-to-head')}</section>
        <div class="board-sm">${boardHTML(s)}</div>`;

    case 'h2h_question':
    case 'h2h_reveal':
      return html`
        <section class="qcard"><p class="hint">Question ${s.h2h.qNum} · ${p?.name} v ${c?.name}</p>
          <h2 class="q-text">${s.h2h.question}</h2>${optionsHTML(s)}
          <p class="locks-line">
            <span class="${s.h2h.playerLocked ? 'ok' : ''}">${p?.name}: ${s.h2h.playerLocked ? 'locked in' : 'thinking'}</span>
            <span class="${s.h2h.chaserLocked ? 'ok' : ''}">${c?.name}: ${s.h2h.chaserLocked ? 'locked in' : 'thinking'}</span>
            ${s.phase === 'h2h_question' && s.h2h.deadlineAt ? html`<span>locks out in <span class="clock" data-ends="${s.h2h.deadlineAt}" data-fmt="sec" data-warn="no">5</span>s</span>` : ''}
          </p></section>
        ${s.phase === 'h2h_question' ? ctl('lockOut', 'Lock out & reveal now', { cls: 'ghost' }) : html`<p class="hint center">Moving on automatically…</p>`}
        <div class="board-sm">${boardHTML(s)}</div>`;

    case 'h2h_over':
      return html`<section class="card center">
        <h2>${s.h2h.outcome === 'home' ? `${p?.name} is home with ${money(s.offers.amount)}` : `${p?.name} was caught`}</h2>
        ${ctl('afterH2H', s.players.some((x) => x.queued) ? 'Next player' : 'On to the final chase')}</section>
        <div class="board-sm">${boardHTML(s)}</div>`;

    case 'final_pick':
      return html`<section class="card"><h2>Question sets</h2>
        <ul class="people">${s.final.finalists.map((id) => html`<li><b>${nameOf(s, id)}</b><span class="grow"></span><span class="pill">${s.final.votes?.[id] ? `Set ${s.final.votes[id]}` : 'not voted'}</span></li>`)}</ul>
        <p class="hint">Majority wins; a tie is settled at random. The chaser plays the other set.</p>
        ${ctl('confirmSets', 'Lock in sets')}</section>`;

    case 'final_team_ready':
      return html`<section class="card center"><h2>Team plays set ${s.final.teamSet}</h2>
        <p>Head start ${s.final.head}. ${s.cfg.finalSeconds} seconds on the clock.</p>
        ${ctl('startFinalTeam', 'Start the team’s clock')}</section>`;

    case 'final_team':
      return html`
        <div class="statbar">${clockEl(s.final.endsAt, { cls: 'lg' })}<span><b>${s.final.teamCorrect}</b> correct</span></div>
        <div class="buzz-line ${s.final.buzzedBy ? 'on' : ''}">${s.final.buzzedBy ? html`<b>${nameOf(s, s.final.buzzedBy)}</b> buzzed` : 'No buzz yet'}</div>
        ${qCard('Team question', s.final.question, s.final.answer)}
        ${markButtons('markFinal', 'Correct', 'Wrong / pass')}`;

    case 'final_team_done':
      return html`<section class="card center"><h2>${s.final.teamCorrect} correct + ${s.final.head} head start</h2>
        <p>${c?.name} needs ${s.final.target} on set ${s.final.chaserSet}.</p>${trackHTML(s.final)}
        ${ctl('startChaserRound', 'Start the chaser’s clock')}</section>`;

    case 'final_chaser':
      return html`
        <div class="statbar">${clockEl(s.final.endsAt, { cls: 'lg' })}<span><b>${s.final.chaserCorrect}</b> of ${s.final.target}</span></div>
        ${qCard(`${c?.name}’s question`, s.final.question, s.final.answer)}
        ${markButtons('markFinal', 'Correct', 'Wrong (stops clock)')}
        ${trackHTML(s.final)}`;

    case 'final_push':
      return html`
        <div class="statbar stopped">${frozenEl(s.final.remainingMs)}<span>Clock stopped</span></div>
        <div class="buzz-line ${s.final.buzzedBy ? 'on' : ''}">${s.final.buzzedBy ? html`<b>${nameOf(s, s.final.buzzedBy)}</b> is answering for the team` : 'Waiting for a team buzz'}</div>
        ${qCard('Same question — team answers', s.final.question, s.final.answer)}
        ${markButtons('markFinal', 'Team correct: push back', 'Team wrong')}`;

    case 'gameover':
      return html`<section class="card center">${resultHTML(s)}${ctl('playAgain', 'Play again with the same players')}</section>`;
    default:
      return html``;
  }
}

const PHASE_LABEL = {
  lobby: 'Lobby', between: 'Between players', cb_ready: 'Cash builder', cb_playing: 'Cash builder',
  cb_done: 'Cash builder', offers_set: 'Offers', offers_choose: 'Offers', h2h_ready: 'Head-to-head',
  h2h_question: 'Head-to-head', h2h_reveal: 'Head-to-head', h2h_over: 'Head-to-head',
  final_pick: 'Final chase', final_team_ready: 'Final chase', final_team: 'Final chase · team',
  final_team_done: 'Final chase', final_chaser: 'Final chase · chaser', final_push: 'Final chase · pushback',
  gameover: 'Game over',
};

export function hostView(s, ctx) {
  return html`
    <div class="host">
      <header class="bar">
        <div><b class="code">${s.code}</b><span class="phase">${PHASE_LABEL[s.phase] ?? s.phase}</span></div>
        <div class="bar-actions">
          <a class="btn small ghost" href="/?screen=${s.code}" target="_blank" rel="noopener">Open big screen</a>
          <button class="btn small ghost" data-local="copyJoin">Copy join link</button>
          <button class="btn small ghost" data-local="copyHost">Copy controller link</button>
        </div>
      </header>
      ${ctx.error ? html`<div class="toast" role="alert">${ctx.error}</div>` : ''}
      <main>${body(s)}</main>
      ${s.log?.length ? html`<details class="log"><summary>Game log</summary><ol>${[...s.log].reverse().map((l) => html`<li>${l}</li>`)}</ol></details>` : ''}
    </div>`;
}

// Y / N keys (and arrows) rule the current question.
export function hostKey(s, key) {
  const right = ['y', 'Y', 'ArrowUp', 'Enter'].includes(key);
  const wrong = ['n', 'N', 'ArrowDown', 'Backspace'].includes(key);
  if (!right && !wrong) return null;
  if (s.phase === 'cb_playing') return { action: 'markCB', arg: { correct: right } };
  if (['final_team', 'final_chaser', 'final_push'].includes(s.phase)) return { action: 'markFinal', arg: { correct: right } };
  return null;
}
