// Concept 1: diagnose by experiment (causal interventions).
// At capture time each engine re-measured the page under neutralizing
// probes (inject/probes.js). Here: does a cross-engine difference survive
// each probe? The probe that removes it names its mechanism, and it doubles
// as fix verification (e.g. "whole-pixel line-height" IS the K1 fix).
import fs from 'node:fs/promises';
import zlib from 'node:zlib';
import { deltas, matchTrees, prep } from './align.mjs';

export const PROBE_ORDER = ['lh', 'font', 'controls'];
export const PROBE_MEANING = {
  lh: 'fractional or font-derived line-height (fix: whole-pixel line-height)',
  font: 'text metrics: advance, shaping or font metrics (no fix needed unless it wraps or clips)',
  controls: 'native form-control defaults (fix: explicit control styling and size)',
  all: 'a combination of text metrics, line-height and control defaults',
  layout: 'persists with text, line-height and controls neutralized: a layout or author-CSS difference',
  'author-font': 'the font probe removes it, but the page asks for DIFFERENT fonts in each engine (author CSS or a failed web font): worth reporting',
};

export async function loadProbes(file, engine) {
  try {
    const raw = JSON.parse(zlib.gunzipSync(await fs.readFile(file)));
    const out = {};
    for (const [id, geo] of Object.entries(raw)) out[id] = prep(geo, engine);
    return out;
  } catch (_) {
    return null;
  }
}

// Precompute matchings once per engine pair: base->probe for each engine,
// and probe(A)->probe(B) for the cross-engine comparison under the probe.
export function probeContext(A, B, PA, PB, M) {
  if (!PA || !PB) return null;
  const ctx = { M };
  for (const id of Object.keys(PA)) {
    if (!PB[id] || id === 'M') continue;
    ctx[id] = { MA: matchTrees(A, PA[id]), MB: matchTrees(B, PB[id]), MP: matchTrees(PA[id], PB[id]) };
  }
  return ctx;
}

// Does this cause's difference survive probe `id`? For "children moved"
// the difference lives in the children (their boxes relative to the
// parent), for "children differ" in which children exist.
function survives(c, p, M) {
  const aNode = c.side === 'A' ? c.node : null;
  const bNode = c.side === 'A' ? c.other : c.node;
  if (aNode && bNode) {
    const ap = p.MA.pairOf.get(aNode.uid);
    const bp = p.MB.pairOf.get(bNode.uid);
    if (!ap || !bp) return null;
    const kinds = c.kinds || [];
    if (kinds.includes('children-differ')) {
      const shown = (n) => n.k.filter((k) => !k.hid).length;
      return shown(ap) !== shown(bp);
    }
    const own = kinds.some((k) => !k.startsWith('children'));
    if (own && deltas(ap, bp, p.MP).any) return true;
    if (kinds.includes('children-moved')) {
      let tested = 0;
      for (const k of aNode.k.slice(0, 40)) {
        const kb = M.pairOf.get(k.uid);
        const kp = p.MA.pairOf.get(k.uid);
        const kbp = kb && p.MB.pairOf.get(kb.uid);
        if (!kp || !kbp) continue;
        tested++;
        if (deltas(kp, kbp, p.MP).any) return true;
      }
      return tested ? false : null;
    }
    return own ? false : null;
  }
  // Present in one engine only: does it get a partner under the probe?
  if (aNode) { const ap = p.MA.pairOf.get(aNode.uid); return ap ? !p.MP.pairOf.has(ap.uid) : null; }
  const bp = p.MB.pairOf.get(bNode.uid);
  return bp ? !p.MP.revPair.has(bp.uid) : null;
}

export function mechanismOf(c, pctx) {
  if (!pctx) return { verdict: 'unknown', reason: 'no probes captured' };
  const results = {};
  for (const id of [...PROBE_ORDER, 'all']) if (pctx[id]) results[id] = survives(c, pctx[id], pctx.M);
  const gone = PROBE_ORDER.find((id) => results[id] === false);
  // The font probe also erases a genuinely different font-family (author
  // CSS per engine, a web font that failed in one engine): not expected.
  if (gone === 'font' && fontsDiffer(c, pctx.M)) return { verdict: 'author-font', explanation: PROBE_MEANING['author-font'], results, verified: true };
  if (gone) return { verdict: gone, explanation: PROBE_MEANING[gone], results, verified: true };
  if (results.all === false) return { verdict: 'all', explanation: PROBE_MEANING.all, results, verified: true };
  if (results.all === true) return { verdict: 'layout', explanation: PROBE_MEANING.layout, results, verified: false };
  return { verdict: 'unknown', results };
}

function fontsDiffer(c, M) {
  const a = c.side === 'A' ? c.node : null;
  if (!a || !M) return false;
  const stack = [a];
  for (let seen = 0; stack.length && seen < 60; seen++) {
    const n = stack.pop();
    const b = M.pairOf.get(n.uid);
    if (n.ff && b && b.ff && n.ff !== b.ff) return true;
    stack.push(...n.k);
  }
  return false;
}
