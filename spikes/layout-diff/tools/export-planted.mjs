// Exports the planted suite (planted/pages.mjs) as static fixtures for the Go
// port (Task 4): testdata/planted/<slug>.html is renderPage()'s exact output,
// and expectations.json holds every page's answer (every field except the
// page source: css, body, header).
//
// usage: node tools/export-planted.mjs [--check] [--out <dir>]
//   --check  write nothing; exit 1 if the fixtures differ from the spike
import fs from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { PAGES, renderPage } from '../planted/pages.mjs';

const HERE = path.dirname(fileURLToPath(import.meta.url));
const args = process.argv.slice(2);
const check = args.includes('--check');
const outIdx = args.indexOf('--out');
const OUT = outIdx >= 0 ? path.resolve(args[outIdx + 1]) : path.resolve(HERE, '../../../testdata/planted');

const SOURCE_FIELDS = new Set(['css', 'body', 'header']);
const answer = (p) => Object.fromEntries(Object.entries(p).filter(([k]) => !SOURCE_FIELDS.has(k)));

const files = new Map(PAGES.map((p) => [`${p.slug}.html`, renderPage(p)]));
files.set('expectations.json', `${JSON.stringify({ source: 'spikes/layout-diff/planted/pages.mjs', pages: PAGES.map(answer) }, null, 2)}\n`);

if (check) {
  const stale = [];
  for (const [name, want] of files) {
    const got = await fs.readFile(path.join(OUT, name), 'utf8').catch(() => null);
    if (got !== want) stale.push(name);
  }
  // audit-*.html are the Go port's own dom-audit pages (Task 35), not the spike's.
  const extra = (await fs.readdir(OUT).catch(() => [])).filter((n) => n.endsWith('.html') && !files.has(n) && !/^audit-/.test(n));
  if (stale.length || extra.length) {
    console.error(`planted fixtures out of date in ${OUT}\n  differ or missing: ${stale.join(', ') || '-'}\n  not in the spike: ${extra.join(', ') || '-'}`);
    process.exit(1);
  }
  console.log(`planted fixtures match the spike (${PAGES.length} pages)`);
} else {
  await fs.mkdir(OUT, { recursive: true });
  for (const [name, content] of files) await fs.writeFile(path.join(OUT, name), content);
  console.log(`wrote ${PAGES.length} pages and expectations.json to ${OUT}`);
}
