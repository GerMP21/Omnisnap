# Layout diff

**Role.** Compare two engines' geometry trees and report each difference **at the element where it starts**, not at every element it pushes down the page. A pure function: captures in, causes out, no I/O. Module `layout-diff`, package [`internal/layoutdiff`](https://github.com/GerMP21/Omnisnap/tree/main/internal/layoutdiff). It is run per page pair by [`pipeline.Run`](https://github.com/GerMP21/Omnisnap/blob/main/internal/pipeline/diff.go).

## Inputs and outputs

- **In:** the baseline's two runs and the engine's two runs, as trees ([`Prep`](https://github.com/GerMP21/Omnisnap/blob/main/internal/layoutdiff/match.go) turns a capture into a [`Tree`](https://github.com/GerMP21/Omnisnap/blob/main/internal/layoutdiff/match.go) of [`Node`](https://github.com/GerMP21/Omnisnap/blob/main/internal/layoutdiff/match.go)s with uids, parents and derived values); optionally a third engine family's captures, for design intent.
- **Out:** an [`Analysis`](https://github.com/GerMP21/Omnisnap/blob/main/internal/layoutdiff/analyze.go): the delta of every matched element, the **causes** ([`Cause`](https://github.com/GerMP21/Omnisnap/blob/main/internal/layoutdiff/analyze.go): the side, the node and its partner, kinds, the delta), the **symptoms** (overflows another element explains), and counts of explained and masked changes. Plus intent violations and [warnings](https://github.com/GerMP21/Omnisnap/blob/main/internal/layoutdiff/warnings.go).

## How it works

### 1. Pair the runs

Each engine has two runs. [`PairRuns`](https://github.com/GerMP21/Omnisnap/blob/main/internal/layoutdiff/pairing.go) compares the runs whose page **states** match best (a slideshow may be on a different slide in each engine): cost `|ΔdocH| + 10·|Δnodes|`, ties keep run a. The other run of each engine is kept for the mask.

### 2. Mask dynamic content

[`DynamicMask`](https://github.com/GerMP21/Omnisnap/blob/main/internal/layoutdiff/masks.go) runs the whole analysis between the two runs **of the same engine**. Whatever changed there (ads, carousels, clocks) changes by itself, so it is masked with its subtree: left out of the causes and shaded in the viewer.

### 3. Match

[`MatchTrees`](https://github.com/GerMP21/Omnisnap/blob/main/internal/layoutdiff/match.go) pairs every baseline element with one engine element, sibling list by sibling list:

1. an LCS over **identity tokens** (tag, id, test id, role, classes, name, `href`/`src` key) anchors the pairs;
2. the leftovers between anchors are paired in order by tag + class, then by tag alone (which rescues dynamic ids and changed text);
3. huge sibling lists fall back to a greedy in-order match with a 50-element lookahead (the dynamic-programming table would be 4M cells).

Unmatched elements are recorded only at the top: a missing subtree is one `only-in-<engine>` element, not one per descendant.

### 4. Measure deltas

[`Deltas`](https://github.com/GerMP21/Omnisnap/blob/main/internal/layoutdiff/deltas.go) compares each matched pair, with the tolerances in [`match.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/layoutdiff/match.go):

| Constant | Value | Meaning |
|---|---|---|
| `TolAbs` | 2 px | a size change below this is noise |
| `TolRel` | 1% | a size change must also exceed this fraction of the larger size to be **reported** (an absolute change still **explains** effects) |
| `TolPos` | 2 px | a position change relative to the parent |
| `TolOvf` | 2 px | content beyond the box before it counts as overflowing |
| `TolOvfDelta` | 4 px | an overflow must appear or disappear **and** change by more than this |

Inline boxes: the height is ignored (the same font's content area differs by engine) and tops tolerate 20% of the box's height. Also measured: line count, a visibility flip, a truncation flip, and a **text shift** (the text inset changes by 3 px or more and by more than the text-width change + 1 px, with the same line count and the box width unchanged).

### 5. Causes versus effects

[`Analyze`](https://github.com/GerMP21/Omnisnap/blob/main/internal/layoutdiff/analyze.go) decides which changed elements are causes. The rules, from the spike's `lib/align.mjs`:

- a child whose parent changed width is **reflowing**: an effect (width direction is tracked: it flows down from a resized parent, or up from content);
- a box that grew because a child grew, wrapped or moved: an effect;
- an item that moved because a sibling resized, or an earlier sibling moved the same way: an effect;
- children that **all** moved with none resized: one `children-moved` cause on the parent (padding, gap, order, justify);
- in flex and grid rows, a box stretched like its siblings: an effect;
- in inline text flow, only the first changed box is a cause;
- an overflow explained by a descendant's width or overflow: a **symptom**, not a cause;
- an element present in one engine only: a cause. When the extras are on **both** sides, or more than 3 under one parent, the content itself differs: one `children-differ` cause on the parent, not N;
- an element that takes no space, or is transparent and out of flow, is inert. A transparent element that still takes space is not: it pushes its siblings.

The cause kinds: `only-in-<engine>`, `hidden-in-<engine>`, `truncated-in-<engine>`, `children-differ`, `children-moved`, `wrap`, `width`, `height`, `position`, `overflow-x`, `overflow-y`, `text-shifted`, `columns`, and `intent-*`. (`appearance` is added by the [Appearance diff](Appearance-Diff).)

### 6. Table columns

[`TableColumns`](https://github.com/GerMP21/Omnisnap/blob/main/internal/layoutdiff/tables.go): when two or more cells of one table change width, the table keeps its width (within `TolAbs`), and the largest cell width change exceeds `max(8, 3 × largest text-width change + 2)` px, one `columns` cause on the table replaces the cell causes (a `table-layout`-type difference). Auto-layout columns that follow their widest text are left alone.

### 7. Design intent

Needs a third engine family. [`IntentViolations`](https://github.com/GerMP21/Omnisnap/blob/main/internal/layoutdiff/intent.go) infers relations between two elements from the renders: shared edges and centres, equal width or height of repeated components (equal height only side by side), equal gaps between consecutive siblings, corresponding parts of repeated components (grid columns for stacked rows, shared lines for side-by-side cards), text columns, and no overlap. Constants in [`intent.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/layoutdiff/intent.go):

- a relation is **intended** when at least two families keep it within 1 px;
- a **violation** is one engine breaking it by 2 px or more (8% or more for sizes), and at most 800 px (more is a state or matching artifact);
- pages with a match rate under 95% are skipped, with a warning;
- at most the first 60 children of a parent are checked; in groups of more than 24, each child is compared with its next 6 siblings only.

Blame goes to the element that moved relative to its parent, against an engine that keeps the relation. **Chrome can be the violator.** The pipeline joins violations to the cause on the same element (or its child); several violating children of a `children-moved` parent merge into it; otherwise a violation becomes its own `intent-align`, `intent-size`, `intent-gap`, `intent-column` or `intent-overlap` cause ([`internal/pipeline/intent.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/pipeline/intent.go)).

### 8. Warnings

[`WarningsOf`](https://github.com/GerMP21/Omnisnap/blob/main/internal/layoutdiff/warnings.go) adds, per comparison: the second run is missing (dynamic content not masked); the layout viewport differs (widths will differ everywhere); the node cap was hit; the page changed during capture; the engines rendered **different page states** versus the page **changes over time** (same-engine runs differ); web fonts failed differently; one side blocked requests the other loaded.

### 9. Selectors

Every cause, instance, intent violation and warning names its element with [`Selector`](https://github.com/GerMP21/Omnisnap/blob/main/internal/layoutdiff/match.go), which calls the shared builder in [`internal/selector`](https://github.com/GerMP21/Omnisnap/blob/main/internal/selector/selector.go) (the [DOM audit](DOM-Audit) calls it too). Up to five segments joined with ` > `, from the nearest ancestor with an id (or `body`, or `html`) down to the element; a segment is the tag with `#id`, or with up to two `.class`es, and `:nth-of-type(n)` when the parent has other children of that tag.

- `n` is the capture's `nt`: the DOM position among same-tag siblings, counting the `display: none` ones the capture leaves out ([Data contracts](Data-Contracts)). So an element one engine hides does not shift its later siblings, and both sides name the same element alike.
- Without `nt` (captures from before `0.2.0`, and the goldens) it counts the captured same-tag siblings, as the spike did. In a new capture a missing `nt` means the tag is unique among the DOM children, so the fallback adds no position either.
- Ids and classes are escaped as `CSS.escape()` does ([`Ident`](https://github.com/GerMP21/Omnisnap/blob/main/internal/selector/selector.go)), so every selector is valid CSS. Anchored, `document.querySelector` returns the element; cut at five segments, the element is among its matches.

## Key files

| File | Holds |
|---|---|
| [`match.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/layoutdiff/match.go) | tolerances, `Node`, `Tree`, `Prep`, `Selector`, `MatchTrees` |
| [`selector.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/selector/selector.go) (package `selector`) | the selector builder shared with the DOM audit: `Path`, `Segment`, `Ident` |
| [`deltas.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/layoutdiff/deltas.go) | `Delta`, `Deltas` |
| [`analyze.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/layoutdiff/analyze.go) | the cause and effect rules |
| [`tables.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/layoutdiff/tables.go) | `columns` |
| [`intent.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/layoutdiff/intent.go) | design intent |
| [`pairing.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/layoutdiff/pairing.go), [`masks.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/layoutdiff/masks.go), [`warnings.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/layoutdiff/warnings.go) | runs, masks, warnings |
| [`severity.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/layoutdiff/severity.go), [`lineinfo.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/layoutdiff/lineinfo.go), [`pitch.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/layoutdiff/pitch.go) | helpers triage uses: severity weight, line count and pitch |

## Tests

- Unit tests on hand-built trees ([`align_test.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/layoutdiff/align_test.go)): anchors and leftovers, deltas, width flowing down, the moved-kids rule, only-in and `children-differ`, table columns, the dynamic mask, run pairing, warnings.
- Selectors ([`selector_test.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/layoutdiff/selector_test.go)): the hidden first card, one rendered `p` of two, an element hidden in one engine only (both sides get one selector), a capture without `nt`, a unique tag. [`internal/selector`](https://github.com/GerMP21/Omnisnap/blob/main/internal/selector/selector_test.go) checks `Ident` against `CSS.escape()` values recorded from Chrome, and the cut and anchor rules.
- **Golden parity** ([`TestGoldenAlignParity`](https://github.com/GerMP21/Omnisnap/blob/main/internal/layoutdiff/golden_test.go)): on the spike's captures in [`testdata/golden/planted`](https://github.com/GerMP21/Omnisnap/tree/main/testdata/golden/planted), the Go port finds the same causes as the spike. `TestGoldenRealWarnings` and `TestGoldenRealPairingMasks` run on local real-site goldens when present (they are third-party content and not committed).

## Where the code differs from the spec

Nothing found: the tolerances, rules, kinds, table rule, intent thresholds and warnings match [`SPEC-layout-diff.md`](https://github.com/GerMP21/Omnisnap/blob/main/specs/SPEC-layout-diff.md). The spec's open questions stand: an element moved to another parent shows as removed plus added, and iframes are not traversed.

## Spec

[`specs/SPEC-layout-diff.md`](https://github.com/GerMP21/Omnisnap/blob/main/specs/SPEC-layout-diff.md).

## Source files

- [`internal/layoutdiff/match.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/layoutdiff/match.go): the tree, `Node`, `Selector` and matching
- [`internal/selector/selector.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/selector/selector.go): selectors (`Path`, `Segment`, `Ident`), shared with the DOM audit
- [`internal/layoutdiff/analyze.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/layoutdiff/analyze.go): `Analyze`: causes and symptoms
- [`internal/layoutdiff/deltas.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/layoutdiff/deltas.go): per-element deltas
- [`internal/layoutdiff/pairing.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/layoutdiff/pairing.go): `PairRuns`
- [`internal/layoutdiff/intent.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/layoutdiff/intent.go): intent checks against a third engine
- [`internal/layoutdiff/lineinfo.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/layoutdiff/lineinfo.go): line information
- [`internal/layoutdiff/pitch.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/layoutdiff/pitch.go): line pitch
- [`internal/layoutdiff/masks.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/layoutdiff/masks.go): `DynamicMask`: what changes between two runs of the same engine
- [`internal/layoutdiff/tables.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/layoutdiff/tables.go): table columns
- [`internal/layoutdiff/severity.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/layoutdiff/severity.go): severity weights
- [`internal/layoutdiff/warnings.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/layoutdiff/warnings.go): pair warnings
