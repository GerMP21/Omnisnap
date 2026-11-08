// Concept 5: novelty across a crawl (unsupervised).
// Every cause gets a FINGERPRINT (engine families, mechanism, kind, element
// role, perceptual events). Across the whole corpus (real + own + crawl),
// fingerprints that occur everywhere (form controls, line-height rounding)
// carry little information; rare ones are surprising. This is inverse
// document frequency for engine differences: surprise = -log2(frequency).
import { FAMILY } from './consensus.mjs';

const role = (f) => {
  if (['input', 'select', 'textarea', 'button'].includes(f.tag)) return 'control';
  if (f.tag === 'a') return 'link';
  if (/^h[1-6]$/.test(f.tag)) return 'heading';
  if (['img', 'svg', 'video', 'canvas', 'picture', 'iframe'].includes(f.tag)) return 'media';
  return f.text ? 'text' : 'container';
};
const kindClass = (k) => (k.startsWith('only-in-') || k === 'children-differ' ? 'presence' : k.startsWith('hidden-in-') ? 'visibility' : k);

export function fingerprint(f, pair) {
  const ev = f.evidence || {};
  return [
    `blink~${FAMILY[pair] || pair}`,
    ev.mechanism?.verdict || 'unknown',
    [...new Set(f.kinds.map(kindClass))].sort().join('+'),
    role(f),
    [...new Set((ev.perception?.events || []).map((e) => e.type))].sort().join('+') || 'no-events',
  ].join('|');
}

// counts: Map fingerprint -> { n: causes, pages: Set of page slugs } over
// the corpus; total: corpus size in causes. otherPages counts the OTHER
// sites showing the same fingerprint: a difference that shows up on many
// unrelated sites is baseline engine behaviour, not this page's bug.
export function surprise(fp, counts, total, slug) {
  const e = counts.get(fp) || { n: 0, pages: new Set() };
  const otherPages = [...e.pages].filter((s) => s !== slug).length;
  return { count: e.n, otherPages, total, bits: total ? -Math.log2(Math.max(e.n, 1) / total) : 0 };
}
