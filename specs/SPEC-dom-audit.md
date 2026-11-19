# Spec: dom-audit

Module id: `dom-audit` · Package: `internal/domaudit` · Depends on: `capture` · Milestone: M0 · Conventions: [CONVENTIONS.md](CONVENTIONS.md)

## Objective

Report layout anomalies **within one engine's render**, without comparing engines: problems a page has even if every browser agrees (PRD 1.0.0 Feature 6, "Injected DOM Layout Anomaly Auditor"). It gives M0 something useful before the cross-engine diff ships, and catches bugs a comparison can't (all engines equally broken).

Not validated by the spike: this spec sets the bar it must reach before it is turned on by default.

## Behaviour

Pure function over one capture file (`<engine>-<run>.json`); no extra page access. Checks:

| Check | Rule (initial thresholds, to be tuned on the planted suite) |
|---|---|
| `page-horizontal-scroll` | document wider than the viewport by ≥ 2 px (`docW > vw + 1`) |
| `clipped-text` | text owner inside an element with `overflow: hidden/clip` whose overflow ≥ 2 px and < one viewport (content pushed beyond a viewport is deliberate hiding, as in image replacement) |
| `truncated-text` | `tr` set (ellipsis or line clamp cutting text), informational |
| `overlap-interactive` | two interactive elements (`ia`) overlap by ≥ 4 px in both directions and neither contains the other |
| `offscreen-interactive` | interactive element entirely left of 0 or right of `docW`, and visible |
| `zero-size-interactive` | interactive, visible, with width or height < 1 px but own text |
| `protrudes-parent` | element extends ≥ 4 px beyond a visible parent with a box of its own, with `overflow: visible`, and overlaps a sibling (the capture carries no parent border or background, so a parent without a box cannot visibly contain anything — the closest honest approximation) |

Each finding: check id, selector, box, severity (`report` for the first two, `info` otherwise until tuned), engine. Results go into `summary.json` as `audit` per result and into the viewer.

Off switch: `--no-audit`; on by default only after the success criteria pass.

## Commands

```
go test ./internal/domaudit/...                 # unit on fixture captures
go test -tags e2e ./test/e2e/... -run Audit     # planted audit pages through the binary
```

## Project structure

`internal/domaudit/{audit.go, checks.go}`; `testdata/domaudit/` captures; new planted pages `testdata/planted/audit-*.html` (one per check, plus controls).

## Code style

Per conventions. Each check is a `func(c *schema.Capture) []Finding`, registered in a table with its id; thresholds are named constants.

## Testing strategy

- **Unit:** one fixture capture per check that must fire, one near-miss that must not.
- **Planted:** 7 audit pages (one per check) + 3 controls in every engine: each fires exactly its check; controls fire nothing.
- **Corpus:** run on the round-6 crawl captures; count findings per page to calibrate severities (no labels; see `SPEC-ground-truth.md` for why).

## Boundaries

- **Always:** stay single-engine and read-only.
- **Ask first:** promoting a check to `report` severity; adding a check that needs a new extracted field.
- **Never:** duplicate a cross-engine finding (a difference between engines belongs to `layout-diff`).

## Success criteria

1. Each check fires on its planted page in Chrome, Firefox and WebKit, at the right element.
2. 0 findings on the control pages (existing planted controls and the 3 new ones) in every engine.
3. On the crawl, median `report`-severity audit findings per page ≤ 2 (otherwise the severity is demoted to `info`).
4. Runs in < 50 ms per capture of 5,000 elements.

## Decisions pending review

- Ships in v1 (2026-11-17), as `info` by default from M0; checks are promoted individually. It is the only module with no spike evidence, so I ship it quiet on purpose and let the crawl decide what gets promoted.

## Open questions

- Accessibility-adjacent checks (tap-target size) belong here or in a later module?
