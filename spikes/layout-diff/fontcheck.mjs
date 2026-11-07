// Precondition check: which installed font does each engine actually use for
// the generic families, and do scripts without an installed font (CJK, ...)
// render differently? If engines resolve `sans-serif` to different fonts,
// every text width differs and layout diff measures the environment, not the
// page.
//
// usage: node fontcheck.mjs [--engines chrome,firefox,webkit]

import { FONT_PARITY, availableEngines, launch } from './lib/engines.mjs';

const PROBE = `(() => {
  const ctx = document.createElement('canvas').getContext('2d');
  const sample = 'The quick brown fox jumps over the lazy dog 0123456789';
  const width = (font, text = sample) => { ctx.font = '16px ' + font; return Math.round(ctx.measureText(text).width * 10) / 10; };
  const candidates = ['Noto Sans', 'DejaVu Sans', 'Liberation Sans', 'Noto Serif', 'DejaVu Serif', 'Liberation Serif', 'Noto Sans Mono', 'DejaVu Sans Mono', 'Liberation Mono'];
  // A missing font silently falls back, so only trust names that measure the
  // same with two different fallbacks behind them.
  const named = candidates.filter((f) => width('"' + f + '", monospace') === width('"' + f + '", serif'));
  const widths = Object.fromEntries(named.map((f) => [f, width('"' + f + '"')]));
  const resolve = (generic) => {
    const w = width(generic);
    const hit = named.find((f) => Math.abs(widths[f] - w) < 0.05);
    return { width: w, font: hit || 'unknown' };
  };
  const span = document.createElement('span');
  span.textContent = 'Docs';
  span.style.font = '16px/1.5 sans-serif';
  document.body.append(span);
  return JSON.stringify({
    'sans-serif': resolve('sans-serif'),
    serif: resolve('serif'),
    monospace: resolve('monospace'),
    'default (no font-family)': resolve(getComputedStyle(document.body).fontFamily),
    'inline box height': span.getBoundingClientRect().height,
    'CJK 日本語のテキスト': width('sans-serif', '日本語のテキスト'),
    'CJK 简体中文文本': width('sans-serif', '简体中文文本'),
    'Hangul 한국어 텍스트': width('sans-serif', '한국어 텍스트'),
    'Arabic نص عربي': width('sans-serif', 'نص عربي'),
  });
})()`;

const argv = process.argv.slice(2);
const ei = argv.indexOf('--engines');
const names = availableEngines(ei >= 0 ? argv[ei + 1].split(',') : null);
console.log(`font parity: ${FONT_PARITY ? 'ON (fontconfig engines pointed at Blink\'s fonts)' : 'OFF (machine as configured)'}`);

const results = {};
for (const name of names) {
  const b = await launch(name);
  try {
    if (b.omnisnap.protocol === 'webdriver') {
      const s = await b.newSession();
      await s.navigate('about:blank');
      results[name] = JSON.parse(await s.exec(PROBE));
      await s.close();
    } else {
      const page = await b.newPage();
      await page.setContent('<!doctype html><html><body></body></html>');
      results[name] = JSON.parse(await page.evaluate(PROBE));
    }
  } catch (e) {
    results[name] = { error: String(e.message || e).split('\n')[0] };
  } finally {
    await b.close();
  }
}

const base = results.chrome || results[names[0]];
const show = (v) => (v == null ? '-' : typeof v === 'object' ? `${v.font} ${v.width}` : String(v));
const same = (a, b) => (a && b && typeof a === 'object' ? a.font === b.font && Math.abs(a.width - b.width) < 0.5 : a === b);
const keys = Object.keys(base).filter((k) => k !== 'error');
console.log(`\n${'probe'.padEnd(26)}${names.map((n) => n.padEnd(22)).join('')}`);
for (const k of keys) {
  console.log(`${k.padEnd(26)}${names.map((n) => {
    const v = results[n][k];
    return `${same(v, base[k]) ? ' ' : '≠'}${show(v)}`.padEnd(22);
  }).join('')}`);
}
for (const n of names) if (results[n].error) console.log(`${n}: ERROR ${results[n].error}`);
console.log('\n≠ = differs from chrome (the baseline)');
