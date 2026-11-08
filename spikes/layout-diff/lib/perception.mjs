// Concept 2: model what the eye notices (psychophysics + Gestalt).
// Compares QUALITATIVE relations between neighbouring boxes in both engines
// instead of raw pixel deltas:
//   - alignment broken: edges collinear in A (<= 0.5px) but >= 1.5px apart in
//     B. Vernier hyperacuity makes broken collinearity visible at 1-2px.
//   - overlap created: boxes that did not intersect now do.
//   - order changed: left/right or above/below order flips.
//   - protrudes parent: a child that fit inside its parent now sticks out.
//   - spacing changed: the gap to a neighbour changes by >= 4px and >= 50%
//     (Gestalt proximity: grouping can split).
//   - size noticeable: >= 8% size change (Weber fraction for length).
// Inline boxes are skipped: they flow with text, their relations are not
// design decisions.

const EDGES = {
  left: (b) => b[0], right: (b) => b[0] + b[2], top: (b) => b[1], bottom: (b) => b[1] + b[3],
  centerX: (b) => b[0] + b[2] / 2, centerY: (b) => b[1] + b[3] / 2,
};
export const EVENT_WEIGHT = {
  'overlap-created': 3, 'alignment-broken': 2.5, 'order-changed': 2.5, 'protrudes-parent': 2,
  'spacing-changed': 1, 'size-noticeable': 1,
};

const visible = (n) => n && !n.hid && n.b[2] > 0 && n.b[3] > 0 && n.d !== 'inline';
const inter = (p, q) => Math.max(0, Math.min(p[0] + p[2], q[0] + q[2]) - Math.max(p[0], q[0])) * Math.max(0, Math.min(p[1] + p[3], q[1] + q[3]) - Math.max(p[1], q[1]));
const gap = (p, q) => {
  const dx = Math.max(0, Math.max(p[0], q[0]) - Math.min(p[0] + p[2], q[0] + q[2]));
  const dy = Math.max(0, Math.max(p[1], q[1]) - Math.min(p[1] + p[3], q[1] + q[3]));
  return Math.hypot(dx, dy);
};

// Relation events between two element pairs (x in A ~ xb in B, y in A ~ yb in B).
function pairEvents(x, xb, y, yb) {
  const ev = [];
  for (const [k, f] of Object.entries(EDGES)) {
    if (Math.abs(f(x.b) - f(y.b)) <= 0.5 && Math.abs(f(xb.b) - f(yb.b)) >= 1.5) ev.push({ type: 'alignment-broken', edge: k });
  }
  const minArea = Math.min(x.b[2] * x.b[3], y.b[2] * y.b[3]);
  if (inter(x.b, y.b) <= 1 && inter(xb.b, yb.b) >= Math.max(4, 0.01 * minArea)) ev.push({ type: 'overlap-created' });
  const sameRowA = inter([0, x.b[1], 1, x.b[3]], [0, y.b[1], 1, y.b[3]]) > 0;
  const sameColA = inter([x.b[0], 0, x.b[2], 1], [y.b[0], 0, y.b[2], 1]) > 0;
  const cx = (b) => b[0] + b[2] / 2;
  const cy = (b) => b[1] + b[3] / 2;
  if (sameRowA && Math.abs(cx(x.b) - cx(y.b)) > 2 && Math.abs(cx(xb.b) - cx(yb.b)) > 2 && Math.sign(cx(x.b) - cx(y.b)) !== Math.sign(cx(xb.b) - cx(yb.b))) ev.push({ type: 'order-changed', axis: 'x' });
  if (sameColA && Math.abs(cy(x.b) - cy(y.b)) > 2 && Math.abs(cy(xb.b) - cy(yb.b)) > 2 && Math.sign(cy(x.b) - cy(y.b)) !== Math.sign(cy(xb.b) - cy(yb.b))) ev.push({ type: 'order-changed', axis: 'y' });
  const gA = gap(x.b, y.b);
  const gB = gap(xb.b, yb.b);
  if (Math.abs(gB - gA) >= 4 && Math.max(gA, gB) / Math.max(1, Math.min(gA, gB)) >= 1.5 && Math.min(gA, gB) < 64) ev.push({ type: 'spacing-changed', from: Math.round(gA), to: Math.round(gB) });
  return ev;
}

function protrudes(child, parent, childB, parentB) {
  const inside = (c, p) => c[0] >= p[0] - 1 && c[1] >= p[1] - 1 && c[0] + c[2] <= p[0] + p[2] + 1 && c[1] + c[3] <= p[1] + p[3] + 1;
  return inside(child.b, parent.b) && !inside(childB.b, parentB.b)
    && Math.max(parentB.b[0] - childB.b[0], childB.b[0] + childB.b[2] - parentB.b[0] - parentB.b[2], childB.b[1] + childB.b[3] - parentB.b[1] - parentB.b[3]) >= 2;
}

export function perceptualEvents(c, M) {
  const a = c.side === 'A' ? c.node : null;
  const b = c.side === 'A' ? c.other : null;
  if (!a || !b) return { events: [], score: null }; // presence: nothing to perceive geometrically
  const events = [];
  const seen = new Set();
  const add = (e, who) => { const key = `${e.type}|${e.edge || e.axis || ''}`; if (!seen.has(key)) { seen.add(key); events.push({ ...e, with: who }); } };
  // 1. The cause against its siblings.
  if (visible(a) && a.parent) {
    for (const s of a.parent.k.slice(0, 80)) {
      const sb = M.pairOf.get(s.uid);
      if (s === a || !visible(s) || !visible(sb)) continue;
      for (const e of pairEvents(a, b, s, sb)) add(e, s);
    }
    const pb = M.pairOf.get(a.parent.uid);
    if (pb && visible(a.parent) && protrudes(a, a.parent, b, pb)) add({ type: 'protrudes-parent' }, a.parent);
  }
  // 2. For "children moved/differ" causes, relations AMONG its children.
  if ((c.kinds || []).some((k) => k.startsWith('children'))) {
    const kids = a.k.filter((k) => visible(k) && visible(M.pairOf.get(k.uid))).slice(0, 40);
    for (let i = 0; i < kids.length; i++) {
      for (let j = i + 1; j < kids.length; j++) {
        for (const e of pairEvents(kids[i], M.pairOf.get(kids[i].uid), kids[j], M.pairOf.get(kids[j].uid))) add(e, kids[j]);
      }
    }
  }
  // 3. Weber: is the size change itself noticeable?
  const d = c.d || {};
  if ((Math.abs(d.dw || 0) >= 2 && Math.abs(d.dw) / Math.max(1, a.b[2]) >= 0.08) || (Math.abs(d.dh || 0) >= 2 && Math.abs(d.dh) / Math.max(1, a.b[3]) >= 0.08)) add({ type: 'size-noticeable' }, null);
  const score = events.reduce((m, e) => Math.max(m, EVENT_WEIGHT[e.type] || 0), 0);
  return { events: events.map(({ with: w, ...e }) => e), score };
}
