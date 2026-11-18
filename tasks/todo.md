# Omnisnap v1: tasks

Plan: [plan.md](plan.md) · Specs: [specs/](../specs/CAPABILITY-MAP.md) · Conventions (commands, layout, Definition of Done): [specs/CONVENTIONS.md](../specs/CONVENTIONS.md)

Every task also meets the shared bar: unit tests and `golangci-lint run ./...` pass, `go build ./...` succeeds, no leftover processes or temp dirs, evidence for any new threshold recorded in a comment.

---

## Phase 0: Foundation and risk

## Task 1: Go repository skeleton and CI
**Description:** Create the Go module, the `omnisnap` entry point (`version` only), lint config and a CI workflow running unit tests and lint on Ubuntu, Windows and macOS.
**Acceptance criteria:**
- [x] `go build ./cmd/omnisnap` produces a binary whose `omnisnap version` prints the version
- [ ] CI runs `go test ./...` and `golangci-lint run ./...` on three OSes and is green
- [x] `cmd/omnisnap/main.go` only calls `cli.Main` (returns an exit code, no `os.Exit` inside)
**Verification:**
- [x] `go build -o bin/omnisnap ./cmd/omnisnap && bin/omnisnap version`
- [ ] CI run green on a PR
**Dependencies:** None (module path: `github.com/GerMP21/Omnisnap`)
**Files likely touched:** `go.mod`, `cmd/omnisnap/main.go`, `internal/cli/root.go`, `.golangci.yml`, `.github/workflows/ci.yml`
**Estimated scope:** M

## Task 2: Safari smoke test on a macOS runner
**Description:** Prove the riskiest assumption in week one: Safari runs unattended on GitHub's macOS runners through `safaridriver` (one-pager assumption).
**Acceptance criteria:**
- [ ] A workflow enables `safaridriver`, opens a session, navigates to a page, evaluates a script and takes a screenshot
- [ ] The result (works / fails, Safari version, timings) is recorded in `specs/SPEC-drivers.md` open questions or the PRD changelog
**Verification:**
- [ ] `gh workflow run safari-smoke.yml` completes; screenshot artifact present
**Dependencies:** Task 1
**Files likely touched:** `.github/workflows/safari-smoke.yml`, `tools/smoke/safari/main.go`
**Estimated scope:** S

## Task 3: Embed the injected scripts
**Description:** Copy `stabilize.js`, `extract.js`, `probes.js` from the spike into `assets/inject/` unchanged and expose them with `go:embed`; `mutate.js` goes to a lab-only package.
**Acceptance criteria:**
- [x] Files are byte-identical to `spikes/layout-diff/inject/` (a test compares hashes recorded in a manifest)
- [x] `mutate.js` is not reachable from `cmd/omnisnap` (import-graph test)
**Verification:**
- [x] `go test ./assets/...`
**Dependencies:** Task 1
**Files likely touched:** `assets/inject/{stabilize,extract,probes}.js`, `assets/inject/embed.go`, `assets/inject/manifest_test.go`, `internal/groundtruth/inject/mutate.js`
**Estimated scope:** S

## Task 4: Planted suite as static fixtures
**Description:** Export the spike's 31 planted pages (`planted/pages.mjs`) to static HTML plus an `expectations.json` (kind, expect ids, maxFindings, intent, onlyTags, kb).
**Acceptance criteria:**
- [x] 31 HTML files render identically to the spike server's pages (byte-equal output of `renderPage`)
- [x] `expectations.json` holds every page's answer
- [x] A Go test server serves them (`internal/groundtruth/server.go`)
**Verification:**
- [x] `node spikes/layout-diff/tools/export-planted.mjs && go test ./internal/groundtruth/...`
**Dependencies:** Task 1
**Files likely touched:** `spikes/layout-diff/tools/export-planted.mjs`, `testdata/planted/*.html`, `testdata/planted/expectations.json`, `internal/groundtruth/server.go`
**Estimated scope:** M

## Task 5: Golden fixtures from the spike
**Description:** Copy the spike's planted-suite captures (Chrome, Firefox, WebKitGTK, runs a/b, probes, screenshots) and its causes/findings into `testdata/golden/`, plus the round-5 captures of the 10 real sites, with a README stating their provenance.
**Acceptance criteria:**
- [x] Golden set for every planted page and 10 real sites; total size documented (screenshots kept only where a test needs them). Real sites are local only, git-ignored (third-party content, decision 2026-11-17)
- [x] Spike findings exported per page and pair as JSON (kinds, selectors, sides, deltas, tiers, features)
**Verification:**
- [x] `go test ./internal/groundtruth/ -run Golden` (fixture integrity check: every capture parses; `go test ./testdata/...` matches no packages, Go ignores `testdata` directories)
**Dependencies:** Task 4
**Files likely touched:** `testdata/golden/**`, `testdata/golden/README.md`, `spikes/layout-diff/tools/export-golden.mjs`
**Estimated scope:** M

### Checkpoint: Foundation
- [ ] CI green on three OSes
- [ ] Safari smoke result known (decides the WebKit plan)
- [ ] Fixtures in place; review with human

---

## Phase 1: Contracts

## Task 6: Job, progress and host schemas
**Description:** Write `job`, `progress` and `host` JSON Schemas with valid/invalid fixtures, Go types and a validation harness. Ask first: JSON Schema validator as a test-only dependency.
**Acceptance criteria:**
- [x] Schemas pass the 2020-12 meta-schema; every valid fixture validates, every invalid one fails
- [x] Go types marshal to schema-valid JSON (round-trip test)
**Verification:**
- [x] `go test ./internal/schema/...`
**Dependencies:** Task 1
**Files likely touched:** `schema/{job,progress,host}.schema.json`, `internal/schema/{job,progress,host}.go`, `internal/schema/validate_test.go`, `testdata/schema/**`
**Estimated scope:** M

## Task 7: Capture schema
**Description:** `capture.schema.json` for the geometry tree and meta, with Go types.
**Acceptance criteria:**
- [x] Every golden capture from Task 5 validates without edits
- [x] Go `Capture` round-trips a golden capture without changing any field
**Verification:**
- [x] `go test ./internal/schema/... -run Capture`
**Dependencies:** Tasks 5, 6
**Files likely touched:** `schema/capture.schema.json`, `internal/schema/capture.go`, `internal/schema/capture_test.go`
**Estimated scope:** S

## Task 8: Summary schema v1
**Description:** `summary.schema.json` (results with per-result host, causes, evidence, intent, blocked, dynamic, status) and Go types.
**Acceptance criteria:**
- [x] A hand-written fixture covering every field validates
- [x] A presence test lists the fields the viewer, merge, Action and wrappers read
**Verification:**
- [x] `go test ./internal/schema/... -run Summary`
**Dependencies:** Task 6
**Files likely touched:** `schema/summary.schema.json`, `internal/schema/summary.go`, `internal/schema/summary_test.go`, `testdata/schema/summary/*.json`
**Estimated scope:** M

### Checkpoint: Contracts
- [ ] All schemas validated and reviewed with human (they are public API)

---

## Phase 2: Walking skeleton (Chrome on Linux)

## Task 9: Driver interface and CDP core
**Description:** The `drivers.Session` interface and typed errors; a CDP client over websocket: connect, navigate (load), evaluate (await promise), set viewport. Ask first: `github.com/coder/websocket`.
**Acceptance criteria:**
- [x] Recorded CDP transcripts replay in unit tests (success, script error, timeout)
- [x] Against a real Chrome (`browsers` tag): navigate to a local page and evaluate `extract.js`
**Verification:**
- [x] `go test ./internal/drivers/...` and `go test -tags browsers ./internal/drivers/cdp/...`
**Dependencies:** Task 3
**Files likely touched:** `internal/drivers/{driver.go,errors.go}`, `internal/drivers/cdp/{conn.go,session.go,session_test.go}`
**Estimated scope:** M

## Task 10: CDP screenshots, preload and network
**Description:** Full-page screenshot with clip (`captureBeyondViewport`), preload scripts, failed requests (with blocked reason), network idle.
**Acceptance criteria:**
- [x] A 5,000 px page screenshot has exactly `docW × docH` pixels
- [x] `FailedRequests` reports a request the test server refuses
- [x] `WaitNetworkIdle` returns after the page's last fetch
**Verification:**
- [x] `go test -tags browsers ./internal/drivers/cdp/...`
**Dependencies:** Task 9
**Files likely touched:** `internal/drivers/cdp/{screenshot.go,network.go,preload.go}`, tests
**Estimated scope:** M

## Task 11: Discovery on Linux
**Description:** `Browser` type and Linux sources: PATH, `.desktop` files, Flatpak, Snap, WebKitGTK paths; versions from package metadata, never by launching.
**Acceptance criteria:**
- [x] Fixture trees reproduce each Linux pitfall (snap private /tmp note, flatpak version from metadata) (in-memory trees, `fstest.MapFS`: same exec bits and symlinks on every OS)
- [ ] On the dev machine: Chrome, Chromium, Brave, Edge, Firefox ESR, WebKitGTK found with versions
**Verification:**
- [ ] `go test ./internal/discovery/...`; `go run ./cmd/omnisnap browsers` (after Task 15)
**Dependencies:** Task 6
**Files likely touched:** `internal/discovery/{discovery.go,linux.go,version.go}`, `testdata/discovery/linux/**`
**Estimated scope:** M

## Task 12: Launch and clean up Blink on Unix
**Description:** Start a Blink browser headless with the spike's flags (incl. no disk cache) in a fresh temp profile and its own process group; `Close` kills the group and deletes the profile.
**Acceptance criteria:**
- [x] Launch flags match `SPEC-process.md` (golden list test)
- [x] 20 launch/close cycles leave 0 processes and 0 `omnisnap-*` dirs
**Verification:**
- [x] `go test ./internal/process/...` and `go test -tags browsers ./internal/process/...`
**Dependencies:** Task 11
**Files likely touched:** `internal/process/{launch.go,profile.go,pgroup_unix.go,blink.go}`
**Estimated scope:** M

## Task 13: Single-engine capture pipeline
**Description:** For one page and one engine: normalize the viewport, navigate, stabilize, extract, screenshot, re-measure, write capture + PNG atomically into the bundle layout.
**Acceptance criteria:**
- [x] Bundle layout and capture files match `SPEC-bundle-schema.md` (schema-valid)
- [x] `meta.vw/vh` = 1280×800; `meta.post` recorded; a crash mid-write leaves no partial file
**Verification:**
- [x] `go test ./internal/capture/...` (fake session) and `go test -tags browsers ./internal/capture/... -run Single`
**Dependencies:** Tasks 7, 10, 12
**Files likely touched:** `internal/capture/{capture.go,page.go,inject.go,write.go}`
**Estimated scope:** M

## Task 14: Viewer (M0): screenshots side by side
**Description:** Generate `report.html` from a summary: page list, two synced panes per pair, no findings yet; data in a JSON island, everything escaped.
**Acceptance criteria:**
- [x] Opens from `file://` with 0 console errors (headless check)
- [x] Synced scrolling keeps the same document fraction in both panes
**Verification:**
- [x] `go test ./internal/viewer/...` and `go test -tags browsers ./internal/viewer/... -run Render`
**Dependencies:** Task 8
**Files likely touched:** `internal/viewer/{viewer.go,template.html,app.js,style.css}`
**Estimated scope:** M

## Task 15: CLI: `omnisnap <url>`, `run`, `browsers`
**Description:** Wire discovery → process → capture → summary → viewer behind `omnisnap <url>` and `omnisnap run job.json`; `browsers [--json]`; exit codes; NDJSON progress with `--json`.
**Acceptance criteria:**
- [x] `omnisnap http://127.0.0.1:<port>/01-card-width.html` with Chrome writes a schema-valid bundle and report
- [x] Exit codes 0/1/2/3/4 per `SPEC-cli.md` (unit tests)
- [x] `--json` lines all validate against the progress schema
**Verification:**
- [x] `go test ./internal/cli/...`; manual: run against the planted server and open the report
**Dependencies:** Tasks 11, 13, 14
**Files likely touched:** `internal/cli/{run.go,browsers.go,progress.go,exit.go}`, `cmd/omnisnap/main.go`
**Estimated scope:** M

### Checkpoint A: Walking skeleton
- [x] `omnisnap <url>` works end to end with Chrome on Linux (verified with Chromium 157, the Blink browser on the dev machine)
- [ ] All tests pass; review with human before adding engines

---

## Phase 3: Engines

## Task 16: BiDi core
**Description:** WebDriver BiDi client: session, navigate (`wait: complete`), evaluate (`awaitPromise`), set viewport.
**Acceptance criteria:**
- [ ] Transcript tests for success, script error, timeout
- [ ] Real Firefox evaluates `extract.js` on a local page
**Verification:**
- [ ] `go test ./internal/drivers/bidi/...` and `-tags browsers`
**Dependencies:** Task 9
**Files likely touched:** `internal/drivers/bidi/{conn.go,session.go,session_test.go}`
**Estimated scope:** M

## Task 17: BiDi screenshots, preload and network
**Description:** Document-origin screenshots with clip (fallback to stitching if unsupported), `script.addPreloadScript`, network events and fetch errors.
**Acceptance criteria:**
- [ ] 5,000 px page screenshot is `docW × docH`
- [ ] Failed requests reported; preload runs before page scripts
**Verification:**
- [ ] `go test -tags browsers ./internal/drivers/bidi/...`
**Dependencies:** Task 16
**Files likely touched:** `internal/drivers/bidi/{screenshot.go,network.go,preload.go}`
**Estimated scope:** M

## Task 18: Gecko launch profile
**Description:** Firefox launch with a temp profile and the prefs from the spike (overlay scrollbars, devPixelsPerPx, no disk cache, font parity); Snap profile location.
**Acceptance criteria:**
- [ ] Prefs file matches the spec (golden test); layout viewport equals Blink's (`meta.cw` equal on a planted page)
- [ ] Snap Firefox profile placed under `~/snap/firefox/common/`
**Verification:**
- [ ] `go test ./internal/process/...` and `-tags browsers -run Gecko`
**Dependencies:** Task 12
**Files likely touched:** `internal/process/{gecko.go,prefs.go}`
**Estimated scope:** S

## Task 19: Go-native scroll-and-stitch
**Description:** Stitch viewport screenshots into a full-page PNG, hiding fixed/sticky elements after the first frame, capped at 2560×16000.
**Acceptance criteria:**
- [ ] Synthetic frames stitch into the exact expected image (unit)
- [ ] Stitching a 10,000 px page costs < 1 s CPU (benchmark)
**Verification:**
- [ ] `go test -bench Stitch ./internal/drivers/...`
**Dependencies:** Task 9
**Files likely touched:** `internal/drivers/{stitch.go,stitch_test.go}`
**Estimated scope:** S

## Task 20: WebDriver Classic client
**Description:** HTTP client: session, `/url`, `/execute/async` with the promise wrapper, `/timeouts` (script 60 s, pageLoad 30 s), window rect, screenshot via stitching; capabilities report no preload/network.
**Acceptance criteria:**
- [ ] Transcript tests incl. the 500 timeout replies seen in the spike
- [ ] `Capabilities()` reports unsupported features; callers get `ErrUnsupported`
**Verification:**
- [ ] `go test ./internal/drivers/classic/...`
**Dependencies:** Task 19
**Files likely touched:** `internal/drivers/classic/{client.go,session.go,session_test.go}`
**Estimated scope:** M

## Task 21: WebKitGTK launch with a headless display
**Description:** Start a private Weston (`--backend=headless`, unique socket), `WebKitWebDriver`, MiniBrowser with `--automation` and font args; stop all with the process.
**Acceptance criteria:**
- [ ] No window appears; `WEBKIT_HEADLESS=0` shows windows (debug)
- [ ] 20 cycles leave no Weston, driver or MiniBrowser processes
**Verification:**
- [ ] `go test -tags browsers ./internal/process/... -run WebKit`
**Dependencies:** Tasks 12, 20
**Files likely touched:** `internal/process/{display_linux.go,webkit.go}`
**Estimated scope:** M

## Task 22: Driver conformance suite
**Description:** One suite run against every available engine: identical `extract.js` output on a fixed page, exact full-page screenshot size, typed errors, viewport 1280×800, goroutine-leak check. Ask first: `go.uber.org/goleak` (test-only).
**Acceptance criteria:**
- [ ] Passes for Chrome, Firefox, WebKitGTK on Linux
- [ ] 0 goroutine leaks after `Close`
**Verification:**
- [ ] `go test -tags browsers ./internal/drivers/... -run Conformance`
**Dependencies:** Tasks 10, 17, 20, 21
**Files likely touched:** `internal/drivers/conformance_test.go`, `testdata/drivers/pages/*`
**Estimated scope:** M

## Task 23: Safari through safaridriver
**Description:** Launch `safaridriver` on macOS, detect "not enabled" as a discovery note, run the conformance suite on the macOS runner.
**Acceptance criteria:**
- [ ] Conformance passes on the macOS runner (or the gap is documented with the Task 2 result)
**Verification:**
- [ ] CI macOS job: `go test -tags browsers ./internal/drivers/... -run Conformance`
**Dependencies:** Tasks 2, 22
**Files likely touched:** `internal/process/safari_darwin.go`, `.github/workflows/ci.yml`
**Estimated scope:** S

## Task 24: Viewport normalization in every engine
**Description:** Hide the root scrollbar and gutter, and run WebKit's measure-and-correct window loop; assert the viewport in each capture.
**Acceptance criteria:**
- [ ] Chrome, Firefox, WebKitGTK all report `vw=1280`, `vh=800`, equal `cw` on the planted suite
**Verification:**
- [ ] `go test -tags browsers ./internal/capture/... -run Viewport`
**Dependencies:** Tasks 13, 21
**Files likely touched:** `internal/capture/normalize.go`, tests
**Estimated scope:** S

### Checkpoint B: Engines
- [ ] Three families capture the planted suite on Linux; Safari status known
- [ ] Conformance green; review with human

---

## Phase 4: Capture discipline

## Task 25: Runs, interleaving and lanes
**Description:** Two runs per engine, run a of all engines before run b, CDP/BiDi lane in parallel with the Classic lane.
**Acceptance criteria:**
- [ ] Fake-session test proves order (all a before any b) and lane overlap
- [ ] Planted suite wall time ≤ 1.2 × the WebKit lane alone
**Verification:**
- [ ] `go test ./internal/capture/... -run Lanes`; timing printed by `-tags browsers`
**Dependencies:** Task 24
**Files likely touched:** `internal/capture/{lanes.go,lanes_test.go,runs.go}`
**Estimated scope:** M

## Task 26: Failure policy and resume
**Description:** Live-page policy (baseline failure or bot wall → `failed.json`, skip run b; failed engine skips its run b), "done" detection, `--resume`, `--retry-failed`.
**Acceptance criteria:**
- [ ] Each policy branch has a fake-session test
- [ ] Killing a run and resuming recaptures nothing already attempted
**Verification:**
- [ ] `go test ./internal/capture/... -run 'Policy|Resume'`
**Dependencies:** Task 25
**Files likely touched:** `internal/capture/{policy.go,resume.go}`, tests
**Estimated scope:** M

## Task 27: Causal probes in captures
**Description:** Run `probes.js` (`lh`, `font`, `controls`, `all`) after the main capture and write `<engine>-<run>.probes.json.gz`; `probes: false` skips.
**Acceptance criteria:**
- [ ] K1 fixture: WebKit terminal height 692 px plain, 722 px under `lh` (spike result)
- [ ] Probes cost ≤ 1.5 s median per capture on the planted suite
**Verification:**
- [ ] `go test -tags browsers ./internal/capture/... -run Probes`
**Dependencies:** Task 13
**Files likely touched:** `internal/capture/probes.go`, tests
**Estimated scope:** S

## Task 28: Font parity check
**Description:** `omnisnap check-fonts` and the per-job pre-flight: each engine resolves generic families to the same fonts; missing-glyph warning for scripts without a font.
**Acceptance criteria:**
- [ ] Mismatch produces a warning naming the fonts; a CJK page without CJK fonts warns
**Verification:**
- [ ] `go test -tags browsers ./internal/capture/... -run Fonts`; `go run ./cmd/omnisnap check-fonts`
**Dependencies:** Task 24
**Files likely touched:** `internal/capture/fonts.go`, `internal/cli/fonts.go`, `testdata/fonts/*`
**Estimated scope:** M

## Task 29: Cleanup hardening
**Description:** Signal handling (SIGINT/TERM/HUP), `owner.lock` per temp dir, start-up sweeper for dead owners, assertion that no browser writes a disk cache.
**Acceptance criteria:**
- [ ] 20 runs killed with SIGKILL: the next start leaves 0 processes and 0 dirs within 2 s
- [ ] Planted-suite capture writes < 200 MB outside the bundle at peak, 0 after exit
**Verification:**
- [ ] `go test ./internal/process/... -run Sweep`; `go test -tags browsers ./internal/process/... -run Kill`
**Dependencies:** Tasks 12, 21
**Files likely touched:** `internal/process/{sweep.go,signals_unix.go,lock.go}`, tests
**Estimated scope:** M

## Task 30: Discovery on Windows
**Description:** Registry and App Paths sources with every pitfall: hashed Chromium key, Opera GX `opera.exe`, IEXPLORE filtered, versions from file metadata.
**Acceptance criteria:**
- [ ] Recorded registry dumps reproduce each pitfall (unit, runs on all OSes)
- [ ] Windows runner: Chrome, Edge, Firefox found with versions
**Verification:**
- [ ] `go test ./internal/discovery/...`; CI Windows `-tags browsers`
**Dependencies:** Task 11
**Files likely touched:** `internal/discovery/{windows.go,winversion.go}`, `testdata/discovery/windows/**`
**Estimated scope:** M

## Task 31: Process on Windows (Job Objects)
**Description:** Launch browsers in a Job Object with kill-on-close; temp profiles under `%TEMP%`.
**Acceptance criteria:**
- [ ] Killing `omnisnap` with TerminateProcess kills every browser (Windows runner test)
**Verification:**
- [ ] CI Windows: `go test -tags browsers ./internal/process/...`
**Dependencies:** Tasks 12, 30
**Files likely touched:** `internal/process/{job_windows.go,launch_windows.go}`
**Estimated scope:** S

## Task 32: Discovery on macOS and timing
**Description:** `mdfind` by bundle id, Info.plist parsing (XML and binary), Safari driver status; discovery timing test; `browsers --json` validated against the host schema.
**Acceptance criteria:**
- [ ] macOS runner: Chrome, Firefox, Edge, Safari found with versions
- [ ] Timing: < 300 ms on Windows and Linux, < 1 s on macOS (median of 10)
**Verification:**
- [ ] CI three OSes: `go test -tags browsers ./internal/discovery/... -run Timing`
**Dependencies:** Tasks 11, 30
**Files likely touched:** `internal/discovery/{darwin.go,plist.go,timing_test.go}`, `testdata/discovery/darwin/**`
**Estimated scope:** M

## Task 33: Golden geometry parity
**Description:** e2e test: capture the planted suite with the binary and compare with the spike's golden captures.
**Acceptance criteria:**
- [ ] Every box within 0.1 px of the golden capture, every engine
- [ ] Run a and run b identical on static pages
**Verification:**
- [ ] `go test -tags e2e ./test/e2e/... -run GoldenGeometry`
**Dependencies:** Tasks 5, 25, 27
**Files likely touched:** `test/e2e/geometry_test.go`, `test/e2e/helpers.go`
**Estimated scope:** S

### Checkpoint C: Capture
- [ ] Planted suite: 0 failed captures in three engines; golden parity; cleanup criteria met
- [ ] Discovery green on three OSes; review with human

---

## Phase 5: M0 completion

## Task 34: dom-audit checks
**Description:** The seven single-engine checks over a capture, with near-miss fixtures.
**Acceptance criteria:**
- [ ] Each check fires on its fixture and not on its near miss
- [ ] < 50 ms per 5,000-element capture
**Verification:**
- [ ] `go test ./internal/domaudit/...`
**Dependencies:** Task 7
**Files likely touched:** `internal/domaudit/{audit.go,checks.go,audit_test.go}`, `testdata/domaudit/*`
**Estimated scope:** M

## Task 35: dom-audit pages and wiring
**Description:** Seven audit pages and three controls in the planted suite; audit results in `summary.json` and the viewer (Info by default).
**Acceptance criteria:**
- [ ] Each audit page fires exactly its check in Chrome, Firefox, WebKit; controls fire nothing
**Verification:**
- [ ] `go test -tags e2e ./test/e2e/... -run Audit`
**Dependencies:** Tasks 15, 34
**Files likely touched:** `testdata/planted/audit-*.html`, `testdata/planted/expectations.json`, `internal/cli/run.go`, `internal/viewer/app.js`
**Estimated scope:** M

## Task 36: install: download and verify
**Description:** Resolve the official Firefox release (release/ESR) per OS/arch, download with progress and resume, verify SHA-512 against `SHA512SUMS` and its signature with Mozilla's pinned key.
**Acceptance criteria:**
- [ ] Tampered archive or sums rejected; nothing lands in the cache
- [ ] Disk-space check before download
**Verification:**
- [ ] `go test ./internal/install/...`
**Dependencies:** Task 15
**Files likely touched:** `internal/install/{install.go,mozilla.go,verify.go,cache.go}`, `testdata/install/*`
**Estimated scope:** M

## Task 37: install: unpack, discover, uninstall
**Description:** Unpack per OS (tar.xz, dmg, Windows archive without running an installer), discovery source `omnisnap-cache`, `uninstall`, CLI hint when no second engine family exists.
**Acceptance criteria:**
- [ ] Weekly network job on three OSes: install ESR, discover, capture a planted page, uninstall leaves nothing
- [ ] No other command contacts Mozilla (network-blocking test)
**Verification:**
- [ ] `go test -tags network ./internal/install/...`
**Dependencies:** Task 36
**Files likely touched:** `internal/install/{unpack_linux.go,unpack_darwin.go,unpack_windows.go}`, `internal/discovery/cache.go`, `internal/cli/install.go`
**Estimated scope:** M

## Task 38: Viewer M0 polish
**Description:** Summary table (status, warnings), audit findings, keyboard navigation, light/dark, XSS test page, large-fixture performance.
**Acceptance criteria:**
- [ ] XSS fixture renders as text; 0 console errors in Chrome, Firefox, Safari
- [ ] 50-page fixture interactive < 2 s; `report.html` < 2 MB
**Verification:**
- [ ] `go test -tags browsers ./internal/viewer/...`
**Dependencies:** Tasks 14, 35
**Files likely touched:** `internal/viewer/{template.html,app.js,style.css}`, `testdata/viewer/*`
**Estimated scope:** M

### Checkpoint D: M0 release (v0.1)
- [ ] `omnisnap <url>` on Linux, Windows, macOS: captures + viewer + audit
- [ ] `install firefox` verified; human review; tag v0.1

---

## Phase 6: Layout diff

## Task 39: Element matching
**Description:** Port hierarchical matching: LCS over identity tokens, then leftovers by tag + class; `onlyA`/`onlyB`.
**Acceptance criteria:**
- [ ] Pairings equal the spike's on golden pairs (count and pairs)
**Verification:**
- [ ] `go test ./internal/layoutdiff/... -run Match`
**Dependencies:** Tasks 5, 7
**Files likely touched:** `internal/layoutdiff/{match.go,match_test.go,tokens.go}`
**Estimated scope:** M

## Task 40: Deltas and tolerances
**Description:** Size, position (relative to the parent), line count, visibility, truncation, text shift, overflow flips with the spike's `TOL` constants and inline rules.
**Acceptance criteria:**
- [ ] Unit cases for each delta incl. inline height ignored and overflow `OvfDelta`
**Verification:**
- [ ] `go test ./internal/layoutdiff/... -run Deltas`
**Dependencies:** Task 39
**Files likely touched:** `internal/layoutdiff/{deltas.go,tolerances.go,deltas_test.go}`
**Estimated scope:** S

## Task 41: Run pairing and dynamic masks
**Description:** Best-matching run pairing (`|ΔdocH| + 10·|Δnodes|`) and same-engine dynamic masks.
**Acceptance criteria:**
- [ ] The synaptech slideshow golden case pairs the matching runs and masks the slideshow
**Verification:**
- [ ] `go test ./internal/layoutdiff/... -run 'Pairing|Mask'`
**Dependencies:** Task 40
**Files likely touched:** `internal/layoutdiff/{masks.go,pairing.go}`, tests
**Estimated scope:** S

## Task 42: Root causes I: box rules
**Description:** Width direction tracking, height, position, children-moved, wrap, sibling and parent explanations.
**Acceptance criteria:**
- [ ] Each rule has a minimal synthetic test; planted pages 01, 02, 05, 06, 12, 13 match the spike's causes
**Verification:**
- [ ] `go test ./internal/layoutdiff/... -run CausesBox`
**Dependencies:** Task 41
**Files likely touched:** `internal/layoutdiff/{causes.go,rules_box.go}`, tests
**Estimated scope:** M

## Task 43: Root causes II: flow, presence and content rules
**Description:** Inline-flow cascade, only-in / children-differ, hidden-in, truncated-in, text-shifted, overflow kinds.
**Acceptance criteria:**
- [ ] Planted pages 03, 04, 11, 17, 22, 23, 27 match the spike's causes
**Verification:**
- [ ] `go test ./internal/layoutdiff/... -run CausesFlow`
**Dependencies:** Task 42
**Files likely touched:** `internal/layoutdiff/rules_flow.go`, tests
**Estimated scope:** M

## Task 44: Table columns
**Description:** Replace cell causes with one `columns` cause on the table when the rule's thresholds hold.
**Acceptance criteria:**
- [ ] Planted 28 yields one `columns` cause; auto-layout Wikipedia tables (golden) unchanged
**Verification:**
- [ ] `go test ./internal/layoutdiff/... -run Tables`
**Dependencies:** Task 42
**Files likely touched:** `internal/layoutdiff/tables.go`, tests
**Estimated scope:** S

## Task 45: Diff warnings
**Description:** Viewport differs, node cap, page changed during capture, different page states vs changes over time, web fonts failed.
**Acceptance criteria:**
- [ ] Stripe and python.org golden cases produce the spike's warnings
**Verification:**
- [ ] `go test ./internal/layoutdiff/... -run Warnings`
**Dependencies:** Task 41
**Files likely touched:** `internal/layoutdiff/warnings.go`, tests
**Estimated scope:** S

## Task 46: Design intent detector
**Description:** Relations (edges, equal sizes, gaps, repeated components, text columns, no overlap), "kept by ≥ 2 families", violations, blame by displacement, match-rate skip.
**Acceptance criteria:**
- [ ] Planted intent pages 16–21: violation on the planted element; controls 0
**Verification:**
- [ ] `go test ./internal/layoutdiff/... -run Intent`
**Dependencies:** Task 41
**Files likely touched:** `internal/layoutdiff/{intent.go,relations.go,blame.go}`, tests
**Estimated scope:** M

## Task 47: Golden parity for causes
**Description:** Harness comparing Go causes with spike causes on every golden page pair; a reviewed diff report when they differ.
**Acceptance criteria:**
- [ ] Parity on all planted pages and the 10 real sites (kinds, selectors, sides, deltas ≤ 0.1 px)
**Verification:**
- [ ] `go test ./internal/layoutdiff/... -run Golden`
**Dependencies:** Tasks 43, 44, 45
**Files likely touched:** `internal/layoutdiff/golden_test.go`, `testdata/golden/**/causes.json`
**Estimated scope:** S

## Task 48: Intent joins causes; performance
**Description:** Join violations to causes (same element, child, or parent with ≥ 2 violating children) or create `intent-*` causes; benchmark a 10,000-element pair.
**Acceptance criteria:**
- [ ] Planted 05/06/19/25 budgets respected (no duplicate causes)
- [ ] 10k-element pair ≤ 2 s, ≤ 200 MB
**Verification:**
- [ ] `go test -bench . ./internal/layoutdiff/...`
**Dependencies:** Tasks 46, 47
**Files likely touched:** `internal/layoutdiff/{result.go,intent_join.go}`, `internal/layoutdiff/bench_test.go`
**Estimated scope:** M

### Checkpoint E: Layout diff
- [ ] Golden parity green; intent 6/6 planted, 0 on controls; performance met; review with human

---

## Phase 7: Appearance

## Task 49: Crops, ink and noise floor
**Description:** Registered crops, histogram-mode background, ink masks with 1 px dilation, agreement, per-page noise floor.
**Acceptance criteria:**
- [ ] Ink agreement within 0.01 of the spike on golden pairs; bordered-box background case correct
**Verification:**
- [ ] `go test ./internal/appearance/... -run Ink`
**Dependencies:** Task 48
**Files likely touched:** `internal/appearance/{crop.go,ink.go,floor.go}`, tests
**Estimated scope:** M

## Task 50: Graphics SSIM and the state alarm
**Description:** SSIM with ±3 px search on matched graphics elements with equal geometry → appearance-only causes; page DSSIM only as a state warning.
**Acceptance criteria:**
- [ ] 0 appearance-only causes on all golden controls
**Verification:**
- [ ] `go test ./internal/appearance/... -run SSIM`
**Dependencies:** Task 49
**Files likely touched:** `internal/appearance/{ssim.go,graphics.go}`, tests
**Estimated scope:** M

## Task 51: Planted appearance pages
**Description:** Image swapped, SVG fill, canvas, CSS filter (Firefox-only), plus two controls; e2e test.
**Acceptance criteria:**
- [ ] Each planted appearance bug reported at the right element; controls 0
**Verification:**
- [ ] `go test -tags e2e ./test/e2e/... -run Appearance`
**Dependencies:** Task 50
**Files likely touched:** `testdata/planted/appearance-*.html`, `testdata/planted/expectations.json`, `test/e2e/appearance_test.go`
**Estimated scope:** S

### Checkpoint: Appearance
- [ ] Ink parity with the spike; planted appearance bugs found; 0 appearance-only causes on controls

---

## Phase 8: Triage and the M1 pipeline

## Task 52: Knowledge base
**Description:** KB entries as data (K1, K2, K4 with its box-change bound and table-cell exemption, K5, K8) and one matcher.
**Acceptance criteria:**
- [ ] KB fixtures 14, 15: every difference explained; K4 does not explain planted 01 or a shrunken paragraph
**Verification:**
- [ ] `go test ./internal/triage/... -run KB`
**Dependencies:** Task 47
**Files likely touched:** `internal/triage/kb.go`, `assets/kb/{K1,K2,K4,K5,K8}.yaml`, tests
**Estimated scope:** M

## Task 53: Consequences and tiers
**Description:** All consequences, the hard set, overflow modes (clipped / scrollbar / visible, viewport-sized hiding exception), rule-based tier.
**Acceptance criteria:**
- [ ] Unit test per consequence; planted 11 is `clipped`; Mozilla image-replacement case is not
**Verification:**
- [ ] `go test ./internal/triage/... -run 'Consequence|Tier'`
**Dependencies:** Task 52
**Files likely touched:** `internal/triage/{consequences.go,tier.go}`, tests
**Estimated scope:** M

## Task 54: Evidence: mechanism
**Description:** Probe contexts and survival per cause (children-aware), verdicts `lh/font/controls/all/layout/author-font`.
**Acceptance criteria:**
- [ ] K1 terminal → `lh`; planted 07 → `author-font`; planted 12 → `layout`
**Verification:**
- [ ] `go test ./internal/triage/... -run Mechanism`
**Dependencies:** Tasks 27, 53
**Files likely touched:** `internal/triage/evidence/mechanism.go`, tests
**Estimated scope:** M

## Task 55: Evidence: perception and calibration
**Description:** Relation events and Weber sizes; page self-calibration (advance ratio, pitch, controls; median/MAD).
**Acceptance criteria:**
- [ ] Parity with the spike's evidence on golden pairs
**Verification:**
- [ ] `go test ./internal/triage/... -run 'Perception|Calibration'`
**Dependencies:** Task 53
**Files likely touched:** `internal/triage/evidence/{perception.go,calibration.go}`, tests
**Estimated scope:** M

## Task 56: Evidence: consensus and novelty
**Description:** Third-family consensus verdicts; fingerprint and novelty from a shipped frequency table.
**Acceptance criteria:**
- [ ] Parity with the spike on golden pairs; missing table → novelty absent, not an error
**Verification:**
- [ ] `go test ./internal/triage/... -run 'Consensus|Novelty'`
**Dependencies:** Task 53
**Files likely touched:** `internal/triage/evidence/{consensus.go,novelty.go}`, `assets/model/fingerprints.json`
**Estimated scope:** S

## Task 57: Grouping and ranking
**Description:** Merge by KB id or structural signature, headline instance, impact, ranking; causes written into the summary shape.
**Acceptance criteria:**
- [ ] HN golden: 32 findings → the spike's grouped causes
**Verification:**
- [ ] `go test ./internal/triage/... -run Group`
**Dependencies:** Tasks 54, 55, 56
**Files likely touched:** `internal/triage/{group.go,rank.go,summary.go}`, tests
**Estimated scope:** M

## Task 58: Diff in the pipeline
**Description:** After capture, run layout-diff, appearance and triage per page pair and write causes into `summary.json`; progress `diff` events.
**Acceptance criteria:**
- [ ] `omnisnap run` on the planted job writes schema-valid causes for every pair
**Verification:**
- [ ] `go test -tags e2e ./test/e2e/... -run Pipeline`
**Dependencies:** Tasks 15, 51, 57
**Files likely touched:** `internal/cli/run.go`, `internal/pipeline/{diff.go,pipeline.go}`
**Estimated scope:** M

## Task 59: Planted gates in CI
**Description:** The ground-truth gates as an e2e test on every PR touching capture, layout-diff or triage.
**Acceptance criteria:**
- [ ] Recall ≥ 90% (Wilson reported), budgets, 0 Report on controls, KB 0/0, intent ≥ 5/6 all asserted
**Verification:**
- [ ] `go test -tags e2e ./test/e2e/... -run Planted`; CI job green
**Dependencies:** Task 58
**Files likely touched:** `test/e2e/planted_test.go`, `internal/groundtruth/gates.go`, `.github/workflows/ci.yml`
**Estimated scope:** M

## Task 60: Viewer M1: findings
**Description:** Report/Info lists, boxes on both panes, click-to-scroll, consequence and evidence chips, KB explanation and fix, intent list, dynamic shading, warnings.
**Acceptance criteria:**
- [ ] Every cause listed and outlined; clicking scrolls both panes; 0 console errors on the planted bundle
**Verification:**
- [ ] `go test -tags browsers ./internal/viewer/... -run Findings`
**Dependencies:** Tasks 38, 58
**Files likely touched:** `internal/viewer/{app.js,template.html,style.css}`, `testdata/viewer/planted-summary.json`
**Estimated scope:** M

## Task 61: `omnisnap report`
**Description:** Re-render `report.html` from an existing bundle.
**Acceptance criteria:**
- [ ] Re-rendering a bundle reproduces the same report bytes
**Verification:**
- [ ] `go test ./internal/cli/... -run Report`
**Dependencies:** Task 60
**Files likely touched:** `internal/cli/report.go`, test
**Estimated scope:** XS

## Task 62: Features
**Description:** The 82-feature vector exactly as `lib/features.mjs` (names, order, encoding).
**Acceptance criteria:**
- [ ] Exact equality with the spike's features on golden findings; name list equals the model metadata
**Verification:**
- [ ] `go test ./internal/triage/... -run Features`
**Dependencies:** Task 57
**Files likely touched:** `internal/triage/{features.go,features_test.go}`
**Estimated scope:** S

## Task 63: Model score
**Description:** Load `gbdt.txt` with `leaves` (ask first), score causes, write `score` to the summary; ranking uses it after tier.
**Acceptance criteria:**
- [ ] Parity ≤ 1e-9 with LightGBM on `parity.csv`; a model whose feature names differ is refused
**Verification:**
- [ ] `go test ./internal/triage/... -run ModelParity`
**Dependencies:** Task 62
**Files likely touched:** `internal/triage/model.go`, `assets/model/{gbdt.txt,gbdt-meta.json}`, test
**Estimated scope:** S

## Task 64: Per-pair time budget
**Description:** Benchmark diff + appearance + triage on golden pairs; fail CI if the budget regresses.
**Acceptance criteria:**
- [ ] ≤ 300 ms triage per pair; ≤ 2.5 s diff+triage on the largest golden pair
**Verification:**
- [ ] `go test -bench . ./internal/pipeline/...`
**Dependencies:** Tasks 58, 63
**Files likely touched:** `internal/pipeline/bench_test.go`
**Estimated scope:** XS

### Checkpoint F: M1 core
- [ ] Planted gates green in CI; viewer shows findings; review with human

---

## Phase 9: Branded browsers

## Task 65: Warmed templates
**Description:** `omnisnap warm <browser>`, template cache, freshness (major version, 7 days), component and filter-list versions in host/summary; profile copies per capture.
**Acceptance criteria:**
- [ ] Brave warmed blocks on the local tracker page; fresh does not
**Verification:**
- [ ] `go test -tags browsers ./internal/branded/... -run Warm`
**Dependencies:** Tasks 29, 58
**Files likely touched:** `internal/branded/{warm.go,templates.go}`, `internal/cli/warm.go`, `testdata/branded/*`
**Estimated scope:** M

## Task 66: Blocked-request attribution
**Description:** Blocked hosts per result, warning, `blockedBy` on causes whose resources came from a blocked host.
**Acceptance criteria:**
- [ ] Collapsing ad-slot test page: cause carries `blockedBy` with the host
**Verification:**
- [ ] `go test ./internal/branded/... -run Blocked`
**Dependencies:** Task 65
**Files likely touched:** `internal/branded/blocked.go`, test, `schema/summary.schema.json` (additive)
**Estimated scope:** S

## Task 67: `--neutral`
**Description:** Fresh profile with blocking disabled where possible; result records `profile: neutral`.
**Acceptance criteria:**
- [ ] Brave `--neutral` blocks 0 requests on the tracker page
**Verification:**
- [ ] `go test -tags browsers ./internal/branded/... -run Neutral`
**Dependencies:** Task 65
**Files likely touched:** `internal/branded/neutral.go`, `internal/cli/run.go`
**Estimated scope:** S

## Task 68: Edge tracking prevention investigation
**Description:** On a Windows runner, headed and headless, warmed: does Edge block under automation? Record the result and decide Edge's support level.
**Acceptance criteria:**
- [ ] Result documented in `specs/SPEC-branded.md` and the PRD changelog
**Verification:**
- [ ] Workflow run artifact with blocked counts
**Dependencies:** Task 65
**Files likely touched:** `.github/workflows/edge-investigation.yml`, `specs/SPEC-branded.md`
**Estimated scope:** S

### Checkpoint G: Branded
- [ ] Chromium, Brave, Edge: 0 causes on the planted suite; Brave attribution works; Edge decision made

---

## Phase 10: Ground truth and the model

## Task 69: Lab binary
**Description:** `cmd/omnisnap-lab` skeleton sharing the planted server and capture pipeline; not part of releases.
**Acceptance criteria:**
- [ ] `omnisnap-lab version`; release tooling excludes it (checked in Task 82)
**Verification:**
- [ ] `go build ./cmd/omnisnap-lab`
**Dependencies:** Task 58
**Files likely touched:** `cmd/omnisnap-lab/main.go`, `internal/groundtruth/lab.go`
**Estimated scope:** XS

## Task 70: Mutation capture
**Description:** Embed `mutate.js` (lab only), several mutants per page load (inject, measure, revert), seeds and operator rotation, same-state page filter.
**Acceptance criteria:**
- [ ] 16 mutants on two pages, all effective
**Verification:**
- [ ] `go test -tags browsers ./internal/groundtruth/... -run Mutate`
**Dependencies:** Task 69
**Files likely touched:** `internal/groundtruth/{mutate.go,inject/embed.go}`, `cmd/omnisnap-lab/mutate.go`
**Estimated scope:** M

## Task 71: Mutant labels
**Description:** Same-engine diff, changed set (dynamic-masked), neighbourhood, pre-existing causes excluded, class-wide and hidden targets.
**Acceptance criteria:**
- [ ] Synthetic pairs cover each rule; smoke: ≥ 80% of mutants get a labeled positive
**Verification:**
- [ ] `go test ./internal/groundtruth/... -run Label`
**Dependencies:** Tasks 58, 70
**Files likely touched:** `internal/groundtruth/{label.go,label_test.go}`
**Estimated scope:** M

## Task 72: Crawl list
**Description:** Tranco-based site list with the content, infrastructure, brand and reachability filters; versioned output with rejection reasons.
**Acceptance criteria:**
- [ ] Filter unit tests (adult terms multilingual, gambling, infrastructure, brand dedupe); a list of 500 generated
**Verification:**
- [ ] `go test ./internal/groundtruth/... -run Sites`; `go run ./cmd/omnisnap-lab sites --n 500`
**Dependencies:** Task 69
**Files likely touched:** `internal/groundtruth/sites.go`, `cmd/omnisnap-lab/sites.go`, `testdata/crawl-sites.json`
**Estimated scope:** M

## Task 73: Lab crawl with guards
**Description:** Crawl with `--limit`, `--resume`, `--shards`, unreachable marking, and disk-space guards (warn < 20 GB, stop < 5 GB on the host drive, WSL-aware).
**Acceptance criteria:**
- [ ] Kill and resume recaptures nothing done; the guard stops a run on a nearly full drive (simulated)
**Verification:**
- [ ] `go test ./internal/groundtruth/... -run 'Crawl|Disk'`
**Dependencies:** Tasks 26, 72
**Files likely touched:** `internal/groundtruth/{crawl.go,disk.go}`, `cmd/omnisnap-lab/crawl.go`
**Estimated scope:** M

## Task 74: Dataset export
**Description:** Training table (positives from mutants, negatives from clean pages, planted separate), dataset metadata (clean page pairs).
**Acceptance criteria:**
- [ ] Rows equal the spike's `dataset.mjs` on the same findings
**Verification:**
- [ ] `go test ./internal/groundtruth/... -run Dataset`
**Dependencies:** Tasks 62, 71
**Files likely touched:** `internal/groundtruth/dataset.go`, `cmd/omnisnap-lab/dataset.go`
**Estimated scope:** S

## Task 75: Trainer
**Description:** Move `train.py` and the parity export to `tools/`, pin requirements, fixed seeds, outputs into `assets/model/`.
**Acceptance criteria:**
- [ ] Same dataset → identical model bytes; Go parity test passes on its `parity.csv`
**Verification:**
- [ ] `python tools/train.py --data <dir> --out assets/model && go test ./internal/triage/... -run ModelParity`
**Dependencies:** Tasks 63, 74
**Files likely touched:** `tools/{train.py,requirements.txt,README.md}`
**Estimated scope:** S

## Task 76: Model decision
**Description:** Run the crawl (≥ 100 sites, ≥ 300 positives), train, evaluate the gate (fewer false alarms at the rules' recall, planted ≥ 90%, controls 0); decide the model's role; record it in the PRD changelog.
**Acceptance criteria:**
- [ ] Decision recorded with the numbers; `assets/model/` updated or the score hidden
**Verification:**
- [ ] Gate report from `tools/train.py`; human approval
**Dependencies:** Tasks 73, 75
**Files likely touched:** `assets/model/*`, `docs/prd.md` (changelog), `specs/SPEC-triage.md`
**Estimated scope:** S

### Checkpoint H: M1 release (v0.2)
- [ ] Layout diff, appearance, triage, branded in the binary; planted gates green; model role decided; tag v0.2

---

## Phase 11: CI report and distribution (v1)

## Task 77: Merge
**Description:** `omnisnap merge`: concatenate results with hosts, copy or hard-link files, rewrite paths, detect conflicts, deterministic output.
**Acceptance criteria:**
- [ ] Order-independent, byte-identical output; each conflict case errors with bundle and field
**Verification:**
- [ ] `go test ./internal/merge/...`
**Dependencies:** Task 58
**Files likely touched:** `internal/merge/{merge.go,paths.go,conflicts.go}`, `internal/cli/merge.go`, `testdata/merge/**`
**Estimated scope:** M

## Task 78: Merged report
**Description:** Host selector and per-host grouping in the viewer; golden merge of three fixture bundles.
**Acceptance criteria:**
- [ ] Merged fixture renders with 0 console errors; Safari results appear next to Linux and Windows ones
**Verification:**
- [ ] `go test -tags browsers ./internal/viewer/... -run Merged`
**Dependencies:** Tasks 60, 77
**Files likely touched:** `internal/viewer/{app.js,template.html}`, `testdata/viewer/merged-summary.json`
**Estimated scope:** S

## Task 79: Action: OS jobs
**Description:** Composite action and reusable workflow: per-OS engine preparation (fonts and WebKitGTK on Linux, `safaridriver --enable` on macOS), app start + wait, `omnisnap run --json`, artifacts, always exit 0; macOS policy input (every PR by default, or only on PRs labeled `omnisnap:safari`).
**Acceptance criteria:**
- [ ] Sample repo: three OS jobs succeed and upload bundles; an injected capture failure still concludes success
**Verification:**
- [ ] Workflow run on the sample repo
**Dependencies:** Tasks 77, 82
**Files likely touched:** `action/action.yml`, `.github/workflows/report.yml`, `action/README.md`
**Estimated scope:** M

## Task 80: Action: merge job and PR comment
**Description:** Merge job; `omnisnap comment` renders the PR comment (top causes per page, links, notes, length cap); upsert by marker; fork PRs via `workflow_run`.
**Acceptance criteria:**
- [ ] One comment, updated on the next push; fork PR gets the comment without secrets
**Verification:**
- [ ] `go test ./internal/cli/... -run Comment`; workflow runs on the sample repo (same-repo and fork PRs)
**Dependencies:** Task 79
**Files likely touched:** `internal/cli/comment.go`, `.github/workflows/report.yml`, `.github/workflows/report-comment.yml`
**Estimated scope:** M

## Task 81: Action self-test
**Description:** A workflow running the Action on the planted suite on three runners, asserting the comment content and fault tolerance; runs on every release candidate.
**Acceptance criteria:**
- [ ] Planted findings appear in the comment; five pages ≤ 10 minutes wall time
**Verification:**
- [ ] `gh workflow run e2e-action.yml`
**Dependencies:** Task 80
**Files likely touched:** `.github/workflows/e2e-action.yml`, `test/action/*`
**Estimated scope:** S

## Task 82: Release tooling
**Description:** Cross-compile the five targets, checksums, GitHub release with notes; exclude `omnisnap-lab`.
**Acceptance criteria:**
- [ ] A dry-run release produces five binaries + SHA256SUMS; `omnisnap version` correct in each
**Verification:**
- [ ] `go run ./tools/release --dry-run`
**Dependencies:** Task 76
**Files likely touched:** `tools/release/main.go`, `.github/workflows/release.yml`
**Estimated scope:** M

## Task 83: npm wrapper
**Description:** `omnisnap` package (API, bin shim, generated TS types, drift check) and five platform packages with the binary; no postinstall.
**Acceptance criteria:**
- [ ] Fresh project on three OSes: `npm install` + `run()` returns a valid summary for the planted job; packages < 15 MB
**Verification:**
- [ ] `cd packages/npm && npm test`; CI install test
**Dependencies:** Task 82
**Files likely touched:** `packages/npm/omnisnap/{index.js,index.d.ts,package.json}`, `packages/npm/platforms/*/package.json`, `packages/types/generate.mjs`
**Estimated scope:** M

## Task 84: PyPI wrapper
**Description:** Platform wheels with the binary, `run()`/`browsers()`/`merge()`/`report()`, generated TypedDicts, console entry point.
**Acceptance criteria:**
- [ ] Fresh venv on three OSes: `pip install` + `run()` returns a valid summary; wheels < 15 MB
**Verification:**
- [ ] `cd packages/pypi && python -m pytest && python -m build`; CI install test
**Dependencies:** Task 82
**Files likely touched:** `packages/pypi/omnisnap/{__init__.py,_run.py,types.py}`, `packages/pypi/pyproject.toml`, `packages/types/generate_py.py`
**Estimated scope:** M

## Task 85: Claim names and domain
**Description:** Register `omnisnap.org` (done 2026-11-17), claim `omnisnap` on npm (and the `@omnisnap` scope), PyPI and GitHub with placeholder releases; point schema URLs at the domain. Human task.
**Acceptance criteria:**
- [ ] All four claimed; schema `$id`s resolve
**Verification:**
- [ ] `npm view omnisnap`, `pip index versions omnisnap`, schema URL fetch
**Dependencies:** None (do early; blocks Tasks 83, 84 publishing)
**Files likely touched:** `schema/*.schema.json` (`$id`), `docs/ideas/omnisnap.md`
**Estimated scope:** XS

## Task 86: v1 documentation
**Description:** Root README with the one-command demo, install paths (binary, npm, PyPI), Action usage, report-only policy, privacy (nothing leaves the machine).
**Acceptance criteria:**
- [ ] A new user follows the README from zero to a report on each OS
**Verification:**
- [ ] Manual walkthrough on three OSes
**Dependencies:** Tasks 81, 83, 84
**Files likely touched:** `README.md`, `action/README.md`, `docs/quickstart.md`
**Estimated scope:** S

### Checkpoint I: v1 (M2 released)
- [ ] Action green on the sample repo across three OSes; wrappers published; README walkthrough done; human sign-off
