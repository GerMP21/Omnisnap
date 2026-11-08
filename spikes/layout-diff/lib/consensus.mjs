// Concept 4: engine consensus (differential testing).
// With a third engine FAMILY (Blink, Gecko, WebKit; the Blink family counts
// once), decide who deviates for each cause:
//   B-odd      the third engine agrees with A (baseline): B is the outlier
//   A-odd      the third engine agrees with B: the BASELINE is the outlier
//   all-differ every engine differs: normal engine variation
//   unknown    no third engine, or the element is missing there
export const FAMILY = { chrome: 'blink', chromium: 'blink', brave: 'blink', edge: 'blink', firefox: 'gecko', webkit: 'webkit' };

const close = (p, q, rel = 0.01, abs = 2) => Math.abs(p - q) <= Math.max(abs, rel * Math.max(Math.abs(p), Math.abs(q)));
// Same rendered geometry: size, position relative to the parent, line count, visibility.
export function agree(x, y) {
  if (!x || !y) return false;
  if (Boolean(x.hid) !== Boolean(y.hid)) return false;
  if (!close(x.b[2], y.b[2]) || !close(x.b[3], y.b[3])) return false;
  if (x.ln && y.ln && x.ln !== y.ln) return false;
  if (x.parent && y.parent) {
    if (!close(x.b[0] - x.parent.b[0], y.b[0] - y.parent.b[0], 0, 2)) return false;
    if (!close(x.b[1] - x.parent.b[1], y.b[1] - y.parent.b[1], 0, 2)) return false;
  }
  return true;
}

// thirds: [{ engine, T, M }] where M matches the baseline tree A to T.
export function consensusOf(c, thirds) {
  if (!thirds.length) return { verdict: 'unknown', reason: 'no third engine family' };
  const a = c.side === 'A' ? c.node : null;
  const b = c.side === 'A' ? c.other : c.node;
  const votes = [];
  for (const { engine, M } of thirds) {
    if (!a) { votes.push({ engine, verdict: 'unknown' }); continue; }
    const y = M.pairOf.get(a.uid);
    if (!b) { // present in A only
      votes.push({ engine, verdict: y ? 'B-odd' : 'A-odd' });
      continue;
    }
    if (!y) { votes.push({ engine, verdict: 'unknown' }); continue; }
    const withA = agree(a, y);
    const withB = agree(b, y);
    votes.push({ engine, verdict: withA && !withB ? 'B-odd' : withB && !withA ? 'A-odd' : !withA && !withB ? 'all-differ' : 'ambiguous' });
  }
  const decided = votes.filter((v) => v.verdict !== 'unknown' && v.verdict !== 'ambiguous');
  if (!decided.length) return { verdict: 'unknown', votes };
  // Several third families: they must agree on who deviates.
  const first = decided[0].verdict;
  return { verdict: decided.every((v) => v.verdict === first) ? first : 'all-differ', votes };
}
