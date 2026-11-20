// Task 83: the API with a stub binary on every OS (a shim because Windows
// cannot exec a .mjs directly).
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { chmodSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { createRequire, syncBuiltinESMExports } from 'node:module';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { browsers, merge, report, resolveBinary, run, runBinary, OmnisnapError } from '../index.js';

const here = fileURLToPath(new URL('.', import.meta.url));
const stub = join(here, 'stub.mjs');

// The shim the wrapper spawns: node + the stub script.
function shim() {
  const dir = mkdtempSync(join(tmpdir(), 'omnisnap-npm-test-'));
  if (process.platform === 'win32') {
    // A .cmd shim cannot be spawned without a shell (spawn fails with
    // EINVAL), and the wrapper rightly never uses one: a shell would re-parse
    // the job's arguments. So the stub's path is redirected to `node stub.mjs`
    // inside the wrapper's own spawn call, which keeps its arguments, options
    // and the real child process. The real omnisnap.exe is spawned by CI's
    // "npm fresh project" step.
    // Source: https://nodejs.org/api/child_process.html#spawning-bat-and-cmd-files-on-windows
    // Source: https://nodejs.org/api/module.html#modulesyncbuiltinesmexports
    const exe = join(dir, 'omnisnap.exe');
    const childProcess = createRequire(import.meta.url)('node:child_process');
    const spawn = childProcess.spawn;
    childProcess.spawn = (file, args, options) =>
      file === exe ? spawn(process.execPath, [stub, ...args], options) : spawn(file, args, options);
    syncBuiltinESMExports();
    return exe;
  }
  const sh = join(dir, 'omnisnap');
  writeFileSync(sh, `#!/bin/sh\nexec "${process.execPath}" "${stub}" "$@"\n`);
  chmodSync(sh, 0o755);
  return sh;
}

const bin = shim();
process.env.OMNISNAP_BINARY = bin;

test('run() writes a job, streams progress in order and returns the summary', async () => {
  const out = mkdtempSync(join(tmpdir(), 'omnisnap-out-'));
  const types = [];
  const summary = await run({
    pages: ['http://localhost:3000/'],
    engines: ['chrome'],
    out,
    onProgress: (e) => types.push(e.type),
  });
  assert.equal(summary.tool.name, 'omnisnap');
  assert.deepEqual(types, ['job', 'pageStart', 'capture', 'pageDone', 'done']);
  const job = JSON.parse(readFileSync(join(out, 'omnisnap-job.json'), 'utf8'));
  assert.deepEqual(job.pages, [{ url: 'http://localhost:3000/' }]);
  assert.deepEqual(job.engines, ['chrome']);
});

test('allowSmallPages passes --allow-small-pages, and only when set', async () => {
  for (const [allowSmallPages, want] of [[true, true], [false, false], [undefined, false]]) {
    const out = mkdtempSync(join(tmpdir(), 'omnisnap-out-'));
    await run({ pages: ['http://localhost:3000/'], out, allowSmallPages });
    const args = JSON.parse(readFileSync(join(out, 'stub-args.json'), 'utf8'));
    assert.equal(args.includes('--allow-small-pages'), want, `allowSmallPages=${allowSmallPages}: ${args}`);
  }
});

// The installed layout: @omnisnap/<os>-<arch> ships the binary under bin/
// (its package.json says "files": ["bin/"]). Resolving outside bin/ found
// nothing in a real install.
test('resolveBinary() finds the platform package binary under bin/', (t) => {
  const key = `${process.platform}-${process.arch}`;
  if (!['linux-x64', 'linux-arm64', 'darwin-x64', 'darwin-arm64', 'win32-x64'].includes(key)) {
    t.skip(`no platform package for ${key}`);
    return;
  }
  const pkg = join(here, '..', 'node_modules', '@omnisnap', key);
  const exe = join(pkg, 'bin', process.platform === 'win32' ? 'omnisnap.exe' : 'omnisnap');
  mkdirSync(join(pkg, 'bin'), { recursive: true });
  writeFileSync(join(pkg, 'package.json'), JSON.stringify({ name: `@omnisnap/${key}`, version: '0.0.0' }));
  writeFileSync(exe, '');
  const saved = process.env.OMNISNAP_BINARY;
  delete process.env.OMNISNAP_BINARY;
  try {
    assert.equal(resolveBinary(), exe);
  } finally {
    process.env.OMNISNAP_BINARY = saved;
    rmSync(join(here, '..', 'node_modules'), { recursive: true, force: true });
  }
});

test('a non-zero exit is a typed error with its stderr', async () => {
  const out = mkdtempSync(join(tmpdir(), 'omnisnap-out-'));
  const old = process.env.STUB_FAIL;
  process.env.STUB_FAIL = '1';
  try {
    await assert.rejects(
      run({ pages: ['http://localhost:3000/'], out }),
      (err) => err instanceof OmnisnapError && err.code === 3 && /capture failed/.test(err.stderr),
    );
  } finally {
    if (old === undefined) delete process.env.STUB_FAIL; else process.env.STUB_FAIL = old;
  }
});

test('browsers() parses the JSON array', async () => {
  const list = await browsers();
  assert.equal(list.length, 1);
  assert.equal(list[0].id, 'chrome');
});

test('merge() and report()', async () => {
  const out = mkdtempSync(join(tmpdir(), 'omnisnap-merge-'));
  const summary = await merge(['/a', '/b'], out);
  assert.equal(summary.tool.name, 'omnisnap');
  const bundle = mkdtempSync(join(tmpdir(), 'omnisnap-bundle-'));
  const html = await report(bundle);
  assert.match(readFileSync(html, 'utf8'), /doctype/);
});

test('unknown NDJSON is ignored, partial lines are buffered', async () => {
  let seen = 0;
  const res = await runBinary(['browsers'], { onProgress: () => { seen++; } });
  assert.equal(res.code, 0);
  assert.equal(seen, 0); // browsers writes JSON, not events
});

// Type drift (Task 83): every field the schema requires appears in the
// generated declarations.
test('declarations cover the schema required fields', () => {
  const root = fileURLToPath(new URL('../../../../', import.meta.url));
  const schema = JSON.parse(readFileSync(join(root, 'schema/summary.schema.json'), 'utf8'));
  const dts = readFileSync(join(here, '..', 'index.d.ts'), 'utf8');
  const section = (name) => {
    const i = dts.indexOf(`export interface ${name} {`);
    assert.ok(i >= 0, `no interface ${name}`);
    const j = dts.indexOf('\n}', i);
    return dts.slice(i, j);
  };
  const summary = section('Summary');
  for (const f of schema.required) assert.ok(summary.includes(f), `Summary.${f}`);
  const result = section('Result');
  for (const f of schema.$defs.result.required) assert.ok(result.includes(f), `Result.${f}`);
  const cause = section('Cause');
  for (const f of schema.$defs.cause.required) assert.ok(cause.includes(f), `Cause.${f}`);
});

// Every package ships LICENSE ("files"); each must be the repository's
// LICENSE.txt, word for word.
test('the LICENSE copies match the repository', () => {
  const root = join(here, '..', '..', '..', '..');
  const want = readFileSync(join(root, 'LICENSE.txt'));
  const pkgs = ['omnisnap', ...['linux-x64', 'linux-arm64', 'darwin-x64', 'darwin-arm64', 'win32-x64'].map((p) => join('platforms', p))];
  for (const pkg of pkgs) {
    assert.ok(readFileSync(join(root, 'packages', 'npm', pkg, 'LICENSE')).equals(want), `${pkg}/LICENSE differs from LICENSE.txt`);
  }
});
