# Scientific grounding: processing images and DOM together

Written 2026-11-07 in an idea-refinement session after the layout-diff spike (rounds 1–3) and the [precision research report](research/Omnisnap%20Precision%20Research%20Report%20-%20From%20Every%20Genuine%20Difference%20to%20Differences%20a%20Developer%20Wants.md). Implementation lives in `spikes/layout-diff/`; status and results are in its README.

## Problem statement

**How might we** combine what the DOM knows (structure, styles, what the author meant) with what the pixels show (what a person actually sees), so that Omnisnap reports **exactly** the cross-engine differences a developer would act on, and backs each one with its cause and fix?

What any advanced method has to beat, from three spike rounds:

| Signal | Why it falls short alone | Evidence |
|---|---|---|
| **Pixels** | Engines draw the same letters differently, so every line of text "differs" | Pages with no bugs fail the PRD's DSSIM threshold of 0.02 (they score 0.027 and 0.045), and two planted bugs score *below* that noise |
| **DOM geometry** | Catches every genuine difference, including invisible and irrelevant ones; blind to what's drawn on screen (WebGL, image content) | Round 1: 16 of 28 trivial, 5 false |
| **Hand-written rules on top** | Reaching their limit, and at risk of overfitting 45 labels from one person | Round 3: 0 false positives reported, but 4 real items pushed into Info; the terminal label had to be corrected |
| **Both** | Get confused when engines capture the page in different states | Stripe (A/B variants, timers), the synaptech slideshow |

So "the best results" needs three things the pipeline doesn't have yet:
- **A model of human perception:** what a person actually notices.
- **Causal evidence instead of rules:** what really caused a difference.
- **Ground truth that scales** beyond one person's labels.

**Constraints chosen:** optimize **precision first**; up to **60 s** of extra processing per page; **any technique may be explored** (what ships is decided later); ground truth from an **unsupervised crawl** and **engine consensus**.

---

Your answers set the bar: **precision first**, up to **60 s** per page, **any technique on the table**, and ground truth from **an unsupervised crawl plus engine consensus**.

**One pushback before the ideas:** neither of those two can *measure* precision. Precision means "would a developer act on this?", and only a person can answer that. A crawl tells you what's *unusual*; consensus tells you *which engine is the odd one out*. Neither tells you what *matters*. I'd use them to **rank and train**, and keep a small human check (about 100 causes, ideally from 2 people) only to *measure* the result. Otherwise we'd be grading the algorithm with the algorithm.

## Eight concepts

**1. Diagnose by experiment (causal interventions).**
Stop guessing the mechanism from signatures and *test it inside the page*:
- inject a neutralizing stylesheet, re-measure, and see whether the difference disappears;
- candidate stylesheets: a font with identical metrics in every engine, whole-pixel line heights, `appearance: none` on form controls, no gaps;
- whichever change removes the difference names the mechanism;
- **delta debugging** (Zeller's ddmin) can pin element sizes from engine A into engine B to find the *smallest* set of elements that explains the whole difference;
- the same loop verifies the suggested fix.

It's script-only, so it works over CDP, BiDi *and* Safari's WebDriver Classic, at about 200 ms per experiment. **This replaces both the hand-written cause/effect rules and the fragile catalog signatures with evidence.**

**2. Model what the eye notices (psychophysics + Gestalt).**
- People spot a **broken alignment** of edges that should line up at 1–2 px (vernier hyperacuity).
- They need roughly a **5–10% change in size** before they notice it (Weber's law).
- They read layout through **grouping**: proximity, alignment, a label belonging to its input.

So: describe each pair of neighboring boxes with 2D interval algebra (aligned, overlapping, left-of, contained…) and report **qualitative changes**: alignment broken, overlap created, order swapped, grouping split. This fits your data: real items at 2 px, trivial ones at 50 px. **It gives relevance without labels, and it's cheap.**

**3. Self-calibrating comparison (robust statistics).**
On each page, estimate how the engines *systematically* differ, using robust regression that ignores outliers (Theil–Sen / RANSAC) over all elements:
- Firefox text 0.4% wider in this font;
- WebKit line pitch = floor(line-height);
- form controls +9 px.

What that page-specific model explains is *expected*; **the outliers are the anomalies.** That's the catalog of expected differences, learned per page instead of hand-written, so it keeps up when engines change.

**4. Engine consensus (differential testing).**
- With 3+ engines (Chrome, Firefox, WebKit; the Blink family counts as one), compute a consensus geometry and report **the engine that deviates**: "only WebKit breaks this".
- When every engine differs from every other, it's normal engine variation, so it goes to Info.
- Compilers have been tested this way for years (Csmith-style majority voting).

The catch: it needs a third engine. Locally that means WebKitGTK; for real Safari, the macOS CI runner.

**5. Novelty across a crawl (unsupervised).**
- Capture 500–1,000 sites in every engine, fingerprint each cause (mechanism, element role, relation change), and learn how often each fingerprint occurs.
- Common fingerprints (form controls, line-height rounding) carry little information; **rare ones are surprising, so they rank first**. It's the inverse-document-frequency idea from search, applied to engine differences.
- It also produces the dataset that ideas 3 and 7 learn from.

**6. DOM-guided image registration + ink comparison.**
- Use the matched element boxes to warp engine B's screenshot into engine A's coordinates, piece by piece.
- Then compare **ink masks** (where something is drawn, binarized and slightly dilated) instead of pixel intensities. Glyph rasterization noise disappears, reflow cascades align, and you get a "visibly changed area" weighted by the first viewport.
- This catches what's drawn but has no geometry (WebGL, images, colors) and removes box moves nobody can see. **It's how pixels can join the decision without their noise.**

**7. A learned judge.**
- Treat ideas 1–6 as **weak voters**, and combine their votes into probabilistic labels with a generative model, the way Snorkel-style data programming does.
- Train boosted trees on those labels.
- Set the reporting threshold with **conformal risk control**, so "≥ 80% precision" becomes a *statistical guarantee* rather than a hope.
- An optional vision-language model can act as one more voter on the top-k items only (about 72% precision alone, per XBIDetective, but useful in a committee).

**8. Inversion: compare against the design's intent, not against Chrome.**
- Infer the intended structure from the render: the alignment grid, equal-height rows, the spacing scale (multiples of 4/8 px), columns that share edges.
- Check **every** engine against that intent, Chrome included. A bug is a violation of what the design is clearly trying to do, not a difference from a baseline browser that might itself be wrong.

**How they fit together:**
- **1, 2 and 4** form a coherent core: *causal mechanism + perceptual relevance + which engine is wrong*, all deterministic and inside the 60 s budget.
- **3 and 6** strengthen it: calibration, and pixels without the noise.
- **5 and 7** turn your chosen ground-truth sources into ranking and guarantees, once a crawl exists.
- **8** is the most provocative, and the one I'm least sure about.

---

## Implementation status (round 4, 2026-11-07)

*Added after implementation. The text above is kept exactly as written.*

All eight concepts run together in `spikes/layout-diff/`; details and numbers in the [spike README, Round 4](../spikes/layout-diff/README.md#round-4-2026-11-07-eight-concepts-in-tandem).

| # | Concept | Module | Status | What the data said |
|---|---|---|---|---|
| 1 | Causal interventions | `inject/probes.js`, `lib/mechanism.mjs` | Built: `lh`, `font`, `controls`, `all` probes captured per run | Works and verifies fixes (K1 terminal 692 → 722 px = Chrome). It answers *why*, not *whether*: votes only for differences that survive every probe. ddmin pinning deferred |
| 2 | Psychophysics / Gestalt | `lib/perception.mjs` | Built: alignment, overlap, order, spacing, Weber 8% | Most causes have no relation change (221/329) |
| 3 | Robust self-calibration | `lib/calibrate.mjs` | Built: median/MAD advance, pitch, controls per page | Explains 23 causes; Theil–Sen not needed yet |
| 4 | Engine consensus | `lib/consensus.mjs` | Built: families blink/gecko/webkit | Finds 17 causes where Chrome is the odd engine. "Odd engine" is not "worth reporting" (quirks are odd too) |
| 5 | Corpus novelty | `lib/novelty.mjs` | Built on a 24-site crawl (not 500–1,000) | Rarity is not relevance in a small corpus; commonness across sites is a useful "baseline behaviour" signal |
| 6 | Registration + ink | `lib/ink.mjs` | Built: per-element registration, histogram-mode background, per-page noise floor | Showed that visible overflow is mostly invisible. Whole-image warping not needed |
| 7 | Learned judge | `lib/judge.mjs` | Weak-supervision label model built (MAP-EM, rules-conditional prior); threshold calibrated on planted pages; boosted trees, conformal control and VLM deferred | Halves the corpus Report tier; independence assumption violated; precision not measured (no ground truth yet) |
| 8 | Design intent | `lib/intent.mjs` | Round 4: near-miss alignments 0.5–3 px; **redesigned in round 5** as a page-wide constraint model (alignment, equal sizes, rhythm, grid columns, no overlap; kept by ≥ 2 engine families, broken by one by ≥ 2 px or 8%) | Round 4: never fired (0/329). Round 5: see the spike README |

**Answer to the pushback:** the unsupervised crawl and engine consensus produced the dataset and the evidence, but not a precision measurement. Manual labels were withdrawn as unreliable (round 5); precision will come from **automated ground truth**: realistic mutations injected into real pages, null pairs, and a small locally trained vision model (PRD changelog 1.4.0).
