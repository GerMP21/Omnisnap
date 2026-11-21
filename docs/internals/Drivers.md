# Drivers

**Role.** One interface that capture uses for every engine, over three protocols spoken directly, with no proxy driver (no chromedriver, no geckodriver). Module `drivers`, package [`internal/drivers`](https://github.com/GerMP21/Omnisnap/tree/main/internal/drivers).

## The interface

[`drivers.Session`](https://github.com/GerMP21/Omnisnap/blob/main/internal/drivers/driver.go) is one page (tab) in a running browser:

| Method | Does |
|---|---|
| `Navigate` | loads a URL and waits for the load event |
| `WaitNetworkIdle` | returns once no request has been in flight for `idle`, or `ErrTimeout` after `limit` |
| `Evaluate` | runs an expression in the page, awaiting a Promise; the injected scripts return JSON strings |
| `AddPreloadScript` | runs a script before any page script on every navigation |
| `SetViewport` | sets the CSS viewport and returns the size actually obtained |
| `Screenshot` | a PNG of a clip in document coordinates |
| `FailedRequests` | the requests that failed since the last navigation, with the engine's error text (`net::ERR_BLOCKED_BY_CLIENT`…) |
| `Capabilities` | which optional features work; capture adapts to them |
| `Close` | closes the page |

Typed errors ([`errors.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/drivers/errors.go)): `ErrTimeout`, `ErrNavigation`, `ErrScript` (carrying the page's own message), `ErrCrashed`, `ErrUnsupported`. Capture's failure policy is written against these: a timeout or navigation error on the baseline makes the page unreachable, and `ErrCrashed` triggers the retry in a fresh browser.

[`Capabilities`](https://github.com/GerMP21/Omnisnap/blob/main/internal/drivers/driver.go) are reported honestly per session:

| | CDP (Blink) | BiDi (Gecko) | Classic (WebKit) |
|---|---|---|---|
| `Preload` | yes | yes | no |
| `NetworkIdle` (and failed requests) | yes | yes, once network events are subscribed | no |
| `FullPageScreenshot` | yes | yes, unless the build lacks `origin: document` | no: scroll and stitch |
| `NavigationErrors` | yes | yes | no: WebKitGTK shows its own error page and the command succeeds |

## The three protocols

| Package | Engine | How |
|---|---|---|
| [`cdp`](https://github.com/GerMP21/Omnisnap/tree/main/internal/drivers/cdp) | Blink: Chrome, Chromium, Edge, Brave, Opera, Vivaldi | Chrome DevTools Protocol over WebSocket. Auto-attaches child targets (`Target.setAutoAttach`, flattened), so a cross-site iframe or worker is in the same ordered event stream and network idle covers it; without that a cross-site iframe kept the page from ever going idle. Full-page screenshots with `captureBeyondViewport` ([`screenshot.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/drivers/cdp/screenshot.go)). |
| [`bidi`](https://github.com/GerMP21/Omnisnap/tree/main/internal/drivers/bidi) | Gecko: Firefox, Firefox ESR | WebDriver BiDi, which Firefox serves on `--remote-debugging-port` (its CDP is gone since Firefox 129). Preload with `script.addPreloadScript`; network events only when subscribed; screenshots with `origin: document`. Firefox ESR builds that answer "unsupported operation" fall back to scroll-and-stitch ([`screenshot.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/drivers/bidi/screenshot.go)). |
| [`classic`](https://github.com/GerMP21/Omnisnap/tree/main/internal/drivers/classic) | WebKit: WebKitGTK, WPE, Safari | WebDriver Classic: plain HTTP, no driver library. No network events and no preload, so capture uses fixed settle waits. The viewport is set through the window rect with a measure-and-correct loop, because window chrome varies and resizing is asynchronous ([`session.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/drivers/classic/session.go)). Screenshots are viewport-only and stitched in Go. |

Two shared pieces:

- [`wsrpc`](https://github.com/GerMP21/Omnisnap/tree/main/internal/drivers/wsrpc): the WebSocket JSON-RPC transport under CDP and BiDi. One goroutine owns the reads; calls are matched to replies by id; events go to subscribers by route and method. Each protocol plugs in a codec for its message format.
- [`stitch.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/drivers/stitch.go): [`ScrollAndStitch`](https://github.com/GerMP21/Omnisnap/blob/main/internal/drivers/stitch.go) scrolls the page a viewport at a time, takes a shot of each, and stitches them in Go (fixed and sticky elements are hidden after the first frame so they appear once). The spike did this in JavaScript and it made WebKit captures about 2.5× slower.

## Failure handling

A closed connection (the browser died) turns every pending and later call into `ErrCrashed` ([`wsrpc/conn.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/drivers/wsrpc/conn.go)), and `Close` into a no-op; CDP also watches `Inspector.targetCrashed` for a crashed page. A call past its context deadline is `ErrTimeout`.

## Tests

- **Unit tests on recorded transcripts.** [`replaytest`](https://github.com/GerMP21/Omnisnap/tree/main/internal/drivers/replaytest) replays JSONL transcripts from [`testdata/drivers/transcripts`](https://github.com/GerMP21/Omnisnap/tree/main/testdata/drivers/transcripts) as a fake WebSocket endpoint for the CDP and BiDi drivers; Classic has its own replay test ([`replay_test.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/drivers/classic/replay_test.go)). No browser needed.
- **The conformance suite** ([`TestConformance`](https://github.com/GerMP21/Omnisnap/blob/main/internal/drivers/conformance_test.go), `browsers` tag): the same scenarios against every engine family installed: the viewport is exactly 1280×800, `extract.js` gives the same geometry tree on a layout-neutral page, the scripts survive a page that overrides DOM methods, the full-page screenshot is `docW × docH`, typed errors, and honest capabilities. Harnesses are named by family and protocol (`blink (cdp)`, `gecko (bidi)`, and the WebKit ones per OS). CI sets `OMNISNAP_EXPECT_ENGINES` per runner so a missing engine fails instead of skipping.

## Where the code differs from the spec

Nothing found: the interface, the typed errors and the per-protocol table in the spec match the code.

## Spec

[`specs/SPEC-drivers.md`](https://github.com/GerMP21/Omnisnap/blob/main/specs/SPEC-drivers.md).

## Source files

- [`internal/drivers/driver.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/drivers/driver.go): `Session` and `Capabilities`: the one interface capture uses
- [`internal/drivers/errors.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/drivers/errors.go): the typed errors capture's failure policy branches on
- [`internal/drivers/stitch.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/drivers/stitch.go): scroll-and-stitch full-page screenshots
- [`internal/drivers/scale.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/drivers/scale.go): `ScalePNG`
- [`internal/drivers/wsrpc/conn.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/drivers/wsrpc/conn.go): the websocket transport CDP and BiDi share
- [`internal/drivers/cdp/conn.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/drivers/cdp/conn.go): CDP's message codec
- [`internal/drivers/cdp/session.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/drivers/cdp/session.go): the CDP session
- [`internal/drivers/cdp/targets.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/drivers/cdp/targets.go): the child targets attached to a session
- [`internal/drivers/cdp/network.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/drivers/cdp/network.go): CDP network tracking
- [`internal/drivers/cdp/preload.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/drivers/cdp/preload.go): CDP preload scripts
- [`internal/drivers/cdp/screenshot.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/drivers/cdp/screenshot.go): CDP screenshots
- [`internal/drivers/bidi/conn.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/drivers/bidi/conn.go): BiDi's message codec
- [`internal/drivers/bidi/session.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/drivers/bidi/session.go): the BiDi session
- [`internal/drivers/bidi/network.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/drivers/bidi/network.go): BiDi network tracking
- [`internal/drivers/bidi/preload.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/drivers/bidi/preload.go): BiDi preload scripts
- [`internal/drivers/bidi/remote.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/drivers/bidi/remote.go): BiDi remote values to JSON
- [`internal/drivers/bidi/screenshot.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/drivers/bidi/screenshot.go): BiDi screenshots
- [`internal/drivers/classic/client.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/drivers/classic/client.go): the WebDriver Classic HTTP client
- [`internal/drivers/classic/session.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/drivers/classic/session.go): the Classic session
- [`internal/drivers/replaytest/replay.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/drivers/replaytest/replay.go): replays recorded transcripts for the unit tests
