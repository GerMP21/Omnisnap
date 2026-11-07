// Triage: from "every genuine difference" to "differences a developer wants".
// Takes the root causes found by align.mjs and decides, for each one:
//   1. MECHANISM: is it a known, expected engine difference? (knowledge base)
//   2. CONSEQUENCES: what can a person actually SEE change?
//   3. TIER: Report (a visible consequence not explained as expected) or Info.
// Findings that share a mechanism are then merged into one CAUSE and ranked
// by impact (see groupCauses). Dependency-free: this is ported to Go.
//
// Based on docs/research/ "Omnisnap Precision Research Report" (consequence
// gate, knowledge base, cause-level reporting) and the spike's labeled data.

const median = (xs) => { const s = [...xs].sort((a, b) => a - b); const m = s.length >> 1; return s.length % 2 ? s[m] : (s[m - 1] + s[m]) / 2; };

// ------------------------------------------------------------ typography

// Distinct line tops of an element's whole text (own + descendants), and the
// line pitch = median gap between consecutive lines (robust to paragraph gaps).
export function lineInfo(n) {
  const tops = [];
  const visit = (x) => { if (x.lt && !x.hid) tops.push(...x.lt); for (const c of x.k) visit(c); };
  visit(n);
  tops.sort((a, b) => a - b);
  const rows = [];
  for (const t of tops) if (!rows.length || t - rows[rows.length - 1] > 1) rows.push(t);
  if (rows.length < 3) return null;
  const gaps = [];
  for (let i = 1; i < rows.length; i++) gaps.push(rows[i] - rows[i - 1]);
  return { lines: rows.length, pitch: median(gaps) };
}

// The typography most of the element's text uses (line-height, size, family, weight).
function dominantType(n) {
  const counts = new Map();
  const visit = (x) => {
    if (x.lh !== undefined && !x.hid) {
      const key = `${x.lh}|${x.fs}|${x.ff}|${x.fw}`;
      counts.set(key, (counts.get(key) || 0) + (x.ln || 1));
    }
    for (const c of x.k) visit(c);
  };
  visit(n);
  let best = null;
  for (const [key, c] of counts) if (!best || c > best[1]) best = [key, c];
  if (!best) return null;
  const [lh, fs, ff, fw] = best[0].split('|');
  return { lh: lh === 'normal' ? 'normal' : Number(lh), fs: Number(fs), ff, fw, key: best[0] };
}

// ------------------------------------------------- knowledge base (KB)
// Expected, systematic engine differences. A match turns findings into one
// explained cause with a fix; it stays in Info unless a consequence shows.

const FORM_CONTROLS = new Set(['input', 'select', 'textarea', 'button']);
const sameFont = (ta, tb) => ta && tb && ta.fs === tb.fs && ta.ff === tb.ff && ta.fw === tb.fw;

export function matchKnowledgeBase(c, engines) {
  const a = c.side === 'A' ? c.node : null;
  const b = c.side === 'A' ? c.other : c.node;
  if (!a || !b) return null;
  const d = c.d || {};
  const ta = dominantType(a);
  const tb = dominantType(b);

  // K1 / K2: line pitch differs, same text, same lines, same font.
  if (ta && tb && sameFont(ta, tb) && String(ta.lh) === String(tb.lh)) {
    const la = lineInfo(a);
    const lb = lineInfo(b);
    if (la && lb && la.lines === lb.lines) {
      const dp = lb.pitch - la.pitch;
      if (ta.lh !== 'normal' && !Number.isInteger(ta.lh) && Math.abs(dp) >= 0.05 && Math.abs(dp) <= 1) {
        return {
          id: 'K1', expected: true, title: 'Fractional line-height rounded differently',
          explanation: `line-height ${ta.lh}px is laid out at ${lb.pitch.toFixed(2)}px per line in ${engines.B} vs ${la.pitch.toFixed(2)}px in ${engines.A}; over ${la.lines} lines that is ${d.dh ? `${d.dh > 0 ? '+' : ''}${d.dh}px` : 'a small drift'}`,
          fix: `use a whole-pixel line-height, e.g. line-height: ${Math.round(ta.lh)}px (or round(calc(...), 1px))`,
        };
      }
      if (ta.lh === 'normal' && Math.abs(dp) >= 0.2) {
        return {
          id: 'K2', expected: true, title: 'line-height: normal resolves differently',
          explanation: `line-height: normal is derived from the font's metrics: ${la.pitch.toFixed(2)}px per line in ${engines.A}, ${lb.pitch.toFixed(2)}px in ${engines.B} (${la.lines} lines)`,
          fix: 'set an explicit numeric line-height',
        };
      }
    }
  }
  // K5: a long unbreakable token (URL, long word) breaks across lines differently.
  if (d.dln && /(:\/\/|\S{25,})/.test(a.tx || '')) {
    return {
      id: 'K5', expected: false, title: 'Long token (URL) breaks differently',
      explanation: `"${(a.tx || '').slice(0, 40)}" wraps to ${a.ln} line(s) in ${engines.A} and ${b.ln} in ${engines.B}: engines apply different line-break opportunities to long tokens`,
      fix: 'overflow-wrap: anywhere (or insert <wbr> break points)',
    };
  }
  // K8: native form control with a different default size.
  if (FORM_CONTROLS.has(a.t) && (d.dw || d.dh)) {
    return {
      id: 'K8', expected: true, title: 'Form control default size differs',
      explanation: `<${a.t}> has engine-specific default dimensions (${d.dw ? `width ${d.dw > 0 ? '+' : ''}${d.dw}px` : ''}${d.dw && d.dh ? ', ' : ''}${d.dh ? `height ${d.dh > 0 ? '+' : ''}${d.dh}px` : ''})`,
      fix: 'give the control an explicit width/height and box-sizing',
    };
  }
  // K4: own text measures differently (shaping/hinting), same font, same lines.
  if ((c.kinds || []).includes('width') && !d.dln && a.tw && b.tw && ta && tb && sameFont(ta, tb)) {
    const ratio = b.tw / a.tw;
    if (ratio > 0.95 && ratio < 1.05) {
      return {
        id: 'K4', expected: true, title: 'Text advance differs',
        explanation: `the same text in the same font measures ${a.tw}px in ${engines.A} and ${b.tw}px in ${engines.B} (glyph advances, hinting)`,
        fix: 'no fix needed unless it causes wrapping, clipping or overflow',
      };
    }
  }
  return null;
}

// ------------------------------------------------------------ consequences

const HARD = new Set(['presence', 'visibility', 'overflow', 'protrusion', 'truncation']);

export function consequences(c, ctx) {
  const out = new Set();
  const kinds = c.kinds || [];
  const d = c.d || {};
  const a = c.side === 'A' ? c.node : null;
  const b = c.side === 'A' ? c.other : c.node;
  if (kinds.some((k) => k.startsWith('only-in-') || k === 'children-differ')) out.add('presence');
  if (kinds.some((k) => k.startsWith('hidden-in-'))) out.add('visibility');
  if (kinds.some((k) => k.startsWith('overflow-'))) out.add('overflow');
  // Sticks out of the viewport in one engine only (horizontal scroll).
  if (a && b) {
    const outA = a.b[0] + a.b[2] > ctx.vwA + 1;
    const outB = b.b[0] + b.b[2] > ctx.vwB + 1;
    if (outA !== outB) out.add('protrusion');
    if (Boolean(a.tr) !== Boolean(b.tr)) out.add('truncation');
  }
  if (d.dln) out.add('line-count');
  // An interactive target (button, control, block-level link) moved: the user
  // notices. Inline links that shift because their paragraph reflowed are not
  // targets that moved; the reflow is the cause (and has its own consequence).
  const moved = (n, dd) => n && n.ia && n.d !== 'inline' && dd && Math.max(Math.abs(dd.dx || 0), Math.abs(dd.dy || 0)) >= 4;
  if (a && moved(a, d)) out.add('interactive-moved');
  if (a && kinds.includes('children-moved') && a.k.some((k) => moved(k, ctx.D.get(k.uid)))) out.add('interactive-moved');
  // Structural: the element itself changed by >= max(8px, 10%) of its size,
  // or its children moved that much relative to their own size.
  const big = (delta, size) => Math.abs(delta || 0) >= Math.max(8, 0.1 * size);
  if (a && (big(d.dw, a.b[2]) || big(d.dh, a.b[3]) || big(d.dx, a.b[2]) || big(d.dy, a.b[3]))) out.add('structural');
  // (Inline children shifting inside reflowed text are not structural moves.)
  if (a && kinds.includes('children-moved') && a.k.some((k) => { const kd = ctx.D.get(k.uid); return k.d !== 'inline' && kd && (big(kd.dx, k.b[2]) || big(kd.dy, k.b[3])); })) out.add('structural');
  return [...out];
}

// ---------------------------------------------------------------- tiering

export function triage(c, ctx) {
  const kb = matchKnowledgeBase(c, ctx.engines);
  const cons = consequences(c, ctx);
  const hard = cons.some((k) => HARD.has(k));
  let tier;
  if (hard) tier = 'report'; // always: things appear, vanish, overflow or get cut
  else if (kb?.expected) tier = 'info'; // explained engine difference, nothing visibly broken
  else tier = cons.length ? 'report' : 'info';
  const weight = hard ? 3 : cons.length ? 2 : 1;
  const y = (c.side === 'A' ? c.node : (c.other || c.node)).b[1];
  const visibility = y < 800 ? 1 : y < 2400 ? 0.7 : 0.4; // first viewport, next two, rest
  return { kb, consequences: cons, tier, weight: kb?.expected ? weight * 0.5 : weight, visibility };
}

// Rank causes: the strongest instance, boosted (not dominated) by breadth.
export const impactOf = (instances) => Math.max(...instances.map((f) => f.weight * f.visibility)) * (1 + Math.log2(instances.length));
