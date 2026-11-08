# Spec: capture

Module id: `capture` · Package: `internal/capture` · Depends on: `drivers`, `bundle-schema` (and `process`, `discovery` through them) · Milestone: M0 · Conventions: [CONVENTIONS.md](CONVENTIONS.md)

## Objective

For every page of a job and every engine, produce a **comparable** capture: the same viewport, the same fonts, the page settled to the same state, geometry and screenshot taken at the same moment, plus the causal probes, written to the bundle. Most of the spike's precision work was capture discipline; this module encodes it.

## Behaviour

### Per page, per engine, per run

1. **Normalize** (launch-time parts live in `process`): viewport 1280×800, DPR 1 (WebKit: measure-and-correct loop), root scrollbar and gutter hidden (`scrollbar-width: none` on the root), font parity verified once per run (`check-fonts`).
2. **Navigate**: `load` within 30 s; then network idle (500 ms quiet, max 8 s) where the driver supports it; Classic: fixed 1.5 s settle.
3. **Stabilize** (`stabilize.js`, unchanged): `readyState`, `document.fonts.ready`, stepped scroll to trigger lazy content (jumping to the bottom misses middle content), wait for images, finish or freeze animations, two animation frames, then **freeze JS timers** (the saved `requestAnimationFrame` is kept for later settling).
4. **Extract** (`extract.js`): geometry tree (format in `SPEC-bundle-schema.md`).
5. **Screenshot**: full page, max 2560×16000 px.
6. **Re-measure** after the screenshot (`meta.post`): if height or node count changed, the result carries a warning (the page changed between geometry and screenshot).
7. **Probes** (`probes.js`): re-extract under `lh` (whole-pixel line-height), `font` (one font), `controls` (neutral form controls), `all`; gzip to `<engine>-<run>.probes.json.gz`. Skippable (`probes: false`).
8. **Write** atomically (temp file + rename).

### Runs, lanes and timing

- **Two runs** per engine (`runs: 2`): run b reveals dynamic content (`layout-diff` masks it and pairs the best-matching runs).
- **Interleaving**: run a of every engine, then run b, so a live page can't drift between engines.
- **Lanes**: CDP/BiDi engines in one lane (sequential), WebKit/Classic in a parallel lane. Wall time is the slower lane, not the sum: the spike measured WebKit at 51 s median per capture vs 20–24 s for Chrome/Firefox on top-site homepages.
- **Failure policy** (live pages only; planted pages are always fully captured):
  - baseline run a fails with a timeout or network error → the other engines skip run a, run b is skipped, the page is marked `unreachable` (`failed.json`);
  - baseline run a has < 50 elements (bot wall, e.g. a challenge page) → `unreachable`;
  - an engine that failed run a skips its run b.
- **Resume**: a page counts as done when every engine was attempted to the end (run b captured or failed); `--resume` skips done pages and pages marked unreachable (`--retry-failed` retries them).

### Fonts

`omnisnap check-fonts` (and a pre-flight inside every job): each engine resolves the generic families to the same fonts; a mismatch is a job warning naming the fonts. Missing-glyph detection: a page using a script with no installed font gets a warning (spike lesson 2: CJK boxes differ per engine).

## Commands

```
go test ./internal/capture/...                         # unit: policy, resume, lane scheduling, atomic writes (fake drivers)
go test -tags browsers ./internal/capture/...          # real engines on the planted suite
go run ./cmd/omnisnap capture --job testdata/jobs/planted.json   # manual
```

## Project structure

`internal/capture/{capture.go, page.go, lanes.go, policy.go, resume.go, fonts.go, inject.go}`; `assets/inject/{stabilize,extract,probes}.js`; `testdata/jobs/`.

## Code style

Per conventions. The pipeline steps are plain functions over a `drivers.Session`; the lane scheduler is separate and tested with fake sessions that record call order and timing.

## Testing strategy

- **Unit (fake sessions):** interleaving order; lanes overlap; failure policy (each branch); resume decisions; atomic writes (no partial files after a crash simulated mid-write).
- **Integration (`browsers`):** the planted suite (31 pages) in Chrome, Firefox, WebKitGTK.
- **Determinism:** run a vs run b of every static planted page produce identical geometry (0 boxes differ by > 0.1 px).
- **Golden:** geometry of the planted suite equals the spike's captures within 0.1 px per box (same injected scripts, same normalization).

## Boundaries

- **Always:** apply the same normalization to every engine; record what could not be normalized as a warning.
- **Ask first:** changing an injected script, a wait, a timeout, the viewport, or the failure policy (all change what is measured).
- **Never:** compare captures taken with different normalization; leave a half-written capture file.

## Success criteria

1. Planted suite, Chrome + Firefox + WebKitGTK, 2 runs: 0 failed captures; run a and run b identical on static pages.
2. Every engine reports exactly 1280×800 CSS px viewport (`meta.vw`, `meta.vh`).
3. Geometry matches the spike captures within 0.1 px per box (golden).
4. Wall time on the planted suite ≤ 1.2 × the WebKit lane alone (lanes work).
5. On a 100-site live crawl: no capture exceeds 120 s; unreachable pages cost ≤ 35 s each; `--resume` after a kill recaptures nothing already done.

## Open questions

- Clock control (fake timers before page scripts): possible on CDP/BiDi, impossible on Classic (PRD 1.2.0 #7). Deferred; how much do dynamic pages hurt in report mode first?
- A/B variants (Stripe served a random variant per session): seed cookies/storage per job? Deferred until report-mode data shows how often states diverge.
- Viewports beyond 1280×800 (mobile widths) in v1, or one viewport only? Proposed: `viewport` is configurable, the default stays 1280×800.
