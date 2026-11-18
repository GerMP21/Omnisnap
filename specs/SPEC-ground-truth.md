# Spec: ground-truth

Module id: `ground-truth` · Packages: `cmd/omnisnap-lab`, `internal/groundtruth`, `tools/` (Python), `testdata/planted/` · Depends on: `capture`, `triage` · Milestone: M1 · Conventions: [CONVENTIONS.md](CONVENTIONS.md)

## Objective

Give every precision and recall claim a **known answer, produced without humans**, and keep it as a regression gate:

1. **Planted suite**: pages with known bugs, controls and expected-difference fixtures (the regression suite and the threshold calibration set).
2. **Mutation testing on real pages**: realistic cross-engine bugs injected into real sites in one engine, labeled automatically.
3. **Training and evaluation of the tree model** that `triage` ships.

Manual labels are not ground truth (withdrawn as unreliable, PRD 1.4.0). Ships nothing to users except the trained model file and the regression gates in CI.

## Behaviour

### 1. Planted suite (`testdata/planted/`, from `spikes/layout-diff/planted/pages.mjs`)

31 pages served from a local server. A blocking script adds `ff` to `<html>` in Firefox; each planted page's `.ff` rule plants exactly one divergence, so every other engine sees a control. Kinds:

- **planted** (24): card width, heading wrap, overflow, missing element, padding, order, font, clip, gap, image, card CTA alignment, grid column, toolbar alignment, list rhythm, equal-height tiles, badge overlap, ellipsis, line-clamp, aspect-ratio, nav wrap, code scroll, hidden notice, table layout, flex min-width. `intent: true` on six.
- **control** (4), **native** (1, genuine form-control differences, findings only on the controls), **kb** (2: K1, K2 fixtures).
- Each page carries its expected answer (`expect` ids, `maxFindings`).

### 2. Mutation testing (`omnisnap-lab mutate`)

Engines: any installed family; on Linux the WPE port (`OMNISNAP_WEBKIT=wpe`, headless, exact viewport) is the cheapest WebKit lane, WebKitGTK under a private Weston display the default.

- **Operators** (`mutate.js`, 19): width shrink, padding, padding-left, margin-top, nudge, hide, invisible, gap, wrap-narrow, font-up, nowrap, align-self, overlap, clip, order, letter-spacing, line-height, border, ellipsis-clip. A seeded random candidate; when its class is shared by ≤ 30 elements, the whole class gets the bug (as a stylesheet bug would).
- Injected **after** the page settled, in Firefox or WebKit only; several mutants share one page load (inject, measure, revert). Same probes and screenshot as clean captures, so mutated causes carry the same evidence.
- **Every scheduled mutant is recorded**: the operator is fixed by the seed (`ops[seed % 19]`), so the same page and seed always inject the same bug. A mutant that moves nothing is kept with `labels.json: {"effective": false}` and becomes label-0 rows in the dataset (counted as `mutantsIneffective`). `--require-effective` rotates to the next operator until the label sees a change and is documented as training-only, because it biases the set toward bugs the labeler can see (`SPEC-ground-truth-mutant-policy.md`).
- Only pages where the engines rendered the same state (node counts within 20%, baseline ≥ 50 elements) are mutated.
- **Labels:** a same-engine diff (clean vs mutated) shows what the bug changed. A cross-engine cause is **positive** when it sits on a changed element in the target's neighbourhood (target, 3 levels up and down, siblings and their children) and was not already a cause in the clean diff. The join is by node identity: `findings.json` causes carry `side` (`A` baseline, `B` compared engine) and `nodeId` (the uid in that side's capture), and a side-A cause crosses through the baseline-vs-clean match. Selector matching remains only as the fallback for bundles written before those fields existed. Each mutant's `labels.json` records the positives twice: selectors for people and `positiveUids` in the clean capture, the exact form the dataset joins on.

### 3. Crawl list (`tools/sites.py` or `omnisnap-lab sites`)

From the Tranco top list: exclude infrastructure, adult content (multilingual terms in domain, title and page text) and gambling; one site per brand; homepage 200 HTML ≥ 5 KB with a title; no bot challenge or sign-in redirect. Versioned output with the rejection reasons.

### 3b. Diffing lab bundles (`omnisnap-lab diff`)

The crawler writes captures, not findings: `omnisnap-lab diff` runs the same
pipeline as the CLI over every page in a bundle (baseline = a Blink engine,
or `--baseline`), reads the screenshots and probes from disk, and writes
`findings.json` with `side`/`nodeId` on every cause. `mutate` does the same for
each mutant against the clean baseline, so a mutant's findings and its
`labels.json` (`causes`, `positiveUids`) describe the same elements.

### 4. Dataset, training, evaluation

- **Dataset** (`omnisnap-lab dataset`): one row per cause with the 82 features of `triage`. Label 1 = mutation positives; label 0 = causes of clean pages (natural engine differences: unlabeled, mostly expected, so positive-unlabeled). Planted pages are never trained on. Like the spike: a page whose engines rendered different states is skipped, a mutant contributes only the positive causes from its `labels.json`, and the metadata carries `cleanPairs`, `pos`, `neg`, `mutants`, `effective`, `skippedDiverged`.
- **Training** (`tools/train.py`, Python + LightGBM): binary objective, 300 rounds, 15 leaves, `scale_pos_weight` = negatives / positives; GroupKFold by **site** (no site in both train and test).
- **Metrics** (precision on this table depends on how many bugs were injected, so it is not reported): recall on injected bugs (Wilson 95%), false alarms on natural causes (share reported, Report causes per clean page), compared with the rules tier **at equal recall**; planted recall and control reports for the model tier.
- **Exports:** `gbdt.txt` (native, loaded by `leaves`), `gbdt.json` (dump), `gbdt-meta.json` (features, τ, metrics), `fingerprints.json`; `parity.csv` for the serving parity test.

### 5. Operating limits (lessons from round 6)

No browser disk caches; one capture process by default (`--shards` to parallelize when memory allows: two processes of three engines exhausted an 8 GB WSL VM); resumable (`--resume`), unreachable pages marked and skipped; data directory configurable; on WSL, the Linux disk lives in a virtual disk on C: that never shrinks: warn when the host drive has < 20 GB free and stop below 5 GB.

## Commands

```
go test -tags e2e ./test/e2e/... -run Planted            # planted gates (CI, every PR touching capture/layout-diff/triage)
go run ./cmd/omnisnap-lab sites --n 500 --out crawl-sites.json
go run ./cmd/omnisnap-lab crawl --list crawl-sites.json --out ~/omnisnap-data/bundles --limit 120
go run ./cmd/omnisnap-lab diff --bundles ~/omnisnap-data/bundles          # findings.json for clean pages
go run ./cmd/omnisnap-lab mutate --bundles ~/omnisnap-data/bundles [--require-effective]
go run ./cmd/omnisnap-lab dataset --bundles ~/omnisnap-data/bundles --group crawl --label 0 --out dataset.csv
python tools/train.py --data ~/omnisnap-data --out assets/model
go test ./internal/triage/... -run ModelParity
```

## Project structure

`cmd/omnisnap-lab/` (crawl, mutate, dataset, sites); `internal/groundtruth/{planted.go, server.go, mutate.go, label.go, dataset.go, gates.go}`; `internal/groundtruth/inject/mutate.js` (lab only: `go:embed` reaches only its own package directory, and a test keeps it out of `cmd/omnisnap`); `testdata/planted/`; `tools/{train.py, requirements.txt}`.

## Code style

Per conventions; Python follows PEP 8 with type hints, one script per step, no notebooks in the repo.

## Testing strategy

- **Gates** (CI, e2e): the success criteria below, on Chrome + Firefox + WebKitGTK.
- **Labeling unit tests:** synthetic clean/mutated tree pairs: changed set, neighbourhood, pre-existing causes excluded, class-wide targets, hidden targets (the parent's vanished child).
- **Mutation smoke:** 16 mutants on two pages, all effective, ≥ 80% with a labeled positive (round 6: 14 of 16).
- **Training reproducibility:** fixed seeds; the same dataset gives the same model bytes.

## Boundaries

- **Always:** split by site; keep planted pages out of training; report recall with Wilson intervals; record dataset and model versions together.
- **Ask first:** changing operators, the labeling neighbourhood, gate thresholds, or shipping a retrained model.
- **Never:** use human labels as ground truth; ship `mutate.js` in the user binary; crawl sites the filters reject.

## Success criteria (CI gates)

1. Planted recall in the Report tier ≥ 90% (round 5: 24/24); every planted page within its Report budget.
2. 0 Report causes on control pages and planted pages in non-Firefox engines; KB fixtures 0 Report / 0 unexplained.
3. Intent: ≥ 5/6 planted intent bugs; 0 violations on controls.
4. Model parity ≤ 1e-9; feature parity exact.
5. Before the model may set the tier: at the rules' recall, the model reports fewer natural causes (cross-validated by site, ≥ 100 sites, ≥ 300 positives), and keeps planted recall ≥ 90% with 0 control reports.

## Open questions

- Round 6 results are pending (training run paused for disk space). The model's role in v1 depends on them.
- Planted pages fire in Firefox only; add WebKit-only planted variants so the suite tests both pairs symmetrically?
- Keep training tooling in Python, or port the trainer to Go once the model choice is settled? Proposed: Python (LightGBM is the reference implementation).
