// Concept 8: compare every engine against the design's INTENT, not against
// Chrome.
//
// The design's intent is inferred from the renders themselves: a relation
// between two elements (shared edge, equal size, equal gap, grid column, no
// overlap) is INTENDED when at least two engine families keep it within
// 1px. One engine breaking an intended relation clearly (>= 2px, or >= 8%
// for sizes) is a violation, whichever engine it is, Chrome included.
//
// Relations are always RELATIVE (between two boxes), so a cascade that
// moves a whole region (a taller header pushing the page down) breaks
// nothing. Violations are blamed on the element that breaks the most
// relations with its group, and reported once per element.
//
// Round 4 had a sibling-only, 0.5-3px near-miss detector: it fired 0 times
// on 329 corpus causes, because real intent breaks are bigger than 3px and
// rarely limited to siblings of an already-found cause.

const HOLD = 1; // px: a relation "holds" in an engine
const BREAK = 2; // px: a relation is clearly broken
const REL = 0.08; // Weber fraction for sizes
const MAX_KIDS = 60;
const WINDOW = 6; // compare each child with its next WINDOW siblings in big groups

const visible = (n) => n && !n.hid && n.d !== 'inline' && n.b[2] >= 4 && n.b[3] >= 4;
const sig = (n) => `${n.t}.${(n.c || '').split(' ').filter(Boolean).sort().slice(0, 3).join('.')}`;
const EDGE = {
  left: (b) => b[0], right: (b) => b[0] + b[2], top: (b) => b[1], bottom: (b) => b[1] + b[3],
  'center-x': (b) => b[0] + b[2] / 2, 'center-y': (b) => b[1] + b[3] / 2,
};
const overlap = (p, q) => Math.min(Math.min(p[0] + p[2], q[0] + q[2]) - Math.max(p[0], q[0]), Math.min(p[1] + p[3], q[1] + q[3]) - Math.max(p[1], q[1]));

// The relations two boxes keep, as named measurements; a relation holds when
// its measurement is ~0 (overlap: when the boxes don't overlap). Equal sizes
// are only intended between repeated components (same tag and classes);
// equal HEIGHT only side by side (a row of cards), never down a column,
// where equal heights of text blocks are a coincidence of line counts.
function relations(x, y, sameKind) {
  const out = [];
  for (const [e, f] of Object.entries(EDGE)) out.push({ kind: `align-${e}`, value: (bx, by) => f(bx) - f(by), size: () => 0 });
  if (sameKind) {
    out.push({ kind: 'equal-width', value: (bx, by) => bx[2] - by[2], size: (bx) => bx[2] });
    const sideBySide = Math.min(x.b[1] + x.b[3], y.b[1] + y.b[3]) - Math.max(x.b[1], y.b[1]) > 0;
    if (sideBySide) out.push({ kind: 'equal-height', value: (bx, by) => bx[3] - by[3], size: (bx) => bx[3] });
  }
  out.push({ kind: 'no-overlap', value: (bx, by) => Math.max(0, overlap(bx, by)), size: () => 0, overlap: true });
  return out;
}

// engines: [{ key, name, box(aNode) -> box or null }], A first.
function checkPair(i, j, x, y, engines, rels, add, extraKind) {
  const boxes = engines.map((e) => [e.box(x), e.box(y)]);
  if (boxes.some(([p, q]) => !p || !q)) return;
  for (const r of rels) {
    const v = boxes.map(([p, q]) => r.value(p, q));
    const holds = v.map((d) => (r.overlap ? d <= HOLD : Math.abs(d) <= HOLD));
    const nHold = holds.filter(Boolean).length;
    if (nHold < 2 || nHold === engines.length) continue;
    for (let e = 0; e < engines.length; e++) {
      if (holds[e]) continue;
      const mag = Math.abs(v[e]);
      const need = r.overlap ? BREAK : Math.max(BREAK, REL * r.size(boxes[e][0]));
      if (mag < need || mag > MAX_MAG) continue;
      if (r.overlap && Math.min(boxes[e][0][2], boxes[e][0][3], boxes[e][1][2], boxes[e][1][3]) < 4) continue;
      add({ engine: e, ref: holds.indexOf(true), kind: extraKind || r.kind, x, y, mag, holdsIn: engines.filter((_, k) => holds[k]).map((g) => g.name) });
    }
  }
}

const MAX_MAG = 800; // px: a "violation" larger than a viewport is a state or matching artifact
const MIN_MATCH = 0.95; // below this match rate the engines rendered different page states

export function intentViolations({ A, B, M, thirds, maskA, maskB, names }) {
  if (!thirds.length) return { violations: [], relations: 0, reason: 'needs a third engine family' };
  const low = [M, ...thirds.map((t) => t.M)].find((m) => m.pairOf.size / A.all.length < MIN_MATCH);
  if (low) return { violations: [], relations: 0, reason: `match rate below ${MIN_MATCH * 100}%: page states differ` };
  const nodeIn = [
    (a) => a,
    (a) => { const b = M.pairOf.get(a.uid); return visible(b) && !maskB.has(b.uid) ? b : null; },
    ...thirds.map((t) => (a) => { const n = t.M.pairOf.get(a.uid); return visible(n) ? n : null; }),
  ];
  const engines = [
    { key: 'A', name: names.A, box: (a) => a.b },
    { key: 'B', name: names.B, box: (a) => nodeIn[1](a)?.b || null },
    ...thirds.map((t, i) => ({ key: 'T', name: t.engine, box: (a) => nodeIn[2 + i](a)?.b || null })),
  ];
  // The same engines, measuring where each element's own TEXT starts (box
  // left + text inset): padding or indent that moves text inside an
  // unchanged box breaks a text column.
  const textEngines = engines.map((e, i) => ({ ...e, box: (a) => { const n = nodeIn[i](a); return n && n.ti != null && n.tx ? [n.b[0] + n.ti, n.b[1], 1, n.b[3]] : null; } }));
  const ok = (n) => visible(n) && !maskA.has(n.uid);
  const raw = [];
  let checked = 0;
  const add = (v) => raw.push(v);
  for (const P of A.all) {
    const kids = P.k.filter(ok).slice(0, MAX_KIDS);
    if (kids.length < 2) continue;
    // siblings
    for (let i = 0; i < kids.length; i++) {
      const lim = kids.length > 24 ? Math.min(kids.length, i + 1 + WINDOW) : kids.length;
      for (let j = i + 1; j < lim; j++) {
        checked++;
        checkPair(i, j, kids[i], kids[j], engines, relations(kids[i], kids[j], sig(kids[i]) === sig(kids[j])), add);
      }
    }
    // rhythm: equal gaps between consecutive siblings (rows and columns)
    for (let i = 0; i + 2 < kids.length; i++) {
      const [a, b, c] = [kids[i], kids[i + 1], kids[i + 2]];
      const row = Math.abs(a.b[1] - b.b[1]) <= HOLD && Math.abs(b.b[1] - c.b[1]) <= HOLD;
      const col = Math.abs(a.b[0] - b.b[0]) <= HOLD && Math.abs(b.b[0] - c.b[0]) <= HOLD;
      if (!row && !col) continue;
      const gap = (p, q) => (row ? q[0] - (p[0] + p[2]) : q[1] - (p[1] + p[3]));
      const boxes = engines.map((e) => [e.box(a), e.box(b), e.box(c)]);
      if (boxes.some((bs) => bs.some((x) => !x))) continue;
      checked++;
      const v = boxes.map(([p, q, r]) => gap(p, q) - gap(q, r));
      const holds = v.map((d) => Math.abs(d) <= HOLD);
      const nHold = holds.filter(Boolean).length;
      if (nHold < 2 || nHold === engines.length) continue;
      for (let e = 0; e < engines.length; e++) {
        if (!holds[e] && Math.abs(v[e]) >= BREAK && Math.abs(v[e]) <= MAX_MAG) {
          // blame the middle element's neighbour whose gap differs from the consensus gap
          const g1 = boxes.map(([p, q]) => gap(p, q));
          const cons = g1.filter((_, k) => holds[k])[0];
          const culprit = Math.abs(g1[e] - cons) > HOLD ? b : c;
          add({ engine: e, ref: holds.indexOf(true), kind: row ? 'equal-gap-x' : 'equal-gap-y', x: culprit, y: culprit === b ? a : b, mag: Math.abs(v[e]), holdsIn: engines.filter((_, k) => holds[k]).map((g) => g.name), blame: culprit });
        }
      }
    }
    // Repeated components (siblings with the same signature and the same
    // parts): corresponding parts keep their edges aligned across the
    // components. Stacked rows -> grid columns (left/right); side-by-side
    // cards -> shared top and bottom lines (a pinned call-to-action).
    const bySig = new Map();
    for (const k of kids) { const g = sig(k); if (!bySig.has(g)) bySig.set(g, []); bySig.get(g).push(k); }
    for (const reps of bySig.values()) {
      if (reps.length < 2) continue;
      const parts = reps.map((r) => r.k.filter(ok));
      for (let i = 0; i + 1 < reps.length; i++) {
        for (let j = i + 1; j < Math.min(reps.length, i + 1 + WINDOW); j++) {
          if (parts[i].length < 1 || parts[i].length !== parts[j].length) continue;
          if (parts[i].some((p, c) => sig(p) !== sig(parts[j][c]))) continue;
          const sideBySide = Math.min(reps[i].b[1] + reps[i].b[3], reps[j].b[1] + reps[j].b[3]) > Math.max(reps[i].b[1], reps[j].b[1]);
          const edges = sideBySide ? ['top', 'bottom'] : ['left', 'right'];
          const rels = edges.map((e) => ({ kind: `${sideBySide ? 'repeat' : 'column'}-${e}`, value: (bx, by) => EDGE[e](bx) - EDGE[e](by), size: () => 0 }));
          for (let c = 0; c < parts[i].length; c++) {
            checked++;
            checkPair(i, j, parts[i][c], parts[j][c], engines, rels, add);
            if (!sideBySide) checkPair(i, j, parts[i][c], parts[j][c], textEngines, [{ kind: 'column-text', value: (bx, by) => bx[0] - by[0], size: () => 0 }], add);
          }
        }
      }
    }
  }
  // Blame: per broken relation, the element that MOVED, i.e. whose box
  // relative to its parent differs most between the violating engine and an
  // engine that keeps the relation; ties go to the element that breaks more
  // relations. One violation per (engine, element).
  const count = new Map();
  for (const v of raw) for (const n of [v.x, v.y]) count.set(`${v.engine}|${n.uid}`, (count.get(`${v.engine}|${n.uid}`) || 0) + 1);
  const rel = (n, e) => { const b = engines[e].box(n); const p = n.parent && engines[e].box(n.parent); return b && p ? [b[0] - p[0], b[1] - p[1], b[2], b[3]] : b; };
  const moved = (n, e, ref) => { const x = rel(n, e); const y = rel(n, ref); return x && y ? x.reduce((s, v, i) => s + Math.abs(v - y[i]), 0) : 0; };
  const per = new Map();
  for (const v of raw) {
    const mx = moved(v.x, v.engine, v.ref);
    const my = moved(v.y, v.engine, v.ref);
    const byCount = (count.get(`${v.engine}|${v.x.uid}`) || 0) >= (count.get(`${v.engine}|${v.y.uid}`) || 0) ? v.x : v.y;
    const culprit = v.blame || (Math.abs(mx - my) > HOLD ? (mx > my ? v.x : v.y) : byCount);
    const key = `${v.engine}|${culprit.uid}`;
    if (!per.has(key)) per.set(key, { engine: v.engine, node: culprit, kinds: new Set(), mag: 0, holdsIn: v.holdsIn, with: new Set() });
    const e = per.get(key);
    e.kinds.add(v.kind);
    e.mag = Math.max(e.mag, v.mag);
    e.with.add((v.x === culprit ? v.y : v.x).uid);
  }
  // Report only what this engine pair owns: the baseline (A) or B. A third
  // engine's violations are reported in its own pair.
  const violations = [...per.values()].filter((v) => engines[v.engine].key !== 'T').map((v) => ({
    engine: engines[v.engine].key,
    node: v.node,
    kinds: [...v.kinds].sort(),
    mag: Math.round(v.mag * 10) / 10,
    holdsIn: v.holdsIn,
    others: v.with.size,
  })).sort((x, y) => y.others - x.others || y.mag - x.mag);
  return { violations, relations: checked };
}

// A short phrase per relation kind, for the report.
export const INTENT_TEXT = {
  'align-left': 'left edge', 'align-right': 'right edge', 'align-top': 'top edge', 'align-bottom': 'bottom edge',
  'align-center-x': 'horizontal centre', 'align-center-y': 'vertical centre', 'equal-width': 'equal width', 'equal-height': 'equal height',
  'no-overlap': 'no overlap', 'repeat-top': 'shared top line', 'repeat-bottom': 'shared bottom line', 'equal-gap-x': 'equal horizontal gap', 'equal-gap-y': 'equal vertical gap', 'column-left': 'grid column (left)', 'column-text': 'text column', 'column-right': 'grid column (right)',
};

// The kind used for a cause created from a violation.
export const intentKind = (kinds) => {
  if (kinds.includes('no-overlap')) return 'intent-overlap';
  if (kinds.some((k) => k.startsWith('equal-width') || k.startsWith('equal-height'))) return 'intent-size';
  if (kinds.some((k) => k.startsWith('column-') || k.startsWith('repeat-'))) return 'intent-column';
  if (kinds.some((k) => k.startsWith('equal-gap'))) return 'intent-gap';
  return 'intent-align';
};
