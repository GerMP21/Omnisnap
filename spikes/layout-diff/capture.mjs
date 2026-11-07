// Capture geometry + screenshots in every installed engine, using the
// browsers already on the machine. THROWAWAY: puppeteer-core (CDP/BiDi) and
// lib/webdriver.mjs (WebDriver Classic) stand in for the future Go drivers;
// inject/*.js is what survives.
//
// usage: node capture.mjs [planted|real|own|all] [--only <slug-substring>] [--engines chrome,firefox,webkit]
// output: out/<slug>/<engine>-{a,b}.{json,png} + out/<slug>/page.json
//         (two runs per engine: run b reveals dynamic content)

import fs from 'node:fs/promises';
import http from 'node:http';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { PNG } from 'pngjs';
import { FONT_PARITY, availableEngines, launch } from './lib/engines.mjs';
import { PAGES, renderPage } from './planted/pages.mjs';

const HERE = path.dirname(fileURLToPath(import.meta.url));
const OUT = path.join(HERE, 'out');
const STABILIZE = await fs.readFile(path.join(HERE, 'inject/stabilize.js'), 'utf8');
const EXTRACT = await fs.readFile(path.join(HERE, 'inject/extract.js'), 'utf8');
const VIEWPORT = { width: 1280, height: 800, deviceScaleFactor: 1 };
const MAX_SHOT = { w: 2560, h: 16000 };
const RUNS = ['a', 'b'];
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

const argv = process.argv.slice(2);
const opt = (name) => { const i = argv.indexOf(name); return i >= 0 ? argv[i + 1] : null; };
const group = argv.find((a, i) => !a.startsWith('--') && !['--only', '--engines'].includes(argv[i - 1])) || 'planted';
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
for (const g of ['real', 'own']) {
  if (group === g || group === 'all') for (const t of corpus[g] || []) targets.push({ ...t, group: g });
}
const selected = targets.filter((t) => !only || t.slug.includes(only));
if (!selected.length) {
  console.error(`nothing to capture for group "${group}"${only ? ` / --only ${only}` : ''} (check corpus.json)`);
  process.exit(1);
}

// ------------------------------------------------- CDP / BiDi (puppeteer)

async function capturePuppeteer(browser, url, outBase) {
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
    await page.goto(url, { waitUntil: 'load', timeout: 45000 });
    await page.waitForNetworkIdle({ idleTime: 500, timeout: 8000 }).catch(() => {});
    const stab = JSON.parse(await page.evaluate(STABILIZE));
    await page.waitForNetworkIdle({ idleTime: 500, timeout: 5000 }).catch(() => {});
    const geo = JSON.parse(await page.evaluate(EXTRACT));
    const w = Math.min(Math.ceil(Math.max(geo.meta.vw, geo.meta.docW)), MAX_SHOT.w);
    const h = Math.min(Math.ceil(geo.meta.docH), MAX_SHOT.h);
    try {
      await page.screenshot({ path: `${outBase}.png`, clip: { x: 0, y: 0, width: w, height: h }, captureBeyondViewport: true });
    } catch (e) {
      await page.screenshot({ path: `${outBase}.png`, fullPage: true });
      geo.meta.shotNote = `clip failed, used fullPage: ${e.message}`;
    }
    // Consistency check: did the page change while we took the screenshot?
    const post = JSON.parse(await page.evaluate(EXTRACT)).meta;
    geo.meta.post = { docH: post.docH, nodes: post.nodes };
    geo.meta.shot = { w, h };
    geo.meta.stabilize = stab;
    geo.meta.failed = failed;
    geo.meta.blocked = failed.filter((f) => /BLOCKED_BY_CLIENT/i.test(f.err)).length;
    geo.meta.ms = Date.now() - t0;
    await fs.writeFile(`${outBase}.json`, JSON.stringify(geo));
    return { ok: true, nodes: geo.meta.nodes, ms: geo.meta.ms, blocked: geo.meta.blocked };
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

async function captureWebDriver(driver, url, outBase) {
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
    const geo = JSON.parse(await sess.exec(EXTRACT));
    if (geo.omnisnapError) throw new Error(`extract: ${geo.omnisnapError}`);
    const shot = await stitch(sess, geo.meta);
    const post = JSON.parse(await sess.exec(EXTRACT)).meta;
    geo.meta.post = { docH: post.docH, nodes: post.nodes };
    await fs.writeFile(`${outBase}.png`, shot.png);
    geo.meta.shot = shot.dims;
    geo.meta.shotNote = shot.note + (fit && (fit[0] !== VIEWPORT.width || fit[1] !== VIEWPORT.height) ? `; viewport could only be sized to ${fit[0]}x${fit[1]}` : '');
    geo.meta.stabilize = stab;
    geo.meta.failed = [];
    geo.meta.blocked = 0;
    geo.meta.ms = Date.now() - t0;
    await fs.writeFile(`${outBase}.json`, JSON.stringify(geo));
    return { ok: true, nodes: geo.meta.nodes, ms: geo.meta.ms };
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
  await fs.writeFile(path.join(OUT, t.slug, 'page.json'), JSON.stringify({ slug: t.slug, url: t.url, group: t.group, fontParity: FONT_PARITY }, null, 2));
}

let enginesInfo = {};
try { enginesInfo = JSON.parse(await fs.readFile(path.join(OUT, 'engines.json'), 'utf8')); } catch (_) { /* first run */ }

const browsers = {};
for (const name of availableEngines(engineFilter)) {
  try {
    browsers[name] = await launch(name);
    enginesInfo[name] = { version: await browsers[name].version(), profile: browsers[name].omnisnap.profile };
    console.log(`${name.padEnd(9)} ${enginesInfo[name].version}  [${browsers[name].omnisnap.protocol}, ${enginesInfo[name].profile}]`);
  } catch (e) {
    console.warn(`${name.padEnd(9)} launch failed: ${String(e.message || e).split('\n')[0]}`);
  }
}
const names = Object.keys(browsers);

// Interleave engines per page so live sites can't change between engines.
for (const t of selected) {
  const cells = [];
  for (const run of RUNS) {
    for (const name of names) {
      const b = browsers[name];
      const outBase = path.join(OUT, t.slug, `${name}-${run}`);
      const r = b.omnisnap.protocol === 'webdriver' ? await captureWebDriver(b, t.url, outBase) : await capturePuppeteer(b, t.url, outBase);
      cells.push(`${name}-${run} ${r.ok ? `${r.nodes}n${r.blocked ? ` ${r.blocked}blk` : ''}` : `FAILED ${r.error}`}`);
    }
  }
  console.log(`  ${t.slug.padEnd(22)} ${cells.join(' | ')}`);
}
for (const name of names) {
  if (browsers[name].omnisnap.protocol === 'webdriver') enginesInfo[name].version = await browsers[name].version();
  await browsers[name].close();
}
await fs.writeFile(path.join(OUT, 'engines.json'), JSON.stringify(enginesInfo, null, 2));
if (server) server.close();
console.log(`\ncaptured ${selected.length} page(s) with ${names.join(', ')} into out/`);
