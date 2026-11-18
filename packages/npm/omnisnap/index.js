// Thin wrapper around the Omnisnap binary (SPEC-wrappers.md): write a job.json,
// run the binary, stream its NDJSON progress, return the parsed summary. No
// capture or diff logic lives here. Zero runtime dependencies.
import { spawn } from 'node:child_process';
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const require = createRequire(import.meta.url);

/** A failed binary run: the exit code and stderr travel with the error. */
export class OmnisnapError extends Error {
  constructor(message, { code = null, stderr = '', args = [] } = {}) {
    super(message);
    this.name = 'OmnisnapError';
    this.code = code;
    this.stderr = stderr;
    this.args = args;
  }
}

/** Find the binary: platform package, then OMNISNAP_BINARY, then PATH. */
export function resolveBinary() {
  if (process.env.OMNISNAP_BINARY) return process.env.OMNISNAP_BINARY;
  const key = `${process.platform}-${process.arch}`;
  const names = { 'linux-x64': 'linux-x64', 'linux-arm64': 'linux-arm64', 'darwin-x64': 'darwin-x64', 'darwin-arm64': 'darwin-arm64', 'win32-x64': 'win32-x64' };
  const pkg = names[key];
  if (pkg) {
    try {
      const manifest = require.resolve(`@omnisnap/${pkg}/package.json`);
      return join(dirname(manifest), process.platform === 'win32' ? 'omnisnap.exe' : 'omnisnap');
    } catch { /* not installed: fall through */ }
  }
  return process.platform === 'win32' ? 'omnisnap.exe' : 'omnisnap';
}

/** Run the binary, parse NDJSON lines as they arrive, collect stderr. */
export function runBinary(args, { onProgress, cwd, env } = {}) {
  const binary = resolveBinary();
  return new Promise((resolvePromise, reject) => {
    const child = spawn(binary, args, { cwd, env: { ...process.env, ...env } });
    let stderr = '';
    let stdout = '';
    let buffer = '';
    let resultLine = null;
    const handle = (line) => {
      line = line.trim();
      if (!line) return;
      let event;
      try { event = JSON.parse(line); } catch { return; } // not NDJSON: ignore
      // Only typed events are progress; `browsers --json` writes an array.
      if (event && !Array.isArray(event) && typeof event === 'object' && event.type) {
        if (onProgress) onProgress(event);
        if (event.type === 'done') resultLine = event;
      }
    };
    child.stdout.on('data', (chunk) => {
      stdout += chunk;
      buffer += chunk;
      let i;
      while ((i = buffer.indexOf('\n')) >= 0) {
        handle(buffer.slice(0, i));
        buffer = buffer.slice(i + 1);
      }
    });
    child.stderr.on('data', (chunk) => { stderr += chunk; });
    child.on('error', (err) => reject(new OmnisnapError(`cannot run ${binary}: ${err.message}`, { args })));
    child.on('close', (code) => {
      handle(buffer); // a last line without a newline
      if (code !== 0) {
        reject(new OmnisnapError(`omnisnap ${args.join(' ')} exited ${code}: ${stderr.trim()}`, { code, stderr, args }));
        return;
      }
      resolvePromise({ code, stdout, stderr, done: resultLine });
    });
  });
}

/** Normalise the API's page list to the job schema. */
function jobOf({ pages, engines, baseline, runs, viewport, timeouts, probes, fontParity }) {
  const out = { pages: [] };
  for (const p of pages || []) out.pages.push(typeof p === 'string' ? { url: p } : { ...p });
  if (engines && engines.length) out.engines = engines;
  if (baseline) out.baseline = baseline;
  if (runs) out.runs = runs;
  if (viewport) out.viewport = viewport;
  if (timeouts) out.timeouts = timeouts;
  if (probes !== undefined) out.probes = probes;
  if (fontParity !== undefined) out.fontParity = fontParity;
  return out;
}

/** Capture and compare a job; returns the parsed summary.json. */
export async function run(options = {}) {
  if (!options.pages || !options.pages.length) throw new OmnisnapError('run() needs at least one page', {});
  const out = resolve(options.out || './omnisnap-out');
  mkdirSync(out, { recursive: true });
  const jobPath = join(out, 'omnisnap-job.json');
  writeFileSync(jobPath, JSON.stringify(jobOf(options), null, 1));
  await runBinary(['run', jobPath, '--json', '--out', out], { onProgress: options.onProgress, cwd: options.cwd });
  return JSON.parse(readFileSync(join(out, 'summary.json'), 'utf8'));
}

/** The installed browsers, as `omnisnap browsers --json` (a JSON array). */
export async function browsers() {
  const { stdout } = await runBinary(['browsers', '--json']);
  return JSON.parse(stdout);
}

/** Merge bundles; returns the merged summary. */
export async function merge(bundles, out, { onProgress } = {}) {
  if (!bundles || !bundles.length) throw new OmnisnapError('merge() needs bundle directories', {});
  const dest = resolve(out);
  await runBinary(['merge', ...bundles.map((b) => resolve(b)), '-o', dest], { onProgress });
  return JSON.parse(readFileSync(join(dest, 'summary.json'), 'utf8'));
}

/** Re-render a bundle's report.html; returns its path. */
export async function report(bundle) {
  const dir = resolve(bundle);
  await runBinary(['report', dir]);
  return join(dir, 'report.html');
}
