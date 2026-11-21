# Ground truth and the model

**Role.** The known answers behind every precision and recall claim: the planted suite, mutation testing on real pages, the crawl, the dataset, the tree model and the CI gates. Module `ground-truth`, package [`internal/groundtruth`](https://github.com/GerMP21/Omnisnap/tree/main/internal/groundtruth), the lab binary [`cmd/omnisnap-lab`](https://github.com/GerMP21/Omnisnap/tree/main/cmd/omnisnap-lab), the trainer [`tools/train.py`](https://github.com/GerMP21/Omnisnap/blob/main/tools/train.py), and the shipped model in [`assets/model`](https://github.com/GerMP21/Omnisnap/tree/main/assets/model).

`omnisnap-lab` is never released: [`tools/release`](https://github.com/GerMP21/Omnisnap/tree/main/tools/release) builds only `./cmd/omnisnap`. Its commands ([`main.go`](https://github.com/GerMP21/Omnisnap/blob/main/cmd/omnisnap-lab/main.go)):

```
omnisnap-lab serve  --dir testdata/planted --addr 127.0.0.1:8099
omnisnap-lab sites  --n 500 --out crawl-sites.json
omnisnap-lab crawl  --list crawl-sites.json --out crawl-out [--shards N --shard I]
omnisnap-lab diff   --bundles <dirs>
omnisnap-lab mutate --bundles <dirs> [--per-engine 4] [--require-effective]
omnisnap-lab dataset --bundles <dirs> --group planted|crawl|mut --label -1|0|1 --out dataset.csv
omnisnap-lab fingerprints --bundles crawl-out --out assets/model/fingerprints.json
```

## The planted suite

[`testdata/planted`](https://github.com/GerMP21/Omnisnap/tree/main/testdata/planted): 31 small synthetic pages served by [`omnisnap-lab serve`](https://github.com/GerMP21/Omnisnap/blob/main/internal/groundtruth/server.go). A blocking script adds `ff` to `<html>` in Firefox, and each planted page's `.ff` rule plants exactly one divergence, so every other engine sees a control. [`expectations.json`](https://github.com/GerMP21/Omnisnap/blob/main/testdata/planted/expectations.json) holds each page's answer ([`LoadPlanted`](https://github.com/GerMP21/Omnisnap/blob/main/internal/groundtruth/planted.go)):

- **planted** (24): card width, heading wrap, overflow, missing element, padding, order, font, clip, gap, image, card CTA alignment, grid column, toolbar alignment, list rhythm, equal-height tiles, badge overlap, ellipsis, line-clamp, aspect ratio, nav wrap, code scroll, hidden notice, table layout, flex min-width. Six are design-intent bugs. Each has `expect` ids and a `maxFindings` budget;
- **control** (4): no bug;
- **kb** (2): the K1 and K2 fixtures, which must be explained;
- **native** (1): genuine form-control differences, not scored.

## The gates

[`TestPlantedGates`](https://github.com/GerMP21/Omnisnap/blob/main/test/e2e/planted_test.go) (`e2e` tag) runs on every PR, on the **golden** captures in [`testdata/golden/planted`](https://github.com/GerMP21/Omnisnap/tree/main/testdata/golden/planted), so it gates the comparison, not the capture (geometry parity checks the capture separately). From [`SPEC-ground-truth.md`](https://github.com/GerMP21/Omnisnap/blob/main/specs/SPEC-ground-truth.md), success criteria:

1. planted recall in the Report tier ≥ 90% (the rules reach 24/24), and every planted page within its budget;
2. 0 Report causes on control pages and on planted pages in non-Firefox engines; KB fixtures 0 Report, 0 unexplained;
3. intent: at least 5 of the 6 planted intent bugs, 0 violations on controls;
4. model parity ≤ 1e-9, feature parity exact.

The same planted gates run on what the npm and PyPI packages return in a fresh install ([`check_summary.py`](https://github.com/GerMP21/Omnisnap/blob/main/test/wrappers/check_summary.py)); there, a planted page without a Report cause is named but does not fail on its own, because those are live captures on the runner's fonts.

## Mutation testing

[`omnisnap-lab mutate`](https://github.com/GerMP21/Omnisnap/blob/main/internal/groundtruth/mutate.go) injects **one** realistic cross-engine bug into a settled real page, in one engine only, measures, and reverts. The 19 operators ([`MutateOps`](https://github.com/GerMP21/Omnisnap/blob/main/internal/groundtruth/mutate.go)): width shrink, padding, padding-left, margin-top, nudge, hide, invisible, gap, wrap-narrow, font-up, nowrap, align-self, overlap, clip, order, letter-spacing, line-height, border, ellipsis-clip. The operator is fixed by the seed, so the same page and seed always inject the same bug; when the target's class is shared by 30 elements or fewer, the whole class gets it, as a stylesheet bug would. Mutants are injected in Firefox and compared with the clean Blink captures, the same pair as the natural corpus. A mutant gets the same screenshot and probes as a clean capture ([`capture.Evidence`](https://github.com/GerMP21/Omnisnap/blob/main/internal/capture/evidence.go)), so its causes carry the same evidence.

**Labels** ([`label.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/groundtruth/label.go)): a same-engine diff (clean versus mutated) shows what the bug changed; a cross-engine cause is **positive** when it sits on a changed element in the target's neighbourhood and was not already a cause in the clean diff. A mutant that moves nothing is kept as ineffective. The policy is in [`SPEC-ground-truth-mutant-policy.md`](https://github.com/GerMP21/Omnisnap/blob/main/specs/SPEC-ground-truth-mutant-policy.md).

## The crawl, the dataset, the model

1. **Sites** ([`sites.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/groundtruth/sites.go), [`tranco.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/groundtruth/tranco.go)): from the Tranco list, excluding infrastructure, adult content and gambling; one site per brand; a homepage of at least 5 KB of HTML with a title, no bot challenge.
2. **Crawl and diff** ([`crawl.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/groundtruth/crawl.go)): the product's capture pipeline, sharded; `omnisnap-lab diff` runs the CLI's analysis over each bundle and writes `findings.json` with `side` and `nodeId` on every cause, the join key for labels.
3. **Dataset** ([`dataset.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/groundtruth/dataset.go)): one row per cause with triage's 82 features. Label 1 = mutation positives; label 0 = the causes of clean pages, which are natural engine differences but **unlabeled** (positive-unlabeled), so precision is never reported. Planted pages are never trained on.
4. **Training** ([`tools/train.py`](https://github.com/GerMP21/Omnisnap/blob/main/tools/train.py)): LightGBM binary objective, 300 rounds, 15 leaves, `scale_pos_weight` = negatives / positives, GroupKFold by **site**, deterministic. It exports `gbdt.txt`, `gbdt.json`, `gbdt-meta.json` and the parity fixture [`testdata/model/parity.json`](https://github.com/GerMP21/Omnisnap/blob/main/testdata/model/parity.json).
5. **Fingerprints** (`omnisnap-lab fingerprints`): the novelty table, [`fingerprints.json`](https://github.com/GerMP21/Omnisnap/blob/main/assets/model/fingerprints.json), 300 fingerprints over 457 pages.

## Why the model only ranks

The model shipped on 2026-11-19 was trained on 4,693 positives and 4,644 negatives from 458 sites, and has a cross-validated ROC AUC of 0.83 ([`gbdt-meta.json`](https://github.com/GerMP21/Omnisnap/blob/main/assets/model/gbdt-meta.json)). The gate that would let it set the tier (success criterion 5) was run on the crawl ([`docs/status-2026-11-19.md`](https://github.com/GerMP21/Omnisnap/blob/main/docs/status-2026-11-19.md)):

| Tier | Injected bugs caught | Natural causes reported | Per clean page | Planted |
|---|---|---|---|---|
| rules | 3038/4693 = 64.7% | 971/4644 = 20.9% | 2.12 | **24/24** |
| model at the rules' recall | 3038/4693 = 64.7% | 1022/4644 = 22.0% | 2.20 | 15/24 |
| model at 90% recall | 4224/4693 = 90.0% | 2544/4644 = 54.8% | 5.57 | 19/24 |

At the rules' recall the model is no quieter (1.05× overall, 1.36× on the pages the corpus covers, with the threshold matched per slice of pages), and it fails the planted suite: its positives are layout mutations of real pages, while the planted bugs are semantic (an element missing, an order changed, a hidden notice). An earlier headline number in its favour came from one global threshold and a novelty feature with two meanings (a class never seen scored as "occurs on no other page"; absent now means absent). So the score ranks findings and the rules keep the tier; the switch is the `tier_tau` key, which the shipped metadata does not carry ([Triage](Triage)).

The crawl's biggest result was a rule instead: 72% of the Report causes on real sites were line-box metrics around inline images, invisible to a person, and demoting that class took the rules from 13.4 to 3.8 Report causes per clean page.

## Tests

- [`TestPlantedGates`](https://github.com/GerMP21/Omnisnap/blob/main/test/e2e/planted_test.go); [`TestGoldenPlantedCoversTheSuite`](https://github.com/GerMP21/Omnisnap/blob/main/internal/groundtruth/golden_test.go); [`planted_test.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/groundtruth/planted_test.go) (the expectations match the pages, strict fields, the server).
- [`mutate_test.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/groundtruth/mutate_test.go), [`label_test.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/groundtruth/label_test.go), [`labels_test.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/groundtruth/labels_test.go), [`dataset_test.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/groundtruth/dataset_test.go), [`sites_test.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/groundtruth/sites_test.go), [`crawl_test.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/groundtruth/crawl_test.go).
- Model and feature parity in [`internal/triage`](https://github.com/GerMP21/Omnisnap/tree/main/internal/triage) ([Triage](Triage), tests).

## Where the code differs from the spec

- **Two stale lines in `SPEC-ground-truth.md` §4.** It says the frequency table "ships empty until the first crawl" (it ships 300 fingerprints) and that `gbdt.txt` is "loaded by `leaves`" (the binary evaluates `gbdt.json` itself; see [Triage](Triage)).
- **`tools/sites.py`** is named in §3 as an alternative; it does not exist at this commit. The crawl list comes from `omnisnap-lab sites`.

## Spec

[`specs/SPEC-ground-truth.md`](https://github.com/GerMP21/Omnisnap/blob/main/specs/SPEC-ground-truth.md) and [`specs/SPEC-ground-truth-mutant-policy.md`](https://github.com/GerMP21/Omnisnap/blob/main/specs/SPEC-ground-truth-mutant-policy.md).

## Source files

- [`cmd/omnisnap-lab/main.go`](https://github.com/GerMP21/Omnisnap/blob/main/cmd/omnisnap-lab/main.go): the lab binary's commands
- [`cmd/omnisnap-lab/capture.go`](https://github.com/GerMP21/Omnisnap/blob/main/cmd/omnisnap-lab/capture.go): the lab's capturer
- [`cmd/omnisnap-lab/diff.go`](https://github.com/GerMP21/Omnisnap/blob/main/cmd/omnisnap-lab/diff.go): re-diffing bundles
- [`cmd/omnisnap-lab/fingerprints.go`](https://github.com/GerMP21/Omnisnap/blob/main/cmd/omnisnap-lab/fingerprints.go): building the novelty table
- [`cmd/omnisnap-lab/mutate.go`](https://github.com/GerMP21/Omnisnap/blob/main/cmd/omnisnap-lab/mutate.go): mutation testing
- [`cmd/omnisnap-lab/webkit_linux.go`](https://github.com/GerMP21/Omnisnap/blob/main/cmd/omnisnap-lab/webkit_linux.go): the lab's WebKit on Linux
- [`cmd/omnisnap-lab/webkit_darwin.go`](https://github.com/GerMP21/Omnisnap/blob/main/cmd/omnisnap-lab/webkit_darwin.go): the lab's WebKit on macOS
- [`cmd/omnisnap-lab/webkit_other.go`](https://github.com/GerMP21/Omnisnap/blob/main/cmd/omnisnap-lab/webkit_other.go): the lab's WebKit elsewhere
- [`internal/groundtruth/planted.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/groundtruth/planted.go): the planted suite
- [`internal/groundtruth/server.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/groundtruth/server.go): the planted server
- [`internal/groundtruth/mutate.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/groundtruth/mutate.go): mutant scheduling
- [`internal/groundtruth/inject/embed.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/groundtruth/inject/embed.go): mutate.js, lab only
- [`internal/groundtruth/label.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/groundtruth/label.go): labelling mutants' causes
- [`internal/groundtruth/labels.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/groundtruth/labels.go): the labels file
- [`internal/groundtruth/crawl.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/groundtruth/crawl.go): the crawl
- [`internal/groundtruth/sites.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/groundtruth/sites.go): the site list and filter
- [`internal/groundtruth/tranco.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/groundtruth/tranco.go): the Tranco list
- [`internal/groundtruth/probe.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/groundtruth/probe.go): probing sites before a crawl
- [`internal/groundtruth/dataset.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/groundtruth/dataset.go): the training dataset
- [`internal/groundtruth/disk.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/groundtruth/disk.go): the disk guard
- [`internal/groundtruth/disk_unix.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/groundtruth/disk_unix.go): free disk space, on Unix
- [`internal/groundtruth/disk_windows.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/groundtruth/disk_windows.go): free disk space, on Windows
- [`tools/train.py`](https://github.com/GerMP21/Omnisnap/blob/main/tools/train.py): trains the model
- [`tools/requirements.txt`](https://github.com/GerMP21/Omnisnap/blob/main/tools/requirements.txt): the training script's Python dependencies
