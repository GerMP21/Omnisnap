# Spec: layout-diff

Module id: `layout-diff` · Package: `internal/layoutdiff` · Depends on: `bundle-schema` · Milestone: M1 · Conventions: [CONVENTIONS.md](CONVENTIONS.md)

## Objective

Compare the baseline engine's capture with another engine's capture of the same page and report every genuine layout difference **at the element where it starts**, not at every element it pushes around. This is the core capability no other tool ships (one-pager). It is a port of the spike's `lib/align.mjs`, `lib/intent.mjs` and the cause-shaping parts of `diff.mjs`, validated over rounds 1–5.

Out of scope here: deciding what is worth reporting (`triage`), pixels (`appearance-diff`).

## Behaviour

Input: baseline captures (runs a, b), engine captures (runs a, b), optional third-family captures (for intent). Output: `Result{Causes, Intent, Masks, Warnings, Stats, Pairing}`. Pure function; no I/O.

### 1. Run pairing and dynamic masks

- Compare the runs whose page **states** match best: cost `|ΔdocH| + 10·|Δnodes|`, ties keep run a.
- The other run of each engine marks **dynamic** elements (same engine, two runs, anything that changed): masked from causes and shaded in the viewer.

### 2. Matching

Hierarchical alignment of siblings: an LCS over identity tokens (tag, id, test id, role, classes, name, href/src key), then leftovers paired by tag + class. Unmatched elements become `only-in-<engine>` (topmost only).

### 3. Deltas (tolerances, spike `TOL`)

| Constant | Value | Meaning |
|---|---|---|
| `Abs` | 2 px | size change below this is noise |
| `Rel` | 1% | relative size tolerance for *reporting* (an absolute change still *explains* effects) |
| `Pos` | 2 px | position change relative to the parent |
| `Ovf` | 2 px | overflow threshold |
| `OvfDelta` | 4 px | overflow must appear/disappear AND change by more than this |

Inline boxes: height ignored (font content area differs by engine for the same font), tops tolerate 20% of their height.
Also: line count (`ln`), visibility flip, truncation flip (`tr`), text shift (`ti` changes ≥ 3 px and by more than the text-width change + 1 px, same line count, box width unchanged).

### 4. Root causes vs effects (spike rules, unchanged)

- A child whose parent changed width is reflowing: effect.
- A box that grew because a child grew, wrapped or moved: effect.
- Items that moved because a sibling resized or an earlier sibling moved: effects.
- Children that all moved point at their parent: `children-moved` on the parent.
- In inline text flow, only the first changed box is a cause.
- Width direction is tracked (flows down from a resized parent, or up from content).
- Kinds: `only-in-*`, `hidden-in-*`, `truncated-in-*`, `children-differ`, `children-moved`, `wrap`, `width`, `height`, `position`, `overflow-x`, `overflow-y`, `text-shifted`, `columns`, `intent-*`.

### 5. Table columns

When ≥ 2 cells of one table change width, the table keeps its width (within `Abs`), and the largest cell width change exceeds `max(8, 3 × largest text-width change + 2)` px: one `columns` cause on the table replaces the cell causes (`table-layout`-type differences). Auto-layout columns that follow their widest text are left alone.

### 6. Design intent (needs a third engine family)

Relations between two elements are inferred from the renders: shared edges (left, right, top, bottom, centres), equal width/height of repeated components (same tag + classes; equal height only side by side), equal gaps between consecutive siblings, corresponding parts of repeated components (grid columns for stacked rows, shared top/bottom lines for side-by-side cards), text columns (box left + text inset), no overlap. A relation is **intended** when ≥ 2 engine families keep it within 1 px; a **violation** is one engine breaking it by ≥ 2 px (≥ 8% for sizes), ≤ 800 px (larger = state or matching artifact). Pages with a match rate < 95% are skipped. Blame goes to the element that moved relative to its parent against an engine that keeps the relation. Violations join the cause on the same element (or its child); several violating children of a `children-moved` parent merge into it; otherwise a violation becomes its own `intent-*` cause. Chrome can be the violator.

### 7. Warnings

Viewport differs; node cap hit; page changed during capture (`meta.post`); engines rendered **different page states** (node gap > 1% and > 10 nodes, or height gap > 2% and > 200 px) vs the page changes over time (same-engine runs differ); web fonts failed differently.

### 8. Selectors

Every cause, instance, intent violation and warning names its element with one selector, built by `internal/selector` (shared with `dom-audit`, so a finding and a cause on the same element carry the same string):

- **Shape (the spike's):** up to 5 segments joined with ` > `, from the element up to the nearest ancestor with an id, `body` or `html`, whichever comes first. A segment is the tag and `#id`, or the tag and up to two `.class`es, plus `:nth-of-type(n)` when the parent has other children of that tag.
- **Position:** `n` is the capture's `nt`, the element's position among its parent's same-tag element children in the DOM, counting the `display: none` ones the capture leaves out, as CSS does. So an element one engine hides does not shift the position of the siblings after it, and the baseline and the engine name the same element alike. A capture without `nt` (one made before `0.2.0`, and every golden) falls back to counting the captured same-tag siblings; in a new capture an absent `nt` means the tag is unique among the DOM children, so the fallback writes no position, which is right. A shadow host's children always carry `nt`: its shadow and light lists share one `k`, and the fallback would count both.
- **Escaping:** ids and classes are serialized as `CSS.escape()` does (CSSOM, "Serializing Identifiers"), so `md:flex` is `md\:flex` and `:r1:` is `\:r1\:`. Tags are written as captured.
- **What it guarantees:** the selector is valid CSS. Anchored at an id, `body` or `html`, `document.querySelector` returns the element (ids assumed unique). Cut at 5 segments, the element is among its matches. A selector never reaches into a shadow root.

## Commands

```
go test ./internal/layoutdiff/...                              # unit + golden parity
go test -run Golden -update ./internal/layoutdiff/...          # regenerate goldens (ask first)
go test -bench . ./internal/layoutdiff/...                     # performance on large captures
```

## Project structure

`internal/layoutdiff/{match.go, deltas.go, causes.go, tables.go, intent.go, masks.go, warnings.go, result.go}`; `internal/selector/selector.go` (selectors, step 8, shared with `dom-audit`); `testdata/golden/<page>/` (spike captures + spike causes as JSON).

## Code style

Per conventions. Every spike constant above is a named constant with a comment citing the round that set it; the rule set in step 4 is one function per rule, applied in a fixed order, each with its own test.

## Testing strategy

- **Golden parity (the main gate):** for every planted page and the 10 real sites captured in round 5, the Go port produces the same causes as the spike (same kinds, same selectors, same sides; deltas within 0.1 px). Differences must be explained and approved before the golden is updated. The golden captures have no `nt`, so they exercise the selector fallback (step 8) and keep the spike's selectors.
- **Unit:** each rule in steps 3–6 with a minimal synthetic tree pair.
- **Performance:** a 10,000-element pair (MDN-sized) in ≤ 2 s; the spike took up to 7 s in JavaScript.

## Boundaries

- **Always:** keep it pure (no I/O, no clock); keep spike parity until a change is approved.
- **Ask first:** any tolerance or rule change (each moves the planted and corpus numbers).
- **Never:** report pixels or decide tiers here.

## Success criteria

1. Golden parity on the planted suite and the round-5 real-site captures.
2. Planted suite (via `triage`): 24/24 planted bugs located at the right element; 0 causes on control pages in Firefox, WebKit, Chromium, Brave and Edge.
3. Match rate ≥ 95% on the owner's site, ≥ 85% on real sites (spike: 99.9–100%).
4. Intent: 6/6 planted intent bugs; 0 violations on control pages.
5. ≤ 2 s for a 10,000-element pair; ≤ 200 MB memory.

## Open questions

- Elements moved to a different parent show as removed + added (spike known limit): worth a move detector in v1?
- iframes are not traversed (extract limit): in scope for v1?
