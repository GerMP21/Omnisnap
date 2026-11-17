// Exports the spike's captures and findings as golden fixtures for the Go
// port's parity tests (Task 5).
//
//   testdata/golden/planted/<slug>/   committed (our own pages)
//   testdata/golden/real/<slug>/      git-ignored: third-party content
//                                     (CONVENTIONS.md, "Never")
//
// Per page: page.json; {chrome,firefox,webkit}-{a,b}.json and
// .probes.json.gz byte for byte; <engine>-a.png (run a only: enough for the
// appearance checks); findings-<engine>.json per pair against the Chrome
// baseline, each finding with its feature vector (lib/features.mjs) added.
// Only the firefox and webkit pairs: the chromium/brave/edge pairs of round 3
// used older captures that are not exported.
//
// usage: node tools/export-golden.mjs [--from <spike out dir>] [--to <dir>] [--groups planted,real]
import fs from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { FEATURE_NAMES, featuresOf } from '../lib/features.mjs';

const HERE = path.dirname(fileURLToPath(import.meta.url));
const args = process.argv.slice(2);
const opt = (name, dflt) => { const i = args.indexOf(name); return i >= 0 ? args[i + 1] : dflt; };
const FROM = path.resolve(opt('--from', path.join(HERE, '../out')));
const TO = path.resolve(opt('--to', path.join(HERE, '../../../testdata/golden')));
const GROUPS = opt('--groups', 'planted,real').split(',');

const ENGINES = ['chrome', 'firefox', 'webkit'];
const PAIRS = ['firefox', 'webkit'];
const RUNS = ['a', 'b'];

const findings = JSON.parse(await fs.readFile(path.join(FROM, 'findings.json'), 'utf8'));
const engines = JSON.parse(await fs.readFile(path.join(FROM, 'engines.json'), 'utf8'));

const pagesOf = (group) => [...new Set(findings.pages.filter((p) => p.group === group).map((p) => p.slug))].sort();

for (const group of GROUPS) {
  const slugs = pagesOf(group);
  if (!slugs.length) throw new Error(`no pages of group ${group} in ${FROM}/findings.json`);
  let bytes = 0;
  let files = 0;
  for (const slug of slugs) {
    const src = path.join(FROM, slug);
    const dst = path.join(TO, group, slug);
    await fs.mkdir(dst, { recursive: true });
    const names = ['page.json'];
    for (const e of ENGINES) {
      for (const r of RUNS) names.push(`${e}-${r}.json`, `${e}-${r}.probes.json.gz`);
      names.push(`${e}-a.png`);
    }
    for (const n of names) {
      await fs.copyFile(path.join(src, n), path.join(dst, n));
      bytes += (await fs.stat(path.join(dst, n))).size;
      files++;
    }
    for (const pair of PAIRS) {
      const page = findings.pages.find((p) => p.group === group && p.slug === slug && p.pair === pair);
      if (!page) throw new Error(`${slug}: no ${pair} pair in findings.json`);
      const out = { ...page, findings: page.findings.map((f) => ({ ...f, features: featuresOf(f, pair) })) };
      const json = `${JSON.stringify(out, null, 1)}\n`;
      await fs.writeFile(path.join(dst, `findings-${pair}.json`), json);
      bytes += Buffer.byteLength(json);
      files++;
    }
  }
  const meta = {
    source: 'spikes/layout-diff (capture.mjs, diff.mjs), round 5',
    generated: findings.generated,
    baseline: findings.engines.A,
    engines: Object.fromEntries(ENGINES.map((e) => [e, engines[e]])),
    tolerances: findings.tol,
    featureNames: FEATURE_NAMES,
    pages: slugs,
  };
  await fs.writeFile(path.join(TO, group, 'meta.json'), `${JSON.stringify(meta, null, 2)}\n`);
  console.log(`${group}: ${slugs.length} pages, ${files} files, ${(bytes / 1e6).toFixed(1)} MB -> ${path.join(TO, group)}`);
}
