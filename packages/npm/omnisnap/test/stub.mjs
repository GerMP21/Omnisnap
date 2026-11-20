#!/usr/bin/env node
// A stub Omnisnap binary for the wrapper tests: it replays the NDJSON events a
// real run produces and writes a minimal summary.
import { mkdirSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';

const args = process.argv.slice(2);
const fail = process.env.STUB_FAIL === '1';
const outIndex = args.indexOf('--out');
const out = outIndex >= 0 ? args[outIndex + 1] : './omnisnap-out';

function progress(type, extra = {}) {
  process.stdout.write(JSON.stringify({ type, ...extra }) + '\n');
}

if (args[0] === 'browsers') {
  process.stdout.write(JSON.stringify([
    { id: 'chrome', name: 'Google Chrome', family: 'blink', protocol: 'cdp', executable: '/usr/bin/chrome', version: '157', source: 'path' },
  ]) + '\n');
  process.exit(0);
}

if (args[0] === 'run') {
  progress('job', { pages: 1 });
  progress('pageStart', { slug: 'home' });
  progress('capture', { slug: 'home', engine: 'chrome', run: 'a', nodes: 42, ms: 12 });
  progress('pageDone', { slug: 'home', status: 'ok' });
  if (fail) {
    process.stderr.write('stub: capture failed\n');
    process.exit(3);
  }
  mkdirSync(out, { recursive: true });
  const summary = {
    $schema: 'https://omnisnap.org/schema/summary/1.json',
    tool: { name: 'omnisnap', version: '1.0.0' },
    generated: '2026-11-18T00:00:00Z',
    job: { pages: [{ url: 'http://localhost:3000/', slug: 'home' }] },
    hosts: [], skipped: [], pages: [], results: [],
  };
  writeFileSync(join(out, 'summary.json'), JSON.stringify(summary));
  writeFileSync(join(out, 'stub-args.json'), JSON.stringify(args));
  progress('done', { summary: join(out, 'summary.json') });
  process.exit(0);
}

if (args[0] === 'merge') {
  const dest = args[args.indexOf('-o') + 1];
  mkdirSync(dest, { recursive: true });
  writeFileSync(join(dest, 'summary.json'), JSON.stringify({ $schema: 'x', tool: { name: 'omnisnap', version: '1.0.0' }, generated: '2026-11-18T00:00:00Z', job: {}, hosts: [], skipped: [], pages: [], results: [] }));
  process.exit(0);
}

if (args[0] === 'report') {
  writeFileSync(join(args[1], 'report.html'), '<!doctype html>');
  process.exit(0);
}

process.stderr.write('stub: unknown command ' + args[0] + '\n');
process.exit(2);
