# Proposal

## Why

A cause's selector reads as CSS, and people paste it into devtools, but it can name the wrong element. `:nth-of-type(N)` is counted among the parent's **captured** children, and the capture drops `display: none` elements, so a hidden sibling before the element shifts N. On synaptech-site's first report (PR 20, run 44777766699, 2026-11-20) an `overflow x` on the first *visible* project card pointed at the hidden card before it, which looked like a false positive. A `Width differs` named the sixth card when the element was the DOM's seventh (backlog B2).

The stable `v0.2.0` is the release most consumers will pin, and its compatibility promise forbids changing selector text in a `0.2.x` patch. So the fix has to land before `v0.2.0`, not after.

## What Changes

- **The capture records each element's position among its same-tag DOM siblings**, counting hidden elements. It is a new optional node field, written only when the parent has two or more children with that tag. The capture schema stays at `capture/1.json`, because the field is additive.
- **Selectors use the DOM position.** `:nth-of-type(N)` is the element's real position in the DOM, and it is written whenever the DOM parent has more than one child with that tag, even if only one of them is rendered. Captures written before this change, which have no such field, keep producing the selectors they produce today, so golden parity holds.
- **Selectors are valid CSS.** Ids and class names are escaped the way `CSS.escape` does (Tailwind classes such as `md:w-1/2`, ids that start with a digit), so `document.querySelector` accepts every selector Omnisnap writes.
- **The same element gets the same selector in every engine.** An element hidden in one engine no longer shifts its siblings' selectors on that side only.
- **DOM audit findings name elements the same way** as causes. Today the audit writes a path with the first class and no position at all.
- **BREAKING (selector text):** causes, intent relations, warnings and audit findings in `findings.json`, `summary.json`, the viewer and the PR comment now name elements with the new selectors. A selector that a consumer recorded from a `0.1.0` candidate may differ after `v0.2.0`. The `0.2.0` release notes say so (change `release-v0-2-0`).
- **No change to matching.** How elements are paired between engines does not depend on the captured sibling index (design.md, "Pairing"), so the causes found, their kinds and their tiers stay as they are.

## Capabilities

### New Capabilities

- `element-selectors`: how Omnisnap names an element in its outputs (causes, instances, intent, warnings, DOM audit findings): which DOM facts the selector is built from, when it carries a position, that it is valid CSS, and what it is guaranteed to match in the captured page.

### Modified Capabilities

None. There are no OpenSpec specs yet. The normative project specs whose wording changes in this change are listed under Impact.

## Impact

- **Code:**
  - `assets/inject/extract.js` gains the sibling position (and its entry in `assets/inject/SHA256SUMS`).
  - `internal/schema/capture.go` gains the node field.
  - `internal/layoutdiff/match.go` (`segment`, `Selector`) uses the field and escapes ids and classes.
  - `internal/domaudit/audit.go` (`childPath`) uses the same selector form.
- **Schema:** `schema/capture.schema.json` gains two optional node properties, `nt` and `sr` (the release review added `sr`, 2026-11-26). There is no new major and the URL is unchanged.
- **Specs:**
  - `specs/SPEC-bundle-schema.md` (capture node format)
  - `specs/SPEC-layout-diff.md` (selector rule)
  - `specs/SPEC-dom-audit.md` (selector form)
  - `specs/SPEC-capture.md` (the extract step, if it names the fields)
- **Docs:** the wiki pages that describe the touched files: `Capture-Pipeline`, `Layout-Diff`, `Data-Contracts`, `Glossary`, `DOM-Audit`, and any others `grep -rl` finds (`CONTRIBUTING.md` rule 7).
- **Tests:**
  - A new fixture page with hidden same-tag siblings.
  - Unit tests for the position, the fallback and the escaping.
  - An end-to-end check that `document.querySelector(selector)` finds the cause's element in a real browser.
  - The golden parity and planted gates must still pass, unchanged.
- **Lab and data:** `labels.json` selectors (people-facing only; the dataset joins on node ids) and the cause ids in `crawl/dataset.csv` change for new captures. Existing rows stay as they are.
- **Out of scope:**
  - Selectors that reach into shadow roots, which `document.querySelector` cannot do in any form. An element inside one is named by its shadow host instead (release review, 2026-11-26).
  - Duplicate ids on a page.
  - Making a selector cut at five segments unique in the whole document.
