# Spec Delta: WebKitGTK viewport

Status: proposed, on hold (not approved 2026-11-18; option C stays in code)

Original: [SPEC-capture.md](SPEC-capture.md) §1 "Normalize" and success
criterion 2 ("every engine reports exactly 1280×800 CSS px"), and
[SPEC-drivers.md](SPEC-drivers.md) "viewport is exactly 1280×800 … (WebKit via
the correction loop)". Only the areas below change; everything else inherits.

## Objective

Stop the WebKit lane from producing cross-engine comparisons at a different
viewport than the other engines, and decide what to do about the driver that
cannot be resized.

## Current behavior

Measured on this machine, WebKitGTK MiniBrowser 2.54.2 with its private
headless Weston:

- `POST /session/{id}/window/rect` is accepted and ignored. A session reports
  `innerWidth×innerHeight` of **671×680** (outer 1024×768), and the
  measure-and-correct loop in `classic.SetViewport` cannot converge.
- MiniBrowser has no window-size option at launch: `--help` lists
  `-g, --geometry=GEOMETRY` as *"Unused. Kept for backwards-compatibility
  only"* and has no `--window-size`.
- Passing `--window-size=1280,800` (or `--width=1280 --height=800`) prevents
  the browser from starting, so `POST /session` times out. Passing
  `--geometry=1280x800` is accepted and has no effect.
- The CLI therefore wrote `meta.vw = 671` for WebKitGTK while Chromium and
  Firefox captured at 1280, and the cross-engine diff compared the two
  layouts as if they were the same width.
- `TestConformance` (WebKit subtests) and `TestViewportAcrossEngines` are
  gated behind `OMNISNAP_WEBKIT_CONFORMANCE=1` / `OMNISNAP_WEBKIT_VIEWPORT=1`
  so the suites stay green with WebKitGTK installed.

Option B from the review ("pass a window-size argument at launch") is
therefore **not possible** on this build; option C (drop WebKitGTK from the
gate) is what the code does today.

## New behavior

1. **The capture records the mismatch and refuses the comparison.** When
   `SetViewport` returns a viewport other than the requested one, the capture
   carries `meta.viewportMismatch: true` plus a warning, and the engine is
   skipped for cross-engine causes in that job: the summary lists it as
   skipped with the reason ("viewport 671×680, requested 1280×800"). No two
   engines are compared at different widths without that note.
2. **WebKitGTK is outside the 1280×800 guarantee** until a mechanism exists.
   The two env gates stay and cite this delta. Mutation capture and same-engine
   diffs are unaffected: they compare one engine with itself, at whatever size
   it renders.
3. **The compositor route was tried and failed** (2026-11-18): a private
   Weston output of 1280×800 plus MiniBrowser `--full-screen` still reports a
   671×680 viewport (outer 1024×768), so full screen is ignored in automation
   mode. The remaining mechanisms are driver-side and out of reach here, so
   option C — WebKitGTK outside the viewport guarantee, tests gated — is the
   state of the code.

## Success criteria

1. A WebKitGTK capture at 671×680 through the CLI produces
   `meta.viewportMismatch: true`, a capture warning naming both sizes, and an
   engine entry in `summary.json` marked skipped with that reason.
2. A Chromium-only or Chromium+Firefox job is unchanged: no new warnings, no
   skipped engines, `GoldenGeometry` parity as today.
3. `go test -tags browsers ./internal/capture/` and `./internal/drivers/` stay
   green with WebKitGTK installed; the gated runs reproduce the defect above
   with the env var set.
4. `go test -tags e2e ./test/e2e/` still passes (WebKit is not used there).

## Boundaries

- Always: report the viewport actually obtained in `meta.vw`/`meta.vh`.
- Ask first: changing the default viewport, the correction loop's attempts, or
  the Westons launch arguments (they change what is measured).
- Never: compare two engines at different viewport widths without a warning
  and a skipped-engine note.

## Out of scope (deferred)

- Making `/window/rect` work (it is the driver's or the compositor's bug; we
  cannot patch WebKitGTK here).
- Safari on macOS, which already honors `SetWindowRect`.
- Replacing the gated tests with a screenshot-based width check.

## Open questions

- Should a viewport mismatch fail the *engine's* captures instead of skipping
  it? Report-only is the plan's rule, so the delta proposes warning + skip;
  confirm.
- Accept WebKitGTK as a "same-engine only" engine for v1, or make the CLI
  report it as skipped on every cross-engine job (the delta's rule, still not
  approved)?
