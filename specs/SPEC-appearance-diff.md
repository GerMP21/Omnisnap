# Spec: appearance-diff

Module id: `appearance-diff` · Package: `internal/appearance` · Depends on: `layout-diff` · Milestone: M1 · Conventions: [CONVENTIONS.md](CONVENTIONS.md)

## Objective

Catch what geometry can't see (an image, SVG, canvas or video drawn differently in the same box) and measure whether a geometric difference is **visible**, without the rasterization noise that made whole-page pixel diffs useless (round 1: bug-free controls failed DSSIM 0.02 because Skia and Firefox rasterize the same glyphs differently).

Two outputs:

1. **Appearance-only causes** for graphics elements whose geometry matched but whose pixels differ beyond the noise.
2. **Ink evidence** for every geometric cause: is the change visible on screen? (feeds `triage`).

## Behaviour

### Registration

Each element is cropped at **its own box in each engine's screenshot** (piecewise registration through the matched DOM boxes), so reflow cascades line up. Crops beyond the screenshot (pages taller than 16,000 px) are skipped.

### Graphics SSIM (appearance-only causes)

- Elements: `img`, `svg`, `canvas`, `video`, `picture`, `iframe`, and elements with a background image, matched, with equal geometry (no layout cause), not dynamic (masked).
- SSIM on the luma of both crops with a ±3 px offset search; report an appearance-only cause when the best SSIM < 0.90 (initial value; the spike's crops were clean on stable pages, threshold to be confirmed on the planted appearance pages).
- Page-level DSSIM is computed only as a **state alarm** ("engines rendered different page states"), never as a finding.

### Ink agreement (evidence)

- Ink mask = pixels that differ from the crop's background luma (histogram mode, 4-level bins) by > 32, dilated by 1 px. A crop border or edge line can't become the background (the spike's first version, which used the border median, counted a bordered card's whole interior as ink).
- Agreement = (A ink near B ink + B ink near A ink) / (A ink + B ink).
- **Per-page noise floor**: 5th percentile of agreement over up to 120 unchanged visible text/media elements (ink ≥ 20 px); clamped to [0.5, 0.97]; default 0.85 with < 10 samples.
- A cause's ink evidence: `{agreement, ink, floor, observable: agreement < floor}`; `null` when nothing is drawn (ink < 20) or beyond the screenshot.

## Commands

```
go test ./internal/appearance/...                         # unit on fixture PNGs
go test -tags e2e ./test/e2e/... -run Appearance          # planted appearance pages
```

## Project structure

`internal/appearance/{crop.go, ssim.go, ink.go, floor.go}`; `testdata/appearance/*.png`; new planted pages `testdata/planted/appearance-*.html`.

## Code style

Per conventions. Image work on `image.Gray` buffers; no per-pixel allocation; crops larger than 400k px sampled every 2nd pixel (spike behaviour).

## Testing strategy

- **Unit:** ink agreement of identical, shifted-by-1 px, recoloured and missing crops; background detection on bordered boxes; noise floor on synthetic samples.
- **Parity:** ink agreement within 0.01 of the spike (`lib/ink.mjs`) on the round-5 planted captures.
- **Planted appearance pages (new):** an image swapped, an SVG fill changed, a canvas drawn differently, a CSS filter applied, each in Firefox only (`.ff` rule); plus 2 controls with images, SVG and canvas.

## Boundaries

- **Always:** compare registered crops only; keep page-level pixel metrics out of findings.
- **Ask first:** changing the SSIM threshold, ink threshold, dilation or noise-floor percentile.
- **Never:** report a whole-page pixel diff across engines.

## Success criteria

1. Each planted appearance bug is reported as an appearance-only cause at the right element.
2. 0 appearance-only causes on all control pages in every engine.
3. Ink parity with the spike within 0.01.
4. ≤ 300 ms per page pair for ink + SSIM on a typical page (≤ 200 graphics elements).

## Open questions

- Video and canvas content that animates: always masked as dynamic, or compared after the timer freeze?
- Background images: include in v1 or limit to replaced elements first?
