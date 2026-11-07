// Head-to-head with the PRD's original approach (Feature 7): compare the
// screenshots themselves, on the SAME captures the layout diff used.
//  - SSIM / DSSIM over 8x8 luminance windows; PRD verdict: fail if DSSIM > 0.02
//  - pixelmatch: antialiasing-aware per-pixel diff (the established version of
//    the PRD's "ignore font-smoothing pixels" filter); magenta heatmap per page
// For planted pages it also measures LOCALIZATION: how much of the divergent
// area lies on the planted element vs. elsewhere (reflow cascades).
//
// usage: node pixeldiff.mjs   (after capture.mjs + diff.mjs)

import fs from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import pixelmatch from 'pixelmatch';
import { PNG } from 'pngjs';
import { dynamicMask, matchTrees, prep, selector } from './lib/align.mjs';
import { PAGES } from './planted/pages.mjs';

// Hybrid: pixels only where geometry is blind. Graphics elements matched by
// the layout diff are cropped at their own position in each engine and
// compared with SSIM. Text is excluded: engines rasterize glyphs differently,
// so text crops always differ a little.
const GRAPHICS = new Set(['canvas', 'img', 'svg', 'video', 'picture', 'iframe']);
const GRAPHIC_SSIM = 0.8;
const MIN_SIDE = 8; // after trimming SHIFT px per side; icons (16-24px) still qualify

// SSIM punishes 1-3px misalignment (rounding of subpixel positions) as much as
// a real difference, so compare the crops at the best offset within +-SHIFT.
const SHIFT = 3;
function cropSsim(img, other, ba, bb) {
  let best = null;
  for (let dy = -SHIFT; dy <= SHIFT; dy++) {
    for (let dx = -SHIFT; dx <= SHIFT; dx++) {
      const v = cropSsimAt(img, other, ba, [bb[0] + dx, bb[1] + dy, bb[2], bb[3]]);
      if (v != null && (best == null || v > best)) best = v;
    }
  }
  return best;
}

function cropSsimAt(img, other, ba, bb) {
  const w = Math.floor(Math.min(ba[2], bb[2])) - 2 * SHIFT;
  const h = Math.floor(Math.min(ba[3], bb[3])) - 2 * SHIFT;
  const ax = Math.round(ba[0]) + SHIFT; const ay = Math.round(ba[1]) + SHIFT;
  const bx = Math.round(bb[0]) + SHIFT; const by = Math.round(bb[1]) + SHIFT;
  if (w < MIN_SIDE || h < MIN_SIDE) return null;
  if (ax < 0 || ay < 0 || bx < 0 || by < 0 || ax + w > img.width || ay + h > img.height || bx + w > other.width || by + h > other.height) return null;
  const take = (src, x0, y0) => {
    const y = new Float32Array(w * h);
    for (let r = 0; r < h; r++) {
      for (let c = 0; c < w; c++) {
        const i = ((y0 + r) * src.width + x0 + c) * 4;
        y[r * w + c] = 0.299 * src.data[i] + 0.587 * src.data[i + 1] + 0.114 * src.data[i + 2];
      }
    }
    return y;
  };
  const { s } = ssimWindows(take(img, ax, ay), take(other, bx, by), w, h);
  let sum = 0;
  for (const v of s) sum += v;
  return s.length ? sum / s.length : null;
}

const HERE = path.dirname(fileURLToPath(import.meta.url));
const OUT = path.join(HERE, 'out');
const WIN = 8;
const WIN_DIVERGENT = 0.9; // a window with SSIM below this counts as divergent
const PRD_DSSIM = 0.02;
const C1 = (0.01 * 255) ** 2;
const C2 = (0.03 * 255) ** 2;

const crop = (img, w, h) => {
  const out = Buffer.alloc(w * h * 4);
  for (let y = 0; y < h; y++) img.data.copy(out, y * w * 4, y * img.width * 4, y * img.width * 4 + w * 4);
  return out;
};
const luma = (buf, n) => {
  const y = new Float32Array(n);
  for (let i = 0; i < n; i++) y[i] = 0.299 * buf[i * 4] + 0.587 * buf[i * 4 + 1] + 0.114 * buf[i * 4 + 2];
  return y;
};

function ssimWindows(ya, yb, w, h) {
  const cols = Math.floor(w / WIN);
  const rows = Math.floor(h / WIN);
  const s = new Float32Array(cols * rows);
  const n = WIN * WIN;
  for (let r = 0; r < rows; r++) {
    for (let c = 0; c < cols; c++) {
      let sa = 0; let sb = 0; let saa = 0; let sbb = 0; let sab = 0;
      for (let dy = 0; dy < WIN; dy++) {
        const row = (r * WIN + dy) * w + c * WIN;
        for (let dx = 0; dx < WIN; dx++) {
          const a = ya[row + dx];
          const b = yb[row + dx];
          sa += a; sb += b; saa += a * a; sbb += b * b; sab += a * b;
        }
      }
      const ma = sa / n;
      const mb = sb / n;
      const va = saa / n - ma * ma;
      const vb = sbb / n - mb * mb;
      const cov = sab / n - ma * mb;
      s[r * cols + c] = ((2 * ma * mb + C1) * (2 * cov + C2)) / ((ma * ma + mb * mb + C1) * (va + vb + C2));
    }
  }
  return { s, cols, rows };
}

const findings = JSON.parse(await fs.readFile(path.join(OUT, 'findings.json'), 'utf8'));
const rows = [];
const onlyIdx = process.argv.indexOf('--only');
const only = onlyIdx >= 0 ? process.argv[onlyIdx + 1] : null;
for (const p of findings.pages) {
  if (only && !p.slug.includes(only)) continue;
  if (p.error) continue;
  const dir = path.join(OUT, p.slug);
  const A = PNG.sync.read(await fs.readFile(path.join(dir, 'chrome-a.png')));
  const B = PNG.sync.read(await fs.readFile(path.join(dir, `${p.pair}-a.png`)));
  const w = Math.min(A.width, B.width);
  const h = Math.min(A.height, B.height);
  const ca = crop(A, w, h);
  const cb = crop(B, w, h);

  const { s, cols, rows: wr } = ssimWindows(luma(ca, w * h), luma(cb, w * h), w, h);
  let sum = 0;
  let divergent = 0;
  let firstY = null;
  for (let i = 0; i < s.length; i++) {
    sum += s[i];
    if (s[i] < WIN_DIVERGENT) { divergent++; if (firstY === null) firstY = Math.floor(i / cols) * WIN; }
  }
  const ssim = sum / s.length;
  const dssim = (1 - ssim) / 2;

  const diff = new PNG({ width: w, height: h });
  const diffPx = pixelmatch(ca, cb, diff.data, w, h, { threshold: 0.1, includeAA: false, diffColor: [255, 0, 255], alpha: 0.15 });
  await fs.writeFile(path.join(dir, `pixel-diff-${p.pair}.png`), PNG.sync.write(diff));

  // Localization on planted pages: share of divergent windows on the planted element.
  let localized = null;
  let target = null;
  const def = PAGES.find((x) => x.slug === p.slug);
  if (def && def.kind === 'planted' && p.pair === 'firefox') {
    const ids = def.expect.flatMap((g) => g.anyOf);
    const f = p.findings.find((x) => ids.includes(x.elId));
    const boxes = f ? [f.a?.b, f.b?.b].filter(Boolean) : [];
    if (boxes.length) {
      target = boxes[0];
      let inside = 0;
      for (let i = 0; i < s.length; i++) {
        if (s[i] >= WIN_DIVERGENT) continue;
        const x = (i % cols) * WIN + WIN / 2;
        const y = Math.floor(i / cols) * WIN + WIN / 2;
        if (boxes.some(([bx, by, bw, bh]) => x >= bx - WIN && x <= bx + bw + WIN && y >= by - WIN && y <= by + bh + WIN)) inside++;
      }
      localized = divergent ? inside / divergent : null;
    }
  }

  // Hybrid: SSIM on matched graphics elements only.
  const ta = prep(JSON.parse(await fs.readFile(path.join(dir, 'chrome-a.json'), 'utf8')), 'chrome');
  const tb = prep(JSON.parse(await fs.readFile(path.join(dir, `${p.pair}-a.json`), 'utf8')), p.pair);
  const M = matchTrees(ta, tb);
  // Graphics that change between two runs of the SAME engine (slideshows,
  // rotating logos) are dynamic, not engine differences: skip them.
  const second = async (e, eng) => { try { return prep(JSON.parse(await fs.readFile(path.join(dir, `${e}-b.json`), 'utf8')), eng); } catch (_) { return null; } };
  const ta2 = await second('chrome', 'chrome');
  const tb2 = await second(p.pair, p.pair);
  const maskA = ta2 ? dynamicMask(ta, ta2) : new Set();
  const maskB = tb2 ? dynamicMask(tb, tb2) : new Set();
  let dynamicSkipped = 0;
  const graphics = [];
  let checked = 0;
  for (const a of ta.all) {
    if (!GRAPHICS.has(a.t) || a.hid) continue;
    const b = M.pairOf.get(a.uid);
    if (!b || b.hid) continue;
    if (maskA.has(a.uid) || maskB.has(b.uid)) { dynamicSkipped++; continue; }
    const g = cropSsim(A, B, a.b, b.b);
    if (g == null) continue;
    checked++;
    if (g < GRAPHIC_SSIM) graphics.push({ sel: selector(a), tag: a.t, ssim: Number(g.toFixed(3)), box: a.b, boxB: b.b, src: a.u || null });
  }

  rows.push({
    graphicsChecked: checked, graphicsDiffer: graphics, dynamicSkipped,
    slug: p.slug, pair: p.pair, group: p.group, ssim, dssim, prdFail: dssim > PRD_DSSIM,
    divergentArea: divergent / s.length, diffPixels: diffPx / (w * h), heightDelta: B.height - A.height,
    firstDivergentY: firstY, localized, target, layoutFindings: p.findings.length,
  });
}

await fs.writeFile(path.join(OUT, 'pixel.json'), JSON.stringify(rows, null, 1));
const pct = (x, d = 1) => (x == null ? '   -' : `${(x * 100).toFixed(d)}%`);
console.log(`${'page'.padEnd(20)}${'vs'.padEnd(9)}${'group'.padEnd(8)}${'SSIM'.padStart(7)}${'DSSIM'.padStart(8)}  PRD   ${'diverg.area'.padStart(11)}${'AA-aware px'.padStart(12)}${'height Δ'.padStart(9)}${'1st diff y'.padStart(11)}${'on target'.padStart(10)}${'layout'.padStart(8)}`);
for (const r of rows) {
  console.log(`${r.slug.padEnd(20)}${r.pair.padEnd(9)}${r.group.padEnd(8)}${r.ssim.toFixed(4).padStart(7)}${r.dssim.toFixed(4).padStart(8)}  ${r.prdFail ? 'FAIL' : 'pass'}  ${pct(r.divergentArea, 2).padStart(11)}${pct(r.diffPixels, 3).padStart(12)}${String(r.heightDelta).padStart(9)}${String(r.firstDivergentY ?? '-').padStart(11)}${pct(r.localized, 0).padStart(10)}${String(r.layoutFindings).padStart(8)}`);
}
console.log('\nwrote out/pixel.json and out/<page>/pixel-diff-<engine>.png (magenta = differing pixels, antialiasing ignored)');

console.log(`\nHybrid: SSIM on matched graphics elements (canvas/img/svg/video/picture/iframe), flagged below ${GRAPHIC_SSIM}`);
for (const r of rows) {
  const top = r.graphicsDiffer.slice(0, 4).map((g) => `${g.tag} ${g.ssim} ${g.sel.slice(-40)}`).join(' | ');
  console.log(`  ${r.slug.padEnd(20)} ${r.pair.padEnd(9)} checked ${String(r.graphicsChecked).padStart(3)}  dynamic ${String(r.dynamicSkipped).padStart(2)}  differ ${String(r.graphicsDiffer.length).padStart(3)}  ${top}`);
}
