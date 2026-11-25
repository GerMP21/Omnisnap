# Testing and CI

How Omnisnap is tested, what each test level needs, and what every workflow in [`.github/workflows`](https://github.com/GerMP21/Omnisnap/tree/main/.github/workflows) does.

## Test levels

| Level | Run with | Needs | What it covers |
|---|---|---|---|
| Unit | `go test ./...` | nothing | every package, on fixtures: fake sessions, recorded driver transcripts, golden captures, schema examples, a fake Firefox for install |
| Golden parity | part of `go test ./...` | nothing | the Go port reproduces the spike's results on [`testdata/golden/planted`](https://github.com/GerMP21/Omnisnap/tree/main/testdata/golden/planted): alignment ([`layoutdiff`](https://github.com/GerMP21/Omnisnap/blob/main/internal/layoutdiff/golden_test.go)), triage ([`triage`](https://github.com/GerMP21/Omnisnap/blob/main/internal/triage/golden_test.go)), ink, model and feature parity |
| Ground-truth gates | `go test -tags e2e ./test/e2e/... -run Planted` | nothing (golden captures) | [`TestPlantedGates`](https://github.com/GerMP21/Omnisnap/blob/main/test/e2e/planted_test.go): planted recall, control pages, KB fixtures, budgets ([Ground truth and the model](Ground-Truth-and-the-Model)) |
| Wiki | `go test ./test/internals/` (part of `go test ./...`) | nothing | the pages in `docs/internals/` against the repository ([below](#the-wikis-own-check)) |
| Browsers | `go test -tags browsers ./internal/...` | installed browsers | discovery on the real machine, launch and cleanup cycles, driver conformance, viewport, the viewer in Chrome, branded warming |
| End to end | `go test -tags e2e ./test/e2e/...` | browsers | geometry parity of live captures, the audit and appearance planted pages |
| Network | `go test -tags network ./internal/install/...` | the internet | installing Mozilla's real Firefox ESR |

Build tags: `browsers` (needs installed browsers), `e2e` (end to end), `network` (downloads), plus the OS tags for per-platform code. Environment switches used by tests: `OMNISNAP_EXPECT_BROWSERS` and `OMNISNAP_EXPECT_ENGINES` (CI lists what the runner image has, so a missing browser fails instead of skipping), `OMNISNAP_COST=1` (lane timing), `OMNISNAP_NO_SANDBOX=1` (restricted hosts), `OMNISNAP_WEBKIT=wpe|gtk`, and `WEBKIT_HEADLESS=0` (show WebKitGTK windows).

Lint: [`.golangci.yml`](https://github.com/GerMP21/Omnisnap/blob/main/.golangci.yml). The in-page scripts are pinned by `SHA256SUMS` and checked by [`inject_test.go`](https://github.com/GerMP21/Omnisnap/blob/main/assets/inject/inject_test.go).

## Workflows

| Workflow | Trigger | Jobs and what they check |
|---|---|---|
| [`ci.yml`](https://github.com/GerMP21/Omnisnap/blob/main/.github/workflows/ci.yml) | push to `main`, every pull request | **test** (Linux, Windows, macOS): build, `omnisnap version`, `go test ./...`, the planted fixtures match the spike (`export-planted.mjs --check`), lint. **planted-gates**: the e2e gates. **conformance** (three OSes): discovery with the runner's browsers, driver conformance, the viewer in Chrome, process cleanup (including `TerminateProcess` on Windows), branded. **packages** (three OSes): the npm and PyPI wrappers' tests, then the packages as published in a fresh project and venv, `run()` on the planted suite, and the planted gates on its summary. |
| [`walkthrough.yml`](https://github.com/GerMP21/Omnisnap/blob/main/.github/workflows/walkthrough.yml) | push to `main`, pull requests, manual | the README walkthrough on every OS |
| [`e2e-action.yml`](https://github.com/GerMP21/Omnisnap/blob/main/.github/workflows/e2e-action.yml) | manual, releases | the composite action from this checkout on the planted suite, on three OSes; Windows injects a failure; the merged comment must name the planted finding ([GitHub Action](GitHub-Action)) |
| [`e2e-report.yml`](https://github.com/GerMP21/Omnisnap/blob/main/.github/workflows/e2e-report.yml) | manual, releases, pull requests touching `report.yml` or `action/` | calls `report.yml` the way a consumer does, then asserts on the result |
| [`install.yml`](https://github.com/GerMP21/Omnisnap/blob/main/.github/workflows/install.yml) | weekly (Mondays 04:17 UTC), manual | the network install test on Linux and Windows ([Install](Install)) |
| [`safari-smoke.yml`](https://github.com/GerMP21/Omnisnap/blob/main/.github/workflows/safari-smoke.yml) | manual only | does Safari run unattended on a hosted macOS runner (hosted macOS is billed at about 10×) |
| [`release.yml`](https://github.com/GerMP21/Omnisnap/blob/main/.github/workflows/release.yml) | a pushed `v*` tag, manual | the unit tests, the five binaries, the GitHub release, and a check that the assets download ([Release process](Release-Process)) |
| [`deploy.yml`](https://github.com/GerMP21/Omnisnap/blob/main/.github/workflows/deploy.yml) | called by `release.yml` after the assets are verified; manual (optional `tag`, else the newest release) | captures the demo through `report.yml`, builds [the website](Website) from the tag and deploys the Worker to `omnisnap.org` |
| [`wiki.yml`](https://github.com/GerMP21/Omnisnap/blob/main/.github/workflows/wiki.yml) | push to `main` touching `docs/internals/`, `scripts/publish-wiki.sh` or itself; manual | publishes this wiki ([below](#the-wikis-own-check)) |
| [`report.yml`](https://github.com/GerMP21/Omnisnap/blob/main/.github/workflows/report.yml) | `workflow_call` | the reusable workflow consumers call; not a test |
| [`report-comment.yml`](https://github.com/GerMP21/Omnisnap/blob/main/.github/workflows/report-comment.yml) | `workflow_call` | the fork-safe comment; not a test |

## The wiki's own check

This wiki's pages are in [`docs/internals/`](https://github.com/GerMP21/Omnisnap/tree/main/docs/internals). [`test/internals/internals_test.go`](https://github.com/GerMP21/Omnisnap/blob/main/test/internals/internals_test.go) checks them on every `go test ./...`, so CI's test job runs it on all three OSes:

| Test | Fails when |
|---|---|
| `TestEveryPageListsItsSourceFiles` | a page other than Home, Glossary, `_Sidebar` and `_Footer` has no final `## Source files` section, or the section lists something other than repository links |
| `TestEveryRepositoryLinkExistsOnMain` | a code link names a path that does not exist, points at a ref other than `main`, carries a line anchor (`#L…`), or uses `blob/` for a directory or `tree/` for a file |
| `TestEverySourceFileIsNamed` | a non-test Go file under `cmd/`, `internal/`, `tools/` or `assets/` is named in no page's "Source files", or a top-level item of `.github/workflows/`, `action/`, `packages/`, `schema/`, `scripts/`, `test/` or `tools/` has nothing under it named |
| `TestWikiLinksResolve` | a link between pages, or to a heading on one, does not resolve (GitHub's anchor rule, with `-1`, `-2` on repeated headings) |
| `TestSidebarLinksEveryPage` | a page is missing from `_Sidebar.md` |

Each test lists every offending page and item at once. Links inside code (fenced or inline) are not checked. The test cannot tell whether a page still says what the code does; the rule in [`CONTRIBUTING.md`](https://github.com/GerMP21/Omnisnap/blob/main/CONTRIBUTING.md) covers that.

Publishing: [`wiki.yml`](https://github.com/GerMP21/Omnisnap/blob/main/.github/workflows/wiki.yml) runs on every push to `main` that changes the pages, the script or itself, and by hand. It clones `Omnisnap.wiki.git` with the workflow's token (`contents: write`), runs [`scripts/publish-wiki.sh`](https://github.com/GerMP21/Omnisnap/blob/main/scripts/publish-wiki.sh), and pushes. The script removes every file in the clone, copies `docs/internals/*.md` in, and commits "Publish docs/internals from <sha>" only when something changed, so the wiki holds exactly the pages of that commit. A newer run cancels an older one. If the clone fails, the wiki has no first page yet: create one in the Wiki tab, then run the workflow again.

## Fixtures

| Directory | Holds |
|---|---|
| [`testdata/golden`](https://github.com/GerMP21/Omnisnap/tree/main/testdata/golden) | the spike's captures and findings for the planted suite ([README](https://github.com/GerMP21/Omnisnap/blob/main/testdata/golden/README.md): Chrome 155, Firefox 153.6 ESR, WebKitGTK 2.50.4, Linux, 1280×800). Real-site goldens are local only. |
| [`testdata/planted`](https://github.com/GerMP21/Omnisnap/tree/main/testdata/planted) | the planted pages and `expectations.json` |
| [`testdata/drivers/transcripts`](https://github.com/GerMP21/Omnisnap/tree/main/testdata/drivers/transcripts) | recorded CDP, BiDi and Classic sessions |
| [`testdata/discovery`](https://github.com/GerMP21/Omnisnap/tree/main/testdata/discovery) | `reg query` dumps and `Info.plist` files |
| [`testdata/schema`](https://github.com/GerMP21/Omnisnap/tree/main/testdata/schema) | valid and invalid examples per schema |
| [`testdata/install`](https://github.com/GerMP21/Omnisnap/tree/main/testdata/install) | a fake Firefox, signed sums, the test key |
| [`testdata/model/parity.json`](https://github.com/GerMP21/Omnisnap/blob/main/testdata/model/parity.json) | 30 rows scored by LightGBM, for the Go evaluator |
| [`testdata/viewer`](https://github.com/GerMP21/Omnisnap/tree/main/testdata/viewer) | summaries for the viewer, single and merged |
| [`testdata/branded/trackers.html`](https://github.com/GerMP21/Omnisnap/blob/main/testdata/branded/trackers.html) | a page with tracker requests, for the branded checks |

## Source files

- [`.github/workflows/ci.yml`](https://github.com/GerMP21/Omnisnap/blob/main/.github/workflows/ci.yml): tests, gates, conformance, packages
- [`.github/workflows/walkthrough.yml`](https://github.com/GerMP21/Omnisnap/blob/main/.github/workflows/walkthrough.yml): the README walkthrough
- [`.github/workflows/install.yml`](https://github.com/GerMP21/Omnisnap/blob/main/.github/workflows/install.yml): the weekly network install test
- [`.github/workflows/safari-smoke.yml`](https://github.com/GerMP21/Omnisnap/blob/main/.github/workflows/safari-smoke.yml): the Safari smoke test
- [`.github/workflows/deploy.yml`](https://github.com/GerMP21/Omnisnap/blob/main/.github/workflows/deploy.yml): deploys omnisnap.org
- [`.github/workflows/wiki.yml`](https://github.com/GerMP21/Omnisnap/blob/main/.github/workflows/wiki.yml): publishes this wiki
- [`scripts/publish-wiki.sh`](https://github.com/GerMP21/Omnisnap/blob/main/scripts/publish-wiki.sh): mirrors this wiki into the GitHub Wiki
- [`test/internals/internals_test.go`](https://github.com/GerMP21/Omnisnap/blob/main/test/internals/internals_test.go): the checks on this wiki
- [`test/e2e`](https://github.com/GerMP21/Omnisnap/tree/main/test/e2e): the end-to-end and ground-truth gates
- [`test/wrappers`](https://github.com/GerMP21/Omnisnap/tree/main/test/wrappers): the wrappers' planted runs
- [`tools/smoke/safari/main.go`](https://github.com/GerMP21/Omnisnap/blob/main/tools/smoke/safari/main.go): the Safari smoke command
- [`tools/site/build.py`](https://github.com/GerMP21/Omnisnap/blob/main/tools/site/build.py): builds omnisnap.org
- [`tools/README.md`](https://github.com/GerMP21/Omnisnap/blob/main/tools/README.md): what each tool is for
- [`internal/testutil/sha256sums.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/testutil/sha256sums.go): reading SHA256SUMS in tests
- [`.golangci.yml`](https://github.com/GerMP21/Omnisnap/blob/main/.golangci.yml): the lint configuration
