# Golden fixtures

The spike's captures and findings: what the Go port must reproduce (golden parity, `specs/CONVENTIONS.md` "Testing strategy"; Tasks 33, 47, 59). Exported by `spikes/layout-diff/tools/export-golden.mjs`; checked by `go test ./internal/groundtruth/ -run Golden`.

## Provenance

| | |
|---|---|
| Source | `spikes/layout-diff/out/`, round 5 (`capture.mjs`, `diff.mjs`); findings generated 2026-11-08T04:15Z |
| Baseline | Chrome 155.0.8059.39 (fresh profile) |
| Compared engines | Firefox 153.6.0 ESR (fresh profile), WebKitGTK MiniBrowser 2.50.4 (fresh profile, headless under weston) |
| Machine | Linux (AlmaLinux 10 / WSL2), viewport 1280×800, DPR 1 |
| Injected scripts | Those pinned in `assets/inject/SHA256SUMS` (unchanged since) |

`meta.json` in each group records the engine versions, the diff tolerances, the 82 feature names and the page list.

### Tolerances

The gate runs on Chrome and Firefox; **WebKit is opt-in** (`OMNISNAP_GOLDEN_WEBKIT=1`) because its goldens come from MiniBrowser 2.50.4 on AlmaLinux 10, and a current build with a different font stack reflows the forms and KB pages by 6–27 px. WebKit is covered everywhere else by conformance, the viewport test and the a/b self-check.

Block boxes, every horizontal coordinate and the tree structure must match within **0.1 px**. **Inline** elements' vertical metrics (y and height) are compared within `layoutdiff.TolAbs` (**2 px**): an inline line box carries the font's ascent and descent, and those differ by fractions of a pixel between the golden machine's WebKitGTK 2.50.4 (AlmaLinux 10) and a current build on another distribution. The pipeline itself ignores box deltas under 2 px, so the gate is never stricter than the tool it guards. Everything the tool would report still has to match within 0.1 px.

## Layout

```
planted/<slug>/          the 31 pages of testdata/planted/        committed
real/<slug>/             10 real sites (corpus.json "real")       NOT committed
```

Per page:

| File | What |
|---|---|
| `page.json` | slug, URL, group, font parity |
| `<engine>-{a,b}.json` | geometry from `extract.js`, two runs (dynamic content is what differs between them) |
| `<engine>-{a,b}.probes.json.gz` | re-measurements under each causal probe (`lh`, `font`, `controls`, `all`) |
| `<engine>-a.png` | full-page screenshot, run a only: enough for the appearance checks (Tasks 49–51) |
| `findings-<engine>.json` | the spike's causes for Chrome vs that engine: kinds, selector, side, deltas, tier, evidence, plus `features` (the tree model's input, `lib/features.mjs`) |

Pairs: firefox and webkit. The chromium/brave/edge pairs of round 3 used older captures and are not exported.

## Size

| Group | Captures | Probes | Screenshots | Findings | Total |
|---|---|---|---|---|---|
| planted (31 pages) | 1.3 MB | 0.5 MB | 8.3 MB | 0.2 MB | **10.2 MB** |
| real (10 sites, local only) | 16.3 MB | 7.6 MB | 49.7 MB | 0.4 MB | **74.0 MB** |

## Why `real/` is not in git

The real-site captures hold third-party content: up to 60 characters of each element's own text, and full-page screenshots of the sites. `CONVENTIONS.md` never commits captured third-party content outside fixtures we own (decision of 2026-11-17). `real/` is git-ignored; `TestGoldenRealSites` and every real-site parity test skip when it is absent, so CI runs planted parity only.

To get it locally, from a checkout that has the spike's `out/` directory:

```
node spikes/layout-diff/tools/export-golden.mjs --from <path>/spikes/layout-diff/out --groups real
```

## Regenerating

These are fixtures, not outputs: regenerate only when the spike is re-run on purpose, and review the diff. The planted set:

```
node spikes/layout-diff/tools/export-golden.mjs --from <path>/spikes/layout-diff/out --groups planted
```
