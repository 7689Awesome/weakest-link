// Tiny templating: html`...` escapes every interpolated value unless it is
// itself the result of html`...` (or an array of them).

class Raw {
  constructor(s) { this.s = s; }
  toString() { return this.s; }
}

const ESC = { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' };
export const esc = (s) => String(s ?? '').replace(/[&<>"']/g, (c) => ESC[c]);

function part(v) {
  if (v instanceof Raw) return v.s;
  if (Array.isArray(v)) return v.map(part).join('');
  if (v === null || v === undefined || v === false) return '';
  return esc(v);
}

export function html(strings, ...vals) {
  let out = strings[0];
  vals.forEach((v, i) => { out += part(v) + strings[i + 1]; });
  return new Raw(out);
}

export const raw = (s) => new Raw(s);

export const CURRENCY = '£';
export const money = (n) => (n < 0 ? '-' : '') + CURRENCY + Math.abs(n).toLocaleString('en-GB');

export const $ = (sel, root = document) => root.querySelector(sel);

// JSON for a data-arg attribute. html`` escapes it, so don't escape twice.
export const jarg = (o) => JSON.stringify(o);
