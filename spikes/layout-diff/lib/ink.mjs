// Concept 6: DOM-guided image registration + ink comparison.
// Each element is cropped at ITS OWN position in each engine's screenshot
// (piecewise registration through the matched DOM boxes), so reflow cascades
// line up. We then compare INK MASKS (where something is drawn: pixels that
// stand out from the crop's background, dilated by 1px) instead of pixel
// intensities, so glyph-rasterization noise mostly disappears.
//
// agreement = (A ink near B ink + B ink near A ink) / (A ink + B ink), 0..1
// The noise floor is measured per page on elements whose geometry is
// identical in both engines: an "observable" change must fall below it.

const LUMA = (d, i) => 0.299 * d[i] + 0.587 * d[i + 1] + 0.114 * d[i + 2];
const MAX_PIXELS = 400000; // larger crops are sampled every 2nd pixel

function crop(png, x0, y0, w, h, step) {
  const W = Math.ceil(w / step);
  const H = Math.ceil(h / step);
  const lum = new Float32Array(W * H);
  const inside = new Uint8Array(W * H);
  for (let r = 0; r < H; r++) {
    const y = Math.round(y0) + r * step;
    for (let c = 0; c < W; c++) {
      const x = Math.round(x0) + c * step;
      if (x < 0 || y < 0 || x >= png.width || y >= png.height) continue;
      lum[r * W + c] = LUMA(png.data, (y * png.width + x) * 4);
      inside[r * W + c] = 1;
    }
  }
  return { lum, inside, W, H };
}

function mask({ lum, inside, W, H }) {
  // Background: the most frequent luminance in the crop (histogram mode,
  // 4-level bins). A box's border or a single edge line cannot win, unlike
  // a median of the crop's border pixels.
  const hist = new Uint32Array(64);
  for (let i = 0; i < lum.length; i++) if (inside[i]) hist[Math.min(63, lum[i] >> 2)]++;
  let mode = 63;
  for (let k = 0; k < 64; k++) if (hist[k] > hist[mode]) mode = k;
  const bg = mode * 4 + 2;
  const m = new Uint8Array(W * H);
  let n = 0;
  for (let i = 0; i < m.length; i++) if (inside[i] && Math.abs(lum[i] - bg) > 32) { m[i] = 1; n++; }
  return { m, n };
}

function dilate(m, W, H) {
  const out = new Uint8Array(m.length);
  for (let r = 0; r < H; r++) {
    for (let c = 0; c < W; c++) {
      if (!m[r * W + c]) continue;
      for (let dr = -1; dr <= 1; dr++) {
        for (let dc = -1; dc <= 1; dc++) {
          const rr = r + dr; const cc = c + dc;
          if (rr >= 0 && cc >= 0 && rr < H && cc < W) out[rr * W + cc] = 1;
        }
      }
    }
  }
  return out;
}

// boxA/boxB: [x, y, w, h] in each engine's document coordinates.
export function inkAgreement(pngA, pngB, boxA, boxB) {
  if (!pngA || !pngB || !boxA || !boxB) return null;
  const w = Math.max(boxA[2], boxB[2]);
  const h = Math.max(boxA[3], boxB[3]);
  if (w < 2 || h < 2) return null;
  if (boxA[1] + h > pngA.height || boxB[1] + h > pngB.height) return null; // beyond the screenshot
  const step = w * h > MAX_PIXELS ? 2 : 1;
  const ca = crop(pngA, boxA[0], boxA[1], w, h, step);
  const cb = crop(pngB, boxB[0], boxB[1], w, h, step);
  const { m: ma, n: na } = mask(ca);
  const { m: mb, n: nb } = mask(cb);
  if (na + nb < 8) return { agreement: 1, ink: 0 }; // nothing drawn in either
  const da = dilate(ma, ca.W, ca.H);
  const db = dilate(mb, cb.W, cb.H);
  let hit = 0;
  for (let i = 0; i < ma.length; i++) { if (ma[i] && db[i]) hit++; if (mb[i] && da[i]) hit++; }
  return { agreement: hit / (na + nb), ink: na + nb };
}

// Noise floor for this page and engine pair: agreement of elements whose
// geometry did not change at all. Returns a low percentile (default 0.85).
export function noiseFloor(samples) {
  const xs = samples.filter((s) => s && s.ink >= 20).map((s) => s.agreement).sort((p, q) => p - q);
  if (xs.length < 10) return { floor: 0.85, n: xs.length, measured: false };
  const p5 = xs[Math.floor(xs.length * 0.05)];
  return { floor: Math.min(0.97, Math.max(0.5, p5)), n: xs.length, measured: true };
}
