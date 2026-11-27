# DOM audit

**Role.** Report layout anomalies **within one engine's render**: problems a page has even when every browser agrees. A pure function over one capture file; it never touches the page. Module `dom-audit`, package [`internal/domaudit`](https://github.com/GerMP21/Omnisnap/tree/main/internal/domaudit).

It is the only module with no spike evidence behind it, so it ships quiet: two checks are `report`, the rest `info`, and checks are promoted one at a time.

## Inputs and outputs

- **In:** one capture (`<engine>-<run>.json`).
- **Out:** a list of [`Finding`](https://github.com/GerMP21/Omnisnap/blob/main/internal/domaudit/audit.go)s: check id, selector, box, severity, engine. The CLI runs [`Audit`](https://github.com/GerMP21/Omnisnap/blob/main/internal/domaudit/audit.go) on the baseline's and the engine's run-a capture of every result, and writes both lists into the result's `audit` ([`bundle.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/bundle.go)). `--no-audit` skips it.

## How it works

The capture is flattened into a walker (every node with its parent), and the seven checks run in reporting order ([`checks.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/domaudit/checks.go)). Thresholds are named in [`audit.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/domaudit/audit.go).

| Check | Fires when | Severity |
|---|---|---|
| `page-horizontal-scroll` | the document is at least 2 px wider than the viewport (`docW > vw + 1`) | report |
| `clipped-text` | a text owner sits inside an `overflow: hidden`/`clip` element whose overflow is at least 2 px and less than one viewport (more than a viewport is deliberate hiding, as in image replacement) | report |
| `truncated-text` | text cut by an ellipsis or a line clamp (`tr`) | info |
| `overlap-interactive` | two interactive elements overlap by at least 4 px in both directions and neither contains the other | info |
| `offscreen-interactive` | a visible interactive element lies entirely left of 0 or right of `docW` | info |
| `zero-size-interactive` | a visible interactive element with its own text but a width or height under 1 px | info |
| `protrudes-parent` | an element extends at least 4 px beyond a visible parent with a box of its own and `overflow: visible`, and overlaps a sibling | info |

`protrudes-parent` needs the parent to have a box of its own because the capture records no border or background: a parent without a box cannot visibly contain anything, so this is the closest honest approximation.

A finding names its element with the selector a layout-diff cause uses ([Layout diff](Layout-Diff), "Selectors"; [`internal/selector`](https://github.com/GerMP21/Omnisnap/blob/main/internal/selector/selector.go)), built from the walker's parent links when the finding is made: the same five-segment cut, the stop at an id, `body` or `html`, the DOM position from `nt`, escaped tags, ids and classes, and the shadow-host rule (an element inside an open shadow root is named by its outermost host). So a finding and a cause on one element carry the same string.

Findings are sorted report first.

## Tests

- [`audit_test.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/domaudit/audit_test.go): one test per check, `TestCleanCaptureHasNoFindings`, and `TestAuditIsFast` (about 5,000 nodes in under 50 ms). [`selector_test.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/domaudit/selector_test.go): the third of three zero-size buttons is `button:nth-of-type(3)`, and every node of a golden capture and of a capture with `nt` gets the same selector as `layoutdiff.Selector`.
- With browsers, [`TestPlantedAuditPages`](https://github.com/GerMP21/Omnisnap/blob/main/test/e2e/audit_test.go): each check fires on its planted page at the right element.

## Where the code differs from the spec

Nothing found. The spec's "Decisions pending review" now describes the same severities.

## Spec

[`specs/SPEC-dom-audit.md`](https://github.com/GerMP21/Omnisnap/blob/main/specs/SPEC-dom-audit.md).

## Source files

- [`internal/domaudit/audit.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/domaudit/audit.go): `Audit` and the thresholds
- [`internal/domaudit/checks.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/domaudit/checks.go): the checks, in reporting order
- [`internal/selector/selector.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/selector/selector.go): the selector builder findings share with layout-diff causes
