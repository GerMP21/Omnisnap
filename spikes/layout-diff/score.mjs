// Score the spike against KNOWN answers only, at the unit a developer sees:
// CAUSES, split into Report and Info tiers.
//
// - Planted differences fire in Firefox only (UA-conditional CSS): recall
//   is measured in the Report tier, with a Wilson 95% interval.
// - Every other engine on the planted pages, and the control pages, are
//   controls: nothing may be reported there.
// - Knowledge-base pages (kind 'kb') may differ, but only as explained Info.
// - Design intent (lib/intent.mjs) is scored separately: violations found on
//   the planted intent pages vs violations on controls (false alarms).
//
// Precision on real sites is NOT measured here: it needs automated ground
// truth (mutation testing on real pages and a locally trained model, see the
// PRD changelog 1.4.0). Manual labels were withdrawn as unreliable.
//
// usage: node score.mjs

import fs from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { HARD } from './lib/triage.mjs';
import { PAGES } from './planted/pages.mjs';

const HERE = path.dirname(fileURLToPath(import.meta.url));
const OUT = process.env.OMNISNAP_OUT || path.join(HERE, 'out');
const data = JSON.parse(await fs.readFile(path.join(OUT, 'findings.json'), 'utf8'));

const pct = (x) => `${(x * 100).toFixed(1)}%`;
const median = (xs) => { const s = [...xs].sort((a, b) => a - b); return s.length ? s[Math.floor((s.length - 1) / 2)] : 0; };
// Wilson score interval: honest bounds for a proportion from few cases.
const wilson = (k, n, z = 1.96) => {
  if (!n) return [0, 1];
  const p = k / n;
  const den = 1 + (z * z) / n;
  const mid = (p + (z * z) / (2 * n)) / den;
  const half = (z * Math.sqrt((p * (1 - p)) / n + (z * z) / (4 * n * n))) / den;
  return [Math.max(0, mid - half), Math.min(1, mid + half)];
};
const ok = data.pages.filter((p) => !p.error);
const def = (slug) => PAGES.find((x) => x.slug === slug);
const report = (p) => p.findings.filter((f) => f.tier === 'report');
const pairs = [...new Set(ok.map((p) => p.pair))];
const rows = [];
const row = (name, target, value, status) => rows.push({ name, target, value, status });
const st = (cond, have) => (have ? (cond ? 'PASS' : 'FAIL') : 'PENDING');
// 'native' pages show genuine engine differences in every engine: never controls.
const isControl = (p) => { const k = def(p.slug)?.kind; return k === 'control' || (p.pair !== 'firefox' && k === 'planted'); };
const hits = (fs, g) => fs.some((f) => (f.memberElIds || []).some((id) => g.anyOf.includes(id)));

// ------------------------------------------------- planted, firefox pair
console.log('Planted pages, chrome vs firefox (planted differences fire in Firefox only)');
let groups = 0;
let found = 0;
let withinMax = 0;
let planted = 0;
let nativeLeaks = 0;
const missed = [];
for (const p of ok.filter((x) => x.group === 'planted' && x.pair === 'firefox')) {
  const d = def(p.slug);
  if (!d || d.kind === 'kb' || d.kind === 'control') continue;
  const rep = report(p);
  let note = '';
  if (d.kind === 'planted') {
    planted++;
    const res = d.expect.map((g) => hits(rep, g));
    groups += res.length;
    found += res.filter(Boolean).length;
    if (!res.every(Boolean)) missed.push(p.slug);
    if (rep.length <= d.maxFindings) withinMax++;
    note = `${res.every(Boolean) ? 'FOUND ' : 'MISSED'}  report ${rep.length} (max ${d.maxFindings}), info ${p.findings.length - rep.length}  ${rep.map((f) => `${f.title} [${f.consequences.join(',')}]`).join('; ')}`;
  } else if (d.kind === 'native') {
    const bad = rep.filter((f) => !d.onlyTags.includes(f.tag));
    nativeLeaks += bad.length;
    note = `${bad.length ? 'LEAKY ' : 'ok    '}  report ${rep.length}, info ${p.findings.length - rep.length}${bad.length ? `: ${bad.map((f) => f.sel).join(', ')}` : ''}`;
  }
  console.log(`  ${p.slug.padEnd(30)} ${note}`);
}
const [rlo, rhi] = wilson(found, groups);
row('Planted bugs reported at the right element', '>= 90%', `${found}/${groups} = ${pct(found / Math.max(1, groups))} [${pct(rlo)}, ${pct(rhi)}]${missed.length ? `; missed ${missed.join(', ')}` : ''}`, st(found / groups >= 0.9, groups));
row('Planted pages within their Report budget', 'all', `${withinMax}/${planted}`, st(withinMax === planted, planted));
row('Native forms: Report causes off the controls', '0', String(nativeLeaks), st(nativeLeaks === 0, planted));

// ---------------------------------------- controls and KB fixtures, all pairs
console.log('\nControls (no planted difference for that engine) and knowledge-base fixtures');
let controlReports = 0;
let controlCauses = 0;
let kbReport = 0;
let kbUnexplained = 0;
for (const e of pairs) {
  const ps = ok.filter((x) => x.group === 'planted' && x.pair === e);
  const controls = ps.filter(isControl);
  const noisy = controls.filter((p) => p.findings.length);
  controlCauses += noisy.reduce((s, p) => s + p.findings.length, 0);
  controlReports += controls.reduce((s, p) => s + report(p).length, 0);
  const kbNotes = ps.filter((p) => def(p.slug)?.kind === 'kb').map((p) => {
    const rep = report(p).length;
    kbReport += rep;
    kbUnexplained += p.findings.filter((f) => f.tier === 'info' && !f.kb).length;
    const ids = [...new Set(p.findings.map((f) => f.kb?.id).filter(Boolean))];
    return `${p.slug.slice(0, 2)}: ${p.findings.length ? `${p.findings.length} cause(s) ${ids.join('/') || 'unexplained'}${rep ? ` REPORT ${rep}` : ''}` : 'identical'}`;
  });
  console.log(`  ${e.padEnd(9)} controls ${String(controls.length).padStart(2)} pages, ${noisy.length ? `with causes: ${noisy.map((p) => `${p.slug} ${p.findings.length}${report(p).length ? ` (REPORT ${report(p).length})` : ''}`).join(', ')}` : 'all clean'}  |  KB fixtures: ${kbNotes.join('; ')}`);
}
row('Report causes on control pages (all engines)', '0', `${controlReports} (${controlCauses} causes in any tier)`, st(controlReports === 0, true));
row('KB fixtures: Report causes / unexplained', '0 / 0', `${kbReport} / ${kbUnexplained}`, st(kbReport === 0 && kbUnexplained === 0, true));

// ------------------------------------------------------ design intent
// Violations of constraints the engines agree on (lib/intent.mjs). On the
// intent pages the planted element must break one; on controls, none may.
console.log('\nDesign intent (constraints >= 2 engine families agree on, broken by one)');
const viol = (p) => p.intent?.violations || [];
let intentPages = 0;
let intentFound = 0;
for (const p of ok.filter((x) => x.group === 'planted' && x.pair === 'firefox')) {
  const d = def(p.slug);
  if (!d?.intent) continue;
  intentPages++;
  const hit = viol(p).some((v) => v.engine === 'B' && v.ids.some((id) => d.expect.some((g) => g.anyOf.includes(id))));
  if (hit) intentFound++;
  console.log(`  ${p.slug.padEnd(30)} ${hit ? 'FOUND ' : 'MISSED'}  ${viol(p).map((v) => v.text).slice(0, 3).join(' | ')}`);
}
const falseAlarms = ok.filter((p) => p.group === 'planted' && isControl(p)).flatMap((p) => viol(p).map((v) => `${p.slug}@${p.pair}: ${v.text}`));
console.log(`  controls: ${falseAlarms.length ? falseAlarms.slice(0, 8).join(' | ') : 'no violations'}`);
const corpusPages = ok.filter((p) => ['real', 'own', 'crawl'].includes(p.group));
const fired = corpusPages.filter((p) => viol(p).length);
console.log(`  real sites: ${fired.reduce((s, p) => s + viol(p).length, 0)} violations on ${fired.length}/${corpusPages.length} engine pairs${fired.length ? `, e.g. ${fired.slice(0, 4).map((p) => `${p.slug}@${p.pair}: ${viol(p)[0].text}`).join(' | ')}` : ''}`);
row('Intent: planted intent bugs found', '>= 90%', `${intentFound}/${intentPages}`, st(intentFound / Math.max(1, intentPages) >= 0.9, intentPages));
row('Intent: violations on control pages', '0', String(falseAlarms.length), st(falseAlarms.length === 0, true));

// ---------------------------------------------------- real / own / crawl
console.log('\nReal, own and crawled pages (no ground truth: counts only)');
const perPage = [];
for (const p of corpusPages) {
  const diverged = p.warnings.some((w) => /different page states/.test(w));
  const rep = report(p);
  if (!diverged) perPage.push(rep.length);
  console.log(`  ${p.slug.slice(0, 28).padEnd(28)} ${p.pair.padEnd(9)} matched ${pct(p.stats.matchRate).padStart(6)}  report ${String(rep.length).padStart(2)}  info ${String(p.findings.length - rep.length).padStart(3)}  intent ${viol(p).length}${diverged ? '  [diverged page states]' : ''}`);
}
const ffReal = ok.filter((p) => p.group === 'real' && p.pair === 'firefox');
const own = ok.filter((p) => p.group === 'own');
const avg = (ps) => ps.reduce((s, p) => s + p.stats.matchRate, 0) / ps.length;
row('Elements matched, your app (all engines)', '>= 95%', own.length ? `${pct(avg(own))} (min ${pct(Math.min(...own.map((p) => p.stats.matchRate)))})` : 'no pages', st(avg(own) >= 0.95, own.length));
row('Elements matched, real sites (firefox)', '>= 85%', ffReal.length ? `${pct(avg(ffReal))} (min ${pct(Math.min(...ffReal.map((p) => p.stats.matchRate)))})` : 'no pages', st(avg(ffReal) >= 0.85, ffReal.length));
row('Report causes per page (median / max)', '<= ~10', `${median(perPage)} / ${Math.max(0, ...perPage)}`, st(median(perPage) <= 10, perPage.length));
row('Precision on real sites', 'lower >= 80%', 'not measured directly: see the tree model (mutation testing) below', 'PENDING');

// ------------------------------------------------ tree model (mutation testing)
if (data.gbdt) {
  const G = data.gbdt;
  const pt = (x) => `recall ${x.tp} = ${pct(x.recall)} [${pct(x.recall_ci[0])}, ${pct(x.recall_ci[1])}], natural causes reported ${pct(x.false_alarm_rate)}, Report causes per clean page ${x.reports_per_clean_page.toFixed(2)}`;
  console.log(`\nTree model (trained ${G.trained} on ${G.rows.positive} injected-bug causes vs ${G.rows.negative} natural causes from ${G.rows.sites} sites; cross-validated by site, ROC AUC ${G.cv.roc_auc.toFixed(3)})`);
  console.log(`  rules tier                  ${pt(G.cv.rules)}`);
  console.log(`  trees at the rules' recall  ${pt(G.cv.trees_at_rules_recall)}`);
  console.log(`  trees at tau ${G.tau.toFixed(3)}           ${pt(G.cv.trees_at_target)}`);
  const treeRep = (f) => f.gbdtTier === 'report';
  let n = 0; let k = 0;
  for (const p of ok.filter((x) => x.group === 'planted' && x.pair === 'firefox')) {
    const d = def(p.slug);
    if (d?.kind !== 'planted') continue;
    for (const g of d.expect) { n++; if (hits(p.findings.filter(treeRep), g)) k++; }
  }
  const leaks = ok.filter((p) => p.group === 'planted' && (isControl(p) || def(p.slug)?.kind === 'kb')).reduce((t, p) => t + p.findings.filter(treeRep).length, 0);
  console.log(`  planted suite (never trained on), tree tier: recall ${k}/${n}, Report causes on controls/KB ${leaks}`);
  row('Trees: false alarms at the rules\' recall', '< rules', `${pct(G.cv.trees_at_rules_recall.false_alarm_rate)} vs rules ${pct(G.cv.rules.false_alarm_rate)} of natural causes`, G.cv.trees_at_rules_recall.false_alarm_rate < G.cv.rules.false_alarm_rate ? 'PASS' : 'FAIL');
  row('Trees: planted recall / control reports (tree tier)', '>= 90% / 0', `${k}/${n} / ${leaks}`, st(k / Math.max(1, n) >= 0.9 && leaks === 0, n));
}

// ------------------------------------------------ judge vs rules (no labels)
if (data.judge) {
  const J = data.judge;
  const hard = (f) => (f.consequences || []).some((c) => HARD.has(c));
  const atTau = (tau) => (f) => hard(f) || (f.p ?? 0) >= tau;
  const byRules = (f) => (f.rulesTier || f.tier) === 'report';
  const plantedRecall = (isRep) => {
    let n = 0; let k = 0;
    for (const p of ok.filter((x) => x.group === 'planted' && x.pair === 'firefox')) {
      const d = def(p.slug);
      if (d?.kind !== 'planted') continue;
      for (const g of d.expect) { n++; if (hits(p.findings.filter(isRep), g)) k++; }
    }
    return `${k}/${n}`;
  };
  const ctl = (isRep) => ok.filter((p) => p.group === 'planted' && (isControl(p) || def(p.slug)?.kind === 'kb')).reduce((t, p) => t + p.findings.filter(isRep).length, 0);
  const corpus = corpusPages.filter((p) => !p.warnings.some((w) => /different page states/.test(w))).flatMap((p) => p.findings);
  console.log(`\nJudge: weak-supervision model fitted on ${J.n} unlabeled corpus causes (${J.corpusFingerprints} fingerprints), prior P(report) = ${J.pi.report.toFixed(2)} if the rules report it, ${J.pi.info.toFixed(2)} if not, tau = ${J.tau} (${J.tauSource})`);
  console.log(`  ${'tier rule'.padEnd(22)} ${'planted'.padEnd(8)} ${'controls'.padEnd(9)} corpus Report causes`);
  const line = (name, isRep) => console.log(`  ${name.padEnd(22)} ${plantedRecall(isRep).padEnd(8)} ${String(ctl(isRep)).padEnd(9)} ${corpus.filter(isRep).length}/${corpus.length}`);
  line('rules', byRules);
  for (const tau of [...new Set([0.1, 0.2, 0.3, 0.5, 0.7, 0.9, J.tau])].sort((x, y) => x - y)) line(`judge tau=${tau}${tau === J.tau ? ' *' : ''}`, atTau(tau));
  console.log('\n  voter        learned accuracy  coverage');
  for (const v of J.voters) console.log(`  ${v.padEnd(12)} ${J.acc[v].toFixed(2).padStart(16)}  ${J.coverage[v]}/${J.n}`);
}

console.log('\nPass criteria');
for (const r of rows) console.log(`  ${r.status.padEnd(8)} ${r.name.padEnd(46)} target ${r.target.padEnd(12)} got ${r.value}`);
await fs.writeFile(path.join(OUT, 'score.json'), JSON.stringify({ scoredAt: new Date().toISOString(), rows }, null, 2));
