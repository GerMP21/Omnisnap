# Tasks

## 1. Before building

- [x] 1.1 Add a task to `tasks/todo.md` for this change (backlog B2, with the acceptance criteria from specs/element-selectors/spec.md and a link to `openspec/changes/fix-selector-sibling-index/`). Set B2's **Status** in `tasks/backlog.md` to "in progress, this change, targeted at `v0.2.0`", replacing "Targeted at `0.2.0`, not blocking `v0.1.0`". Record the pairing finding (design.md, "Pairing") under B2's first open question. Verify: both files link to the change, and `go test ./test/internals/` passes.
- [x] 1.2 Write `testdata/planted/selector-hidden-sibling.html` (design decision 6) with:
  - a `div#projects-list` of seven `div.project-card`, the first with `hidden`, and a `.ff` rule that widens the third card;
  - a `ul#steps` whose second `li` is `display: none` under `.ff` only, and a `.ff` rule that changes the fifth `li`'s padding;
  - `data-expect` on the two planted elements and no `id` on them;
  - at least 50 elements, so the bot-wall rule passes without `allow-small-pages`.

  Allow the `selector-` prefix next to `audit-` and `appearance-` in `spikes/layout-diff/tools/export-planted.mjs`'s extra-page filter, with a comment naming this change. Verify: `node spikes/layout-diff/tools/export-planted.mjs --check` prints "planted fixtures match the spike", and today's binary (`omnisnap <the page> --engines chrome,firefox`) reports a selector ending in `div.project-card:nth-of-type(2)` for the third card. That's the bug, reproduced; record it in the todo task.

## 2. The capture records the DOM position

- [x] 2.1 In `assets/inject/extract.js`, count each child list's same-tag element children in the parent's loop (the light children and the open shadow root's children as separate lists, read through `childrenOf`/`shadowOf`). Pass the 1-based position into `walk()`, and set `node.nt` only when that tag occurs two or more times in the list. Document `nt` in the header comment's field list. Update `assets/inject/SHA256SUMS`. Verify: `go test ./internal/testutil/` (the SHA256SUMS check) passes.
- [x] 2.2 Add `nt` to `schema/capture.schema.json`'s node (`integer`, `minimum: 1`, description "1-based position among same-tag DOM siblings, hidden included; only when the tag repeats") and `NT int \`json:"nt,omitempty"\`` to `internal/schema/capture.go`'s `Node`. Verify: `go test ./internal/schema/...` passes. A capture with `nt` and a golden capture without it both validate against `capture/1.json`.
- [ ] 2.3 Add a same-tag-siblings case with a `display: none` first sibling to the driver conformance test (`internal/drivers/conformance_test.go`), so every engine's `extract.js` result is checked. Verify: `go test -tags browsers ./internal/drivers/... -run Conformance` passes locally with Chrome and Firefox. The CI conformance job passes on all three OSes, including Safari on macOS.
- [x] 2.4 Document `nt` in `specs/SPEC-bundle-schema.md` (capture node format) and in `specs/SPEC-capture.md` if its extract step lists node fields. Update the wiki pages that `grep -rl 'assets/inject/extract.js\|capture.schema.json\|internal/schema/capture.go' docs/internals/` finds (at least `Capture-Pipeline.md` and `Data-Contracts.md`). Add a line to `testdata/golden/README.md` saying the goldens predate `nt` and exercise the fallback. Verify: `go test ./test/internals/` passes.

## 3. One selector builder

- [x] 3.1 Add the identifier escaping (design decision 4, CSSOM "serialize an identifier") as a function in a leaf package that both `layoutdiff` and `domaudit` can import. Verify: a table test covers `md:w-1/2`, `1col`, `-1a`, `-`, `a b`, `:r1:`, a NUL and a control character, and non-ASCII. The same table run through `CSS.escape()` in Chrome (a `browsers`-tagged test, or expected values recorded from Chrome with the command in a comment) gives identical strings.
- [x] 3.2 Move the segment logic (tag, `#id` or up to two `.class`, `:nth-of-type`) into the same package: use `NT` when it is set, and fall back to counting captured same-tag siblings when it is not (design decision 2). Escape ids and classes. Make `layoutdiff.Selector` use it, and keep its five-segment cut and its stop at an id, `body` or `html`. Verify: unit tests in `internal/layoutdiff` cover the hidden first card (`:nth-of-type(2)`), one rendered `p` of two (`p:nth-of-type(2)`), an engine-specific hidden sibling (same selector computed from the A and the B tree), a node without `NT` among captured duplicates (today's selector), and a unique tag (no `:nth-of-type`).
- [x] 3.3 Run the golden gates unchanged. Verify: `go test ./internal/layoutdiff/ ./internal/triage/ ./internal/groundtruth/` passes with no golden file edited, and `go test -tags e2e ./test/e2e/... -run Planted` passes with the same recall and no new Report cause on the controls.
- [x] 3.4 Update `specs/SPEC-layout-diff.md` (the selector rule: DOM position, escaping, the five-segment guarantee, the fallback for captures without `nt`). Update `specs/SPEC-bundle-schema.md`'s description of `selector`: valid CSS, it selects the element when anchored, and it matches it when cut. Update the wiki pages that `grep -rl 'internal/layoutdiff/match.go' docs/internals/` finds (at least `Layout-Diff.md` and `Glossary.md`), adding the new package's file to the relevant page's `## Source files`. Verify: `go test ./test/internals/` passes.

## 4. DOM audit findings use the same selectors

- [x] 4.1 Replace `childPath` in `internal/domaudit/audit.go` with the shared segment function, keeping each entry's captured siblings for the fallback, and the same cut and anchor rule. Verify: unit tests in `internal/domaudit` check that the third of three zero-size `button` siblings is reported as `…button:nth-of-type(3)`, and that a finding's selector equals `layoutdiff.Selector` for the same node on a fixture capture. `go test -tags e2e ./test/e2e/... -run Audit` still passes (it may match on selectors; update its expectations only where the new form is the correct one, and say so in the commit).
- [x] 4.2 Update `specs/SPEC-dom-audit.md` (the selector is the same as a cause's) and the wiki pages `grep -rl 'internal/domaudit' docs/internals/` finds (at least `DOM-Audit.md`). Verify: `go test ./test/internals/` passes.

## 5. Proof in a real browser

- [ ] 5.1 Add an end-to-end test (`test/e2e`, `e2e` tag) that captures `selector-hidden-sibling.html` and the planted pages in the engines available (Chrome as the baseline and Firefox at least), runs the comparison, and then, for every cause, evaluates `document.querySelector(selector)` in the baseline page. It checks that the element's border box equals the cause's baseline box within 0.1 px. On the fixture it also checks that the element carries `data-expect`, and that both planted elements are reported. Verify: `go test -tags e2e ./test/e2e/... -run Selector` passes locally. The CI job that runs the e2e tests passes on the PR.
- [ ] 5.2 Re-check the original report: run the PR's binary on synaptech-site's `/work` (or a local copy) and compare with run 44777766699. Verify: the overflow on the first visible card is named `div.project-card:nth-of-type(2)` (or whatever the page has today), and pasting each Report selector into devtools selects the element whose box the report shows. Record the result in the todo task.

## 6. Hand-off to the release

- [x] 6.1 Give the `release-v0-2-0` change its release-notes line: selectors now follow the DOM and are escaped CSS, so a selector recorded from a `0.1.0` candidate may differ, and bundles from a candidate and from `0.2.0` should not be merged together. Verify: the line is in that change's release-notes list (or in its tasks, if the notes are drafted there).
- [ ] 6.2 Tick the todo task from 1.1 with the run IDs of 2.3, 3.3 and 5.1. Mark B2 done in `tasks/backlog.md`. Verify: `go test ./...` passes and the boxes are ticked with evidence.

## Workflow follow-up

- Archive the change once it has shipped in `v0.2.0-rc.1` and its criteria are ticked: `openspec archive fix-selector-sibling-index`, which creates `openspec/specs/element-selectors/spec.md`.
