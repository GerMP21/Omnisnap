# Capability Map: Omnisnap v1

**Status: approved 2026-11-08.** Shared conventions: [CONVENTIONS.md](CONVENTIONS.md). Module ids are stable: specs, plans and tasks refer to them.

v1 = the local **Inspector** plus a **report-only GitHub Action**, shipped as one Go binary with thin npm/PyPI wrappers (PRD changelogs 1.1.0–1.4.0, [one-pager](docs/ideas/omnisnap.md)). Each module has its own spec, `SPEC-<id>.md` in this folder (linked in the table).

## Modules

| Module id | Responsibility | Depends on | Milestone | Spike source |
|---|---|---|---|---|
| [`bundle-schema`](SPEC-bundle-schema.md) | The contracts: `job.json` (input), artifact bundle layout, capture format, `summary.json`, NDJSON progress events, schema versioning | — | M0 | `capture.mjs` outputs, `findings.json` |
| [`discovery`](SPEC-discovery.md) | Find installed browsers on Windows (registry), Linux (.desktop, PATH, Flatpak, Snap), macOS (mdfind); family, channel, version; < 300 ms | `bundle-schema` | M0 | `lib/engines.mjs` |
| [`process`](SPEC-process.md) | Launch and clean up browsers: Job Objects / process groups, temp profiles, warmed-template copies, headless display for WebKitGTK, no orphans or leftover caches | `discovery` | M0 | `lib/engines.mjs`, `lib/webdriver.mjs` |
| [`drivers`](SPEC-drivers.md) | Protocol clients: CDP (Blink), WebDriver BiDi (Gecko), WebDriver Classic (WebKit/Safari): navigate, evaluate, preload scripts, network events, failed requests, screenshots (native or scroll-and-stitch) | `process` | M0 | puppeteer-core, `lib/webdriver.mjs` |
| [`capture`](SPEC-capture.md) | The capture pipeline: environment normalization (font parity, scrollbars, viewport fit), stabilization, geometry extraction, causal probes, screenshots, 2 runs per engine, parallel engine lanes, consistency re-measure, failure handling | `drivers`, `bundle-schema` | M0 | `capture.mjs`, `inject/stabilize.js`, `extract.js`, `probes.js` |
| [`dom-audit`](SPEC-dom-audit.md) | Single-engine layout anomalies on one capture (overflow, overlap, clipped text, offscreen content) | `capture` | M0 | PRD Feature 6 (not in the spike) |
| [`layout-diff`](SPEC-layout-diff.md) | Cross-engine comparison: element matching, dynamic masks, root causes vs effects, table columns, text shifts, truncation, design intent (relations kept by ≥ 2 engine families) | `bundle-schema` | M1 | `lib/align.mjs`, `lib/intent.mjs`, `diff.mjs` |
| [`appearance-diff`](SPEC-appearance-diff.md) | What geometry can't see: SSIM / ink comparison on matched crops of graphics elements, per-page noise floor | `layout-diff` | M1 | `pixeldiff.mjs`, `lib/ink.mjs` |
| [`triage`](SPEC-triage.md) | From causes to what a developer wants: knowledge base, consequences, overflow modes, Report/Info tiers, evidence (mechanism, perception, calibration, consensus, novelty), grouping and ranking, tree-model score | `layout-diff`, `appearance-diff` | M1 | `lib/triage.mjs`, evidence modules, `lib/gbdt.mjs` |
| [`branded`](SPEC-branded.md) | Branded browsers as shipped: warmed template profiles, filter-list versions, blocked-request attribution, `--neutral` | `capture`, `triage` | M1 | `warm.mjs`, blocked-host warnings |
| [`ground-truth`](SPEC-ground-truth.md) | Known-answer suites and the model: planted pages, mutation testing, crawl list, dataset, LightGBM training, parity test, regression gates in CI | `capture`, `triage` | M1 | `planted/`, `inject/mutate.js`, `sites.mjs`, `dataset.mjs`, `train/` |
| [`viewer`](SPEC-viewer.md) | Self-contained HTML report: synced side-by-side screenshots, outlined findings, Report/Info tiers, evidence, dynamic regions | `bundle-schema` | M0 (capture) / M1 (findings) | `lib/report.mjs` |
| [`cli`](SPEC-cli.md) | The `omnisnap` command: `omnisnap <url>`, `run job.json`, flags, exit codes, progress output, orchestration of all runtime modules | all runtime modules | M0–M2 | `capture.mjs` + `diff.mjs` flow |
| [`install`](SPEC-install.md) | Opt-in `omnisnap install firefox`: official Mozilla build into a cache, verified, never automatic | `discovery` | M0 | — |
| [`merge`](SPEC-merge.md) | `omnisnap merge`: combine bundles from several machines (per-result host metadata) into one report | `bundle-schema`, `viewer` | M2 | — |
| [`ci-action`](SPEC-ci-action.md) | GitHub Action: Ubuntu, Windows and macOS runners (real Safari), one merged PR comment, report-only, never fails the build | `cli`, `merge` | M2 | — |
| [`wrappers`](SPEC-wrappers.md) | npm and PyPI packages: platform binaries, thin SDK (write `job.json`, run the binary, read NDJSON + `summary.json`) | `cli`, `bundle-schema` | M2 | — |
| [`website`](SPEC-website.md) | `omnisnap.org`: landing page, user docs, a live demo report captured each release, the schema files; deployed with each release | `bundle-schema`, `viewer`, `ci-action` | M2 | — |

## Build order

```
bundle-schema
  → discovery → process → drivers → capture → dom-audit
  → layout-diff → appearance-diff → triage → branded
  → ground-truth            (parallel with viewer once triage exists)
  → viewer → cli → install
  → merge → ci-action, wrappers → website
```

## Not in v1

Pass/fail gate (waits for report-mode noise data) · MCP / AI layer · multi-machine capture network · mobile browsers · GitLab, Bitbucket, self-hosted runners · Arc, Tor Browser · whole-page pixel diff · automatic browser downloads · manual labels as ground truth.
