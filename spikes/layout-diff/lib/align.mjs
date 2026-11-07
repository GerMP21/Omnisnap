// Element matching and root-cause analysis between two geometry trees
// (output of inject/extract.js). Dependency-free on purpose: this module is
// the algorithm the Go core will port.

export const TOL = {
  abs: 2, // px a box size may differ before it counts as changed...
  rel: 0.01, // ...or this fraction of the larger size, whichever is bigger
  pos: 2, // px a parent-relative position may shift
  ovf: 2, // px of content beyond a box before it counts as overflowing
  ovfDelta: 4, // ...and the overflow itself must change by more than this
};

// ---------------------------------------------------------------- tree prep

export function prep(tree, engine) {
  let uid = 0;
  const all = [];
  const visit = (n, parent) => {
    n.uid = uid++;
    n.parent = parent;
    n.k = n.k || [];
    all.push(n);
    for (const c of n.k) visit(c, n);
  };
  visit(tree.root, null);
  tree.all = all;
  tree.engine = engine;
  return tree;
}

export function* subtree(n) {
  yield n;
  for (const c of n.k) yield* subtree(c);
}

// Human-readable selector. nth-of-type counts rendered siblings only, so it is
// a label for people, not a guaranteed querySelector.
function segment(n) {
  if (n.id) return `${n.t}#${n.id}`;
  let s = n.t;
  if (n.c) s += n.c.split(' ').filter(Boolean).slice(0, 2).map((c) => `.${c}`).join('');
  const p = n.parent;
  if (p) {
    const same = p.k.filter((x) => x.t === n.t);
    if (same.length > 1) s += `:nth-of-type(${same.indexOf(n) + 1})`;
  }
  return s;
}

export function selector(n) {
  const parts = [];
  for (let cur = n; cur && parts.length < 5; cur = cur.parent) {
    parts.unshift(segment(cur));
    if (cur.id || cur.t === 'body' || cur.t === 'html') break;
  }
  return parts.join(' > ');
}

// ----------------------------------------------------------------- matching

// Strongest available identity for an element; LCS aligns sibling lists on it.
function token(n) {
  if (n.id) return `i:${n.id}`;
  if (n.tid) return `t:${n.tid}`;
  if (n.u) return `${n.t}|u:${n.u}`;
  if (n.n) return `${n.t}|n:${n.n}`;
  if (n.th) return `${n.t}|h:${n.th}`;
  return `${n.t}|c:${n.c || ''}`;
}

function lcsPairs(ta, tb) {
  const n = ta.length;
  const m = tb.length;
  if (!n || !m) return [];
  if (n * m > 4e6) { // huge sibling lists: greedy in-order match with lookahead
    const pairs = [];
    let j = 0;
    for (let i = 0; i < n && j < m; i++) {
      let k = j;
      while (k < m && k - j < 50 && tb[k] !== ta[i]) k++;
      if (k < m && tb[k] === ta[i]) { pairs.push([i, k]); j = k + 1; }
    }
    return pairs;
  }
  const W = m + 1;
  const dp = new Uint32Array((n + 1) * W);
  for (let i = n - 1; i >= 0; i--) {
    for (let j = m - 1; j >= 0; j--) {
      dp[i * W + j] = ta[i] === tb[j] ? dp[(i + 1) * W + j + 1] + 1 : Math.max(dp[(i + 1) * W + j], dp[i * W + j + 1]);
    }
  }
  const pairs = [];
  let i = 0;
  let j = 0;
  while (i < n && j < m) {
    if (ta[i] === tb[j]) { pairs.push([i, j]); i++; j++; } else if (dp[(i + 1) * W + j] >= dp[i * W + j + 1]) i++; else j++;
  }
  return pairs;
}

// Anchors via LCS on identity tokens; leftovers between anchors are paired in
// order by tag+class, then by tag alone (rescues dynamic ids and changed text).
function alignChildren(ka, kb) {
  const anchors = lcsPairs(ka.map(token), kb.map(token));
  const pairs = [...anchors];
  const bounds = [[-1, -1], ...anchors, [ka.length, kb.length]];
  for (let g = 0; g < bounds.length - 1; g++) {
    const [i0, j0] = bounds[g];
    const [i1, j1] = bounds[g + 1];
    const ga = [];
    const gb = [];
    for (let i = i0 + 1; i < i1; i++) ga.push(i);
    for (let j = j0 + 1; j < j1; j++) gb.push(j);
    if (!ga.length || !gb.length) continue;
    for (const key of [(n) => `${n.t}.${n.c || ''}`, (n) => n.t]) {
      let from = 0;
      for (let x = 0; x < ga.length; x++) {
        if (ga[x] < 0) continue;
        for (let y = from; y < gb.length; y++) {
          if (gb[y] < 0) continue;
          if (key(ka[ga[x]]) === key(kb[gb[y]])) {
            pairs.push([ga[x], gb[y]]);
            ga[x] = -1;
            gb[y] = -1;
            from = y + 1;
            break;
          }
        }
      }
    }
  }
  return pairs;
}

export function matchTrees(A, B) {
  const pairOf = new Map(); // A uid -> B node
  const revPair = new Map(); // B uid -> A node
  const onlyA = []; // topmost A nodes with no partner (their parent matched)
  const onlyB = [];
  const stack = [[A.root, B.root]];
  while (stack.length) {
    const [a, b] = stack.pop();
    pairOf.set(a.uid, b);
    revPair.set(b.uid, a);
    const pairs = alignChildren(a.k, b.k);
    const usedA = new Set(pairs.map((p) => p[0]));
    const usedB = new Set(pairs.map((p) => p[1]));
    a.k.forEach((c, i) => { if (!usedA.has(i)) onlyA.push(c); });
    b.k.forEach((c, j) => { if (!usedB.has(j)) onlyB.push(c); });
    for (const [i, j] of pairs) stack.push([a.k[i], b.k[j]]);
  }
  return { pairOf, revPair, onlyA, onlyB };
}

// ------------------------------------------------------------------- deltas

function changed(va, vb, abs, rel) {
  const d = vb - va;
  const limit = Math.max(abs, rel * Math.max(Math.abs(va), Math.abs(vb)));
  return Math.abs(d) > limit ? Math.round(d * 10) / 10 : 0;
}

// Nearest ancestor with a real box (display: contents has none).
function boxParent(n) {
  let p = n.parent;
  while (p && p.b[2] === 0 && p.b[3] === 0 && p.parent) p = p.parent;
  return p;
}

function deltas(a, b, M) {
  // An inline box's height is the font's content area (ascent + descent),
  // which engines compute differently for the SAME font; it never moves the
  // layout (line-height does). Ignore it, and give inline tops a tolerance
  // proportional to that content area.
  const inline = a.d === 'inline' && b.d === 'inline';
  // dw/dh decide whether a size change is REPORTED (relative tolerance, so a
  // 4px change on an 800px box is not news); rw/rh decide whether it EXPLAINS
  // something else (absolute only, since that 4px still moves everything below).
  const d = {
    dw: changed(a.b[2], b.b[2], TOL.abs, TOL.rel),
    dh: inline ? 0 : changed(a.b[3], b.b[3], TOL.abs, TOL.rel),
    rw: changed(a.b[2], b.b[2], TOL.abs, 0),
    rh: inline ? 0 : changed(a.b[3], b.b[3], TOL.abs, 0),
    dx: 0,
    dy: 0,
    dln: 0,
    ovf: '',
  };
  const pa = boxParent(a);
  const pb = pa ? M.pairOf.get(pa.uid) : null;
  if (pa && pb) {
    d.dx = changed(a.b[0] - pa.b[0], b.b[0] - pb.b[0], TOL.pos, 0);
    const yTol = inline ? Math.max(TOL.pos, 0.2 * Math.max(a.b[3], b.b[3])) : TOL.pos;
    d.dy = changed(a.b[1] - pa.b[1], b.b[1] - pb.b[1], yTol, 0);
  }
  if (a.ln && b.ln && a.ln !== b.ln) d.dln = b.ln - a.ln;
  d.vis = Boolean(a.hid) !== Boolean(b.hid);
  // Text spilling 1-3px past a tight box is font-metric noise; an overflow
  // counts only if it appears/disappears AND changes by more than ovfDelta.
  const oa = a.o || [0, 0];
  const ob = b.o || [0, 0];
  const flips = (x, y) => (x > TOL.ovf) !== (y > TOL.ovf) && Math.abs(x - y) > TOL.ovfDelta;
  if (flips(oa[0], ob[0])) d.ovf += 'x';
  if (flips(oa[1], ob[1])) d.ovf += 'y';
  d.any = Boolean(d.dw || d.dh || d.dx || d.dy || d.dln || d.ovf || d.vis);
  return d;
}

// ------------------------------------------------------------- root causes

// Decide which changed elements are CAUSES and which only moved or resized
// because of something else (EFFECTS). Rules, in plain words:
//  - a child whose parent changed width is reflowing: effect
//  - a box that grew because a child grew/wrapped/moved: effect
//  - an item that moved because a sibling resized or an earlier sibling moved
//    the same way: effect
//  - children that all moved with none resized: one cause on the parent
//    (padding, gap, order, justify)
//  - in flex/grid rows, a box stretched like its siblings: effect
//  - overflow explained by a descendant's width/overflow: symptom, not cause
//  - an element present in only one engine: cause
export function analyze(A, B, M, maskA = new Set(), maskB = new Set()) {
  const D = new Map();
  for (const a of A.all) {
    const b = M.pairOf.get(a.uid);
    if (b) D.set(a.uid, deltas(a, b, M));
  }
  const unmatchedUnder = new Set(); // A uids whose children differ in membership
  for (const n of M.onlyA) unmatchedUnder.add(n.parent.uid);
  for (const n of M.onlyB) {
    const pa = M.revPair.get(n.parent.uid);
    if (pa) unmatchedUnder.add(pa.uid);
  }

  const sized = (n) => { const d = D.get(n.uid); return Boolean(d && (d.rw || d.rh || d.dln)); };
  const moved = (n) => { const d = D.get(n.uid); return Boolean(d && (d.dx || d.dy)); };
  // Inert: takes no space (zero area), or is transparent AND out of flow.
  // A transparent element that still occupies space is NOT inert: it pushes
  // its siblings around (Stripe: a fading-in panel expanded in Chrome,
  // collapsed in Firefox, was the biggest difference on the page).
  const noBox = (n) => n.b[2] === 0 || n.b[3] === 0 || (n.hid && (n.p === 'absolute' || n.p === 'fixed'));
  const invisible = (a) => { const b = M.pairOf.get(a.uid); return noBox(a) && (!b || noBox(b)); };
  const isMasked = (a) => maskA.has(a.uid) || (M.pairOf.has(a.uid) && maskB.has(M.pairOf.get(a.uid).uid));

  const flagMemo = new Map();
  const descFlags = (n) => {
    if (flagMemo.has(n.uid)) return flagMemo.get(n.uid);
    const f = { w: false, h: false, ox: false, oy: false };
    for (const c of n.k) {
      const d = D.get(c.uid);
      if (d) {
        f.w ||= Boolean(d.dw);
        f.h ||= Boolean(d.dh || d.dln);
        f.ox ||= d.ovf.includes('x');
        f.oy ||= d.ovf.includes('y');
      }
      const cf = descFlags(c);
      f.w ||= cf.w; f.h ||= cf.h; f.ox ||= cf.ox; f.oy ||= cf.oy;
    }
    flagMemo.set(n.uid, f);
    return f;
  };

  const movesKids = new Set();
  for (const a of A.all) {
    if (!D.has(a.uid) || unmatchedUnder.has(a.uid)) continue;
    const kids = a.k.filter((c) => D.has(c.uid) && !invisible(c));
    if (!kids.length || kids.some(sized)) continue;
    const m = kids.filter(moved).length;
    if (m >= 2 || (m === 1 && kids.length === 1)) movesKids.add(a.uid);
  }

  // Does child c keep the same offset or the same proportion to parent p in
  // both engines? Then its width just follows the parent (block fill,
  // percentage widths, SVG viewBox scaling).
  const tracks = (c, p) => {
    const cb = M.pairOf.get(c.uid);
    const pb = M.pairOf.get(p.uid);
    if (!cb || !pb) return false;
    if (Math.abs((pb.b[2] - cb.b[2]) - (p.b[2] - c.b[2])) <= TOL.abs) return true;
    return p.b[2] > 0 && pb.b[2] > 0 && Math.abs(cb.b[2] / pb.b[2] - c.b[2] / p.b[2]) <= TOL.rel;
  };
  const textChanged = (a) => {
    const b = M.pairOf.get(a.uid);
    return Boolean(b && a.tw != null && b.tw != null && changed(a.tw, b.tw, TOL.abs, TOL.rel));
  };
  // Width changes flow DOWN (parent resized, child follows) or UP (content
  // got wider, a shrink-to-fit parent follows). Only the source is a cause.
  const widthCause = (a, d, p, pd, kids, sibs) => {
    if (!d.dw) return false;
    if (pd && pd.rw && tracks(a, p)) return false; // follows its parent
    if (kids.some((k) => D.get(k.uid).rw && !tracks(k, a))) return false; // pushed by its content
    if (textChanged(a)) return true; // its own text got wider or narrower
    if (pd && Math.sign(pd.rw) === Math.sign(d.dw)) return false; // share of a resized parent (table cells, flex-grow)
    if (p && /flex|grid/.test(p.d || '') && sibs.some((s) => D.get(s.uid).rw)) return false; // redistributed by a sibling
    if (unmatchedUnder.has(a.uid)) return false; // children added or removed
    return true; // explicitly sized differently
  };

  const causes = [];
  const symptoms = [];
  let explained = 0;
  let masked = 0;

  for (const a of A.all) {
    const d = D.get(a.uid);
    if (!d || invisible(a)) continue;
    if (!d.any && !movesKids.has(a.uid)) continue;
    const p = a.parent;
    const pd = p ? D.get(p.uid) : null;
    const parentReflow = Boolean(pd && pd.rw);
    const rowish = Boolean(p && /flex|grid/.test(p.d || ''));
    const sibs = p ? p.k.filter((s) => s !== a && D.has(s.uid)) : [];
    const kids = a.k.filter((c) => D.has(c.uid));
    const hasUnmatched = unmatchedUnder.has(a.uid);
    const kinds = [];

    // Inline flow cascades forward: once one piece of a line changes, every
    // later inline box in that block moves. Only the first one is a cause.
    if (p && /^inline/.test(a.d || '')) {
      const idx = p.k.indexOf(a);
      if ((pd && (pd.dln || textChanged(p))) || p.k.slice(0, idx).some((s) => D.get(s.uid)?.any)) { explained++; continue; }
    }

    // Shown in one engine, transparent/hidden in the other. visibility is
    // inherited, so only the topmost element whose state flips is a cause.
    if (d.vis && !(pd && pd.vis)) kinds.push(`hidden-in-${a.hid ? A.engine : B.engine}`);
    if (d.dln && !d.rw) kinds.push('wrap');
    if (widthCause(a, d, p, pd, kids, sibs)) kinds.push('width');
    if (d.dh && !d.dln && !d.rw && !parentReflow && !kids.some(sized) && !kids.some(moved) && !hasUnmatched) {
      const stretched = rowish && sibs.some((s) => Math.sign(D.get(s.uid).rh) === Math.sign(d.dh));
      if (!stretched) kinds.push('height');
    }
    if (movesKids.has(a.uid)) kinds.push('children-moved');
    if ((d.dx || d.dy) && !parentReflow && !(p && movesKids.has(p.uid))) {
      const idx = p ? p.k.indexOf(a) : 0;
      const sibSized = sibs.some(sized) || Boolean(p && unmatchedUnder.has(p.uid));
      const earlierSame = sibs.some((s) => {
        if (p.k.indexOf(s) > idx) return false;
        const sd = D.get(s.uid);
        return (d.dy && Math.sign(sd.dy) === Math.sign(d.dy)) || (d.dx && Math.sign(sd.dx) === Math.sign(d.dx));
      });
      // Right- or center-aligned boxes move when they themselves grow.
      const xOwn = !d.dx || (d.rw && Math.abs(d.dx) <= Math.abs(d.rw) + TOL.pos);
      const yOwn = !d.dy || (d.rh && Math.abs(d.dy) <= Math.abs(d.rh) + TOL.pos);
      if (!sibSized && !earlierSame && !(xOwn && yOwn)) kinds.push('position');
    }
    if (d.ovf) {
      const f = descFlags(a);
      const sel = selector(a);
      if (a.t === 'html') symptoms.push({ sel, kind: `page-overflow-${d.ovf}` });
      else {
        if (d.ovf.includes('x')) { if (!f.w && !f.ox) kinds.push('overflow-x'); else symptoms.push({ sel, kind: 'overflow-x' }); }
        if (d.ovf.includes('y')) { if (!f.h && !f.oy) kinds.push('overflow-y'); else symptoms.push({ sel, kind: 'overflow-y' }); }
      }
    }

    if (!kinds.length) { explained++; continue; }
    if (isMasked(a)) { masked++; continue; }
    const note = kinds.some((k) => k === 'height' || k === 'children-moved') ? pitchNote(a, M.pairOf.get(a.uid)) : null;
    causes.push({ side: 'A', node: a, other: M.pairOf.get(a.uid), kinds, d, note });
  }

  // Elements present in one engine only, grouped by their (matched) parent.
  // Extras on BOTH sides, or more than 3, mean the content itself differs
  // (a swapped logo, a rotated list): one finding on the parent, not N.
  const extras = new Map(); // A parent uid -> { a: [], b: [] }
  const bucket = (uid) => { if (!extras.has(uid)) extras.set(uid, { a: [], b: [] }); return extras.get(uid); };
  for (const n of M.onlyA) {
    if (noBox(n)) continue;
    if (maskA.has(n.uid)) { masked++; continue; }
    bucket(n.parent.uid).a.push(n);
  }
  for (const n of M.onlyB) {
    if (noBox(n)) continue;
    if (maskB.has(n.uid)) { masked++; continue; }
    const pa = M.revPair.get(n.parent.uid);
    if (pa) bucket(pa.uid).b.push(n);
  }
  const byUid = new Map(A.all.map((n) => [n.uid, n]));
  for (const [uid, { a: ea, b: eb }] of extras) {
    if ((ea.length && eb.length) || ea.length + eb.length > 3) {
      const parent = byUid.get(uid);
      if (isMasked(parent)) { masked++; continue; }
      causes.push({ side: 'A', node: parent, other: M.pairOf.get(uid), kinds: ['children-differ'], d: D.get(uid) || null,
        note: `${ea.length} child element(s) only in ${A.engine}, ${eb.length} only in ${B.engine}` });
      continue;
    }
    for (const n of ea) causes.push({ side: 'A', node: n, other: null, kinds: [`only-in-${A.engine}`], d: null });
    for (const n of eb) causes.push({ side: 'B', node: n, other: null, kinds: [`only-in-${B.engine}`], d: null });
  }
  return { D, causes, symptoms, explained, masked };
}

// Distance between consecutive text lines inside an element. Engines round
// fractional line-heights differently (WebKit lays 22.75px lines out at 22px),
// and the difference compounds with every line: 35 terminal lines -> 24px.
export function linePitch(n) {
  if (n.tx && !n.k.length && n.ln > 1) return { pitch: n.b[3] / n.ln, lines: n.ln };
  const centers = [];
  const visit = (x) => { if (x.tx && !x.hid && x.b[3] > 0) centers.push(x.b[1] + x.b[3] / 2); x.k.forEach(visit); };
  visit(n);
  centers.sort((p, q) => p - q);
  const rows = [];
  for (const c of centers) if (!rows.length || c - rows[rows.length - 1] > 4) rows.push(c);
  if (rows.length < 3) return null;
  return { pitch: (rows[rows.length - 1] - rows[0]) / (rows.length - 1), lines: rows.length };
}

function pitchNote(a, b) {
  if (!b) return null;
  const pa = linePitch(a);
  const pb = linePitch(b);
  if (!pa || !pb || pa.lines !== pb.lines || Math.abs(pa.pitch - pb.pitch) < 0.2) return null;
  return `line pitch ${pa.pitch.toFixed(1)} -> ${pb.pitch.toFixed(1)}px over ${pa.lines} lines (fractional line-height rounded differently)`;
}

// The largest ancestor that grew/shrank the same way: "-> section.pricing +24px".
export function effectOf(cause, D) {
  const start = cause.side === 'A' ? cause.node : null;
  if (!start) return null;
  const own = D.get(start.uid);
  let sign = own ? Math.sign(own.dh) : 0;
  let top = null;
  for (let p = start.parent; p && p.t !== 'body' && p.t !== 'html'; p = p.parent) {
    const pd = D.get(p.uid);
    if (!pd || !pd.dh) break;
    if (!sign) sign = Math.sign(pd.dh);
    if (Math.sign(pd.dh) !== sign) break;
    top = p;
  }
  return top ? { sel: selector(top), dh: D.get(top.uid).dh } : null;
}

// Elements that change between two runs of the SAME engine are dynamic
// (ads, carousels, timestamps). Returns the uids to ignore in X.
export function dynamicMask(X, X2) {
  const M = matchTrees(X, X2);
  const r = analyze(X, X2, M);
  const mask = new Set();
  for (const c of r.causes) {
    if (c.side === 'A') for (const s of subtree(c.node)) mask.add(s.uid);
    else {
      const pa = M.revPair.get(c.node.parent.uid);
      if (pa) mask.add(pa.uid);
    }
  }
  return mask;
}

export function severity(kinds, d) {
  if (kinds.some((k) => k.startsWith('only-in-') || k.startsWith('hidden-in-') || k === 'overflow-x')) return 3;
  if (kinds.includes('wrap') || kinds.includes('overflow-y')) return 2;
  if (d && Math.max(Math.abs(d.dw), Math.abs(d.dh), Math.abs(d.dx), Math.abs(d.dy)) >= 10) return 2;
  return 1;
}
