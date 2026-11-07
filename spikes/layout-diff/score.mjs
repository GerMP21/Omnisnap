// Score the spike against the pass criteria fixed BEFORE running it
// (docs/ideas/omnisnap.md), at the unit a developer sees: CAUSES, split into
// Report and Info tiers (lib/triage.mjs). Precision is judged on the Report
// tier with a Wilson 95% interval; recall must survive in the Report tier.
//
// Planted differences fire in Firefox only (UA-conditional CSS); for every
// other engine the planted pages are extra controls. Knowledge-base pages
// (kind 'kb') may differ, but only as explained Info causes.
//
// usage: node score.mjs

import fs from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { PAGES } from './planted/pages.mjs';

const HERE = path.dirname(fileURLToPath(import.meta.url));
const data = JSON.parse(await fs.readFile(path.join(HERE, 'out/findings.json'), 'utf8'));
let labels = {};
try { labels = JSON.parse(await fs.readFile(path.join(HERE, 'labels.json'), 'utf8')).labels || {}; } catch (_) { /* not labeled yet */ }

const pct = (x) => `${(x * 100).toFixed(1)}%`;
const median = (xs) => { const s = [...xs].sort((a, b) => a - b); return s.length ? s[Math.floor((s.length - 1) / 2)] : 0; };
// Wilson score interval: honest bounds for a proportion from few labels.
const wilson = (k, n, z = 1.96) => {
  if (!n) return [0, 1];
  const p = k / n;
  const den = 1 + (z * z) / n;
  const mid = (p + (z * z) / (2 * n)) / den;
  const half = (z * Math.sqrt((p * (1 - p)) / n + (z * z) / (4 * n * n))) / den;
  return [Math.max(0, mid - half), Math.min(1, mid + half)];
};
// A cause's label: set directly, else derived from its instances' labels.
const labelOf = (f) => {
  if (labels[f.id]) return labels[f.id];
  const ls = (f.memberIds || []).map((id) => labels[id]).filter(Boolean);
  if (!ls.length) return null;
  if (ls.includes('real')) return 'real';
  if (labels[f.memberIds[0]] === 'false' || ls.every((l) => l === 'false')) return 'false';
  return 'trivial';
};
const ok = data.pages.filter((p) => !p.error);
const def = (slug) => PAGES.find((x) => x.slug === slug);
const report = (p) => p.findings.filter((f) => f.tier === 'report');
const pairs = [...new Set(ok.map((p) => p.pair))];
const rows = [];
const row = (name, target, value, status) => rows.push({ name, target, value, status });
const st = (cond, have) => (have ? (cond ? 'PASS' : 'FAIL') : 'PENDING');

// ------------------------------------------------- planted, firefox pair
console.log('Planted pages, chrome vs firefox (planted differences fire in Firefox only)');
let groups = 0;
let hits = 0;
let withinMax = 0;
let planted = 0;
let nativeLeaks = 0;
for (const p of ok.filter((x) => x.group === 'planted' && x.pair === 'firefox')) {
  const d = def(p.slug);
  if (!d || d.kind === 'kb' || d.kind === 'control') continue;
  const rep = report(p);
  let note = '';
  if (d.kind === 'planted') {
    planted++;
    const res = d.expect.map((g) => rep.some((f) => (f.memberElIds || []).some((id) => g.anyOf.includes(id))));
    groups += res.length;
    hits += res.filter(Boolean).length;
    if (rep.length <= d.maxFindings) withinMax++;
    note = `${res.every(Boolean) ? 'FOUND ' : 'MISSED'}  report ${rep.length} (max ${d.maxFindings}), info ${p.findings.length - rep.length}  ${rep.map((f) => `${f.title} [${f.consequences.join(',')}]`).join('; ')}`;
  } else if (d.kind === 'native') {
    const bad = rep.filter((f) => !d.onlyTags.includes(f.tag));
    nativeLeaks += bad.length;
    note = `${bad.length ? 'LEAKY ' : 'ok    '}  report ${rep.length}, info ${p.findings.length - rep.length}${bad.length ? `: ${bad.map((f) => f.sel).join(', ')}` : ''}`;
  }
  console.log(`  ${p.slug.padEnd(26)} ${note}`);
}
row('Planted bugs reported at the right element', '>= 9/10', `${hits}/${groups}`, st(hits / groups >= 0.9, groups));
row('Planted pages within their Report budget', 'all', `${withinMax}/${planted}`, st(withinMax === planted, planted));
row('Native forms: Report causes off the controls', '0', String(nativeLeaks), st(nativeLeaks === 0, planted));

// ---------------------------------------- controls and KB fixtures, all pairs
console.log('\nControls (no planted difference for that engine) and knowledge-base fixtures');
let controlCauses = 0;
let kbReport = 0;
let kbUnexplained = 0;
for (const e of pairs) {
  const ps = ok.filter((x) => x.group === 'planted' && x.pair === e);
  const controls = ps.filter((p) => { const k = def(p.slug)?.kind; return k === 'control' || (e !== 'firefox' && k === 'planted'); });
  const noisy = controls.filter((p) => p.findings.length);
  controlCauses += noisy.reduce((s, p) => s + p.findings.length, 0);
  const kb = ps.filter((p) => def(p.slug)?.kind === 'kb');
  const kbNotes = kb.map((p) => {
    const rep = report(p).length;
    const unexplained = p.findings.filter((f) => f.tier === 'info' && !f.kb).length;
    kbReport += rep;
    kbUnexplained += unexplained;
    const ids = [...new Set(p.findings.map((f) => f.kb?.id).filter(Boolean))];
    return `${p.slug.slice(0, 2)}: ${p.findings.length ? `${p.findings.length} cause(s) ${ids.join('/') || 'unexplained'}${rep ? ` REPORT ${rep}` : ''}` : 'identical'}`;
  });
  console.log(`  ${e.padEnd(9)} controls ${String(controls.length).padStart(2)} pages, ${noisy.length ? `NOISY: ${noisy.map((p) => `${p.slug} ${p.findings.length}`).join(', ')}` : 'all clean'}  |  KB fixtures: ${kbNotes.join('; ')}`);
}
row('Causes on control pages (all engines)', '0', String(controlCauses), st(controlCauses === 0, true));
row('KB fixtures: Report causes / unexplained', '0 / 0', `${kbReport} / ${kbUnexplained}`, st(kbReport === 0 && kbUnexplained === 0, true));

// ---------------------------------------------------- real / own pages
console.log('\nReal and own pages (pages with diverged page states are excluded from precision)');
const tally = { report: { real: 0, trivial: 0, false: 0, unlabeled: 0 }, info: { real: 0, trivial: 0, false: 0, unlabeled: 0 } };
const perPage = [];
for (const p of ok.filter((x) => x.group === 'real' || x.group === 'own')) {
  const diverged = p.warnings.some((w) => /different page states/.test(w));
  const rep = report(p);
  perPage.push(rep.length);
  const show = (fs) => fs.map((f) => `${f.title}${f.count > 1 ? ` ×${f.count}` : ''}=${labelOf(f) || '?'}`).join('; ');
  console.log(`  ${p.slug.padEnd(14)} ${p.pair.padEnd(9)} matched ${pct(p.stats.matchRate).padStart(6)}  report ${String(rep.length).padStart(2)}  info ${String(p.findings.length - rep.length).padStart(2)}${diverged ? '  [diverged: excluded]' : ''}  ${show(rep)}`);
  if (diverged) continue;
  for (const f of p.findings) tally[f.tier][labelOf(f) || 'unlabeled']++;
}
const R = tally.report;
const labeledR = R.real + R.trivial + R.false;
const [lo, hi] = wilson(R.real, labeledR);
const ffReal = ok.filter((p) => p.group === 'real' && p.pair === 'firefox');
const own = ok.filter((p) => p.group === 'own');
const avg = (ps) => ps.reduce((s, p) => s + p.stats.matchRate, 0) / ps.length;
row('Elements matched, your app (all engines)', '>= 95%', own.length ? `${pct(avg(own))} (min ${pct(Math.min(...own.map((p) => p.stats.matchRate)))})` : 'no pages', st(avg(own) >= 0.95, own.length));
row('Elements matched, real sites (firefox)', '>= 85%', ffReal.length ? `${pct(avg(ffReal))} (min ${pct(Math.min(...ffReal.map((p) => p.stats.matchRate)))})` : 'no pages', st(avg(ffReal) >= 0.85, ffReal.length));
row('Report causes per page (median / max)', '<= ~10', `${median(perPage)} / ${Math.max(0, ...perPage)}`, st(median(perPage) <= 10, perPage.length));
const coverage = labeledR / Math.max(1, labeledR + R.unlabeled);
row('Report causes that are real (Wilson 95%)', 'lower >= 80%',
  labeledR ? `${R.real}/${labeledR} = ${pct(R.real / labeledR)} [${pct(lo)}, ${pct(hi)}]; ${R.unlabeled} unlabeled` : 'not labeled',
  labeledR && coverage >= 0.9 ? (lo >= 0.8 ? 'PASS' : 'FAIL') : 'PENDING');
const I = tally.info;
row('Real causes demoted to Info', 'few', `${I.real} real, ${I.trivial} trivial, ${I.false} false, ${I.unlabeled} unlabeled in Info`, I.real <= 2 ? 'PASS' : 'FAIL');

console.log('\nPass criteria');
for (const r of rows) console.log(`  ${r.status.padEnd(8)} ${r.name.padEnd(46)} target ${r.target.padEnd(12)} got ${r.value}`);
await fs.writeFile(path.join(HERE, 'out/score.json'), JSON.stringify({ scoredAt: new Date().toISOString(), rows, tally }, null, 2));
