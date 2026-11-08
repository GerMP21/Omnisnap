# Spec: viewer

Module id: `viewer` · Package: `internal/viewer` · Depends on: `bundle-schema` · Milestone: M0 (captures side by side) / M1 (findings) · Conventions: [CONVENTIONS.md](CONVENTIONS.md)

## Objective

One self-contained `report.html` in the bundle that a developer opens from disk (or a CI artifact) and immediately sees, per page and engine pair: the screenshots side by side with synced scrolling, every Report cause outlined on both, why it differs, and how to fix it when known. The one-pager's "short demo of one command showing every engine" is this page.

## Behaviour

- **Generated** from `summary.json` (+ screenshots referenced by relative path, never inlined: bundles reach hundreds of MB).
- **Summary table:** pages × engines with status (ok / diverged / failed / unreachable), Report and Info counts, match rate, warnings.
- **Per page and pair:**
  - two panes, baseline and engine, synced proportional scrolling;
  - boxes on both screenshots for every cause (and its other instances), numbered; clicking a cause highlights and scrolls both panes to it;
  - Report causes listed first: title, ×count, consequence chips, selector, delta in px, the KB explanation and fix, the evidence (mechanism and its verified fix, which engine is odd, perception events, ink vs noise floor, intent relations), the score;
  - Info causes collapsed;
  - dynamic regions shaded (hatched) on both panes; warnings shown above the panes (different page states, page changed during capture, blocked hosts, fonts);
  - intent violations listed with the engines that keep the relation.
- **M0** (no diff yet): the same panes without findings, plus `dom-audit` findings.
- **Merged bundles** (`SPEC-merge.md`): a host selector; results grouped by host.
- **No network:** no external scripts, fonts or stylesheets; works from `file://`; no tracking.
- **Theming and accessibility:** light and dark via `prefers-color-scheme`; keyboard navigation of causes (↑/↓, Enter); boxes have text equivalents in the list; contrast AA.

## Commands

```
go test ./internal/viewer/...                                   # template rendering on fixture summaries
go test -tags browsers ./internal/viewer/... -run Render        # opens report.html headless, asserts no console errors
go run ./cmd/omnisnap report ./omnisnap-out                     # re-render a bundle
```

## Project structure

`internal/viewer/{viewer.go, template.html, app.js, style.css}` embedded with `go:embed`; `testdata/viewer/` summaries (small, planted, large: 300 causes).

## Code style

Per conventions for Go. The page script is plain ES2020, no framework, no build step; data is one JSON island (`<script type="application/json">`) like the spike's `lib/report.mjs`; all text inserted with `textContent` (selectors and page text are untrusted).

## Testing strategy

- **Unit:** template renders for each fixture; JSON island escapes `</script>`; untrusted strings never reach `innerHTML`.
- **Headless render (`browsers`):** load each fixture in Chrome; assert 0 console errors, the number of listed causes and drawn boxes equals the summary's, clicking a cause activates its boxes (the spike's `viewcheck` did this for 365 causes).
- **Performance:** the large fixture (50 pages, 300 causes) is interactive in < 2 s on a CI runner.

## Boundaries

- **Always:** escape everything from the page; work offline.
- **Ask first:** adding any third-party script or asset.
- **Never:** load anything from the network; inline screenshots as data URIs; include labeling UI (removed in round 5).

## Success criteria

1. Opens from `file://` in Chrome, Firefox and Safari with 0 console errors.
2. Every cause in `summary.json` is listed and outlined on both screenshots at its box.
3. Synced scrolling keeps the same document fraction in both panes; clicking a cause scrolls both panes to it.
4. Large fixture interactive in < 2 s; `report.html` itself < 2 MB for 50 pages.
5. An XSS test page whose text and selectors contain markup renders as text.

## Open questions

- A diff overlay (blink between engines over one pane)? Proposed: after v1.
- Should the viewer show the probe screenshots (what the page looks like with whole-pixel line-height) to prove a fix? Probes store geometry only today.
