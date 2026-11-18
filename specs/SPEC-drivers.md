# Spec: drivers

Module id: `drivers` · Packages: `internal/drivers` (interface), `internal/drivers/{cdp,bidi,classic}` · Depends on: `process` · Milestone: M0 · Conventions: [CONVENTIONS.md](CONVENTIONS.md)

## Objective

Speak each engine family's automation protocol directly, with no intermediate proxy (PRD Feature 2), behind one small interface that `capture` uses identically for every engine:

- **CDP** for Blink (Chrome, Chromium, Edge, Brave, Opera, Vivaldi);
- **WebDriver BiDi** for Gecko (Firefox; its CDP is disabled since v129);
- **WebDriver Classic** for WebKit (Safari via `safaridriver`, WebKitGTK via `WebKitWebDriver`).

## Tech stack

`github.com/coder/websocket` for CDP and BiDi; `net/http` for Classic; `image/png` for decoding and stitching; no protocol code generation (hand-written subset).

## Interface

```go
type Session interface {
	Navigate(ctx context.Context, url string) error                  // waits for "load"
	WaitNetworkIdle(ctx context.Context, idle, limit time.Duration) error // ErrUnsupported on Classic
	Evaluate(ctx context.Context, expr string) (string, error)        // expr may return a Promise; result is a string (scripts return JSON strings)
	AddPreloadScript(ctx context.Context, src string) error           // ErrUnsupported on Classic
	SetViewport(ctx context.Context, w, h int, dpr float64) (Viewport, error) // returns the size actually obtained
	Screenshot(ctx context.Context, clip Rect) ([]byte, error)        // full-page PNG of clip, document coordinates
	FailedRequests() []FailedRequest                                  // url, error text (e.g. ERR_BLOCKED_BY_CLIENT)
	Capabilities() Capabilities                                       // preload, networkIdle, fullPageScreenshot, ...
	Close(ctx context.Context) error
}
```

Typed errors: `ErrTimeout`, `ErrNavigation`, `ErrScript` (with the page's message), `ErrCrashed`, `ErrUnsupported`.

### Per protocol

| Need | CDP | BiDi | Classic |
|---|---|---|---|
| Navigate + load | `Page.navigate` + `Page.loadEventFired` | `browsingContext.navigate` `wait: complete` | `POST /url` (pageLoad timeout 30 s) |
| Evaluate (await promise) | `Runtime.evaluate` `awaitPromise` | `script.evaluate` `awaitPromise` | `POST /execute/async` with a `Promise.resolve(expr).then(done)` wrapper (works whether or not the driver awaits sync results) |
| Preload script | `Page.addScriptToEvaluateOnNewDocument` | `script.addPreloadScript` | unsupported (so no network-quiet signal: PRD 1.1.0) |
| Network idle / failures | `Network.*` events, `loadingFailed.blockedReason` | `network.beforeRequestSent` / `network.fetchError` | unsupported; fixed settle waits in `capture` |
| Child targets (cross-site iframes, workers) | `Target.setAutoAttach` (flatten, `waitForDebuggerOnStart`), then `Network.enable` and `Runtime.runIfWaitingForDebugger` per child; one ordered event stream for the page and its children (added 2026-11-17: without it a cross-site iframe keeps the page from ever going idle) | network events cover every browsing context | unsupported |
| Viewport | `Emulation.setDeviceMetricsOverride` | `browsingContext.setViewport` | window rect, measure-and-correct loop (window chrome varies, resize is async) |
| Full-page screenshot | `Page.captureScreenshot` `captureBeyondViewport` + clip | `browsingContext.captureScreenshot` `origin: document` + clip | scroll-and-stitch viewport shots; fixed/sticky elements hidden after the first frame |
| Timeouts | per call | per call | `POST /timeouts` script 60 s, pageLoad 30 s |

Stitching happens in Go (fast PNG decode/encode), unlike the spike's pure-JS PNG library, which made WebKit captures about 2.5× slower than the other engines.

## Commands

```
go test ./internal/drivers/...                       # unit: message encoding, stitching, error mapping (recorded transcripts)
go test -tags browsers ./internal/drivers/...        # conformance suite against real engines + local test server
```

## Project structure

`internal/drivers/{driver.go, errors.go, stitch.go}`, `internal/drivers/cdp/`, `internal/drivers/bidi/`, `internal/drivers/classic/`; `testdata/drivers/transcripts/` recorded protocol exchanges; `testdata/drivers/pages/` conformance pages.

## Code style

Per conventions. One goroutine owns each websocket's reads; requests are matched by id; every call has a context deadline. No retries inside drivers (policy lives in `capture`).

## Testing strategy

- **Unit:** replay recorded transcripts (CDP, BiDi, Classic) for each method, including error replies and timeouts.
- **Conformance (`browsers`):** the same suite runs against every available engine with a local test server:
  - `Evaluate` returns identical JSON for the same expression (an `extract.js` run on a fixed page gives the same tree in all three families);
  - a full-page screenshot of a 5,000 px page has exactly `docW × docH` pixels and matches a reference within 1% pixel difference per engine;
  - failed requests include a blocked request in Brave with a warmed profile (`ERR_BLOCKED_BY_CLIENT`);
  - viewport is exactly 1280×800 CSS px after `SetViewport` in every engine (WebKit via the correction loop inside a private display; WPE via `--size` at launch, since its driver reports `setWindowRect: false`);
  - Classic engines report `NavigationErrors: false` (the driver accepts the URL and renders its own error page) and CDP/BiDi `true`; the conformance suite branches on it;
  - each typed error surfaces from a page built to trigger it.

## Boundaries

- **Always:** report capabilities honestly (`capture` adapts to them); surface the page's own error text in `ErrScript`.
- **Ask first:** using a protocol feature outside the table (each is maintenance across three protocols).
- **Never:** route through a proxy driver (chromedriver, geckodriver) where the protocol is reachable directly; swallow errors.

## Success criteria

1. The conformance suite passes on: Chrome, Edge, Firefox (all OSes), WebKitGTK (Linux), Safari (macOS runner).
2. WebKit full-page capture of a 10,000 px page completes in ≤ 2× the Chrome time on the same machine (spike: about 2.5× slower with JS stitching).
3. Zero goroutine leaks after `Close` (checked with `goleak` in tests; ask first: test-only dependency).
4. The planted suite captured through these drivers yields the same geometry as the spike's captures (golden comparison, ≤ 0.1 px per box).

## Open questions

- BiDi screenshot `origin: document` support in the Firefox ESR in use: confirm; fallback is scroll-and-stitch as for Classic.
- Should Blink use BiDi too (via its CDP-backed BiDi), to reduce protocols to two? Proposed: no; CDP is richer and stable.
