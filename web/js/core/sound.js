// Synthesized sound effects (Web Audio, no files). Browsers keep audio locked
// until a user gesture, so call unlock() from a click handler.

let ctx = null;

export function unlock() {
  try {
    ctx ||= new (window.AudioContext || window.webkitAudioContext)();
    if (ctx.state === 'suspended') ctx.resume();
  } catch { /* audio is optional */ }
}

function tone(freq, start, dur, { type = 'sine', gain = 0.16, slideTo } = {}) {
  if (!ctx || ctx.state !== 'running') return;
  const t = ctx.currentTime + start;
  const osc = ctx.createOscillator();
  const g = ctx.createGain();
  osc.type = type;
  osc.frequency.setValueAtTime(freq, t);
  if (slideTo) osc.frequency.exponentialRampToValueAtTime(slideTo, t + dur);
  g.gain.setValueAtTime(0.0001, t);
  g.gain.exponentialRampToValueAtTime(gain, t + 0.02);
  g.gain.exponentialRampToValueAtTime(0.0001, t + dur);
  osc.connect(g).connect(ctx.destination);
  osc.start(t);
  osc.stop(t + dur + 0.05);
}

const SFX = {
  correct: () => { tone(660, 0, 0.12); tone(990, 0.1, 0.22); },
  wrong: () => tone(200, 0, 0.35, { type: 'sawtooth', slideTo: 110, gain: 0.12 }),
  chaserStep: () => { tone(110, 0, 0.18, { type: 'square', gain: 0.1 }); tone(98, 0.2, 0.25, { type: 'square', gain: 0.1 }); },
  lock: () => tone(520, 0, 0.08, { type: 'triangle' }),
  tick: () => tone(880, 0, 0.05, { type: 'triangle', gain: 0.08 }),
  buzz: () => tone(300, 0, 0.4, { type: 'square', gain: 0.12 }),
  start: () => { tone(392, 0, 0.14); tone(523, 0.14, 0.14); tone(659, 0.28, 0.3); },
  home: () => [523, 659, 784, 1047].forEach((f, i) => tone(f, i * 0.13, 0.3)),
  caught: () => [330, 262, 196, 147].forEach((f, i) => tone(f, i * 0.2, 0.35, { type: 'sawtooth', gain: 0.1 })),
  horn: () => tone(150, 0, 0.9, { type: 'sawtooth', gain: 0.12 }),
};

export function play(name) {
  try { SFX[name]?.(); } catch { /* ignore */ }
}
