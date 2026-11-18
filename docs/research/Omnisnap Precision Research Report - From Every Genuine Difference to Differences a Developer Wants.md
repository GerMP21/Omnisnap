# Omnisnap Precision Research Report: From "Every Genuine Difference" to "Differences a Developer Wants"

**Bottom line:** Omnisnap's precision problem is mostly not a detection problem. It comes from what Omnisnap reports and how it counts. Report one *cause* per mechanism, ranked by its *visible consequence* (clipping, overlap, protrusion, wrapping, moved interactive targets, broken alignment, content change inside a bounded container). Downgrade systematic, explainable engine differences (fractional line-height rounding, font-metric heights, form-control defaults, text advance) into an "expected engine differences" tier with a fix. On Appendix A this plausibly moves strict precision from 25% to roughly 60–80% at the cause level. That is a projection, to be measured with the experiments below; the rest of the gap is blocked by label noise rather than by the algorithm.

Evidence tags: **[P]** published evidence (paper, spec, engine bug tracker or source), **[V]** vendor documentation or production practice, **[I]** analyst's inference.

## TL;DR

- **Change the unit and the gate, not the detector:** report per cause/mechanism, ranked by visible consequence, and count only findings with a consequence feature toward precision. X-PERT's relative-layout comparison reached 76% precision / 95% recall while collapsing "domino" duplicates [P], and DOM-only layout checkers produce "non-observable issues" that a visual-verification step removes (VISER) [P].\[1\]\[2\] This is the largest gain at low cost (Go plus a few injected-JS fields).
- **Codify expected engine differences as a knowledge base with signatures and fixes.** The key entry is fractional line-height. WebKit has floored fractional line-heights since Safari 14.1 (WebKit bug 225695, still NEW) and only began removing integral snapping in March 2024; Gecko keeps fractional app units (1/60 px) and has an open 2008 bug about uneven line spacing [P].\[3\]\[4\]\[5\] That one entry explains 7 of the 28 Appendix A findings and turns the terminal case into one explained cause with a fix ("use a whole-pixel line-height").
- **Fix measurement before tuning the model:** 28 labels give a 95% Wilson interval of roughly 13–43% around the 25% estimate, so no method can currently be shown to hit 80%. Grow to ≥100 cause-level labels from ≥2 raters (≥246 for a ±5-point margin), add CSS-mutation recall testing, and only then train a small logistic-regression ranker in pure Go [I].

## Key Findings

1. **The problem is triage, not detection, and the literature has seen it before.** Browserbite's image comparison had 98% recall but only 66% precision until a learned post-classifier raised precision to 96.4% (recall 88.6%) [P, Semenenko, Dumas & Saar, ICSM 2013]. The same paper cites CrossCheck at 36% precision and WebDiff at 21% [P].\[6\] Omnisnap (10/10 planted recall, 0 control findings, 25% strict precision) has the same shape: an accurate but oversensitive detector without a relevance layer.
2. **Relative layout beats absolute geometry for human relevance.** In a 100-site study, X-PERT's authors "tended to recognize a structure XBI through a difference in the relative position of an element with respect to its immediate neighbors, rather than a difference of its absolute size or position", and absolute size/position "resulted in many false positives" [P, ICSE 2013].\[1\] A later classifier study on 72 responsive web apps (Watanabe, dos Santos & de Oliveira, ACM TWEB 17(4), October 2023) frequently selected X-PERT relative-position features. Its combined model "reported the highest F1-score for detecting external layout failures (0.65) and internal layout failures (0.35)" [P]. Applitools' Layout match level, recommended for cross-environment baselines, "only flags differences to the overall structure and relative positioning of elements" [V].\[7\]\[8\]
3. **Size cannot separate real from trivial here; consequences plausibly can.** The real terminal drift (24 px) and the false HN drift (32 px) share a mechanism and magnitude. What plausibly separates them is whether the drift changes what is visible inside a bounded container or breaks an alignment, versus merely lengthening a page in normal flow [I].
4. **DOM-based layout checks over-report non-observable issues, and humans disagree about them.** VISER automatically classified 117 ReDeCheck-reported failures on 20 pages by toggling element opacity and comparing pixels. It disagreed with the prior manual classification on 28 (24%), including 3 correctly reclassified as false positives [P, Althomali, Kapfhammer & McMinn, ICST 2019].\[2\] This is direct precedent for failure case B (p#mwnQ, "no visible change") and case E (label noise).
5. **Fractional line-height is engine-divergent and in flux.**
   - Blink and WebKit lay out in 1/64 px LayoutUnits; Gecko uses 1/60 px app units [P, WebKit LayoutUnit wiki; Mozilla2:Units].\[9\]\[10\]\[11\]\[12\]\[13\]\[14\]
   - WebKit regressed to flooring fractional line-heights in Safari 14.1 (bug 225695: 16 px × 1.2 = 19.2 "but is now resulting 19") [P].\[3\]
   - Commit 275601@main (3 March 2024) is "in preparation for removing integral (vertical) snapping" [P].\[5\] The Safari 27.0 release post (17 September 2026) says "inline layout places elements with subpixel precision" [V].\[15\]
   - Gecko bug 442139 (uneven line-height for non-integer values) remains open [P].\[4\]
   - **Consequence:** the synaptech WebKitGTK 2.50 findings may not reproduce on real Safari 27; check per engine version [I].
6. **Commercial tools reduce noise by triage and grouping and publish almost no precision numbers.**
   - BrowserStack launched Percy's Visual Review Agent on 14 October 2025. Its release note claims "reducing review time by 3x & filtering out up to 40% of visual changes"; the press release says "up to 3x" [V].
   - Chromatic's docs say its "default threshold is .063, which balances high visual accuracy with low false positives (for example, from artifacts like anti-aliasing)", and anti-aliased pixels are ignored by default [V].
   - Argos uses deterministic odiff with multi-pass thresholds and clustering, plus flaky fingerprints and auto-ignore [V].\[16\]\[17\]
   - None publishes cross-engine precision/recall.
7. **Vision-language models are not a substitute.** XBIDetective (Grewal et al., University of Alberta and Mozilla, arXiv 2512.15804, December 2025) used Gemini 2.0 Flash on 1,052 Bugzilla/WebCompat reports. Fine-tuned it reaches 72% precision / 59% recall; in "thinking" mode, 69% / 48%; the base model only 57% precision [P]. That is below target, non-deterministic, and incompatible with the 15 MB / no-Python constraint.
8. **Clock control works on two of three protocols.** CDP has `Emulation.setVirtualTimePolicy`, which is experimental and Chromium-only [V].\[18\] WebDriver BiDi's `script.addPreloadScript` runs "before a new document's own scripts" [P].\[19\] WebDriver Classic has nothing comparable, so real Safari needs a network-level workaround [I].

## Details

### R1. Reporting unit and ranking

**Answer:** the unit is a **cause**, meaning one mechanism at one root element (or one style rule), with its instances (×N) beneath it. Causes are ranked by **visible consequence**, not pixels changed [I, grounded in P].

- **Why cause:** X-PERT targeted "only one XBI per actual error" because "the movement of a single element on a page can have a domino effect on the positions of all elements below it" [P].\[1\] The owner-site result (1/6 per finding, 1/1 per cause) is the same effect. The existing cause/effect rules do the hard part; what is missing is a mechanism-level merge across unrelated elements (all 12 synaptech line-pitch findings are one mechanism) [I].
- **Why not visual region:** cross-engine rasterization noise (DSSIM 0.027–0.045 on bug-free pages) makes region boundaries unstable [I].
- **Why not defect type alone:** ReDeCheck's classes (collision, protrusion, wrapping) are the right *consequence vocabulary*, but one cause can produce several types. Use type as a ranking feature [I].

**Ranking (deterministic, ties broken by document order):** `impact = max over instances (consequence_weight × visibility) × (1 + log2(instances))`.

- `visibility` is 1.0 in the first viewport (baseline y < 800), 0.7 within 3 viewports, and 0.4 otherwise.
- The log term rewards breadth without letting ×29 trivia dominate.

**Metrics:**
- **Per-cause strict precision**: the headline number, gated at ≥ 80%.
- **Precision@3 per page**: a PR comment shows about 3 items. Gate at ≥ 85%.
- **nDCG@5** with gains real = 2, trivial = 0.5, false = 0. It rewards putting the terminal above the 11 paragraphs.
- **Finding-level precision**: kept only for continuity.

### R2. Relevance and severity signals

| Signal | Evidence | Strength |
|---|---|---|
| Ink overlap / collision | ReDeCheck "element collision" class; VISER showed overlaps of box regions are often non-observable, so test *ink*, not boxes [P] | Strong for ink, weak for boxes\[2\] |
| Protrusion / horizontal scroll | ReDeCheck element and viewport protrusion [P]; already severity 3 | Strong |
| Wrapping change | ReDeCheck "wrapping" class [P]; mwUw real | Medium (also a top trivial source in reference lists) |
| Relative alignment broken | X-PERT alignment graph and human-perception observation [P]; Applitools Layout [V] | Strong\[1\] |
| Interactive targets moved/resized/covered | Obscured links/buttons cause "loss of functionality" (VISER/ReDeCheck) [P]; go.dev buttons real | Medium\[2\] |
| Text truncated / clipped | Planted "clipped text" bug; no cross-engine study with numbers found | Medium (evidence weak) |
| Position / above the fold, area | Browserbite used ROI X/Y and size as classifier features [P] | Weak alone\[6\] |
| Repetition | No published evidence; HN ×29 trivial | Use for grouping, not relevance |
| Contrast / salience | No cross-browser evidence found | Missing; do not build first |

The model itself is specified in Deliverable 4.

### R3. What to compare

- **Relative layout should be primary** for findings that are not presence, visibility or overflow [P: X-PERT; V: Applitools Layout].
  - For each changed parent, build X-PERT's alignment edges among its children: above/below/leftOf/rightOf; top/bottom/left/right/centre-aligned; contained-in.\[1\]
  - Report only edges whose relation changes. Size and position deltas become *explanations*, not findings.
- **ReDeCheck's classes become consequence features**, not separate detectors [P].
- **Perception-based comparison should be a verifier only**, in the VISER manner (does the element's ink change anything visible?) [P + I].
- **Better tree matching (GumTree, RTED/APTED) is not worth it now.** Matching is 97.3–100%, and none of the 12 non-real Appendix A rows stems from a mismatch [I]. Revisit only if the unmatched 0.3–2.7% produces "only-in" false findings (Deliverable 6, step 0).

### R4. Expected engine differences

The full knowledge base is in Deliverable 5. Commercial practice is to coarsen the comparison (Applitools Strict / Layout / Content / Ignore Colors / Exact) or to ignore regions [V].\[20\]\[21\] Omnisnap has geometry and computed styles, so it can *recognise the mechanism* and turn it into one explained cause with a fix [I].

### R5. Determinism and noise floor

| Protocol / engine | Clock before page scripts | Identical content | Recommendation |
|---|---|---|---|
| CDP (Blink) | `Emulation.setVirtualTimePolicy` (advance / pause / pauseIfNetworkFetchesPending, budget): experimental, Chromium-only [V]; `Page.addScriptToEvaluateOnNewDocument` [I] | Fetch-domain interception for replay [I] | Use the shared JS fake clock, so all engines run the same mechanism\[18\]\[22\] |
| BiDi (Gecko) | `script.addPreloadScript` [P] | `network.*` interception [V] | Same fake-clock script as a preload script\[19\]\[23\] |
| Classic (Safari, WebKitGTK) | None. WebKit PR #74706 adds BiDi preload scripts to WebKitGTK, maturity unverified [V] | Add Cookie only | Local record/replay HTTP(S) proxy that injects `<script src=omnisnap-clock.js>` as the first child of `<head>` [I]\[24\] |

- **The fake clock:** Playwright's official Clock docs (playwright.dev/docs/clock, added in v1.45) list the surface to override: `Date`, `setTimeout`, `clearTimeout`, `setInterval`, `clearInterval`, `requestAnimationFrame`, `cancelAnimationFrame`, `requestIdleCallback`, `cancelIdleCallback`, `performance` and `Event.timeStamp`. The install call "MUST occur before any other clock related calls" [V]. Ship a minimal implementation of the same surface, and also seed `Math.random` and `crypto.getRandomValues` for client-side A/B assignment [I].
- **A/B seeding:** record once in Chrome and replay identical responses, including `Set-Cookie`, to every engine [I].
- **Safari HTTPS:** proxy injection needs a locally trusted CA. That is acceptable on macOS CI runners and intrusive on laptops, so make it opt-in (`--replay`) [I].
- **Noise floor:** with 2 runs, an element that varies with per-run probability *p* is caught only with probability 2p(1−p) ≤ 0.5.
  - Use 3 runs per engine when the clock is uncontrolled, 2 when it is controlled.
  - Report a cross-engine delta only if it exceeds `max(tolerance, 2 × noise_A, 2 × noise_B)`, where noise is the maximum same-engine |Δ| [I].
- **Stripe-class pages:** if cross-engine identity-token divergence exceeds 30% of a subtree, emit one "page state differs" cause for it [I].

### R6. Root-cause explanation

- **Computed-style diff on cause candidates only.** A second injected call reads a whitelist via `getComputedStyle`:
  - `line-height`, `font-size`, `font-family`, `font-weight`, `letter-spacing`, `padding-*`, `margin-*`, `gap`, `width`, `height`, `box-sizing`, `display`, `flex-*`, `grid-template-*`, `white-space`, `overflow-wrap`, `word-break`, `hyphens`, `text-wrap`, `text-overflow`, `-webkit-line-clamp`.
  - Go normalises the values. Same style but different geometry means an engine-intrinsic difference (look it up in the KB). Different style means an author, UA or feature-support cause, named directly.
  - All 10 planted bugs were UA-conditional CSS, so all will show a computed-style difference, which protects planted recall [I].
- **Cascade tracing without CDP:** walk `document.styleSheets` and test `el.matches(selectorText)` for whitelisted properties. Cross-origin sheets throw, so report "rule in cross-origin stylesheet" [I]. CDP's `CSS.getMatchedStylesForNode` is Blink-only and must stay out of the core path.
- **Counterfactual fix verification instead of XFix-style search.** XFix's search-based repair is reported as resolving 86% of X-PERT-detected XBIs (ISSTA 2017 technical paper); a later citation of the XFix tool paper by Grewal et al. (2025) gives "a median of 93%" [P]. Either way it needs many re-renders. Omnisnap only needs to inject one KB fix (e.g. `#terminal-output{line-height:23px}`) into both engines, re-measure the subtree, and mark the fix "verified" if the delta falls under tolerance. This costs about 100–300 ms per cause and works on all three protocols [I].

### R7. Pixel metrics for the graphics layer only

| Metric | Fit for cropped img/svg/canvas/video | Cost per 300×300 crop | Licence / Go availability | Verdict |
|---|---|---|---|---|
| SSIM (current) | Structural change; rasterization-sensitive | < 5 ms [I] | Trivial in Go | Keep |
| MS-SSIM | Tolerates resampling/blur differences | ~2–3× SSIM [I] | ~150 lines of Go | Use for raster images |
| FLIP | Perceptual, good error maps | ~10–30 ms CPU [I] | BSD-3-Clause (NVIDIA) [V]; Rust port flip-rs; no Go port found | Optional later; port feasible\[25\]\[26\] |
| LPIPS | Learned CNN | Heavy | Needs neural runtime | Plugin only |
| butteraugli | Perceptual, accurate | Slow | Apache-2.0 C++ (per brief, not re-verified); cgo breaks static cross-compiles | Exclude from core |

Before running any pixel metric, compare the crop's intrinsic state (`naturalWidth/Height`, `currentSrc`, SVG `viewBox`, canvas size). A different `currentSrc` (srcset choice) is a real, explainable cause on its own [I].

### R8. Evaluation methodology

- **Wilson 95% intervals:**
  - 7/28 = 25% → about 13–43%.
  - 21/45 = 46.7% → about 33–61%.
  - Showing a *lower bound* ≥ 80% needs about 88/100 real.
  - A ±10-point margin at p = 0.8 needs n = 62; ±5 points needs n = 246 [I].
- **Inter-rater agreement:** use Cohen's κ for 2 raters and Krippendorff's α (ordinal false < trivial < real) for 3 or more.
  - Browserbite used 8 judgments per item from 40 raters with a ≥5/8 majority, reporting inter-rater reliability of 0.947 on a binary question [P].\[6\]
  - VISER's 24% disagreement shows three-way labels are much noisier [P].\[2\]
  - Target κ/α ≥ 0.6 on a 60-item overlap set [I].
- **Synthetic bug injection:** Walsh, Kapfhammer & McMinn evaluated layout-regression detection with CSS mutation operators [P, STVR 2020].\[27\] Omnisnap can inject a mutation stylesheet into one engine and stratify by magnitude (2, 5, 10, 30 px) to get a recall-versus-magnitude curve [I].
- **Reusable datasets:** X-PERT released its artifacts (14 subjects) [P];\[1\] ReDeCheck and WebSee are on GitHub [P/V]. These are 2013–2017 pages built for IE and old Firefox, useful as mutation hosts but not as modern labeled ground truth. No public labeled Blink/Gecko/WebKit XBI dataset was found [I].

### R9. Prior art

See Deliverable 3.

---

## Deliverable 1: Executive summary (ranked by expected precision gain ÷ cost)

| # | Recommendation | Expected effect | Cost | Confidence | Fixes (cases / Appendix A rows) |
|---|---|---|---|---|---|
| 1 | **Cause-level reporting, mechanism merge, impact ranking**; gate per cause plus P@3 | Owner site 1/6 → 1/1 observed; ×29, ×11 collapse | Low (Go, 1–2 days) | High | A, C: synaptech ×5, HN upvote ×29, unpin ×2 |
| 2 | **Consequence gate (day-1 heuristic)**; non-consequence findings go to an uncounted "info" tier | Removes most of 16 trivial and 3–4 of 5 false | Low-medium (≈8 JS fields; 3–5 days) | Medium-high | A, B, C: code block, div.fixed, navbar ×2, span#mwA74, navbox li, td h+3, HN tbody |
| 3 | **Knowledge base of expected engine differences** with signatures and fixes | Mechanism findings become 1 explained cause each; supplies the fix | Medium (1 week) | High for line-pitch; medium otherwise | A, B, C: synaptech rows, HN tbody, HN input |
| 4 | **Ink/observability verification** for children-moved/position | Removes "no visible change" | Low-medium | Medium | B: p#mwnQ; A: a#mwAsQ ×5, span#mwA74 |
| 5 | **Computed-style diff + counterfactual fix verification** | Correct blame, verified fixes; protects planted recall | Medium (1 week) | Medium-high | B: wrong-element cases; C: terminal fix |
| 6 | **Label programme** (≥2 raters, rubric, Wilson CIs, active learning; logistic regression after ≥150 labels) | Makes 80% testable; reduces E | Low engineering, ongoing human time | High | E: cite_note-31 vs -36 |
| 7 | **Page-state determinism** (shared fake-clock preload, replay proxy for Safari, 3-run noise floor) | Fixes Stripe-class pages | High (2–3 weeks) | Medium | D: Stripe (17 labels) |

---

## Deliverable 2: Technique dossiers

### Dossier 1: Cause-level reporting and impact ranking

- **What it is:** merge findings sharing (mechanism, engine pair, root element or style rule) into one cause with instances, ranked as in R1.
- **Evidence:** X-PERT's one-report-per-error design [P]; grouped review in Percy and Argos [V]; owner-site 1/1 per cause.\[1\]\[17\]
- **Pipeline:** group → rank → report.
- **Appendix A:**
  - **Fixes:** all synaptech line-pitch rows become one cause ("WebKit lays out fractional line-height as whole pixels"), with #terminal-output (−24 px in a bounded panel) as the headline, so the cause is real. HN upvote ×29 becomes one instance list. Unpin ×2 merge.
  - **Breaks:** no recall loss. A real instance could be hidden under a trivial headline; always show the top-impact instance.
- **Sketch (Go):**
  - `causeKey = hash(mechanism, engineB, style rule or KB id, consequence class)`.
  - The report gains `causes[] {id, mechanism, explanation, fix, verified, impact, instances[]}`.
  - No new JS is needed.
- **Runtime:** milliseconds.
- **Risks:** over-merging distinct bugs (also key on the selector stem); old labels must be re-aggregated.
- **Experiment:** re-aggregate the 28 + 6 labels. A cause is real if any instance is real; false if its headline instance is false.
  - **Pass:** per-cause precision ≥ per-finding precision on every site.
  - **Pass:** causes ≤ 0.6 × findings.
  - **Pass:** planted bugs remain 10/10 single causes.
  - **Pass:** 0 causes on control pages.

### Dossier 2: Consequence gate (day-1 relevance heuristic)

- **What it is:** a finding counts only if it has at least one consequence:
  - presence or visibility flip;
  - new clipping or truncation;
  - new ink overlap;
  - protrusion out of a clipping ancestor or the viewport;
  - content change inside a bounded container;
  - line-count change;
  - interactive target moved ≥ 4 px or newly covered;
  - alignment relation changed.

  Everything else goes to a collapsed, uncounted "info" tier.
- **Evidence:** ReDeCheck failure classes [P]; X-PERT relative layout [P]; Applitools Layout [V].
- **Pipeline:** classify → rank.

**Predicted effect on Appendix A rows (to be verified):**

| Row | Label | Consequence present? | Predicted tier |
|---|---|---|---|
| span#mwUw | real | lines 1→2 | Report |
| terminal-output | real | bounded panel, content −24 px (verify) | Report |
| go-dev Playground-buttons x +5.9 | real | interactive moved ≥ 4 px | Report |
| python-org main-content | real | alignment edge with sidebar? | Report if alignment changed |
| cite_note-31 (−61.7), cite_note-12 (−15.3), li#mwAUU (y +2.1) | real | likely none | **Risk: info** |
| cite_note-36, cite_note-4 ×2, navbox th, navbar ×2, unpin ×2, span#mwA74, a#mwAsQ ×5 | trivial | none / ink unchanged | Info |
| tailwind code (−36), div.fixed (−30.7) | trivial | inside scroll container, no scroll-state change / no ink (verify) | Info |
| synaptech rows (5) | trivial | page flow only | Info, merged under the terminal cause |
| HN upvote ×29 (w −4) | trivial | interactive, not moved | Info, only if the rule uses "moved", not "resized %" |
| HN tbody +32, HN input −9, navbox li +3.9, td +3, p#mwnQ | false | flow drift / form-control KB / none / ink unchanged | Info |

- **Projection:** about 4–5 reported causes, nearly all real, so 80–100% per cause, while 2–3 real rows are demoted [I]. Whether those are true losses or label noise (cite_note-31 real at 61.7 px vs cite_note-36 trivial at 49.7 px, with no visible feature separating them) is what Dossier 6 must resolve.
- **Sketch (injected JS, standard DOM, all three protocols):**
  - `lr`: own-text per-line rects (cap 50), taken from the existing `Range.getClientRects()` pass.
  - `clip`: text-overflow, -webkit-line-clamp, sw > cw under overflow hidden.
  - `ink`: the element paints itself (visible text, background, border, shadow, or a graphics tag).
  - `ia`: interactive (a[href], button, form controls, summary, interactive roles, tabindex ≥ 0).
  - `sc`: is a scroll container, plus its scroll state.
  - Go, bounded container: the nearest ancestor with equal height across engines (±1 px) that clips or scrolls, where descendant content differs by ≥ 1 line pitch or ≥ 8 px.
  - Go, ink overlap: new intersections between ink boxes of non-ancestor pairs within the changed parent.
- **Runtime:** about +10–20% extraction time; Go well under 1 s per page [I].
- **Risks:** losing subtle real findings (li#mwAUU type). Mitigated by the info tier and by the learned model.
- **Experiment:** run the 11 sites, 61 controls and the planted suite with the gate on.
  - **Pass:** per-cause precision ≥ 0.75.
  - **Pass:** no planted bug drops to info.
  - **Pass:** 0 control findings.
  - **Pass:** ≤ 2 of the 7 real rows demoted.

### Dossier 3: Knowledge base of expected engine differences

- **What it is:** versioned signatures (Deliverable 5), each with a detection rule, explanation, fix and default tier. A match turns N findings into one explained cause, which is reported only if a consequence (Dossier 2) is also present.
- **Evidence:**
  - WebKit bug 225695 [P], commit 275601@main [P], Safari 27.0 notes [V].\[3\]\[5\]\[15\]
  - Mozilla bug 442139 [P].\[4\]
  - LayoutUnit 1/64 vs app units 1/60 [P].\[10\]\[11\]\[13\]
  - A production fix in a public repo (October 2026): `line-height: round(calc(var(--leading) * 1em), 1px)` inside `@supports`, which changed most screenshot heights by "1 to 4px" [V].\[28\]
- **Pipeline:** classify → group → report.
- **Appendix A:**
  - **Fixes:** synaptech ×5 (K1), HN tbody (K2), HN input (K8), unpin and td h +3 (font metrics), navbar/navbox widths (K4).
  - **Breaks:** a planted font-change bug could be mistaken for K4. Guard: KB entries require identical computed font properties.
- **Sketch:**
  - JS per text element: `lh` (computed line-height), `fs`, `ff0`, `fw`, `ws`, `ow`, `hy`, `tw2`.
  - JS meta: `CSS.supports` for `text-wrap: pretty|balance` and `round()`.
  - Go: line pitch = median gap between consecutive `lr` tops.
  - Go: K1 fires when `lh` is equal across engines, the pitch differs by 0.05–1.0 px, `ln` is equal, and Δh ≈ lines × Δpitch ± 2 px.
- **Runtime:** negligible.
- **Risks:** KB rot as engines change (Safari 27 may retire K1). Add per-engine-version fields and a CI job that re-validates each entry on a fixture page.
- **Experiment:** 12 minimal fixture pages, one per entry, in all engines.
  - **Pass:** 12/12 matched and 0 KB matches on planted bugs.
  - **Pass:** synaptech's 6 line-pitch rows map to one cause, with the terminal ranked first.
  - **Pass:** HN tbody maps to K2.

### Dossier 4: Ink / observability verification

- **What it is:** before reporting children-moved or position findings, verify that *painted* content moved.
  - (a) Geometry test: own-text line rects or ink children moved beyond tolerance.
  - (b) Optional VISER-style probe within one engine: crop with `opacity:0` and with opacity restored; identical crops mean the move is unobservable.
- **Evidence:** VISER classified all 117 reports, reclassified 3 as false positives, and ran in seconds [P].\[2\]
- **Pipeline:** classify (before cause/effect).
- **Appendix A:**
  - **Fixes:** p#mwnQ (false), a#mwAsQ ×5, span#mwA74.
  - **Risk:** python.org main-content (real, no own delta). The test must consider children's ink.
- **Sketch:**
  - JS: `ink` and `lr`.
  - Probe: `__omnisnapProbe(idx)` toggles opacity, waits 2 ticks via the saved `__omnisnapRAF`, and the driver takes a viewport screenshot.
  - Cap at 10 probes per page.
- **Runtime:** the geometry test is negligible; each probe takes about 200–400 ms [I].
- **Risks:** compositing changes; scrolling under WebDriver Classic could re-trigger lazy loading (timers are already frozen).
- **Experiment:**
  - **Pass:** p#mwnQ and a#mwAsQ demoted.
  - **Pass:** go-dev, python-org and all planted bugs retained.

### Dossier 5: Computed-style diff and counterfactual fix verification

- **What it is:** diff whitelisted computed styles. If they differ, blame the topmost element whose own value differs. Otherwise consult the KB. Then inject the fix in both engines and re-measure.
- **Evidence:** XFix's layout-deviation fitness resolved 86% of X-PERT XBIs in the ISSTA 2017 technical paper (Grewal et al., 2025, cite the XFix tool paper as "a median of 93%") [P], which validates the measure-after-change loop. A one-shot check suffices here [I].
- **Pipeline:** extract (2nd pass) → classify → report.
- **Appendix A:**
  - **Fixes:** wrong-element blame (a cell whose text wrapped instead of `td` h +3).
  - **Fixes:** the terminal gets "line-height 22.75px → 22px in WebKit; fix: leading-[23px] (verified: Δh 24 → 0)".
  - **Breaks:** none expected.
- **Sketch:**
  - JS: `__omnisnapStyles(indices, props)`; `__omnisnapApply(cssText)` inserts `<style id=omnisnap-fix>`.
  - extract.js gains a `root` parameter, so only the subtree is re-measured.
  - Go: normalisation and a blame precedence table.
- **Runtime:** about 300 ms per verified fix per engine; cap at 5 per page [I].
- **Risks:**
  - Serialization differences (font-family quoting, colour formats) need strict normalisation.
  - An unverified fix is neutral, not negative evidence.
- **Experiment:**
  - **Pass:** 10/10 planted bugs name the planted property.
  - **Pass:** synaptech's fix verifies (Δh ≤ 2 px).
  - **Pass:** 0 spurious style diffs on the 61 controls.

### Dossier 6: Label programme, learned ranker, active learning

- **What it is:**
  - A cause-level rubric (real / trivial / false / can't tell) and 2–3 raters, who see crops and consequence features.
  - A pure-Go L2 logistic regression on ≤ 15 features once ≥ 150 cause labels exist.
  - Uncertainty sampling to pick the next 20 labels.
- **Evidence:**
  - Browserbite's classifier lifted precision from 66% to 96% [P], but on 1,200 items × 8 judgments. Its decision tree still reached 84% precision at 79% recall [P], so simpler models capture much of the gain.\[6\]
  - VISER's 24% manual disagreement motivates multiple raters [P].\[2\]
- **Pipeline:** rank; offline training, with weights embedded as versioned JSON (deterministic).
- **Appendix A:** addresses E. If raters disagree on cite_note-31 vs -36, the item is ambiguous and excluded.
- **Sketch:**
  - Leave-one-site-out cross-validation, since instances within a site correlate.
  - Threshold: maximise recall subject to CV precision ≥ 0.85.
  - Active learning: choose p ∈ [0.35, 0.65], ≤ 3 per site per round.
  - Every 5th pick is random, and precision is reported on random picks only.
- **Runtime:** microseconds.
- **Risks:**
  - Overfitting to 11 sites (target 40).
  - Learned weights could demote planted bugs, so presence, visibility and overflow are hard-coded to always report.
- **Experiment:** 100 cause labels from 2 raters on 40 sites.
  - **Pass:** α ≥ 0.6.
  - **Pass:** heuristic Wilson lower bound ≥ 0.70.
  - **Pass:** the model beats the heuristic on leave-one-site-out nDCG@5 by ≥ 0.05 with no recall loss.

### Dossier 7: Page-state determinism

- **What it is:** a shared `omnisnap-clock.js` (fake Date, timers, rAF, `performance.now`, seeded randomness) installed before page scripts:
  - CDP: `Page.addScriptToEvaluateOnNewDocument`.
  - BiDi: `script.addPreloadScript`.
  - Classic: an opt-in record/replay proxy that injects the script into HTML.

  Record once in Blink and serve identical bytes to all engines. Use 3 runs and a noise floor where the clock is uncontrolled.
- **Evidence:** BiDi preload semantics [P]; CDP virtual time is experimental and Chromium-only [V]; Playwright installs its clock before navigation [V]; Argos recommends freezing time and fixing flakiness at the source [V].\[18\]\[19\]\[29\]\[30\]\[31\]
- **Pipeline:** capture → stabilize.
- **Appendix A:** no effect on the 28 rows. Fixes Stripe's 17 labels (D), for example "children-differ" on animated counters.
- **Sketch:**
  - Go: an embedded MITM proxy (stdlib `httputil` plus a generated CA) and a HAR-like store.
  - Inject `<script>` after `<head>`.
  - Strip CSP only in replay mode, and disclose it in the report.
- **Runtime/complexity:** largest item (2–3 weeks).
- **Risks:** HTTPS trust on macOS; proxy detection; service-worker timers are not faked (also a Playwright limitation [V]); relaxed CSP changes behaviour.\[32\]
- **Experiment:** Stripe home, 5 sessions per engine.
  - **Pass:** same-engine identity divergence < 1%.
  - **Pass:** no "page state differs" warning.
  - **Pass:** ≤ 10 findings at ≥ 80% per-cause precision.

---

## Deliverable 3: Prior-art table

| Tool / paper | Year | Comparison method | Noise handling | Reported precision / recall | Availability |
|---|---|---|---|---|---|
| WebDiff (Roy Choudhary et al., ICSM) | 2010 | DOM + screenshot regions | Variable-element detection | ~21% precision (as cited by Browserbite) [P] | Prototype\[6\] |
| CrossCheck (ICST) | 2012 | Crawl + DOM/visual features + ML | ML classifier | ~36% precision [P, via Browserbite] | Prototype\[6\] |
| X-PERT (ICSE) | 2013 | Relative layout (alignment graph), text diff, χ² histograms on leaves | Relative layout; one report per error | **76% P / 95% R**, 14 subjects [P] | Artifacts released\[1\] |
| Browserbite (ICSM; SPE) | 2013 / 2016 | Screenshot ROI segmentation + ML | NN / tree on 1,200 pairs × 8 raters | Plain 66/98; tree 84.4/79.2; **NN 96.4/88.6**; Mogotest 75/82 [P]; F > 90% on 140 pages [P] | Commercial (defunct)\[6\]\[33\] |
| WebSee (ICST) | 2015 | Oracle image vs page, CV, rendering maps | Exclusion regions | Detection 100%, localisation 93% (slides) [P] | GitHub\[34\] |
| XFix (ISSTA) | 2017 | Search-based CSS repair | Layout-deviation fitness | 86% of X-PERT XBIs fixed (technical paper); median 93% as cited for the tool paper by Grewal et al. 2025; 99% of human-observed [P] | Research |
| ReDeCheck (ISSTA) | 2017 | Responsive layout graph; RLF classes | Manual TP/FP/NOI | 33 RLFs in 16 of 26 pages [P] | GitHub\[35\]\[36\] |
| VISER (ICST) | 2019 | Opacity toggling + pixel check | Removes non-observable issues | All 117 classified; 28 differ from manual [P] | Research\[2\] |
| VERVE (STVR) / LayoutDR (ICST) | 2021 / 2022 | Visual classification / repair of RLFs | Automated | VERVE less subjective (no numbers retrieved); LayoutDR 92% preferred repairs [P] | Public / research\[37\]\[38\] |
| ML layout XBI classifier (Watanabe, dos Santos & de Oliveira, TWEB 17(4)) | 2023 | DOM features incl. X-PERT relative, 72 responsive apps | Feature selection | F1 0.65 external, 0.35 internal [P] | Paper |
| XBIDetective (Grewal et al., arXiv) | 2025 | Vision-language model (Gemini 2.0 Flash), 1,052 Bugzilla/WebCompat reports | Fine-tuning | 72/59 fine-tuned; 69/48 thinking; base 57% P (42% accuracy) [P] | Paper |
| Applitools Eyes | current | Visual AI, match levels | Layout level for cross-environment; regions | None published [V] | Commercial\[8\] |
| Percy | current | Pixel diff + Visual Review Agent | "Up to 40%" filtered; 3× review claim | None [V] | Commercial\[39\] |
| Chromatic | current | Pixel diff | diffThreshold 0.063; anti-aliasing ignored | None [V] | Commercial\[40\] |
| Argos | current | odiff, multi-pass, clustering | Flaky fingerprints, auto-ignore | None [V] | Open source + SaaS\[16\]\[17\] |
| Happo, Lost Pixel, BackstopJS, SmartUI, BrowserStack | current | Pixel thresholds (e.g. BackstopJS misMatchThreshold) | Thresholds, ignore regions | None found [I] | Mixed |

**Takeaway:** the only peer-reviewed cross-browser precision ≥ 76% came from relative-layout comparison with one report per error (X-PERT), or from a classifier trained on a large multi-rater set (Browserbite).\[1\]\[6\] Omnisnap should adopt the first now and a small version of the second later [I].

---

## Deliverable 4: Relevance model specification

| # | Feature | Source | Type |
|---|---|---|---|
| f1 | presence / visibility flip | existing | binary |
| f2 | new clipping / truncation | new `clip` | binary |
| f3 | new ink overlap with a non-ancestor | new `lr`, `ink` | binary |
| f4 | protrusion beyond a clipping ancestor or viewport / horizontal scroll | existing `o`, `b` | binary |
| f5 | bounded-container content change (≥ 1 pitch or ≥ 8 px) | existing + rule | binary |
| f6 | own-text line-count change | existing `ln` | int |
| f7 | interactive target moved ≥ 4 px or covered (`elementsFromPoint` at centre) | new `ia` | binary |
| f8 | alignment relations changed | Go from `b` | count |
| f9 | above the fold | existing | binary |
| f10 | log ink area affected | `lr` | real |
| f11 | KB signature matched | Dossier 3 | binary (negative) |
| f12 | page-flow-only drift | cause/effect rules | binary (negative) |
| f13 | inside scroll container, scroll state unchanged | new `sc` | binary (negative) |
| f14 | computed-style difference on the cause element | Dossier 5 | binary |
| f15 | instance count | grouping | log count |

**Day-1 score (deterministic):**

`score = 3·(f1+f2+f3+f4+f14) + 2·f5 + 2·f7 + min(f8,2) + [f6≠0] + [f6≠0 ∧ (heading ∨ label ∨ interactive ∨ ln ≤ 3)] + f9 − 2·f11 − 2·f12 − f13`

- **Tiers:** report when score ≥ 3; info when 1 ≤ score < 3; drop when ≤ 0.
- **Hard rules:**
  - f1, f2 and f4 always report. This protects the missing-banner, overflow and clipped-text planted bugs.
  - f14 on a layout property always reports. This protects the width, padding, order, font, gap and SVG planted bugs.
- **Why these weights [I]:** 3 for consequences that literature and planted bugs agree are defects; 2 for context-dependent consequences; 1 for weak signals; −2 for documented expected mechanisms without a consequence.
- **Worked example:** the terminal scores via f5 (plus merged-cause impact); the 11 paragraphs score −2 and are dropped.

**Learning procedure (later):**
1. Unit = cause. The label is real (1) vs not (0); false is tracked separately as a detector bug.
2. L2 logistic regression with λ set by leave-one-site-out CV. Positive features must keep non-negative weights; otherwise revert to the heuristic weight.
3. Pairwise (RankNet-style) loss within a page once ≥ 30 pages have ≥ 2 labeled causes.
4. Threshold = the highest recall at which the CV Wilson lower bound for per-cause precision is ≥ 0.80.
5. Active learning: 20 per round, stratified, every 5th pick random.
6. Weights ship as versioned JSON, and the report prints the model version.

**Reaching ≥ 80% without losing recall:**
- Precision is measured on the Report tier only, and the CI gate uses only that tier.
- Recall guards, recomputed on every model change: planted 10/10, a ≥ 200-mutant suite with ≥ 95% detection at ≥ 5 px, and 0 control findings.
- Honest expectation: the heuristic plausibly reaches 70–85% per cause; reliable > 80% needs the learned model trained on new labels [I].

---

## Deliverable 5: Knowledge-base outline of expected engine differences

| # | Entry | Signature / detection rule | Explanation / fix | Evidence |
|---|---|---|---|---|
| K1 | Fractional line-height rounding | `lh` equal and non-integer; pitch differs by 0.05–1.0 px; `ln` equal; \|Δh − n·Δpitch\| ≤ 2 | "22.75px laid out as 22px in WebKit; 24px over 30 lines." Fix: whole-pixel line-height (`leading-[23px]`) or `round(calc(1.625em), 1px)` with fallback | WebKit 225695, 275601@main, Safari 27 notes; Mozilla 442139 [P/V]. **Verify:** WebKitGTK 2.50 vs Safari 27\[3\]\[4\]\[28\] |
| K2 | `line-height: normal` from font metrics | `lh == normal` in both; pitch diff > 0.2 px | "Derived from ascent/descent/line gap, rounded differently." Fix: explicit numeric line-height | Third-party description of Blink rounding ascent/descent (not checked against Chromium source); HN 17.1 vs 17.6 [I]\[41\] |
| K3 | Inline content-area height | Inline height differs, pitch equal | Already ignored | Brief |
| K4 | Text advance / shaping width | `tw` ratio 0.97–1.03; `ln` equal; identical font properties | No fix; report only with wrap/clip/overflow | Brief's rasterization finding [I] |
| K5 | URL / long-token line breaking | `ln` differs; token matches URL/long-word regex | Fix: `overflow-wrap: anywhere` or `<wbr>`; keep counted (mwUw real) | [I] |
| K6 | Hyphenation dictionaries | `hyphens: auto` and `ln` differs | Fix: `hyphens: manual` + `&shy;` | [I]; verify per platform |
| K7 | `text-wrap: pretty/balance` support | `CSS.supports` differs and `ln` differs | Progressive enhancement; accept | [I]; check at run time |
| K8 | Form-control UA defaults | Control tag, no author width (cascade trace), Δw | "Width from `size` × average char width." Fix: explicit `width` + `box-sizing` | HN input false [I] |
| K9 | Inner scrollbars | `sc` and Δ clientWidth 8–20 px | Fix: `scrollbar-gutter: stable` | [I] |
| K10 | Subpixel precision (1/64 vs 1/60) | \|Δ\| < 0.5 px per item | Ignore | WebKit LayoutUnit wiki; Mozilla2:Units [P]\[10\]\[12\]\[13\] |
| K11 | Table auto-layout distribution | Sibling cells Δw sum ≈ 0 | Report only with consequence | [I] |
| K12 | Emoji fallback fonts | Extended_Pictographic code points + Δ | Fix: explicit emoji font stack | [I] |

Applitools handles these by coarsening the comparison (Layout ignores content and colour) [V].\[42\]\[43\] Omnisnap's KB is more explainable because it names the mechanism and verifies the fix [I].

---

## Deliverable 6: Experiment plan for the next spike round

0. **Baseline (1 day).** Compute Wilson CIs, re-aggregate labels to causes, and audit unmatched elements for "only-in" findings. Always go.
1. **Cause-level reporting (2 days).**
   - Metrics: per-cause precision, P@3, nDCG@5.
   - Go if per-cause precision ≥ per-finding precision and the planted and control criteria hold.
2. **Second rater (parallel, 1 week).** A second frontend developer labels the 34 rows blind, with crops.
   - If κ < 0.4, rewrite the rubric with anchors (terminal = real, paragraph ×11 = trivial, p#mwnQ = false) before modelling.
3. **KB line-pitch + per-line rects (3 days).**
   - Go if ≥ 6/7 line-pitch rows match K1/K2, with 0 matches on planted bugs.
   - Run synaptech on real Safari 27 and record whether K1 is version-specific.
4. **Consequence gate (1 week).**
   - Go if per-cause precision ≥ 0.75 with ≤ 2 real rows demoted; otherwise inspect the demoted rows.
5. **Ink verification (3 days).** Pass if p#mwnQ and a#mwAsQ are demoted and go-dev, python-org and the planted bugs are kept.
6. **Computed-style diff + fix verification (1 week).** Pass at 10/10 planted causes, ≥ 1 verified fix, and 0 spurious diffs on controls.
7. **Grow the corpus (2–3 weeks).**
   - 40 sites (add e-commerce, docs, dashboards, RTL, CJK) × Firefox, WebKitGTK and Safari via macOS CI; 100 cause labels from 2 raters; α ≥ 0.6; every 5th label random.
   - CI gate in report-only mode when the Wilson lower bound is ≥ 0.75 (about 85/100); pass/fail mode at ≥ 0.80 (88/100).
8. **Mutation recall (1 week).** At least 200 mutants across 12 operators at 2/5/10/30 px. Pass at ≥ 95% detection for ≥ 5 px.
9. **Learned ranker (after ≥ 150 labels).** Pass at nDCG@5 +0.05 in leave-one-site-out CV with no loss on steps 7–8.
10. **Determinism (Dossier 7).** Only if ≥ 10% of the 40 sites trigger "page state differs"; Stripe criteria as in Dossier 7.

---

## Deliverable 7: What not to do

1. **Don't use page or region pixel diffs as the detector.** Control pages fail at DSSIM 0.027–0.045 while planted bugs sit at 0.003–0.007. WebDiff reached about 21% precision, and X-PERT's authors attribute earlier false positives partly to image comparison [P].\[1\]\[6\]
2. **Don't raise size thresholds.** Real findings exist at 2.1 and 5.9 px and a false one at 32 px. Humans judge relative change, not absolute [P].\[1\]
3. **Don't put a VLM/LLM in the core.** 69–72% precision [P], non-deterministic, and too large for the package.\[44\] An optional plugin could draft explanations.
4. **Don't train a neural net on tens of labels.** Browserbite's 96% used 1,200 items × 8 judgments [P].\[6\]
5. **Don't use CDP-only mechanisms in the core path** (virtual time, `CSS.getMatchedStylesForNode`). They make Blink incomparable with Gecko and WebKit [V/I].
6. **Don't invest in tree-edit-distance matching now.** No Appendix A error stems from matching [I].
7. **Don't copy same-engine pixel thresholds** (Chromatic 0.063, BackstopJS misMatchThreshold).\[40\] They assume identical rasterization [V/I].
8. **Don't make fingerprint auto-ignore the primary strategy.** Argos-style suppression hides mechanisms instead of explaining them. Use it only for a user "accepted differences" file [V/I].\[17\]
9. **Don't treat branded Blink browsers as engines.** They were pixel-identical (SSIM 1.0000) except where product features act.

---

## Deliverable 8: Evidence register (claim → source → type)

| Claim | Source (venue, date) | Type |
|---|---|---|
| X-PERT 76% P / 95% R; relative-layout insight; domino duplicates; 23/100 sites with XBIs | Roy Choudhary, Prasad, Orso, ICSE, May 2013 | Published\[1\] |
| Browserbite 66/98 plain, 96.4/88.6 NN, 84.4/79.2 tree; IRR 0.947; CrossCheck 36%, WebDiff 21% | Semenenko, Dumas, Saar, ICSM, Sept 2013 | Published\[6\] |
| Browserbite F > 90% on 140 pages | Saar et al., Software: Practice and Experience 46(11), 2016 | Published\[33\]\[45\] |
| ReDeCheck 33 RLFs in 16/26 pages; RLF classes | Walsh, Kapfhammer, McMinn, ISSTA, 10 July 2017 | Published\[36\]\[46\] |
| VISER 117 RLFs, 28 differ, 3 reclassified FP | Althomali, Kapfhammer, McMinn, ICST 2019 | Published\[2\] |
| VERVE; LayoutDR 92% | Althomali et al., STVR 31(4) 2021; ICST 2022 | Published\[37\]\[38\] |
| CSS mutation operators for layout regressions | Walsh et al., STVR 30(6), 2020 | Published\[27\] |
| XFix 86% / 99% (median 93% as cited for the tool paper) | Mahajan et al., ISSTA 2017; Grewal et al., arXiv 2512.15804, 2025 | Published |
| WebSee 100% detection, 93% localisation | Mahajan & Halfond, ICST 2015 (tool-track slides) | Published\[34\] |
| TWEB classifier F1 0.65 / 0.35 on 72 responsive apps | Watanabe, dos Santos & de Oliveira, ACM TWEB 17(4), Oct 2023, doi 10.1145/3580518 | Published |
| XBIDetective 72/59, 69/48; base 57% P | Grewal et al., arXiv 2512.15804, Dec 2025 | Published (preprint) |
| LayoutUnit 1/64 px | WebKit Trac wiki "LayoutUnit" | Engine docs\[10\]\[13\] |
| 60 app units per CSS px | MozillaWiki "Mozilla2:Units"; O'Callahan blog, Feb 2007 | Engine docs\[12\]\[47\] |
| WebKit floors fractional line-height; bug NEW | WebKit bug 225695 (12 May 2021; modified 4 Jan 2024) | Bug tracker\[3\] |
| WebKit removing integral vertical snapping | Commit 275601@main, 3 Mar 2024 | Engine source\[5\] |
| Safari 27 subpixel inline layout | webkit.org, 17 Sept 2026 | Vendor\[15\] |
| Gecko uneven fractional line-height open | Mozilla bug 442139 (2008; updated Aug 2025) | Bug tracker\[4\] |
| Production `round(..., 1px)` line-height fix | GitHub residuum PR #742, 17 Oct 2026 | Production practice\[28\] |
| BiDi preload scripts | W3C WebDriver BiDi; MDN | Spec\[19\]\[23\] |
| WebKitGTK BiDi preload implementation | WebKit PR #74706 | Vendor (unverified maturity)\[24\] |
| CDP virtual time, experimental | chromedp cdproto docs; third-party PRs | Vendor\[18\]\[48\] |
| Playwright clock surface, install before other clock calls | playwright.dev/docs/clock (v1.45+) | Vendor |
| Applitools match levels | Applitools docs | Vendor\[42\] |
| Percy 3× / up to 40% | BrowserStack release note, launched 14 Oct 2025 | Vendor |
| Chromatic 0.063 default | Chromatic docs, "Threshold for changes" | Vendor |
| Argos odiff, flaky detection | Argos docs | Vendor\[16\]\[17\] |
| FLIP BSD-3-Clause; flip-rs | NVlabs/flip; flip-rs | Vendor\[25\]\[26\] |
| CIs, sample sizes, weights, projections | This report | Inference |

---

## Caveats

- **Projections are not measurements.** Tier predictions depend on fields not yet extracted (line rects, clip, ink, bounded containers). Three real rows (cite_note-31, cite_note-12, li#mwAUU) may be lost under any consequence gate. Either the labels are noisy or a signal is missing that this research could not identify.
- **Brief inconsistency.** The terminal is described as line-height 22.75 px laid out at 22 px (section 4C), but Appendix A gives a pitch of 26.8 → 26.0 px. Both yield about −24 px over 30 lines. Confirm which value the extractor measured, because K1 depends on the computed line-height.
- **Engine versions.** Safari 27 may have removed the WebKit line-pitch mechanism while WebKitGTK 2.50 still shows it; this is unverified. Blink's ascent/descent rounding rests on a third-party description, not Chromium source. Verify with per-version fixture pages before shipping K1/K2 text.
- **Vendor numbers are claims.** Percy's "40%" and "3×" are unaudited, and no commercial tool publishes cross-engine precision/recall.
- **Older academic results.** X-PERT, Browserbite, WebDiff and CrossCheck were measured on IE8–9 / Firefox 3–16 era pages.\[1\]\[6\] They inform method more than magnitudes.
- **Safari replay proxy.** It alters the page (injected script, possibly relaxed CSP) and needs a trusted local CA. Keep it opt-in and disclosed.

## Sources

1. [X-PERT: Accurate Identiﬁcation of Cross-Browser Issues in Web Applications](http://shauvik.com/public/pubs/roychoudhary13icse_cr.pdf)
2. <https://philmcminn.com/publications/althomali2019.pdf>
3. [225695 – REGRESSION (Safari 14.1): Fractional CSS line-height value translates to rounded (floored) element height](https://bugs.webkit.org/show_bug.cgi?id=225695)
4. [442139 - Uneven line-height for non-integer values (should round/snap line-height to device pixels)](https://bugzilla.mozilla.org/show_bug.cgi?id=442139)
5. [\[webkit-changes\] \[WebKit/WebKit\] 8ebb25: \[IFC\]\[Subpixel\] do no round-to-integral RenderStyl...](https://www.mail-archive.com/webkit-changes@lists.webkit.org/msg211216.html)
6. <https://kodu.ut.ee/~dumas/pubs/icsm2013Browserbite.pdf>
7. [Get started with Applitools Eyes](https://applitools.com/docs/eyes/sdks/playwright-ts-standard/quickstart)
8. [Defining Eyes Visual AI Settings](https://applitools.com/docs/preflight/content/topics/applitools%20settings.htm)
9. [Infinite Pixels](https://news.ycombinator.com/item?id=44824056)
10. [2014 Meeting Subpixel Layout](https://trac.webkit.org/wiki/2014%20Meeting%20Subpixel%20Layout)
11. [How CSS width is computed in Gecko? - otsukare](https://www.otsukare.info/2016/05/26/css-width-gecko)
12. [Mozilla2:Units - MozillaWiki](https://wiki.mozilla.org/Mozilla2:Units)
13. [LayoutUnit - Timeline - WebKit](https://trac.webkit.org/wiki/LayoutUnit)
14. [Cannot print with border-width of less than 1pt \[40149824\] - Chromium](https://issues.chromium.org/issues/40149824)
15. [WebKit Features for Safari 27.0](https://webkit.org/blog/18325/webkit-features-for-safari-27-0/)
16. [How Argos detects visual differences](https://argos-ci.com/docs/learn/platform-fundamentals/how-argos-detects-visual-differences)
17. [Flaky test detection](https://argos-ci.com/docs/learn/reliability-and-flakiness/flaky-test-detection)
18. [test(headless): run the client on Chrome's virtual time by master-g · Pull Request #31 · master-g/emukc · GitHub](https://github.com/master-g/emukc/pull/31)
19. [script.addPreloadScript command - WebDriver](https://developer.mozilla.org/en-US/docs/Web/WebDriver/Reference/BiDi/Modules/script/addPreloadScript)
20. [Core Concepts](https://applitools.com/docs/eyes/playwright/core-concepts)
21. [Applitools Explained: Visual Testing for Modern Web Applications](https://learnwithnishtha.com/2026/01/02/applitools-explained-visual-testing-for-modern-web-applications/)
22. [emulation package - github.com/clex/cdp/protocol/emulation - Go Packages](https://pkg.go.dev/github.com/clex/cdp/protocol/emulation)
23. [WebDriver BiDi](https://www.w3.org/TR/webdriver-bidi/)
24. [\[WebDriver\]\[BiDi\] Execute preload scripts at document initialization by sergedeh · Pull Request #74706 · WebKit/WebKit · GitHub](https://github.com/WebKit/WebKit/pull/74706)
25. [GitHub - Tavrin/flip-rs: Pure-Rust port of NVIDIA FLIP v1.7: perceptual LDR/HDR image difference, validated against the C++ reference, WASM-ready · GitHub](https://github.com/Tavrin/flip-rs)
26. [GitHub - NVlabs/flip: A tool for visualizing and communicating the errors in rendered images. · GitHub](https://github.com/NVlabs/flip)
27. [Automatically identifying potential regressions in the layout of ...](https://eprints.whiterose.ac.uk/id/eprint/163400/8/stvr.1748.pdf)
28. [fix: whole-pixel line boxes and layout-free float placement for webkit by BearFlinn · Pull Request #742 · Grizzly-Endeavors/residuum · GitHub](https://github.com/Grizzly-Endeavors/residuum/pull/742)
29. [Playwright Clock API: Mock Time & Fake Timers Guide (2026)](https://qaskills.sh/blog/playwright-clock-api-testing-guide-2026)
30. [Stabilize screenshots](https://argos-ci.com/docs/learn/reliability-and-flakiness/flaky-tests)
31. [Stabilize date & time](https://argos-ci.com/docs/learn/reliability-and-flakiness/flaky-tests/stabilize-date-and-time)
32. [Playwright Clock + Install/Fakers: Test Time-Sensitive Code](https://qaskills.sh/blog/playwright-clock-install-fakers-guide)
33. [Browserbite: cross‐browser testing via image processing - Saar - 2016 - Software: Practice and Experience - Wiley Online Library](https://onlinelibrary.wiley.com/doi/abs/10.1002/spe.2387)
34. [WebSee: A Tool for Debugging HTML Presentation Failures](https://viterbi-web.usc.edu/~halfond/papers/mahajan15icst-tool-slides.pdf)
35. [ReDeCheck: An Automatic Layout Failure Checking Tool ...](https://www.gregorykapfhammer.com/download/research/papers/key/Walsh2017a-paper.pdf)
36. [Automated layout failure detection for responsive web pages without an explicit oracle](https://gregorykapfhammer.com/research/papers/walsh2017/)
37. [Automated repair of responsive web page layouts](https://gregorykapfhammer.com/research/papers/althomali2022/)
38. [Automated visual classification of DOM-based presentation failure reports for responsive web pages](https://www.gregorykapfhammer.com/research/papers/althomali2021/)
39. [🚀 Introducing Percy's Visual Review Agent - Browserstack](https://www.browserstack.com/release-notes/en/introducing-percys-visual-review-agent-iqB3J3UT)
40. [Threshold • Chromatic docs](https://instructure.chromatic.com/docs/threshold/)
41. [fix(paragraph): round ascent and descent before splitting a set line height by klbostee · Pull Request #41 · builtbyfew/effing-skia · GitHub](https://github.com/builtbyfew/effing-skia/pull/41)
42. [Check settings](https://applitools.com/docs/eyes/sdks/xcui/check-settings)
43. [Applitools Visual Testing Guide -- Eyes, Visual AI, and Setup](https://qaskills.sh/blog/applitools-visual-ai-testing-guide)
44. [XBIDetective: Leveraging Vision Language Models for Identifying Cross-Browser Visual Inconsistencies](https://arxiv.org/pdf/2512.15804)
45. [A Systematic Literature Review in Cross-browser Testing](https://portal.amelica.org/ameli/jatsRepo/30/3026001/html/index.html)
46. [ReDeCheck: an automatic layout failure checking tool for responsively designed web pages](https://dl.acm.org/doi/10.1145/3092703.3098221)
47. [Units Patch Landed](https://robert.ocallahan.org/2007/02/units-patch-landed_07.html)
48. [emulation package - github.com/chromedp/cdproto/emulation - Go Packages](https://pkg.go.dev/github.com/chromedp/cdproto/emulation)
