# Viewer

**Role.** `report.html`: one self-contained file that shows the results, works offline from `file://`, and loads nothing from the network. Module `viewer`, package [`internal/viewer`](https://github.com/GerMP21/Omnisnap/tree/main/internal/viewer).

## Inputs and outputs

- **In:** a [`Summary`](https://github.com/GerMP21/Omnisnap/blob/main/internal/schema/summary.go), and the screenshots, referenced by their relative paths in the bundle (never inlined: bundles reach hundreds of MB).
- **Out:** `<bundle>/report.html`, written atomically ([`Write`](https://github.com/GerMP21/Omnisnap/blob/main/internal/viewer/viewer.go)). `omnisnap report <bundle>` re-renders it from `summary.json`.

## How it works

[`Render`](https://github.com/GerMP21/Omnisnap/blob/main/internal/viewer/viewer.go) fills [`template.html`](https://github.com/GerMP21/Omnisnap/blob/main/internal/viewer/template.html) with three embedded files: [`style.css`](https://github.com/GerMP21/Omnisnap/blob/main/internal/viewer/style.css), the summary as a JSON **data island**, and [`app.js`](https://github.com/GerMP21/Omnisnap/blob/main/internal/viewer/app.js). `encoding/json` escapes `<`, `>` and `&`, so no page text can close the script tag, and `app.js` inserts every string with `textContent`; links are only made from `http(s)` URLs, so a page URL can never become a `javascript:` link. There is no framework: plain ES2020.

What `app.js` draws:

- **The summary table:** pages × engines (and hosts, for a merged bundle), with status, Report and Info counts, match rate and warnings.
- **Per result:** two panes, baseline and engine, with **synced proportional scrolling** (a programmatic scroll marks its pane, so the partner does not echo it back); a numbered box on both screenshots for every cause; clicking a cause highlights it and scrolls both panes to it.
- **The causes:** Report first, then Info collapsed in a `<details>`; per cause the title, ×count (with its other instances), consequence chips, selector, delta, the KB explanation and fix, the evidence (mechanism and its verified fix, consensus, perception events, ink against the noise floor, intent), and the score.
- **Dynamic regions** shaded on both panes; warnings above the panes; the design-intent list; the DOM-audit findings of both captures.
- **Merged bundles:** a host selector that filters the table and the results; its choice is kept in `localStorage`, so `#rN` anchors keep working.
- **Theme:** light and dark follow `prefers-color-scheme`, with a toggle.
- **Keyboard:** `j` and `k` (and Home and End) move between result sections; Enter or Space on a cause selects it.

## Tests

- [`viewer_test.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/viewer/viewer_test.go): the data island is escaped, XSS markers render as text, the report is self-contained (no external URL), no inline event handlers, audit findings rendered, a 50-page report's size and speed, a merged report's host selector and groups.
- [`render_browsers_test.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/viewer/render_browsers_test.go) (`browsers` tag), in Chrome: the report renders, clicking a cause scrolls both panes, the host filter works, a 50-page report stays fast. Fixtures in [`testdata/viewer`](https://github.com/GerMP21/Omnisnap/tree/main/testdata/viewer).

## Where the code differs from the spec

- **The viewer does not read `findings.json`.** `SPEC-bundle-schema.md` says the per-page `findings.json` holds the full evidence "which the viewer reads". The viewer renders only the summary's data island, with the compact evidence; `findings.json` is written for other consumers and is not loaded (a page opened from `file://` could not fetch it anyway).
- **Keyboard keys.** The spec names ↑/↓ and Enter for moving between causes; the code uses `j`/`k`, Home and End between result sections, and Enter or Space on a focused cause.

## Spec

[`specs/SPEC-viewer.md`](https://github.com/GerMP21/Omnisnap/blob/main/specs/SPEC-viewer.md).

## Source files

- [`internal/viewer/viewer.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/viewer/viewer.go): `Render` and `Write`
