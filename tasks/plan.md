# Implementation Plan: Omnisnap v1

## Overview

Build Omnisnap v1 as specified in [`specs/`](../specs/CAPABILITY-MAP.md): one Go binary that discovers the installed browsers, captures every engine, compares element geometry against the Blink baseline, triages the differences and writes a self-contained report; then a report-only GitHub Action across Linux, Windows and macOS, and thin npm/PyPI wrappers. Milestones: **M0** capture + viewer, **M1** layout diff, **M2** CI report (v1 = M2 released). Tasks: [`todo.md`](todo.md) (86 tasks, 12 phases, a checkpoint after each phase).

## Architecture decisions

- **Walking skeleton first.** Phase 2 delivers `omnisnap <url>` end-to-end with Chrome only (discovery → process → CDP → capture → viewer → cli) before any second engine. Every later task extends a working binary.
- **Highest risks first.** Safari on a macOS runner (Task 2) and the WebDriver Classic + scroll-and-stitch path (Phase 3) come before diff work: they decide whether WebKit is in v1 at all.
- **The spike is the oracle.** Injected scripts are embedded unchanged (Task 3); the planted suite and spike captures become fixtures (Tasks 4, 5); layout-diff, appearance and triage are ported against golden parity tests, not re-designed.
- **Contracts before consumers.** Schemas (Phase 1) precede every module that writes or reads artifacts; wrappers and the Action only consume `job.json`, NDJSON and `summary.json`.
- **Tier stays rule-based** (PRD 1.3.0 #4). The tree model is wired as a ranking score; switching the tier is a separate, gated decision (Task 76).
- **Ground-truth tooling is a separate, unshipped binary** (`cmd/omnisnap-lab`); `mutate.js` never ships to users.
- **Dependencies need approval** (CONVENTIONS "Ask first"): `github.com/coder/websocket` (Task 9), a JSON Schema validator for tests (Task 6), `github.com/dmitryikh/leaves` (Task 63), `go.uber.org/goleak` for tests (Task 22). Each task that adds one starts with that approval.

## Phases

| Phase | Tasks | Delivers | Milestone |
|---|---|---|---|
| 0 Foundation and risk | 1–5 | repo, CI, Safari smoke, fixtures from the spike | M0 |
| 1 Contracts | 6–8 | JSON Schemas + Go types | M0 |
| 2 Walking skeleton | 9–15 | `omnisnap <url>` with Chrome → bundle + viewer | M0 |
| 3 Engines | 16–24 | Firefox (BiDi), WebKitGTK and Safari (Classic), conformance | M0 |
| 4 Capture discipline | 25–33 | runs, lanes, failure policy, resume, probes, fonts, cleanup, all-OS discovery, golden geometry | M0 |
| 5 M0 completion | 34–38 | dom-audit, `install firefox`, viewer polish | **M0 release (v0.1)** |
| 6 Layout diff | 39–48 | matching, causes, tables, intent, parity | M1 |
| 7 Appearance | 49–51 | ink evidence, graphics SSIM | M1 |
| 8 Triage and M1 pipeline | 52–64 | KB, consequences, tiers, evidence, model score, diff in the CLI and viewer, planted gates | M1 |
| 9 Branded | 65–68 | warmed templates, blocked-request attribution, `--neutral` | M1 |
| 10 Ground truth | 69–76 | lab binary, mutation testing, crawl, dataset, training, model decision | **M1 release (v0.2)** |
| 11 CI and distribution | 77–86 | merge, Action, release tooling, npm, PyPI, names | **v1 (M2)** |

Parallel lanes once Phase 2 is done: Phase 3's three protocols are independent (one agent each); Phase 6 and Phase 5 can overlap; in Phase 11, merge/Action and the wrappers are independent after release tooling (Task 82).

## Risks and mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| Safari doesn't run unattended on hosted macOS runners | High: no real Safari in v1 | Task 2 in week one; fallback: WebKitGTK as the WebKit result, Safari documented as local-only |
| Classic scroll-and-stitch too slow or inexact | High: WebKit captures dominate time (spike: 2.5× Chrome) | Go-native stitching (Task 19), lanes (Task 25), conformance thresholds (Task 22) |
| Port drifts from the spike's validated behaviour | High: precision work lost | golden parity gates (Tasks 33, 47, 59) before any change; changes need approval |
| Tree model doesn't beat the rules | Medium | tier stays rule-based; score hidden if it adds nothing (Task 76) |
| Memory / disk on developer machines (WSL: 8 GB VM, virtual disk on C:) | Medium | no disk caches, cleanup layers, one browser per engine, disk-space guards (Tasks 29, 73) |
| Edge tracking prevention inactive under automation | Medium: Edge differences unexplained | Task 68 investigates headed on Windows before claiming support |
| Name availability (npm, PyPI) | Medium: package names | Task 85 early (it is not code: can happen any time). `omnisnap.org` registered 2026-11-17 |
| Fork PRs can't comment with the default token | Low | `workflow_run` pattern (Task 80) |

## Open questions

None block a task. The non-blocking ones are listed in each spec.

Resolved (2026-11-17):

- Schema host: `omnisnap.org` is registered; schema `$id`s use `https://omnisnap.org/schema/` from Task 6 on and are published by the `omnisnap-schemas` Worker (`.github/workflows/deploy.yml`).
- Go module path: `github.com/GerMP21/Omnisnap` (the repository).
- `dom-audit` ships in v1, Info-only (Tasks 34–35).
- Tree model's role: left to Task 76, decided on the round-6 data.
- macOS runner policy: each project chooses through an input; the default is `labelled` (Safari only on PRs labelled `omnisnap:safari`; `every` opts back in, `never` drops the job) — Task 79.
