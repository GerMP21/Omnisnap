// Concept 3: self-calibrating comparison (robust statistics).
// Learns, per page and engine pair, how the engines SYSTEMATICALLY differ,
// with estimators that ignore outliers (median / MAD, a Theil-Sen slope
// through the origin):
//   - text advance: ratio of text widths for identical text in identical fonts
//   - line pitch: per computed line-height, B pitch minus A pitch
//   - form controls: per control type, the typical size difference
// A cause the page model explains is expected; outliers are anomalies.
import { lineInfo } from './triage.mjs';

const median = (xs) => { if (!xs.length) return null; const s = [...xs].sort((p, q) => p - q); const m = s.length >> 1; return s.length % 2 ? s[m] : (s[m - 1] + s[m]) / 2; };
const mad = (xs, med) => median(xs.map((x) => Math.abs(x - med))) || 0;
const CONTROLS = new Set(['input', 'select', 'textarea', 'button']);
const ownPitch = (n) => {
  if (!n.lt || n.lt.length < 2) return null;
  const gaps = [];
  for (let i = 1; i < n.lt.length; i++) gaps.push(n.lt[i] - n.lt[i - 1]);
  return median(gaps);
};

export function calibrate(A, M) {
  const ratios = [];
  const pitch = new Map(); // lh key -> [dpitch]
  const controls = new Map(); // tag:type -> {dw:[], dh:[]}
  for (const a of A.all) {
    const b = M.pairOf.get(a.uid);
    if (!b || a.hid || b.hid) continue;
    if (a.th && a.th === b.th && a.ln === b.ln && a.ff === b.ff && a.fs === b.fs && a.tw > 20 && b.tw > 0) ratios.push(b.tw / a.tw);
    if (a.ln >= 2 && b.ln === a.ln && String(a.lh) === String(b.lh)) {
      const pa = ownPitch(a);
      const pb = ownPitch(b);
      if (pa && pb) {
        const key = `${a.lh}|${a.fs}|${a.ff}`;
        if (!pitch.has(key)) pitch.set(key, []);
        pitch.get(key).push(pb - pa);
      }
    }
    if (CONTROLS.has(a.t)) {
      const key = `${a.t}:${a.n || ''}`.split(':')[0];
      if (!controls.has(key)) controls.set(key, { dw: [], dh: [] });
      controls.get(key).dw.push(b.b[2] - a.b[2]);
      controls.get(key).dh.push(b.b[3] - a.b[3]);
    }
  }
  const r = median(ratios);
  const model = {
    advance: r ? { ratio: r, mad: mad(ratios, r), n: ratios.length } : null,
    pitch: new Map([...pitch].map(([k, xs]) => { const m = median(xs); return [k, { dpitch: m, mad: mad(xs, m), n: xs.length }]; })),
    controls: new Map([...controls].map(([k, v]) => [k, { dw: median(v.dw), dh: median(v.dh), n: v.dw.length }])),
  };
  return model;
}

// Does the page model explain this cause's measured change?
export function calibrated(c, model) {
  const a = c.side === 'A' ? c.node : null;
  const d = c.d || {};
  if (!a || !model) return { explained: false };
  const out = [];
  // Text advance: own text wider/narrower by the page-wide ratio.
  if (d.dw && a.tw && model.advance && model.advance.n >= 5) {
    const expected = (model.advance.ratio - 1) * a.tw;
    const tol = 1 + 3 * model.advance.mad * a.tw;
    if (Math.abs(d.dw - expected) <= tol) out.push({ model: 'advance', expected: +expected.toFixed(1), tol: +tol.toFixed(1) });
  }
  // Line pitch: height change = lines x the page-wide pitch difference.
  if (d.dh) {
    const info = lineInfo(a);
    // The typography most of this element's text uses (the mode).
    const counts = new Map();
    const visit = (x) => { if (x.lh !== undefined) { const k = `${x.lh}|${x.fs}|${x.ff}`; counts.set(k, (counts.get(k) || 0) + 1); } x.k.forEach(visit); };
    visit(a);
    const key = [...counts].sort((p, q) => q[1] - p[1])[0]?.[0];
    const m = key && model.pitch.get(key);
    if (info && m && m.n >= 2 && Math.abs(m.dpitch) >= 0.05) {
      const expected = info.lines * m.dpitch;
      const tol = 2 + info.lines * m.mad;
      if (Math.abs(d.dh - expected) <= tol) out.push({ model: 'pitch', expected: +expected.toFixed(1), tol: +tol.toFixed(1) });
    }
  }
  // Form controls: same difference as the page's other controls of its type.
  const cm = CONTROLS.has(a.t) && model.controls.get(a.t);
  if (cm && cm.n >= 2 && Math.abs((d.dw || 0) - cm.dw) <= 2 && Math.abs((d.dh || 0) - cm.dh) <= 2) out.push({ model: 'controls', expected: { dw: cm.dw, dh: cm.dh } });
  return { explained: out.length > 0, by: out };
}
