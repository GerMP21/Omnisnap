# Extending Omnisnap

Recipes for the changes people make most often: which files to touch, which tests must pass, and which spec to update. The rules behind them are in [`CONTRIBUTING.md`](https://github.com/GerMP21/Omnisnap/blob/main/CONTRIBUTING.md) and [`specs/CONVENTIONS.md`](https://github.com/GerMP21/Omnisnap/blob/main/specs/CONVENTIONS.md): anything that contradicts a spec changes that spec in the same contribution, and some changes need approval first (the injected scripts, capture discipline constants, thresholds, new dependencies, public flags).

**One trap applies to every analysis change.** The tree model's input is a fixed list of 82 features ([`features.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/triage/features.go)): cause kinds, consequences, KB ids, mechanisms, perceptual events, consensus verdicts, roles and engines. If a change adds a value to one of those lists, the shipped model no longer matches this binary's features. [`Compatible`](https://github.com/GerMP21/Omnisnap/blob/main/internal/triage/model.go) then refuses it, and every score becomes `null`, hidden rather than guessed. Either keep the new value out of the feature lists, or retrain the model and regenerate its parity fixture ([Ground truth and the model](Ground-Truth-and-the-Model)).

## A knowledge-base entry

An expected engine difference that should be explained away (or, like K5, named with its fix).

1. Add the matcher to [`Match`](https://github.com/GerMP21/Omnisnap/blob/main/internal/triage/kb.go) in `internal/triage/kb.go`: the signature, and a `KB{ID, Title, Explanation, Fix, Expected}`. Order matters: the first entry that matches wins.
2. Add a planted `kb` fixture page that triggers it ([below](#a-planted-page)) and expect it to be explained.
3. Add a unit test next to [`kb_test.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/triage/kb_test.go). Mutation-test the bound if it could explain a real bug away (that is how K4's bound was found).
4. Decide the feature question: adding the id to `FeatureKB` means retraining.
5. Update the table in [`SPEC-triage.md`](https://github.com/GerMP21/Omnisnap/blob/main/specs/SPEC-triage.md) §1, and the [Triage](Triage) page.

## A cause kind

1. Emit it in [`Analyze`](https://github.com/GerMP21/Omnisnap/blob/main/internal/layoutdiff/analyze.go) (or in its own pass, like [`tables.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/layoutdiff/tables.go)), with the cause-versus-effect rule that keeps it from repeating on every child.
2. Give it a consequence in [`consequences`](https://github.com/GerMP21/Omnisnap/blob/main/internal/triage/consequences.go) if a person can see it, and decide whether that consequence is hard.
3. Give it a title in triage and a severity weight in [`severity.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/layoutdiff/severity.go).
4. Tests: a hand-built tree in [`align_test.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/layoutdiff/align_test.go); golden parity must still pass ([`TestGoldenAlignParity`](https://github.com/GerMP21/Omnisnap/blob/main/internal/layoutdiff/golden_test.go)); the planted gates must still pass.
5. The feature question: `FeatureKinds`.
6. Update [`SPEC-layout-diff.md`](https://github.com/GerMP21/Omnisnap/blob/main/specs/SPEC-layout-diff.md) §4, the README's "What it detects", and the [Layout diff](Layout-Diff) page.

## A DOM-audit check

1. Write the check as a function of the walker in [`checks.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/domaudit/checks.go), add it to `checks` in reporting order, and name its threshold in [`audit.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/domaudit/audit.go).
2. Start it at `info`; promoting a check to `report` needs approval and crawl evidence (median findings per page at most 2).
3. Tests: a unit test in [`audit_test.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/domaudit/audit_test.go), a planted page it fires on and a control it does not (`TestPlantedAuditPages` in [`test/e2e/audit_test.go`](https://github.com/GerMP21/Omnisnap/blob/main/test/e2e/audit_test.go)); `TestAuditIsFast` must stay under 50 ms.
4. Add its human name to `auditCheckName` in the viewer's [`app.js`](https://github.com/GerMP21/Omnisnap/blob/main/internal/viewer/app.js).
5. Update [`SPEC-dom-audit.md`](https://github.com/GerMP21/Omnisnap/blob/main/specs/SPEC-dom-audit.md) and the [DOM audit](DOM-Audit) page.

## A browser or an engine

**A browser of an existing family** (another Blink browser, say):

1. Add it to the [catalog](https://github.com/GerMP21/Omnisnap/blob/main/internal/discovery/catalog.go) with every name it goes by, and to the per-OS sources that should find it (the Windows catalog, the macOS bundle ids).
2. Add a fixture for each OS source (a `reg query` dump, an `Info.plist`) and a test in that OS's discovery test file.
3. If it blocks requests itself, it is a branded browser: see [Branded browsers](Branded-Browsers).
4. Update [`SPEC-discovery.md`](https://github.com/GerMP21/Omnisnap/blob/main/specs/SPEC-discovery.md).

**A new engine family or protocol** is a larger change across five modules:

- [discovery](Discovery): the family and protocol;
- [process](Process-Management): a `Start…` with its launch flags, isolation and cleanup tests;
- [drivers](Drivers): a `drivers.Session` implementation, honest `Capabilities`, and a harness in the conformance suite;
- [`canDrive`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/env.go) and `launch` in `internal/cli/env.go`;
- the lane it belongs to in [`lanes`](https://github.com/GerMP21/Omnisnap/blob/main/internal/capture/lanes.go).

It needs a spec change in each module first.

## A planted page

The planted suite is generated from the spike, so the Go fixtures are never edited by hand:

1. Add the page to [`spikes/layout-diff/planted/pages.mjs`](https://github.com/GerMP21/Omnisnap/blob/main/spikes/layout-diff/planted/pages.mjs): the base page, its `.ff` rule that plants exactly one divergence in Firefox, its kind (`planted`, `control`, `kb`, `native`) and its expected answer (`expect`, `maxFindings`, `intent`).
2. Export: `node spikes/layout-diff/tools/export-planted.mjs` writes `testdata/planted/<slug>.html` and `expectations.json` ([`export-planted.mjs`](https://github.com/GerMP21/Omnisnap/blob/main/spikes/layout-diff/tools/export-planted.mjs)). CI runs it with `--check` and fails if the fixtures drift.
3. Capture its golden captures with the spike and export them (`export-golden.mjs`), so the gates and parity tests cover it ([`testdata/golden/README.md`](https://github.com/GerMP21/Omnisnap/blob/main/testdata/golden/README.md)).
4. Tests that must pass: `TestLoadPlantedMatchesTheSuite`, `TestGoldenPlantedCoversTheSuite`, and `TestPlantedGates` (recall stays at or above 90%; controls stay at 0 Report).
5. Update the list in [`SPEC-ground-truth.md`](https://github.com/GerMP21/Omnisnap/blob/main/specs/SPEC-ground-truth.md) §1.

## Source files

- [`CONTRIBUTING.md`](https://github.com/GerMP21/Omnisnap/blob/main/CONTRIBUTING.md): the contribution rules
- [`specs/CONVENTIONS.md`](https://github.com/GerMP21/Omnisnap/blob/main/specs/CONVENTIONS.md): the spec conventions
- [`spikes/layout-diff/planted/pages.mjs`](https://github.com/GerMP21/Omnisnap/blob/main/spikes/layout-diff/planted/pages.mjs): the planted suite's source
- [`spikes/layout-diff/tools/export-planted.mjs`](https://github.com/GerMP21/Omnisnap/blob/main/spikes/layout-diff/tools/export-planted.mjs): exports the planted fixtures
