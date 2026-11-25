# Repository layout

Every tracked top-level entry on `main`, then every package under `internal/`. Local-only directories (`bin/`, `crawl/`, `ci-logs/`, `_site/`, `dist-py/`, …) are ignored by git and not listed.

## Top level

| Path | What it holds |
|---|---|
| [`cmd/`](https://github.com/GerMP21/Omnisnap/tree/main/cmd) | The two binaries: [`omnisnap`](https://github.com/GerMP21/Omnisnap/tree/main/cmd/omnisnap) (the product; `main` calls `cli.Main`) and [`omnisnap-lab`](https://github.com/GerMP21/Omnisnap/tree/main/cmd/omnisnap-lab) (ground-truth tooling, never released). |
| [`internal/`](https://github.com/GerMP21/Omnisnap/tree/main/internal) | All Go code of the product, one package per module (below). |
| [`assets/`](https://github.com/GerMP21/Omnisnap/tree/main/assets) | Files embedded in the binary: [`inject/`](https://github.com/GerMP21/Omnisnap/tree/main/assets/inject) (the JavaScript run in the page: `stabilize.js`, `extract.js`, `probes.js`, with `SHA256SUMS`) and [`model/`](https://github.com/GerMP21/Omnisnap/tree/main/assets/model) (the tree model `gbdt.txt`, its metadata, and the `fingerprints.json` novelty table). |
| [`schema/`](https://github.com/GerMP21/Omnisnap/tree/main/schema) | The public JSON Schemas: job, capture, host, progress, summary. Published at `omnisnap.org/schema/…`. |
| [`action/`](https://github.com/GerMP21/Omnisnap/tree/main/action) | The composite GitHub Action (`action.yml`), its Linux engine setup script and its README. |
| [`packages/`](https://github.com/GerMP21/Omnisnap/tree/main/packages) | The npm packages (`omnisnap` and five `@omnisnap/<os>-<arch>` platform packages) and the PyPI package. |
| [`tools/`](https://github.com/GerMP21/Omnisnap/tree/main/tools) | Release tooling ([`release/`](https://github.com/GerMP21/Omnisnap/tree/main/tools/release): binaries, staging, `wheels.py`), the site builder for omnisnap.org ([`site/build.py`](https://github.com/GerMP21/Omnisnap/blob/main/tools/site/build.py)), the Safari smoke test ([`smoke/`](https://github.com/GerMP21/Omnisnap/tree/main/tools/smoke)), and the model trainer ([`train.py`](https://github.com/GerMP21/Omnisnap/blob/main/tools/train.py)). |
| [`test/`](https://github.com/GerMP21/Omnisnap/tree/main/test) | Tests that need real browsers or a built binary: [`e2e/`](https://github.com/GerMP21/Omnisnap/tree/main/test/e2e) (planted pages, geometry parity, audit, appearance) [`wrappers/`](https://github.com/GerMP21/Omnisnap/tree/main/test/wrappers) (the npm and PyPI packages, installed fresh, on the planted suite) and [`internals/`](https://github.com/GerMP21/Omnisnap/tree/main/test/internals) (the checks on this wiki). |
| [`testdata/`](https://github.com/GerMP21/Omnisnap/tree/main/testdata) | Fixtures: planted pages and their expectations, golden captures from the spike, recorded driver transcripts, discovery fixtures per OS, schema examples (valid and invalid), install fixtures (a fake Firefox and signed sums), the model parity fixture, viewer summaries. |
| [`specs/`](https://github.com/GerMP21/Omnisnap/tree/main/specs) | The capability map, the conventions, and one spec per module. Normative. |
| [`tasks/`](https://github.com/GerMP21/Omnisnap/tree/main/tasks) | `plan.md` (architecture decisions and phase order), `todo.md` (every task with acceptance criteria and verification), `backlog.md` (problems not planned yet). |
| [`docs/`](https://github.com/GerMP21/Omnisnap/tree/main/docs) | User docs (quick start, Action setup, fork comments), the [`ideas/`](https://github.com/GerMP21/Omnisnap/tree/main/docs/ideas) one-pager, the [status note of 2026-11-19](https://github.com/GerMP21/Omnisnap/blob/main/docs/status-2026-11-19.md), and [`internals/`](https://github.com/GerMP21/Omnisnap/tree/main/docs/internals), this wiki's pages. |
| [`scripts/`](https://github.com/GerMP21/Omnisnap/tree/main/scripts) | [`publish-wiki.sh`](https://github.com/GerMP21/Omnisnap/blob/main/scripts/publish-wiki.sh), which mirrors `docs/internals/` into the GitHub Wiki. |
| [`spikes/layout-diff/`](https://github.com/GerMP21/Omnisnap/tree/main/spikes/layout-diff) | The Node.js spike the Go port reproduces. Its outputs are the golden fixtures; its features define the model's vector. Not built or shipped. |
| [`openspec/`](https://github.com/GerMP21/Omnisnap/tree/main/openspec) | OpenSpec configuration and change proposals (planning artifacts). |
| [`site/`](https://github.com/GerMP21/Omnisnap/tree/main/site), [`src/worker.js`](https://github.com/GerMP21/Omnisnap/blob/main/src/worker.js), [`wrangler.json`](https://github.com/GerMP21/Omnisnap/blob/main/wrangler.json) | The [website](Website) at `omnisnap.org`: the landing page and stylesheet, and the Cloudflare Worker that serves the site [`tools/site/build.py`](https://github.com/GerMP21/Omnisnap/blob/main/tools/site/build.py) builds. |
| [`.github/`](https://github.com/GerMP21/Omnisnap/tree/main/.github) | Workflows, issue templates, and the [fork workflow template](https://github.com/GerMP21/Omnisnap/tree/main/.github/workflow-templates). |
| `.agents/`, `.claude/`, [`AGENTS.md`](https://github.com/GerMP21/Omnisnap/blob/main/AGENTS.md), [`CLAUDE.md`](https://github.com/GerMP21/Omnisnap/blob/main/CLAUDE.md), `skills-lock.json` | Configuration for coding agents. |
| `.golangci.yml`, `.gitattributes`, `.gitignore`, `go.mod`, `go.sum` | Lint, git and module configuration. `go.mod` also retracts withdrawn release candidates. |
| `README.md`, `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, `LICENSE.txt`, `THIRD-PARTY-NOTICES.md` | The usual project files. The license is AGPL-3.0-or-later. |

## `internal/`

| Package | Module | One line |
|---|---|---|
| [`appearance`](https://github.com/GerMP21/Omnisnap/tree/main/internal/appearance) | `appearance-diff` | SSIM on graphics crops and ink agreement: what geometry cannot see. |
| [`branded`](https://github.com/GerMP21/Omnisnap/tree/main/internal/branded) | `branded` | Warmed template profiles, blocked hosts, and their attribution to causes. |
| [`capture`](https://github.com/GerMP21/Omnisnap/tree/main/internal/capture) | `capture` | One page in every engine: lanes, runs, probes, retries, files. |
| [`cli`](https://github.com/GerMP21/Omnisnap/tree/main/internal/cli) | `cli` | The command line: parsing, the job, orchestration, progress, the bundle. |
| [`discovery`](https://github.com/GerMP21/Omnisnap/tree/main/internal/discovery) | `discovery` | Installed browsers per OS, without launching them. |
| [`domaudit`](https://github.com/GerMP21/Omnisnap/tree/main/internal/domaudit) | `dom-audit` | Single-engine anomalies on one capture. |
| [`drivers`](https://github.com/GerMP21/Omnisnap/tree/main/internal/drivers) | `drivers` | One interface over three protocols: [`cdp`](https://github.com/GerMP21/Omnisnap/tree/main/internal/drivers/cdp), [`bidi`](https://github.com/GerMP21/Omnisnap/tree/main/internal/drivers/bidi), [`classic`](https://github.com/GerMP21/Omnisnap/tree/main/internal/drivers/classic); [`wsrpc`](https://github.com/GerMP21/Omnisnap/tree/main/internal/drivers/wsrpc) is the shared WebSocket JSON-RPC client, [`replaytest`](https://github.com/GerMP21/Omnisnap/tree/main/internal/drivers/replaytest) replays recorded transcripts in tests. |
| [`fsutil`](https://github.com/GerMP21/Omnisnap/tree/main/internal/fsutil) | — | File helpers shared by the modules that write the bundle. |
| [`groundtruth`](https://github.com/GerMP21/Omnisnap/tree/main/internal/groundtruth) | `ground-truth` | The planted server, expectations, crawl list, mutation, dataset, gates. |
| [`install`](https://github.com/GerMP21/Omnisnap/tree/main/internal/install) | `install` | `omnisnap install firefox`: download, verify, unpack, uninstall. |
| [`layoutdiff`](https://github.com/GerMP21/Omnisnap/tree/main/internal/layoutdiff) | `layout-diff` | Matching, deltas, root causes, tables, intent, warnings. |
| [`merge`](https://github.com/GerMP21/Omnisnap/tree/main/internal/merge) | `merge` | Several machines' bundles into one. |
| [`pipeline`](https://github.com/GerMP21/Omnisnap/tree/main/internal/pipeline) | — | The analysis half for one page pair; writes `findings.json`. |
| [`process`](https://github.com/GerMP21/Omnisnap/tree/main/internal/process) | `process` | Starting browsers isolated and never leaving one behind. |
| [`schema`](https://github.com/GerMP21/Omnisnap/tree/main/internal/schema) | `bundle-schema` | Go types of every public contract, and their validation. |
| [`testutil`](https://github.com/GerMP21/Omnisnap/tree/main/internal/testutil) | — | Test helpers. |
| [`triage`](https://github.com/GerMP21/Omnisnap/tree/main/internal/triage) | `triage` | KB, consequences, tier, evidence (in [`evidence/`](https://github.com/GerMP21/Omnisnap/tree/main/internal/triage/evidence)), model score, grouping. |
| [`viewer`](https://github.com/GerMP21/Omnisnap/tree/main/internal/viewer) | `viewer` | `report.html` from a summary: template, script and style, embedded. |

## Source files

- [`internal`](https://github.com/GerMP21/Omnisnap/tree/main/internal): every module's package
- [`cmd`](https://github.com/GerMP21/Omnisnap/tree/main/cmd): the two binaries
- [`specs`](https://github.com/GerMP21/Omnisnap/tree/main/specs): the specs
- [`docs/internals`](https://github.com/GerMP21/Omnisnap/tree/main/docs/internals): this wiki
