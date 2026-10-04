// The shared big-screen display (TV). Read-only; never receives answers early.
import { html, money } from '../core/dom.js';
import { play } from '../core/sound.js';
import {
  boardHTML, trackHTML, optionsHTML, rosterChips, clockEl, frozenEl, resultHTML,
  nameOf, chaserOf, currentOf, stepsToHome,
} from './components.js';

const joinUrl = (code) => `${location.host}/?join=${code}`;

function lockTiles(s) {
  const h = s.h2h;
  const p = currentOf(s);
  const c = chaserOf(s);
  const waiting = (h.playerLocked !== h.chaserLocked) && s.phase === 'h2h_question';
  return html`
    <div class="locks">
      <div class="lock ${h.playerLocked ? 'in' : ''}"><b>${p?.name}</b><span>${h.playerLocked ? 'Locked in' : 'Thinking…'}</span></div>
      ${waiting ? html`<div class="window"><span class="clock" data-ends="${h.deadlineAt}" data-fmt="sec">5</span></div>` : html`<div class="window vs">v</div>`}
      <div class="lock chaser ${h.chaserLocked ? 'in' : ''}"><b>${c?.name}</b><span>${h.chaserLocked ? 'Locked in' : 'Thinking…'}</span></div>
    </div>`;
}

function h2hPanel(s) {
  const h = s.h2h;
  const p = currentOf(s);
  switch (s.phase) {
    case 'h2h_ready': {
      const need = s.cfg.home - h.start;
      return html`<div class="callout"><h2>${p?.name} plays for ${money(s.offers.amount)}</h2>
        <p>${need} correct answers to bring it home. The chaser needs to land on ${p?.name}’s step.</p></div>`;
    }
    case 'h2h_over':
      return h.outcome === 'home'
        ? html`<div class="banner home"><h2>Home!</h2><p>${p?.name} brings ${money(s.offers.amount)} to the team</p></div>`
        : html`<div class="banner caught"><h2>Caught</h2><p>${p?.name} is out</p></div>`;
    default:
      return html`${lockTiles(s)}<h2 class="q-text">${h.question}</h2>${optionsHTML(s)}`;
  }
}

function offerCards(s) {
  const o = s.offers;
  const card = (key, label, amount) => html`
    <div class="offer ${key} ${o.choice === key ? 'chosen' : ''}">
      <h3>${label}</h3>
      <div class="amt">${money(amount)}</div>
      <p>${stepsToHome(s, key)} correct answers to get home</p>
    </div>`;
  return html`<div class="offers">
    ${card('higher', 'Higher offer', o.higher)}
    ${card('middle', 'Cash builder', o.middle)}
    ${card('lower', 'Lower offer', o.lower)}
  </div>`;
}

function body(s) {
  const p = currentOf(s);
  const c = chaserOf(s);
  switch (s.phase) {
    case 'lobby':
      return html`<div class="lobby">
        <div><p class="hint">Join on your phone at <b>${joinUrl(s.code)}</b> or go to this site and enter</p>
        <div class="roomcode" aria-label="Room code">${s.code}</div></div>
        ${s.players.length ? rosterChips(s) : html`<p class="hint">Nobody here yet.</p>`}
        <p class="hint">${c ? `${c.name} is the Chaser.` : 'The quizmaster will pick the Chaser.'}</p>
      </div>`;

    case 'between': {
      const next = s.players.filter((x) => x.queued);
      return html`<div class="between"><h1>${next.length ? 'Who’s next?' : 'Final chase'}</h1>${rosterChips(s)}</div>`;
    }

    case 'cb_ready':
      return html`<div class="hero"><p class="hint">Cash builder</p><h1 class="name">${p?.name}</h1>
        <p>60 seconds. Every correct answer is worth ${money(s.cfg.cashPerCorrect)}.</p></div>`;

    case 'cb_playing':
      return html`<div class="cb">
        <div class="cb-top"><div><p class="hint">Cash builder</p><h2>${p?.name}</h2></div>
          ${clockEl(s.cb.endsAt, { fmt: 'sec', cls: 'xl' })}
          <div class="pot"><p class="hint">Built</p><b>${money(s.cb.total)}</b></div></div>
        <h2 class="q-text">${s.cb.question}</h2></div>`;

    case 'cb_done':
      return html`<div class="hero"><p class="hint">${p?.name} built</p><h1 class="money">${money(s.cb.total)}</h1>
        <p>${s.cb.correct} correct</p></div>`;

    case 'offers_set':
      return html`<div class="hero"><p class="hint">${p?.name} has ${money(s.offers.middle)}</p>
        <h1>${c?.name} is considering an offer…</h1></div>`;

    case 'offers_choose':
      return html`<div class="offers-wrap"><h2>${p?.name}, what’s it going to be?</h2>${offerCards(s)}</div>`;

    case 'h2h_ready': case 'h2h_question': case 'h2h_reveal': case 'h2h_over':
      return html`<div class="h2h"><div class="board-col">${boardHTML(s)}</div><div class="panel">${h2hPanel(s)}</div></div>`;

    case 'final_pick':
      return html`<div class="hero"><p class="hint">Final chase</p><h1>Choose a question set</h1>
        <p>${s.final.finalists.length} ${s.final.finalists.length === 1 ? 'player' : 'players'} in. Majority picks the team’s set; the chaser gets the other.</p>
        <ul class="chips">${s.final.finalists.map((id) => html`<li class="chip ${s.final.voted.includes(id) ? 'voted' : ''}"><b>${nameOf(s, id)}</b><small>${s.final.voted.includes(id) ? 'Voted' : 'Choosing…'}</small></li>`)}</ul></div>`;

    case 'final_team_ready':
      return html`<div class="hero"><p class="hint">Final chase</p><h1>Set ${s.final.teamSet} for the team</h1>
        <p>2 minutes. Buzz in, answer, and the quizmaster rules. Head start: ${s.final.head} ${s.final.head === 1 ? 'step' : 'steps'}.</p>
        <p class="pot-line">${money(s.bank)} in the bank</p></div>`;

    case 'final_team':
      return html`<div class="final">
        <div class="cb-top">
          <div><p class="hint">Team round</p><h2>${s.final.teamCorrect} correct</h2></div>
          ${clockEl(s.final.endsAt, { cls: 'xl' })}
          <div class="pot"><p class="hint">Bank</p><b>${money(s.bank)}</b></div></div>
        <h2 class="q-text">${s.final.question}</h2>
        <div class="buzzed ${s.final.buzzedBy ? 'on' : ''}">${s.final.buzzedBy ? html`<b>${nameOf(s, s.final.buzzedBy)}</b> buzzed in` : 'Buzzers are live'}</div>
        ${trackHTML(s.final)}</div>`;

    case 'final_team_done':
      return html`<div class="hero"><p class="hint">Time</p><h1>${s.final.teamCorrect} correct + ${s.final.head} head start</h1>
        <p class="target">The chaser needs ${s.final.target}</p>${trackHTML(s.final)}</div>`;

    case 'final_chaser':
    case 'final_push':
      return html`<div class="final chaser-round">
        <div class="cb-top">
          <div><p class="hint">${c?.name}</p><h2>${s.final.chaserCorrect} of ${s.final.target}</h2></div>
          ${s.phase === 'final_push' ? frozenEl(s.final.remainingMs) : clockEl(s.final.endsAt, { cls: 'xl' })}
          <div class="pot"><p class="hint">Bank</p><b>${money(s.bank)}</b></div></div>
        <h2 class="q-text">${s.final.question}</h2>
        ${s.phase === 'final_push'
          ? html`<div class="buzzed on stop"><b>Clock stopped</b> ${s.final.buzzedBy ? html`${nameOf(s, s.final.buzzedBy)} is answering for the team` : 'Team: buzz in to push the chaser back'}</div>`
          : html`<div class="buzzed">${c?.name} is answering</div>`}
        ${trackHTML(s.final)}</div>`;

    case 'gameover':
      return html`<div class="hero">${resultHTML(s)}${s.final ? trackHTML(s.final) : ''}</div>`;
    default:
      return html``;
  }
}

export function screenView(s) {
  return html`
    <div class="screen phase-${s.phase}">
      <header class="hud">
        <span class="brand">Chase Night</span>
        <span class="code">${s.code}</span>
        <span class="bank">${s.bank > 0 ? html`Bank <b>${money(s.bank)}</b>` : ''}</span>
      </header>
      <main>${body(s)}</main>
    </div>`;
}

// Sound effects keyed to what changed between two snapshots.
export function screenSfx(prev, next) {
  if (!prev) return;
  const was = prev.phase;
  const is = next.phase;
  if (was !== is) {
    if (is === 'cb_playing' || is === 'final_team' || is === 'final_chaser') play('start');
    if (is === 'h2h_reveal') {
      const r = next.h2h.reveal;
      if (r.chaserRight && !r.playerRight) play('chaserStep');
      else if (r.playerRight) play('correct');
      else play('wrong');
    }
    if (is === 'h2h_over') play(next.h2h.outcome === 'home' ? 'home' : 'caught');
    if (is === 'final_push') play('wrong');
    if (was === 'final_push' && is === 'final_chaser' && next.final.pushbacks > prev.final.pushbacks) play('correct');
    if (is === 'final_team_done' || (is === 'gameover' && was !== 'gameover')) play('horn');
    if (is === 'gameover' && next.result?.winner === 'team') play('home');
    return;
  }
  if (is === 'cb_playing' && next.cb.correct > prev.cb.correct) play('correct');
  if (is === 'final_team' && next.final.teamCorrect > prev.final.teamCorrect) play('correct');
  if (is === 'final_team' && next.final.qNum > prev.final.qNum && next.final.teamCorrect === prev.final.teamCorrect) play('wrong');
  if (is === 'final_team' && !prev.final.buzzedBy && next.final.buzzedBy) play('buzz');
  if (is === 'final_chaser' && next.final.chaserCorrect > prev.final.chaserCorrect) play('chaserStep');
  if (is === 'final_push' && !prev.final.buzzedBy && next.final.buzzedBy) play('buzz');
  if (is === 'h2h_question' && (next.h2h.playerLocked !== prev.h2h.playerLocked || next.h2h.chaserLocked !== prev.h2h.chaserLocked)) play('lock');
}
