// Training table for the tree model, from out/findings.json (run diff.mjs
// first). One row per CAUSE:
//   label 1: a mutant's cause that the injected bug produced (diff.mjs
//            labelMutant: same-engine diff, target neighbourhood);
//   label 0: a cause of a clean real/own/crawl page (natural engine
//            differences: unlabeled, mostly expected; positive-unlabeled).
// Mutant causes that are not positives duplicate clean causes: skipped.
// Pages whose engines rendered different states are skipped.
// `site` groups rows for cross-validation (a mutant belongs to its base).
// Planted pages are written to a separate file: never trained on, used as
// an independent test.
//
// usage: node dataset.mjs   -> out/dataset.csv, out/dataset-planted.csv

import fs from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { FEATURE_NAMES, featuresOf } from './lib/features.mjs';

const HERE = path.dirname(fileURLToPath(import.meta.url));
const OUT = process.env.OMNISNAP_OUT || path.join(HERE, 'out');
const data = JSON.parse(await fs.readFile(path.join(OUT, 'findings.json'), 'utf8'));
const diverged = (p) => p.warnings.some((w) => /different page states/.test(w));
const head = ['site', 'page', 'pair', 'group', 'id', 'label', ...FEATURE_NAMES].join(',');
const csv = (x) => (typeof x === 'string' ? `"${x.replace(/"/g, '""')}"` : x);
const rows = [];
const planted = [];
const tally = { pos: 0, neg: 0, mutants: 0, effective: 0, skippedDiverged: 0 };
for (const p of data.pages) {
  if (p.error) continue;
  if (p.group === 'planted') {
    for (const f of p.findings) planted.push([p.slug, p.slug, p.pair, p.group, f.id, -1, ...featuresOf(f, p.pair)].map(csv).join(','));
    continue;
  }
  if (diverged(p)) { tally.skippedDiverged++; continue; }
  if (p.group === 'mut') {
    tally.mutants++;
    if (p.mutant?.effective) tally.effective++;
    for (const f of p.findings.filter((x) => x.mutLabel)) { rows.push([p.base, p.slug, p.pair, p.group, f.id, 1, ...featuresOf(f, p.pair)].map(csv).join(',')); tally.pos++; }
  } else if (['real', 'own', 'crawl'].includes(p.group)) {
    for (const f of p.findings) { rows.push([p.slug, p.slug, p.pair, p.group, f.id, 0, ...featuresOf(f, p.pair)].map(csv).join(',')); tally.neg++; }
  }
}
await fs.writeFile(path.join(OUT, 'dataset.csv'), [head, ...rows].join('\n'));
await fs.writeFile(path.join(OUT, 'dataset-planted.csv'), [head, ...planted].join('\n'));
// Clean page pairs (with or without causes): the denominator of "Report causes per page".
const cleanPairs = data.pages.filter((p) => !p.error && ['real', 'own', 'crawl'].includes(p.group) && !diverged(p)).length;
await fs.writeFile(path.join(OUT, 'dataset-meta.json'), JSON.stringify({ cleanPairs, ...tally }, null, 1));
const sites = new Set(rows.map((r) => r.split(',')[0]));
console.log(`dataset: ${rows.length} rows (${tally.pos} positive, ${tally.neg} negative) from ${sites.size} sites, ${FEATURE_NAMES.length} features`);
console.log(`mutants: ${tally.mutants}, effective ${tally.effective}; pages skipped for diverged states: ${tally.skippedDiverged}`);
console.log(`planted (test only): ${planted.length} rows`);
