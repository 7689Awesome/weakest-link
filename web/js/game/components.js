// Render helpers shared by the screen, host and phone views.
import { html, raw, money } from '../core/dom.js';

export const LETTERS = ['A', 'B', 'C'];

export const nameOf = (s, id) => s.players.find((p) => p.id === id)?.name ?? '';
export const chaserOf = (s) => s.players.find((p) => p.isChaser);
export const currentOf = (s) => s.players.find((p) => p.id === s.currentId);
export const contestants = (s) => s.players.filter((p) => !p.isChaser);

export const stepsToHome = (s, choice) => {
  const start = { lower: s.cfg.startLower, middle: s.cfg.startMiddle, higher: s.cfg.startHigher }[choice];
  return s.cfg.home - start;
};

// The funnel board from the show: red chaser pad on top, seven steps, home at
// the bottom. Rows between the chaser and the player are lit teal.
export function boardHTML(s) {
  const { cfg, h2h, offers } = s;
  const chaser = chaserOf(s);
  const cur = currentOf(s);
  const rows = [];
  for (let i = 0; i <= cfg.home; i += 1) {
    const hasC = i === h2h.chaserPos;
    const hasP = i === h2h.playerPos;
    let cls = 'row';
    if (i === 0) cls += ' pad';
    if (i === cfg.home) cls += ' homerow';
    if (hasC && hasP) cls += ' caught';
    else if (hasC) cls += ' has-chaser';
    else if (hasP) cls += ' has-player';
    else if (i > h2h.chaserPos && i < h2h.playerPos) cls += ' gap';
    else if (i < h2h.chaserPos) cls += ' past';
    else cls += ' ahead';
    rows.push(html`
      <div class="${cls}" style="--i:${i}">
        ${hasC ? html`<span class="who c"><b>${chaser?.name}</b></span>` : ''}
        ${hasP ? html`<span class="who p"><i class="arr l"></i><span class="amt">${money(offers?.amount ?? 0)}</span><small>${cur?.name}</small><i class="arr r"></i></span>` : ''}
        ${i === 0 && !hasC ? html`<svg class="chev" viewBox="0 0 100 40" aria-hidden="true"><path d="M4 4 L50 36 L96 4" /></svg>` : ''}
        ${i === cfg.home && !hasP ? html`<span class="homelabel">Home</span>` : ''}
      </div>`);
  }
  return html`<div class="board" role="img" aria-label="Chase board">${rows}</div>`;
}

// Final chase: a segmented bar. The chaser eats segments from the left; the
// colour of each remaining segment says where the team's lead came from.
export function trackHTML(f) {
  const segs = [];
  for (let i = 0; i < f.target; i += 1) {
    let kind;
    if (i < f.chaserCorrect) kind = 'chased';
    else if (i < f.head) kind = 'head';
    else if (i < f.head + f.teamCorrect) kind = 'team';
    else kind = 'push';
    segs.push(html`<i class="seg ${kind}"></i>`);
  }
  return html`
    <div class="track-wrap">
      <div class="track" role="img" aria-label="Chaser has ${f.chaserCorrect} of ${f.target} steps">${segs}</div>
      <div class="track-key">
        <span><i class="seg head"></i>Head start ${f.head}</span>
        <span><i class="seg team"></i>Team ${f.teamCorrect}</span>
        <span><i class="seg push"></i>Pushed back ${f.pushbacks}</span>
        <span><i class="seg chased"></i>Chaser ${f.chaserCorrect}</span>
      </div>
    </div>`;
}

export function optionsHTML(s, { buttons = false, mine = null, locked = false } = {}) {
  const h = s.h2h;
  const revealed = !!h.reveal;
  const player = currentOf(s);
  const chaser = chaserOf(s);
  return html`
    <div class="opts ${buttons ? 'as-buttons' : ''}">
      ${h.options.map((text, i) => {
        let cls = 'opt';
        if (h.correct === i) cls += ' correct'; // server only sends this to the host, or after the reveal
        if (revealed && h.correct !== i && (h.reveal.playerPick === i || h.reveal.chaserPick === i)) cls += ' wrong';
        if (mine === i) cls += ' mine';
        const tags = revealed ? html`
          ${h.reveal.playerPick === i ? html`<em class="tag p">${player?.name}</em>` : ''}
          ${h.reveal.chaserPick === i ? html`<em class="tag c">${chaser?.name}</em>` : ''}` : '';
        const inner = html`<span class="ltr">${LETTERS[i]}</span><span class="txt">${text}</span>${tags}`;
        return buttons
          ? html`<button class="${cls}" data-send="act" data-action="lock" data-arg='${JSON.stringify({ choice: i })}' ${locked ? 'disabled' : ''}>${inner}</button>`
          : html`<div class="${cls}">${inner}</div>`;
      })}
    </div>`;
}

export function rosterChips(s) {
  return html`<ul class="chips">
    ${s.players.map((p) => html`
      <li class="chip ${p.isChaser ? 'is-chaser' : ''} ${p.connected ? '' : 'away'} ${p.status}">
        <b>${p.name}</b>
        ${p.isChaser ? html`<small>Chaser</small>` : ''}
        ${p.status === 'home' ? html`<small>Home ${money(p.banked)}</small>` : ''}
        ${p.status === 'caught' ? html`<small>Caught</small>` : ''}
        ${p.status === 'waiting' && p.cashBuilt > 0 ? html`<small>${money(p.cashBuilt)}</small>` : ''}
      </li>`)}
  </ul>`;
}

export const clockEl = (endsAt, { fmt = 'clock', cls = '' } = {}) =>
  html`<span class="clock ${cls}" data-ends="${endsAt}" data-fmt="${fmt}">–</span>`;

export const frozenEl = (ms) => html`<span class="clock frozen" data-frozen="${ms}">–</span>`;

export function resultHTML(s) {
  const r = s.result;
  if (!r) return '';
  const names = r.finalists.map((id) => nameOf(s, id)).join(', ');
  return r.winner === 'team'
    ? html`<div class="result team"><h2>The team beat the Chaser</h2><p class="prize">${money(r.perPlayer)} each</p><p>${money(r.bank)} shared between ${names}</p></div>`
    : html`<div class="result chaser"><h2>The Chaser wins</h2><p>${r.finalists.length ? 'The team was caught in the final chase.' : 'Nobody made it home.'}</p></div>`;
}

export { raw };
