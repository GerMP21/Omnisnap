# Design

## Context

See proposal.md for the motivation and specs/element-selectors/spec.md for the required behavior. What shapes the approach:

- **Where the index goes wrong.** `segment()` in `internal/layoutdiff/match.go:118` counts same-tag nodes in `p.Kids`, the parent's captured children. `walk()` in `assets/inject/extract.js` returns `null` for `display: none` (line 110), for the `SKIP` tags, past `MAX_NODES`, and below an `<svg>`. So every one of those elements is missing from `Kids`. `visibility: hidden` and `opacity: 0` elements are captured (with `hid`), so they already count. `display: none` is the only skip that can be the same tag as a rendered element in practice.
- **`Selector()` callers.** `layoutdiff/analyze.go` (causes and instances), `layoutdiff/warnings.go`, `pipeline/intent.go`, `triage/group.go` (grouping), and `groundtruth/label.go` (lab labels).
- **The DOM audit builds its own path.** `internal/domaudit/audit.go`'s `childPath` writes `tag#id` or `tag.firstClass` joined with ` > ` from the root, with no position at all. So it has the same bug, worse.
- **Escaping.** Neither path escapes ids or classes. A Tailwind class (`md:flex`) or a React id (`:r1:`) makes the selector invalid CSS. The committed planted goldens have no such characters (all 30 golden findings checked), so escaping does not move them.
- **Captured children mix trees.** `walk()` concatenates the open shadow root's children and the light children into one `k`. `document.querySelector` cannot reach into a shadow root in any case.
- **Gates that read selectors.** `test/e2e/planted_test.go` decides a hit by `strings.Contains(selector, "#"+id)`. The planted ids are plain words, so escaping does not change them. The golden parity tests (`internal/layoutdiff/golden_test.go`, `internal/groundtruth` "Golden") compare `sel` strings with the spike's findings, computed from captures that have no sibling positions.
- **The injected scripts are pinned.** `assets/inject/SHA256SUMS` holds `extract.js`'s hash, checked by `internal/testutil/sha256sums_test.go`. `testdata/golden/README.md` records that the goldens were made with the pinned scripts.

### Pairing (backlog B2's open question)

Pairing does **not** depend on the captured sibling index. `MatchTrees` calls `alignChildren`, which:

```
 children of A            children of B
 [hdr, card, card, ftr]   [hdr, card(hidden in B: absent), card, ftr]
        |                        |
        v                        v
   identity tokens (id, testid, url, name, text hash, tag|class)
        |
        +--> LCS anchors (equal tokens, in order)
        +--> leftovers between anchors: in order by tag.class, then by tag
        +--> unpaired --> OnlyA / OnlyB
```

No step uses a position number. An element that one engine hides becomes `OnlyA` or `OnlyB`, and the siblings after it still pair by token or by order. So B2 is a labelling bug only: the causes found, their kinds and their tiers do not change, and this change leaves `MatchTrees` alone.

## Goals / Non-Goals

**Goals:**
- The position in a selector is the DOM's, computed in the page where the DOM is available, at almost no cost.
- Old captures (the goldens, bundles from the candidates) produce exactly the selectors they produce today, apart from escaping.
- One selector builder for causes, intent, warnings and the DOM audit.

**Non-Goals:**
- Selectors that reach into shadow roots.
- Unique selectors when a five-segment path stops before an anchor.
- Handling duplicate ids.
- Changing the matcher, the tolerances or which causes are reported.
- A new capture schema major.

## Decisions

### 1. Record the DOM position in the capture (backlog approach 1)

`extract.js` adds `nt` to a node: its 1-based position among the element children of its parent that have the same `tagName`. The children are read through the platform `children` getter the script already uses (`childrenOf`), never a page-shadowable property. The count is computed once per child list in the parent's loop, before `walk()` is called on each child, and passed in. It covers every element child: `display: none`, `SKIP` tags and anything else `walk()` drops. The open shadow root's children and the light children are counted as two separate lists, each against its own parent, as CSS counts them.

*Alternative (backlog approach 2):* keep counting captured siblings, but write it as something that is not CSS (`div.card[2nd shown]`). Rejected: the selector would still not select the element, and the spec's querySelector guarantee is the point of the item.

*Alternative:* compute the position in Go from the captured tree. Impossible: the hidden siblings are not in the capture.

### 2. `nt` only when it is needed, so no format flag

`nt` is written only when the parent has two or more children with that tag. An absent `nt` in a new capture therefore means "the only one of its tag in the DOM". It needs no version marker, because the fallback gives the same answer for that case:

```
 node has nt?  --yes-->  :nth-of-type(nt)
      |
      no
      v
 count captured same-tag siblings (today's rule)
      - old capture: today's selector, unchanged (golden parity)
      - new capture: absent nt means unique in the DOM, so unique among
        the captured siblings too (captured is a subset of DOM),
        so no :nth-of-type -- the right answer
```

This keeps captures small (most nodes have no same-tag sibling) and needs no meta flag or schema major. The property goes into `schema/capture.schema.json` as an optional integer of at least 1. `internal/schema` gets `NT int \`json:"nt,omitempty"\``. `capture/1.json` keeps its URL, because the node object does not forbid additional properties and the field is optional.

### 3. One selector builder, shared

`segment` and the path walk move into a small function that takes a node's tag, id, classes, `nt` and its captured siblings (for the fallback), and returns the escaped segment. `layoutdiff.Selector` (which walks `Parent` links) and `domaudit` (which builds the path while walking down) both call it. The DOM audit keeps its own walk but stops writing its own segment form. It also adopts the same five-segment cut and anchor rule, so an audit finding and a cause on the same element carry the same string. Where the function lives is an implementation choice (a leaf package both can import, to avoid `domaudit` importing `layoutdiff`).

### 4. Escape as `CSS.escape()` does

Ids and each class are serialized with the CSSOM "serialize an identifier" algorithm, the one behind `CSS.escape()`. That algorithm:
- turns NUL into U+FFFD,
- writes control characters, and a digit at the start (or after a leading `-`), as a code point plus a space (`\31 `),
- writes a lone `-` as `\-`,
- passes ASCII letters, digits, `-`, `_` and non-ASCII through,
- backslash-escapes everything else.

Tags are written as captured: lowercase HTML names and custom-element names are already valid identifiers. The unit test compares the Go function with `CSS.escape()` run in Chrome on a table of inputs.

Source: CSSOM, "Serializing Identifiers", https://drafts.csswg.org/cssom/#serialize-an-identifier

### 5. Keep the shape of the selector

There are still two classes at most, the cut at five segments, and the stop at an id, `body` or `html`. Changing the shape would move every selector in every golden for no gain on the bug. The spec states what a cut selector guarantees (the element is among its matches), and the bundle schema's `selector` description says it.

### 6. Prove it in a real browser

A new fixture page, `testdata/planted/selector-hidden-sibling.html`, is not part of `expectations.json`: the planted gate scores hits by id, and an id on the target would end the selector before the position under test. It holds:
- a list whose first card is `display: none` in every engine, with a planted width difference on a later card, and
- a list whose second item is `display: none` in Firefox only (`.ff` rule, as in `04-missing`), with a difference on a later item.

The expected elements carry `data-expect`, which the capture does not record.

An end-to-end test captures that page and the planted suite in the engines the CI lane has. It runs the pipeline, then for each cause evaluates `document.querySelector(selector)` in the baseline page and checks two things:
- the element's border box equals the cause's baseline box within 0.1 px, and
- on the fixture, the element has `data-expect`.

## Risks / Trade-offs

- **[Risk] A merged report mixes a `0.1.0` candidate's bundle and a `0.2.0` bundle**, and the same element gets two selectors. The comment, which groups by title and selector, then lists it twice. → Mitigation: `report.yml` runs one version on every host, so this needs a hand-made merge. The `0.2.0` release notes say not to mix them.
- **[Risk] The selector change surprises a consumer who tracks selectors across runs.** → Mitigation: this is why it lands before the stable release. It is marked **BREAKING** in the proposal and goes in the `0.2.0` release notes.
- **[Risk] `extract.js` behaves differently in an engine** (Safari, WebKitGTK, WPE) when counting children. → Mitigation: it uses only `children` and `tagName` through the platform getters the script already uses everywhere. The driver conformance test, which runs `extract.js` in every engine, gains a same-tag-siblings case with a hidden one.
- **[Trade-off] The lab's data keys change.** `labels.json` selectors and the `crawl/dataset.csv` cause ids (`slugBase|kinds|selector`) differ for new captures. → The dataset joins on node ids (`SPEC-ground-truth.md`), and the selector join is only a fallback for old bundles, so existing rows stay valid. A re-crawl writes new keys.
- **[Trade-off] Captures grow a little.** `nt` appears only on nodes with same-tag siblings. That's a few bytes each, small next to `b` and the text fields, and it stays inside `MAX_NODES`.

## Migration Plan

- It ships in `v0.2.0-rc.1` with the rest of `0.2.0`. Nothing to migrate: old bundles still open in the viewer and still merge, with their old selectors.
- Rollback is reverting the change. Captures that carry `nt` still validate and still work with the old code, which ignores the field.

## Open Questions

- Should a selector that is cut at five segments grow until it is unique, using the capture to check uniqueness? It would only change the selector for deep elements. It can be added later in a `0.x` minor, since it changes selector text, and nothing in this change depends on it.
