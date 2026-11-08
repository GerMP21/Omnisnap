// Concept 7: a judge that combines every other concept.
// Each concept is a WEAK VOTER: +1 (looks like a difference worth reporting),
// -1 (looks expected or invisible), 0 (abstains). A generative label model
// (one-coin Dawid-Skene / Snorkel-style data programming) learns how
// accurate each voter is from the UNLABELED corpus by EM, then turns each
// cause's votes into P(worth reporting). No human labels are involved.

export const VOTERS = ['consequence', 'perception', 'mechanism', 'calibration', 'consensus', 'ink', 'intent', 'novelty', 'catalog'];

export function votesFor(f, novelty) {
  const ev = f.evidence || {};
  const v = {};
  // Hand-written visible consequences (lib/triage.mjs): only votes FOR.
  v.consequence = (f.consequences || []).some((c) => ['presence', 'visibility', 'clipped', 'scrollbar', 'overflow', 'protrusion', 'truncation', 'line-count', 'interactive-moved', 'structural', 'intent-broken', 'content-moved'].includes(c)) ? 1 : 0;
  // Perception: qualitative relation changes the eye notices.
  const ps = ev.perception?.score;
  v.perception = ps == null ? 0 : ps >= 2 ? 1 : ps === 0 ? -1 : 0;
  // Causal probes: surviving every neutralizing probe = a layout or
  // author-CSS difference, evidence FOR. Vanishing under a probe names the
  // mechanism and its fix, but says WHY, not WHETHER it matters: an expected
  // mechanism with a visible consequence is still worth reporting (a button
  // row that wraps because of control defaults). That judgement is already the rules-tier
  // prior (catalog + consequences), so voting against would count it twice.
  const mv = ev.mechanism?.verdict;
  v.mechanism = mv === 'layout' || mv === 'author-font' ? 1 : 0;
  // Page self-calibration: explained by the page's systematic model.
  v.calibration = ev.calibration?.explained ? -1 : 0;
  // Consensus: every engine family renders it differently = no engine is
  // "right", typical of text metrics. One odd engine says WHICH engine
  // deviates, not whether it matters (engine quirks are odd ones too), so
  // it only feeds the report, not the vote.
  v.consensus = ev.consensus?.verdict === 'all-differ' ? -1 : 0;
  // Ink: nothing visibly different in the registered crops.
  v.ink = ev.ink && ev.ink.ink >= 20 && ev.ink.observable === false ? -1 : 0;
  // Design intent: a near-miss alignment the other engines get exactly right.
  v.intent = ev.intent?.verdict === 'B-violates' || ev.intent?.verdict === 'A-violates' ? 1 : 0;
  // Novelty: the same fingerprint on >= 3 OTHER sites is baseline engine
  // behaviour (IDF). Rarity is not evidence FOR: in a small corpus almost
  // everything is rare, so rarity cannot separate bugs from noise.
  v.novelty = novelty && novelty.otherPages >= 3 ? -1 : 0;
  // Hand-written catalog of expected engine differences (lib/triage.mjs).
  v.catalog = f.kb?.expected ? -1 : 0;
  return v;
}

const clamp = (x, lo, hi) => Math.min(hi, Math.max(lo, x));
const logit = (p) => Math.log(p / (1 - p));

// One-coin Dawid-Skene, fitted by MAP-EM over unlabeled items.
// Small corpora make plain EM degenerate (the class prior collapses to 0 and
// voters flip below chance), so, as in data programming:
//  - each voter's accuracy has a Beta prior centred on `init` worth
//    `strength` pseudo-votes, and is kept >= 0.5 (every voter is assumed
//    better than chance: the identifiability assumption of Dawid-Skene);
//  - the class prior is CONDITIONAL on the round-3 rules tier (a covariate):
//    pi.report for causes the rules would report, pi.info for the rest,
//    each with a Beta prior worth `priorStrength` pseudo-items, centred on
//    what the tiers are meant to be (mostly real / mostly not). The rules
//    tier already encodes the hand-written consequences and the catalog,
//    so those two voters do not vote again (no double counting).
// With a large crawl the data dominates all priors.
export const PRIOR_VOTERS = ['consequence', 'catalog'];
const ACTIVE = VOTERS.filter((v) => !PRIOR_VOTERS.includes(v));
export function fitLabelModel(items, { iterations = 100, prior = { report: 0.6, info: 0.15 }, priorStrength = 10, init = 0.7, strength = 20 } = {}) {
  const pi = { ...prior };
  const acc = Object.fromEntries(VOTERS.map((v) => [v, init]));
  let q = [];
  for (let it = 0; it < iterations; it++) {
    q = items.map((x) => posterior(x.votes, pi[x.rules], acc));
    for (const r of ['report', 'info']) {
      const qs = q.filter((_, i) => items[i].rules === r);
      pi[r] = clamp((qs.reduce((s, x) => s + x, 0) + prior[r] * priorStrength) / (qs.length + priorStrength), 0.02, 0.98);
    }
    for (const v of ACTIVE) {
      let num = 0; let den = 0;
      items.forEach(({ votes }, i) => {
        if (votes[v] === 1) { num += q[i]; den++; } else if (votes[v] === -1) { num += 1 - q[i]; den++; }
      });
      acc[v] = clamp((num + init * strength) / (den + strength), 0.5, 0.95);
    }
  }
  const coverage = Object.fromEntries(VOTERS.map((v) => [v, items.filter((x) => x.votes[v] !== 0).length]));
  return { pi, acc, coverage, n: items.length };
}

export function posterior(votes, pi, acc) {
  let lr = logit(pi);
  for (const v of ACTIVE) {
    if (votes[v] === 1) lr += logit(acc[v]);
    else if (votes[v] === -1) lr -= logit(acc[v]);
  }
  return 1 / (1 + Math.exp(-lr));
}
