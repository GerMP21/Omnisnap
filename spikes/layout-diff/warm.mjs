// Build warmed template profiles for branded browsers (as-shipped mode) and
// test the one-pager assumption: a FRESH profile hasn't downloaded its
// blocking lists yet, a WARMED one has, so only the warmed one behaves like
// a real user's browser.
//
// usage: node warm.mjs [--seconds 90] [brave edge]

import fs from 'node:fs/promises';
import http from 'node:http';
import path from 'node:path';
import puppeteer from 'puppeteer-core';
import { ENGINES, PROFILES, availableEngines, launch } from './lib/engines.mjs';

const argv = process.argv.slice(2);
const si = argv.indexOf('--seconds');
const SECONDS = si >= 0 ? Number(argv[si + 1]) : 90;
const wanted = argv.filter((a, i) => !a.startsWith('--') && argv[i - 1] !== '--seconds');
const branded = availableEngines(wanted.length ? wanted : ['brave', 'edge']).filter((n) => ENGINES[n].template);
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

// Well-known third-party trackers/ads plus a web font: what built-in blockers act on.
const PROBE = `<!doctype html><html><head><meta charset="utf-8"><title>probe</title>
<link rel="stylesheet" href="https://fonts.googleapis.com/css2?family=Inter&display=swap"></head><body><h1>blocking probe</h1>
<script src="https://www.googletagmanager.com/gtag/js?id=G-OMNISNAP01"></script>
<script src="https://www.google-analytics.com/analytics.js"></script>
<script src="https://securepubads.g.doubleclick.net/tag/js/gpt.js"></script>
<script src="https://connect.facebook.net/en_US/fbevents.js"></script>
<script src="https://static.hotjar.com/c/hotjar-1.js?sv=6"></script>
<img src="https://www.facebook.com/tr?id=1&ev=PageView&noscript=1" width="1" height="1" alt="">
</body></html>`;
const server = http.createServer((_, res) => res.writeHead(200, { 'content-type': 'text/html' }).end(PROBE));
await new Promise((r) => server.listen(0, '127.0.0.1', r));
const probeUrl = `http://127.0.0.1:${server.address().port}/`;

async function probe(browser) {
  const page = await browser.newPage();
  const failed = [];
  page.on('requestfailed', (req) => failed.push({ host: new URL(req.url()).host, err: req.failure()?.errorText || '' }));
  await page.goto(probeUrl, { waitUntil: 'load', timeout: 30000 }).catch(() => {});
  await sleep(3000);
  await page.close();
  const blocked = failed.filter((f) => /BLOCKED_BY_CLIENT/i.test(f.err));
  return { blocked: blocked.length, hosts: [...new Set(blocked.map((f) => f.host))], other: failed.filter((f) => !/BLOCKED_BY_CLIENT/i.test(f.err)).map((f) => `${f.host}:${f.err}`) };
}

async function components(browser, scheme) {
  const page = await browser.newPage();
  try {
    await page.goto(`${scheme}://components`, { waitUntil: 'load', timeout: 15000 });
    await sleep(1000);
    const text = await page.evaluate(() => document.body.innerText);
    return text.split('\n').map((l) => l.trim()).filter((l) => /block|filter|tracking|trust|protection|shields/i.test(l)).slice(0, 8);
  } catch (e) { return [`(could not read ${scheme}://components: ${e.message.split('\n')[0]})`]; } finally { await page.close(); }
}

const results = [];
if (availableEngines(['chrome']).length) {
  const chrome = await launch('chrome');
  results.push({ name: 'chrome', mode: 'baseline', ...(await probe(chrome)) });
  await chrome.close();
}
for (const name of branded) {
  const cfg = ENGINES[name];
  const scheme = name === 'edge' ? 'edge' : 'brave';

  const cold = await launch(name, { cold: true });
  results.push({ name, mode: 'fresh profile', ...(await probe(cold)) });
  await cold.close();

  const dir = path.join(PROFILES, name);
  await fs.mkdir(dir, { recursive: true });
  console.log(`warming ${name} for ${SECONDS}s (component updater downloads its lists) ...`);
  // Chromium delays the first component check; fast-update shortens it, and
  // the components page's "Check for update" buttons force a check now.
  const warmer = await puppeteer.launch({ browser: cfg.browser, executablePath: cfg.executablePath, args: [...cfg.args, '--component-updater=fast-update'], userDataDir: dir, headless: true });
  const p = await warmer.newPage();
  await p.goto('https://example.com/', { waitUntil: 'load', timeout: 30000 }).catch(() => {});
  try {
    await p.goto(`${scheme}://components`, { waitUntil: 'load', timeout: 15000 });
    await sleep(2000);
    const clicked = await p.evaluate(() => {
      const buttons = [...document.querySelectorAll('button')].filter((b) => /check for update/i.test(b.textContent || ''));
      buttons.forEach((b) => b.click());
      return buttons.length;
    });
    console.log(`  ${name}: forced an update check on ${clicked} component(s)`);
  } catch (e) { console.log(`  ${name}: could not force component updates (${e.message.split('\n')[0]})`); }
  await sleep(SECONDS * 1000);
  const comps = await components(warmer, scheme);
  await warmer.close();

  const warm = await launch(name);
  results.push({ name, mode: `warmed template (${SECONDS}s)`, ...(await probe(warm)), comps });
  await warm.close();
}
server.close();

console.log('\nBlocking probe: 7 third-party requests (5 trackers/ads, 1 pixel, 1 web font)');
for (const r of results) {
  console.log(`  ${r.name.padEnd(7)} ${r.mode.padEnd(26)} blocked ${r.blocked}  ${r.hosts.join(', ')}`);
  if (r.other.length) console.log(`  ${''.padEnd(35)} other failures: ${r.other.slice(0, 4).join(' ; ')}`);
  if (r.comps) for (const c of r.comps) console.log(`  ${''.padEnd(35)} component: ${c}`);
}
console.log(`\ntemplates in ${path.relative(process.cwd(), PROFILES) || PROFILES}/ (gitignored); capture.mjs copies them per run`);
