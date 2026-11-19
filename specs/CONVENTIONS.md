# Shared conventions (all module specs)

Every `SPEC-<id>.md` covers the six core areas for its module and inherits everything here. A module spec only overrides this file where it says so.

## Tech stack

| Concern | Choice |
|---|---|
| Language | Go, latest stable release (1.26 or newer), pinned in `go.mod` |
| Binary | One static binary, `omnisnap`, for linux/amd64, linux/arm64, darwin/arm64, darwin/amd64, windows/amd64. `CGO_ENABLED=0` |
| Injected JavaScript | The spike's `stabilize.js`, `extract.js`, `probes.js` (and `mutate.js` for test tooling), embedded with `go:embed`, **unchanged** except through their own reviewed changes |
| Dependencies | Standard library first. Allowed without asking: `golang.org/x/sys` (Windows Job Objects, registry), `github.com/coder/websocket` (CDP/BiDi transport), `github.com/dmitryikh/leaves` (tree model). Anything else: ask first |
| Training tooling | Python 3.12 + LightGBM, outside the binary (`tools/`), see `SPEC-ground-truth.md` |

## Project structure

```
cmd/omnisnap/            the shipped binary (main only: flag parsing hands off to internal/cli)
cmd/omnisnap-lab/        NOT shipped: ground-truth tooling (mutation capture, dataset export)
internal/<module>/       one package per module id (bundle-schema -> internal/schema, layout-diff -> internal/layoutdiff, ...)
internal/drivers/{cdp,bidi,classic}/
assets/inject/           embedded JS (from spikes/layout-diff/inject/)
assets/kb/               knowledge base entries (YAML or JSON, versioned)
assets/model/            trained tree model (gbdt.txt) + metadata
schema/                  JSON Schemas (job, capture, summary, progress, host), published at
                         omnisnap.org/schema/ by the schema Worker (.github/workflows/deploy.yml)
testdata/                planted pages, golden captures and findings (shared fixtures)
test/e2e/                end-to-end tests through the binary
tools/                   Python training and crawl tooling
action/                  GitHub Action (SPEC-ci-action)
packages/npm/, packages/pypi/   wrappers (SPEC-wrappers)
specs/                   this folder
docs/                    PRD, research, ideas
spikes/                  throwaway spike code (reference only, never imported)
```

## Commands

```
go build -o bin/omnisnap ./cmd/omnisnap          # build
go test ./...                                     # unit tests: no browsers, no network
go test -race ./...                               # before every commit touching concurrency
go test -tags browsers ./internal/...             # integration: needs installed engines
go test -tags e2e ./test/e2e/...                  # planted suite through the binary
golangci-lint run ./...                           # lint (gofmt, goimports, govet, errcheck, staticcheck, revive)
go run ./cmd/omnisnap-lab <subcommand>            # ground-truth tooling
```

## Code style

```go
// Package discovery finds installed browsers without launching them.
package discovery

// Browser is one installed browser on this machine.
type Browser struct {
	ID         string `json:"id"`         // stable id: "chrome", "firefox-esr", "brave"
	Family     Family `json:"family"`     // blink | gecko | webkit
	Executable string `json:"executable"` // absolute path
	Version    string `json:"version"`    // "" when it can't be read without launching
}

// Find returns every launchable browser, cheapest sources first.
// It never starts a browser to read a version: that alone costs seconds.
func Find(ctx context.Context, opts Options) ([]Browser, error) {
	found, err := platformSources(opts).scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("discovery: %w", err)
	}
	return dedupe(found), nil
}
```

- `gofmt` + `goimports`; lowercase package names matching the module id without dashes.
- `context.Context` first on anything that does I/O or can block; no package-level mutable state.
- Errors wrap with `%w` and name the module (`"capture: navigate %s: %w"`); typed errors where callers branch (`drivers.ErrTimeout`).
- Comments explain **why** (the spike lesson, the engine quirk), not what.
- Numbers that came from the spike (tolerances, thresholds) are named constants with a comment pointing at the evidence.
- JSON field names are the schema's (`SPEC-bundle-schema.md`), never ad hoc.

## Testing strategy

| Level | What | Where | Runs |
|---|---|---|---|
| Unit | Pure logic: matching, triage, schema, parsing, merge | `*_test.go` next to the code, table-driven, fixtures in `testdata/` | every commit, all OSes |
| Golden parity | Go ports reproduce the spike's outputs on the same captures | `testdata/golden/` (captures + spike findings) | every commit |
| Integration | Real engines through drivers and capture | build tag `browsers` | CI on Linux (Chrome, Firefox, WebKitGTK) and Windows (Chrome, Edge, Firefox); macOS (Safari) through the manual `safari-smoke.yml` workflow |
| End-to-end | The planted suite through the binary; the pass criteria of `SPEC-ground-truth.md` | `test/e2e/`, build tag `e2e` | CI on every PR touching capture, layout-diff, triage |

Coverage: ≥ 80% statements on pure-logic packages (`schema`, `layoutdiff`, `triage`, `merge`); integration packages are judged by their integration tests, not line coverage.

## Boundaries

- **Always:** run unit tests and lint before committing; keep the planted suite green; record every threshold's evidence; write captures and reports only under the job's output directory and the OS temp directory; clean up every process and temp profile.
- **Ask first:** adding a dependency; changing a schema (needs a version bump); changing an injected script; changing a tolerance or threshold; anything that downloads; changing the Report/Info tier rule.
- **Never:** use the user's real browser profile; download browsers or models without an explicit command; send page content or results anywhere; fail a CI build in report-only mode; label findings by hand as ground truth; commit captured third-party page content outside `testdata/` fixtures we own.

## Evidence

Spike: `spikes/layout-diff/` (README rounds 1–5, round 6 in progress). Decisions: `docs/prd.md` changelog 1.1.0–1.4.0, `docs/SCIENTIFIC_GROUNDING.md`.
