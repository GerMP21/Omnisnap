// Capture geometry + screenshots in every installed engine, using the
// browsers already on the machine. THROWAWAY: puppeteer-core (CDP/BiDi) and
// lib/webdriver.mjs (WebDriver Classic) stand in for the future Go drivers;
// inject/*.js is what survives.
//
// usage: node capture.mjs [planted|real|own|crawl|all|mut] [--only <slug-substring>] [--engines chrome,firefox,webkit] [--shard i/n]
//        --shard i/n: this process takes every n-th page (run n processes in parallel)
//        --resume: skip pages whose captures already exist (restart an interrupted run)
//                  and pages marked unreachable (failed.json; RETRY_FAILED=1 retries them)
// output: out/<slug>/<engine>-{a,b}.{json,png} + out/<slug>/page.json
//         (two runs per engine: run b reveals dynamic content)
//         OMNISNAP_OUT=<dir> writes elsewhere (large crawls: use a Linux disk)
//
// mut: mutation testing on already captured real/own/crawl pages. For each
// page and target engine (default firefox, webkit), MUT_PER_ENGINE (4)
// mutants: one run of that engine with ONE realistic bug injected
// (inject/mutate.js), into out/<slug>~<engine>~<i>/. The baseline, the
// third engine and the clean runs are reused from out/<slug>/.

import fs from 'node:fs/promises';
import http from 'node:http';
import path from 'node:path';
import zlib from 'node:zlib';
import { fileURLToPath } from 'node:url';
import { PNG } from 'pngjs';
import { FONT_PARITY, availableEngines, launch } from './lib/engines.mjs';
import { PAGES, renderPage } from './planted/pages.mjs';

const HERE = path.dirname(fileURLToPath(import.meta.url));
const OUT = process.env.OMNISNAP_OUT || path.join(HERE, 'out');
const STABILIZE = await fs.readFile(path.join(HERE, 'inject/stabilize.js'), 'utf8');
const EXTRACT = await fs.readFile(path.join(HERE, 'inject/extract.js'), 'utf8');
const PROBES = await fs.readFile(path.join(HERE, 'inject/probes.js'), 'utf8');
const MUTATE = await fs.readFile(path.join(HERE, 'inject/mutate.js'), 'utf8');
const MUT_OPS = ['width-shrink', 'padding', 'padding-left', 'margin-top', 'nudge', 'hide', 'invisible', 'gap', 'wrap-narrow', 'font-up', 'nowrap', 'align-self', 'overlap', 'clip', 'order', 'letter-spacing', 'line-height', 'border', 'ellipsis-clip'];
const MUT_PER_ENGINE = Number(process.env.MUT_PER_ENGINE || 4);
const hash32 = (s) => { let h = 2166136261; for (const ch of s) { h ^= ch.charCodeAt(0); h = Math.imul(h, 16777619); } return h >>> 0; };
// Several mutants share one settled page load (JS timers are frozen after
// stabilize.js): inject one bug, measure, revert, inject the next.
async function eachMutant(evaluate, mutants, measure) {
  await evaluate(MUTATE);
  const out = [];
  for (const m of mutants) {
    try {
      const mutation = JSON.parse(await evaluate(`window.__omnisnapMutate(${JSON.stringify(m.ops)}, ${m.seed})`));
      if (!mutation.ok) throw new Error(`mutation: no candidate for ${m.ops.length} operators`);
      out.push(await measure(m.outBase, mutation));
    } catch (e) {
      await fs.writeFile(`${m.outBase}.error.txt`, String(e.stack || e));
      out.push({ ok: false, error: String(e.message || e).split('\n')[0] });
    }
    await evaluate('window.__omnisnapMutateRevert()').catch(() => {});
  }
  return out;
}
const PROBE_IDS = ['lh', 'font', 'controls', 'all'];

// Causal interventions (inject/probes.js): re-measure the page under each
// neutralizing probe and store the geometries next to the capture.
// PROBES=0 skips them.
async function runProbes(evaluate, outBase) {
  if (process.env.PROBES === '0') return null;
  const t0 = Date.now();
  await evaluate(PROBES);
  const out = {};
  for (const id of PROBE_IDS) {
    const applied = JSON.parse(await evaluate(`window.__omnisnapProbe.apply(${JSON.stringify(id)})`));
    out[id] = JSON.parse(await evaluate(EXTRACT));
    out[id].meta.probe = applied;
    await evaluate('window.__omnisnapProbe.revert()');
  }
  await fs.writeFile(`${outBase}.probes.json.gz`, zlib.gzipSync(JSON.stringify(out)));
  return { ids: PROBE_IDS, ms: Date.now() - t0 };
}
const VIEWPORT = { width: 1280, height: 800, deviceScaleFactor: 1 };
const MAX_SHOT = { w: 2560, h: 16000 };
const RUNS = ['a', 'b'];
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

const argv = process.argv.slice(2);
const opt = (name) => { const i = argv.indexOf(name); return i >= 0 ? argv[i + 1] : null; };
const group = argv.find((a, i) => !a.startsWith('--') && !['--only', '--engines', '--shard'].includes(argv[i - 1])) || 'planted';
const [shardI, shardN] = (opt('--shard') || '0/1').split('/').map(Number);
const ext = async (f) => { try { return JSON.parse(await fs.readFile(f, 'utf8')); } catch (_) { return null; } };
const only = opt('--only');
const engineFilter = opt('--engines') ? opt('--engines').split(',') : null;

// ------------------------------------------------------------------ targets

const corpus = JSON.parse(await fs.readFile(path.join(HERE, 'corpus.json'), 'utf8'));
let server = null;
const targets = [];
if (group === 'planted' || group === 'all') {
  server = http.createServer((req, res) => {
    const slug = decodeURIComponent(req.url.slice(1)).replace(/\.html$/, '');
    const page = PAGES.find((p) => p.slug === slug);
    if (!page) { res.writeHead(404).end(); return; }
    res.writeHead(200, { 'content-type': 'text/html; charset=utf-8' }).end(renderPage(page));
  });
  await new Promise((r) => server.listen(0, '127.0.0.1', r));
  const base = `http://127.0.0.1:${server.address().port}`;
  for (const p of PAGES) targets.push({ slug: p.slug, url: `${base}/${p.slug}.html`, group: 'planted' });
}
// crawl-sites.json (generated by sites.mjs) extends the hand-picked crawl list.
// CRAWL_LIMIT=n caps the whole crawl at n sites: already captured sites
// first, then the rest by rank.
let generated = (await ext(path.join(HERE, 'crawl-sites.json')))?.sites || [];
if (process.env.CRAWL_LIMIT) {
  const room = Math.max(0, Number(process.env.CRAWL_LIMIT) - (corpus.crawl || []).length);
  const captured = new Set();
  for (const s of generated) { try { await fs.access(path.join(OUT, s.slug, 'chrome-a.json')); captured.add(s.slug); } catch (_) { /* not yet */ } }
  generated = [...generated.filter((s) => captured.has(s.slug)), ...generated.filter((s) => !captured.has(s.slug)).sort((a, b) => a.rank - b.rank)].slice(0, room);
}
const lists = { ...corpus, crawl: [...(corpus.crawl || []), ...generated.filter((s) => !(corpus.crawl || []).some((c) => c.slug === s.slug))] };
for (const g of ['real', 'own', 'crawl']) {
  if (group === g || (group === 'all' && g !== 'crawl')) for (const t of lists[g] || []) targets.push({ ...t, group: g });
}
if (group === 'mut') {
  const mutEngines = engineFilter || ['firefox', 'webkit'];
  for (const entry of (await fs.readdir(OUT, { withFileTypes: true })).filter((e) => e.isDirectory() && !e.name.includes('~'))) {
    const page = await ext(path.join(OUT, entry.name, 'page.json'));
    if (!page || !['real', 'own', 'crawl'].includes(page.group)) continue;
    try { await fs.access(path.join(OUT, entry.name, 'chrome-a.json')); } catch (_) { continue; }
    for (const engine of mutEngines) {
      // Only pages where the engines rendered the same state are worth
      // mutating (others are excluded from training): node counts within 20%.
      const [ca, ea] = [await ext(path.join(OUT, entry.name, 'chrome-a.json')), await ext(path.join(OUT, entry.name, `${engine}-a.json`))];
      if (!ca || !ea || ca.meta.nodes < 50 || Math.abs(ca.meta.nodes - ea.meta.nodes) / Math.max(ca.meta.nodes, ea.meta.nodes) > 0.2) continue;
      for (let i = 0; i < MUT_PER_ENGINE; i++) {
        const seed = hash32(`${page.slug}|${engine}|${i}`);
        const start = seed % MUT_OPS.length; // operators rotate; the next ones are fallbacks
        const ops = [...MUT_OPS.slice(start), ...MUT_OPS.slice(0, start)];
        targets.push({ slug: `${page.slug}~${engine}~${i}`, base: page.slug, url: page.url, group: 'mut', engine, mutate: { ops, seed } });
      }
    }
  }
}
// Shards split by page (a mutant follows its base page, so one load serves all its mutants).
const shardOf = (t) => { let h = 0; for (const ch of t.base || t.slug) h = (h * 31 + ch.charCodeAt(0)) >>> 0; return h % shardN; };
const resume = argv.includes('--resume');
const exists = async (f) => { try { await fs.access(f); return true; } catch (_) { return false; } };
const done = async (t) => {
  if (t.mutate) return exists(path.join(OUT, t.slug, `${t.engine}-a.json`));
  if (process.env.RETRY_FAILED !== '1' && (await exists(path.join(OUT, t.slug, 'failed.json')))) return true;
  // Done = every engine was attempted to the end (run b captured, or failed).
  const attempted = async (e) => (await exists(path.join(OUT, t.slug, `${e}-b.json`))) || (await exists(path.join(OUT, t.slug, `${e}-b.error.txt`))) || (await exists(path.join(OUT, t.slug, `${e}-a.error.txt`)));
  return (await Promise.all(availableEngines(engineFilter).map(attempted))).every(Boolean);
};
const selected = [];
for (const t of targets) if ((!only || t.slug.includes(only)) && shardOf(t) === shardI && !(resume && (await done(t)))) selected.push(t);
if (resume) console.log(`resume: ${selected.length} page(s) left to capture`);
if (!selected.length) {
  if (resume && targets.length) { console.log('nothing left to capture'); process.exit(0); }
  console.error(`nothing to capture for group "${group}"${only ? ` / --only ${only}` : ''} (check corpus.json)`);
  process.exit(1);
}

// ------------------------------------------------- CDP / BiDi (puppeteer)

async function capturePuppeteer(browser, url, outBase, mutants = null) {
  let ctx = null;
  let page;
  try {
    ctx = await browser.createBrowserContext();
    page = await ctx.newPage();
  } catch (_) {
    ctx = null;
    page = await browser.newPage();
  }
  // Failed requests explain branded-browser differences: a blocked font or
  // script (ERR_BLOCKED_BY_CLIENT) is the CAUSE of the layout change it makes.
  const failed = [];
  page.on('requestfailed', (req) => {
    if (failed.length < 300) failed.push({ url: req.url().slice(0, 200), err: req.failure()?.errorText || '' });
  });
  const t0 = Date.now();
  try {
    await page.setViewport(VIEWPORT);
    await page.goto(url, { waitUntil: 'load', timeout: 30000 });
    await page.waitForNetworkIdle({ idleTime: 500, timeout: 8000 }).catch(() => {});
    const stab = JSON.parse(await page.evaluate(STABILIZE));
    await page.waitForNetworkIdle({ idleTime: 500, timeout: 5000 }).catch(() => {});
    const evaluate = (expr) => page.evaluate(expr);
    const measure = async (base, mutation) => {
      const geo = JSON.parse(await page.evaluate(EXTRACT));
      if (mutation) geo.meta.mutation = mutation;
      const w = Math.min(Math.ceil(Math.max(geo.meta.vw, geo.meta.docW)), MAX_SHOT.w);
      const h = Math.min(Math.ceil(geo.meta.docH), MAX_SHOT.h);
      try {
        await page.screenshot({ path: `${base}.png`, clip: { x: 0, y: 0, width: w, height: h }, captureBeyondViewport: true });
      } catch (e) {
        await page.screenshot({ path: `${base}.png`, fullPage: true });
        geo.meta.shotNote = `clip failed, used fullPage: ${e.message}`;
      }
      // Consistency check: did the page change while we took the screenshot?
      const post = JSON.parse(await page.evaluate(EXTRACT)).meta;
      geo.meta.post = { docH: post.docH, nodes: post.nodes };
      geo.meta.probes = await runProbes(evaluate, base);
      geo.meta.shot = { w, h };
      geo.meta.stabilize = stab;
      geo.meta.failed = failed;
      geo.meta.blocked = failed.filter((f) => /BLOCKED_BY_CLIENT/i.test(f.err)).length;
      geo.meta.ms = Date.now() - t0;
      await fs.writeFile(`${base}.json`, JSON.stringify(geo));
      return { ok: true, nodes: geo.meta.nodes, ms: geo.meta.ms, blocked: geo.meta.blocked, mutation };
    };
    if (mutants) return { ok: true, mutants: await eachMutant(evaluate, mutants, measure) };
    return await measure(outBase, null);
  } catch (e) {
    await fs.writeFile(`${outBase}.error.txt`, String(e.stack || e));
    return { ok: false, error: String(e.message || e).split('\n')[0] };
  } finally {
    await (ctx ? ctx.close() : page.close()).catch(() => {});
  }
}

// ------------------------------------------------- WebDriver Classic (WebKit)

function downscale(img, f) {
  const w = Math.round(img.width * f);
  const h = Math.round(img.height * f);
  const out = new PNG({ width: w, height: h });
  for (let y = 0; y < h; y++) {
    const sy = Math.min(img.height - 1, Math.floor(y / f));
    for (let x = 0; x < w; x++) {
      const sx = Math.min(img.width - 1, Math.floor(x / f));
      img.data.copy(out.data, (y * w + x) * 4, (sy * img.width + sx) * 4, (sy * img.width + sx) * 4 + 4);
    }
  }
  return out;
}

// Classic screenshots cover the viewport only: scroll and stitch. Fixed and
// sticky elements are hidden after the first frame so they appear once.
async function stitch(sess, meta) {
  const W = Math.min(Math.ceil(meta.vw), MAX_SHOT.w);
  const H = Math.min(Math.ceil(meta.docH), MAX_SHOT.h);
  const out = new PNG({ width: W, height: H });
  out.data.fill(255);
  await sess.exec(`(() => { for (const el of document.querySelectorAll('body *')) { const p = getComputedStyle(el).position; if (p === 'fixed' || p === 'sticky') el.setAttribute('data-omnisnap-fixed', ''); } return 0; })()`);
  let frames = 0;
  let scale = 1;
  for (let y = 0; y < H; y += meta.vh) {
    const sy = await sess.exec(`new Promise((r) => { scrollTo(0, ${y}); const raf = window.__omnisnapRAF || requestAnimationFrame; raf(() => raf(() => r(Math.round(scrollY)))); })`);
    if (frames === 1) await sess.exec(`(() => { const s = document.createElement('style'); s.id = 'omnisnap-hide-fixed'; s.textContent = '[data-omnisnap-fixed] { visibility: hidden !important; }'; document.documentElement.append(s); return 0; })()`);
    let frame = PNG.sync.read(await sess.screenshot());
    scale = frame.width / meta.vw;
    if (Math.abs(scale - 1) > 0.01) frame = downscale(frame, 1 / scale);
    const rows = Math.min(frame.height, H - sy);
    const cols = Math.min(frame.width, W);
    for (let r = 0; r < rows; r++) frame.data.copy(out.data, (sy + r) * W * 4, r * frame.width * 4, (r * frame.width + cols) * 4);
    frames++;
    if (frame.height > meta.vh + 1 || sy + rows >= H) break; // full-document screenshot, or done
  }
  await sess.exec(`(() => { document.getElementById('omnisnap-hide-fixed')?.remove(); for (const el of document.querySelectorAll('[data-omnisnap-fixed]')) el.removeAttribute('data-omnisnap-fixed'); scrollTo(0, 0); return 0; })()`);
  return { png: PNG.sync.write(out), dims: { w: W, h: H }, note: `scroll-and-stitch, ${frames} frame(s)${Math.abs(scale - 1) > 0.01 ? `, screenshots at ${scale.toFixed(2)}x downscaled` : ''}` };
}

async function captureWebDriver(driver, url, outBase, mutants = null) {
  const t0 = Date.now();
  let sess = null;
  try {
    sess = await driver.newSession();
    // Size the WINDOW so the VIEWPORT is exactly 1280x800: window chrome varies
    // and the window manager applies resizes asynchronously, so measure and
    // correct until it fits (a viewport of the wrong height breaks vh units).
    await sess.navigate('about:blank');
    let fit = null;
    for (let attempt = 0; attempt < 5; attempt++) {
      const [iw, ih] = JSON.parse(await sess.exec('JSON.stringify([innerWidth, innerHeight])'));
      fit = [iw, ih];
      if (iw === VIEWPORT.width && ih === VIEWPORT.height) break;
      const rect = await sess.getWindow();
      await sess.setWindow(rect.width + VIEWPORT.width - iw, rect.height + VIEWPORT.height - ih);
      await sleep(300);
    }
    await sess.navigate(url);
    await sleep(1500); // Classic has no network events, so no network-idle signal
    const stab = JSON.parse(await sess.exec(STABILIZE));
    if (stab.omnisnapError) throw new Error(`stabilize: ${stab.omnisnapError}`);
    await sleep(500);
    const evaluate = (expr) => sess.exec(expr);
    const measure = async (base, mutation) => {
      const geo = JSON.parse(await sess.exec(EXTRACT));
      if (geo.omnisnapError) throw new Error(`extract: ${geo.omnisnapError}`);
      if (mutation) geo.meta.mutation = mutation;
      const shot = await stitch(sess, geo.meta);
      const post = JSON.parse(await sess.exec(EXTRACT)).meta;
      geo.meta.post = { docH: post.docH, nodes: post.nodes };
      geo.meta.probes = await runProbes(evaluate, base);
      await fs.writeFile(`${base}.png`, shot.png);
      geo.meta.shot = shot.dims;
      geo.meta.shotNote = shot.note + (fit && (fit[0] !== VIEWPORT.width || fit[1] !== VIEWPORT.height) ? `; viewport could only be sized to ${fit[0]}x${fit[1]}` : '');
      geo.meta.stabilize = stab;
      geo.meta.failed = [];
      geo.meta.blocked = 0;
      geo.meta.ms = Date.now() - t0;
      await fs.writeFile(`${base}.json`, JSON.stringify(geo));
      return { ok: true, nodes: geo.meta.nodes, ms: geo.meta.ms, mutation };
    };
    if (mutants) return { ok: true, mutants: await eachMutant(evaluate, mutants, measure) };
    return await measure(outBase, null);
  } catch (e) {
    await fs.writeFile(`${outBase}.error.txt`, String(e.stack || e));
    return { ok: false, error: String(e.message || e).split('\n')[0] };
  } finally {
    if (sess) await sess.close();
  }
}

// --------------------------------------------------------------------- run

await fs.mkdir(OUT, { recursive: true });
for (const t of selected) {
  await fs.mkdir(path.join(OUT, t.slug), { recursive: true });
  await fs.writeFile(path.join(OUT, t.slug, 'page.json'), JSON.stringify({ slug: t.slug, url: t.url, group: t.group, fontParity: FONT_PARITY, ...(t.base ? { base: t.base, engine: t.engine, seed: t.mutate.seed } : {}) }, null, 2));
}

let enginesInfo = {};
try { enginesInfo = JSON.parse(await fs.readFile(path.join(OUT, 'engines.json'), 'utf8')); } catch (_) { /* first run */ }

const browsers = {};
const needed = group === 'mut' ? [...new Set(selected.map((t) => t.engine))] : engineFilter;
for (const name of availableEngines(needed)) {
  try {
    browsers[name] = await launch(name);
    enginesInfo[name] = { version: await browsers[name].version(), profile: browsers[name].omnisnap.profile };
    console.log(`${name.padEnd(9)} ${enginesInfo[name].version}  [${browsers[name].omnisnap.protocol}, ${enginesInfo[name].profile}]`);
  } catch (e) {
    console.warn(`${name.padEnd(9)} launch failed: ${String(e.message || e).split('\n')[0]}`);
  }
}
const names = Object.keys(browsers);

// Two lanes run in parallel on the same page: the CDP/BiDi engines (Chrome,
// Firefox, ...) one after another, and WebKit over WebDriver Classic, which
// is as slow as the others together (no load events, scroll-and-stitch
// screenshots). Wall time is the slower lane, not the sum, and the engines
// load a live page even closer together in time.
const lanes = [names.filter((n) => browsers[n].omnisnap.protocol !== 'webdriver'), names.filter((n) => browsers[n].omnisnap.protocol === 'webdriver')].filter((l) => l.length);
const capture = (name, url, outBase, mutants) => (browsers[name].omnisnap.protocol === 'webdriver' ? captureWebDriver(browsers[name], url, outBase, mutants) : capturePuppeteer(browsers[name], url, outBase, mutants));

// Mutants: one page load per (base page, engine), all its mutants in it; a
// page's engines run in parallel lanes.
const mutByBase = new Map();
for (const t of selected.filter((x) => x.mutate)) {
  if (!browsers[t.engine]) continue;
  if (!mutByBase.has(t.base)) mutByBase.set(t.base, new Map());
  const byEngine = mutByBase.get(t.base);
  if (!byEngine.has(t.engine)) byEngine.set(t.engine, []);
  byEngine.get(t.engine).push(t);
}
for (const byEngine of mutByBase.values()) {
  const lines = [];
  await Promise.all([...byEngine.values()].map(async (ts) => {
    const mutants = ts.map((t) => ({ ...t.mutate, outBase: path.join(OUT, t.slug, `${t.engine}-a`) }));
    const r = await capture(ts[0].engine, ts[0].url, mutants[0].outBase, mutants);
    const each = r.ok ? r.mutants : ts.map(() => r);
    ts.forEach((t, i) => lines.push(`  ${t.slug.padEnd(30)} ${each[i].ok ? `${each[i].nodes}n ${each[i].mutation.op} on ${each[i].mutation.tag}${each[i].mutation.scope > 1 ? ` (x${each[i].mutation.scope})` : ''}` : `FAILED ${each[i].error}`}`));
  }));
  console.log(lines.sort().join('\n'));
}

// Clean pages: two runs, each run captured by all engines (lanes in parallel).
const cellText = (name, run, r) => `${name}-${run} ${r.ok ? `${r.nodes}n${r.blocked ? ` ${r.blocked}blk` : ''}` : `FAILED ${r.error}`}`;
for (const t of selected.filter((x) => !x.mutate)) {
  const res = {};
  const live = t.group !== 'planted';
  for (const run of RUNS) {
    // A live site that the baseline can't load in run a is unreachable:
    // don't spend every other engine's timeout on it. Marked for --resume.
    // So is a bot wall: a baseline page with almost no elements.
    const base = res[`${names[0]}-a`] || '';
    if (run === 'b' && live && (base.includes('FAILED') || Number((/ (\d+)n/.exec(base) || [])[1]) < 50)) {
      await fs.writeFile(path.join(OUT, t.slug, 'failed.json'), JSON.stringify({ at: new Date().toISOString(), runA: Object.values(res) }, null, 1));
      res.b = 'skipped run b: unreachable';
      break;
    }
    await Promise.all(lanes.map(async (lane) => {
      for (const name of lane) {
        const outBase = path.join(OUT, t.slug, `${name}-${run}`);
        const baseA = res[`${names[0]}-a`] || '';
        if (run === 'a' && name !== names[0] && live && baseA.includes('FAILED') && /timeout|ERR_/i.test(baseA)) { res[`${name}-a`] = `${name}-a skipped`; continue; }
        // An engine that could not load run a won't be compared on this page: skip its run b.
        if (run === 'b' && live && (res[`${name}-a`] || '').includes('FAILED')) { res[`${name}-b`] = `${name}-b skipped`; continue; }
        res[`${name}-${run}`] = cellText(name, run, await capture(name, t.url, outBase));
      }
    }));
  }
  const cells = [...RUNS.flatMap((run) => names.map((n) => res[`${n}-${run}`]).filter(Boolean)), res.b].filter(Boolean);
  console.log(`  ${t.slug.padEnd(22)} ${cells.join(' | ')}`);
}
for (const name of names) {
  if (browsers[name].omnisnap.protocol === 'webdriver') enginesInfo[name].version = await browsers[name].version();
  await browsers[name].close();
}
await fs.writeFile(path.join(OUT, 'engines.json'), JSON.stringify(enginesInfo, null, 2));
if (server) server.close();
console.log(`\ncaptured ${selected.length} page(s) with ${names.join(', ')} into out/`);
