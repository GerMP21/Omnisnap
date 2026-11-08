# Spike: cross-engine layout diff

**Question:** can comparing element geometry between browser engines report the differences a developer actually wants to know about?

**Status (2026-11-07, round 5):**
- **Detection works.** 24/24 planted bugs (16 realistic cross-engine bug patterns added this round) are found at the right element; bug-free control pages produce nothing; 97–100% of elements are matched. Design intent catches 6/6 planted intent bugs with no false alarm on controls.
- **Precision is the open problem.** Round 3 moved it from per-element findings to **causes in two tiers** (Report / Info), checked against a **catalog of expected engine differences**. Round 4 adds the eight concepts of [SCIENTIFIC_GROUNDING.md](../../docs/SCIENTIFIC_GROUNDING.md) as evidence per cause plus a label-model judge, run on a 24-site unlabeled crawl. **The evidence is solid; precision on real sites is not measured yet.** Manual labels were withdrawn as unreliable; measuring precision now needs automated ground truth (mutation testing and a locally trained model, PRD changelog 1.4.0). See [Results](#results) and the PRD changelog ([docs/prd.md](../../docs/prd.md)).

> **Throwaway code.** `puppeteer-core` and the small WebDriver client stand in for the future Go drivers. What survives is listed below.

| File | Role | Fate |
|---|---|---|
| `inject/stabilize.js` | Settles the page: fonts, lazy content, animations, then **freezes JS timers** | Embedded unchanged in the Go core |
| `inject/extract.js` | Element geometry, line boxes, typography, interactivity, truncation | Embedded unchanged in the Go core |
| `lib/align.mjs` | Element matching across engines and root-cause analysis | **Ported to Go** |
| `lib/triage.mjs` | Catalog of expected engine differences (the knowledge base, "KB"), visible consequences, Report/Info tiers, impact | **Ported to Go** |
| `inject/probes.js` | Causal probes: re-measures the page with line-height pinned, one font forced, form controls neutralized | Embedded unchanged in the Go core |
| `lib/mechanism.mjs`, `perception.mjs`, `calibrate.mjs`, `consensus.mjs`, `ink.mjs`, `intent.mjs`, `novelty.mjs`, `judge.mjs` | The eight concepts of [docs/SCIENTIFIC_GROUNDING.md](../../docs/SCIENTIFIC_GROUNDING.md), one module each (round 4) | Evidence modules ported to Go; the judge stays experimental (see Round 4) |
| `lib/webdriver.mjs` | Minimal W3C WebDriver Classic client (WebKit/Safari), headless through Weston | Reference for the Go client |
| `planted/pages.mjs` | Pages with known answers: planted bugs, controls, knowledge-base fixtures | **Regression suite** |
| `inject/mutate.js` | Mutation testing: injects one realistic cross-engine bug (19 operators) into a real page, in one engine | Embedded unchanged in the Go test tooling |
| `lib/features.mjs`, `lib/gbdt.mjs` | Feature vector of a cause; evaluator for the LightGBM model | **Ported to Go** (the model loads through `leaves`) |
| `models/` | The trained tree model (`gbdt.txt`, `gbdt.json`) and its cross-validated metrics (`gbdt-meta.json`) | **Shipped** (retrained as the corpus grows) |
| `sites.mjs`, `dataset.mjs`, `train/` | Automated crawl list (Tranco), training table, LightGBM training, parity test, full-run script | Reference for the training pipeline |
| everything else | Capture (`capture.mjs`, `lib/engines.mjs`), report (`diff.mjs`, `lib/report.mjs`), scoring, experiments | Deleted after the spike |

## Setup (Linux; tested on AlmaLinux 10 / WSL2)

Omnisnap uses the browsers already installed. The spike needs:

| What | Why | Install (AlmaLinux / EPEL) |
|---|---|---|
| Chrome (baseline) + Firefox | The two main engines | `google-chrome-stable`, `firefox` |
| Chromium, Brave, Edge (optional) | Blink-family and branded-browser checks | `chromium` (EPEL), vendor repos for Brave and Edge |
| WebKitGTK 6.0 (optional) | WebKit engine: `WebKitWebDriver` + `MiniBrowser` | `webkitgtk6.0 webkitgtk6.0-devel` |
| Weston | Headless display for WebKitGTK (no windows on your desktop) | `weston` |
| Fonts | Same fonts for every engine, including CJK | `liberation-fonts dejavu-sans-fonts google-noto-sans-fonts google-noto-sans-cjk-fonts` |
| Node 20+ | Runs the spike | `npm install` (puppeteer-core, pngjs, pixelmatch; no browser download) |

Engines whose binaries are missing are skipped automatically. Paths can be overridden with `CHROME_PATH`, `CHROMIUM_PATH`, `BRAVE_PATH`, `EDGE_PATH`, `FIREFOX_PATH`, `WEBKIT_DRIVER` and `WEBKIT_BROWSER`.

## Run

| Command | Does |
|---|---|
| `npm run check:fonts` | Precondition: every engine resolves the generic fonts the same way (font parity is on by default; `FONT_PARITY=0` turns it off) |
| `npm run warm` | Builds warmed template profiles for Brave and Edge, so they run as shipped (blocking lists downloaded), and probes their blocking |
| `npm run capture:planted` / `capture:own` / `capture:real` | Captures each page group in all installed engines (real sites in Chrome + Firefox only), 2 runs each, engines interleaved per page, plus the causal probes (`PROBES=0` skips them) |
| `node capture.mjs crawl --engines chrome,firefox,webkit` | Captures the crawl: 24 hand-picked sites (`c-*`) plus `crawl-sites.json` (`t-*`) |
| `npm run sites` | Builds `crawl-sites.json`: 500 sites from the Tranco top list, filtered (no infrastructure, adult or gambling sites, one per brand, real HTML homepage, no bot challenge or sign-in wall) |
| `npm run capture:mut` | Mutation testing: `MUT_PER_ENGINE` (4) mutants per captured page and engine, several per page load |
| `npm run dataset` / `npm run train` | Training table from `findings.json`; LightGBM training (grouped cross-validation by site) and the JS parity test |
| `npm run train:all` | The whole training run on a Linux disk (`OMNISNAP_OUT`, default `~/omnisnap-data/out`), two capture shards in parallel |
| `npm run diff` | Matches elements, finds root causes, triages them into Report/Info causes. Writes `out/findings.json` and `out/report.html` |
| `npm run score` | Checks the pass criteria on known answers: planted recall (Wilson interval), controls, KB fixtures, design intent |
| `npm run pixels` | Pixel comparison: whole-page SSIM/DSSIM, `pixelmatch`, SSIM on graphics crops |
| `npm run all` | Everything above, except warm and pixels |

Add your own pages to `corpus.json` under `"own"`. Options: `capture.mjs … --only <slug>` and `--engines chrome,webkit`; `diff.mjs --show <slug|group>`.

## Pipeline

1. **Capture** (`capture.mjs`):
   - viewport 1280×800, device pixel ratio 1;
   - 2 runs per engine, interleaved per page so live sites can't change in between;
   - Brave and Edge start from a copy of their warmed template.
2. **Normalize:**
   - **font parity:** engines that use fontconfig (Gecko, WebKitGTK) get Blink's generic fonts;
   - **root scrollbar and gutter hidden** in every engine;
   - **WebKit viewport fitted** with a measure-and-correct loop.
3. **Stabilize** (`stabilize.js`), in order:
   - wait for `readyState` and `document.fonts.ready`;
   - stepped scroll to trigger lazy loading;
   - wait for images;
   - finish or freeze animations;
   - two `requestAnimationFrame` ticks;
   - **freeze all JS timers**.
4. **Extract** (`extract.js`): per element:
   - identity: tag, id, test id, role, classes, name, `href`/`src`;
   - border box;
   - own text: hash, line boxes, line tops, text extent;
   - typography: line-height, font size/family/weight, white-space;
   - display, position, visibility, overflow;
   - interactive flag; truncation flag.

   After the screenshot the page is **measured again**, and a warning is raised if it changed.
5. **Pick runs:** each engine pair compares the runs whose page states match best. The other run of each engine marks **dynamic regions** (slideshows, rotating logos), which are ignored and shaded gray in the report.
6. **Match** (`align.mjs`): a hierarchical alignment of siblings, using an LCS over identity tokens, then pairing leftovers by tag + class.
7. **Root causes** (`align.mjs`): each changed element is either a cause or an effect. Examples:
   - width flows down from a resized parent or up from content;
   - siblings move because a sibling resized;
   - children that all moved point at their parent;
   - in inline text flow, only the first changed box is a cause.
8. **Triage** (`triage.mjs`):
   - **Knowledge base:**
     - K1: fractional line-height rounded differently;
     - K2: `line-height: normal` from font metrics;
     - K4: text advance;
     - K5: long tokens/URLs breaking differently;
     - K8: form-control defaults.
   - **Consequences:**
     - presence or visibility change;
     - overflow or protrusion beyond the viewport;
     - truncation;
     - line-count change;
     - interactive target moved;
     - structural change (≥ max(8 px, 10%)).
   - **Tier:**
     - **Report** when a consequence is not explained as expected;
     - presence, visibility, overflow, protrusion and truncation are **always** Report;
     - otherwise **Info**.
9. **Causes** (`diff.mjs`):
   - findings with the same mechanism (KB entry) or structure (the same CSS on N items) merge into one cause;
   - the headline is the most impactful instance;
   - ranked by `impact = max(weight × visibility) × (1 + log2(instances))`.

10. **Evidence** (round 4, one module per concept of [SCIENTIFIC_GROUNDING.md](../../docs/SCIENTIFIC_GROUNDING.md)), per cause:
    - **mechanism** (`mechanism.mjs`): does the difference survive each probe captured with the run (`lh`, `font`, `controls`, `all`)? The probe that removes it names the mechanism and verifies its fix. A font probe that removes a difference between *different* requested fonts is `author-font`;
    - **perception** (`perception.mjs`): relation changes the eye notices against siblings and among children (alignment broken, overlap created, order flipped, spacing changed), and Weber's law for sizes (≥ 8% and ≥ 2 px);
    - **calibration** (`calibrate.mjs`): the page's own systematic engine model (text advance ratio, line pitch per typography, control sizes; median/MAD) and whether it predicts the change;
    - **consensus** (`consensus.mjs`): the third engine family decides which engine is odd (Blink counts once);
    - **ink** (`ink.mjs`): each engine's screenshot cropped at its own box, ink masks compared, against this page's noise floor (agreement of unchanged elements);
    - **intent** (`intent.mjs`): relations at least two engine families keep (shared edges, equal sizes of repeated components, even gaps, grid and text columns, shared top/bottom lines of side-by-side components, no overlap), broken by one engine by ≥ 2 px (≥ 8% for sizes), Chrome included. Blamed on the element that moved; a violation without a cause becomes one. Skipped below 95% matched;
    - **novelty** (`novelty.mjs`): how many other sites show the same cause fingerprint.
11. **Judge** (`judge.mjs`, corpus-level in `diff.mjs`):
    - each concept is a weak voter (+1 / −1 / abstain);
    - a one-coin Dawid–Skene label model is fitted by MAP-EM on the **unlabeled** real + own + crawl causes. The class prior is conditional on the round-3 rules tier, and accuracies get Beta priors and are kept ≥ 0.5;
    - P(worth reporting) **ranks** causes; the **tier stays rule-based** (round 5: the judge tier lost recall, 19/24 vs 24/24). `JUDGE=1` makes the judge decide: Report = a hard consequence or P ≥ τ;
    - τ is **calibrated on the planted pages** (strictest τ with planted recall ≥ 90% and no control Report causes, else the best recall). `JUDGE_TAU` overrides it.

12. **Mutation testing and the tree model** (round 6):
    - `inject/mutate.js` injects one realistic bug into a settled real page, in Firefox or WebKit only: width, padding, margins, nudges, hiding, visibility, gaps, wrapping, font size, `white-space`, `align-self`, overlap, clipping, `order`, letter-spacing, line-height, borders, ellipsis. When the target has a class shared by ≤ 30 elements, the whole class gets it, as a stylesheet bug would. Several mutants share one page load (inject, measure, revert);
    - **labels without humans:** a same-engine diff (clean vs mutated) shows exactly what the bug changed. A cross-engine cause is positive when it sits on a changed element near the target and was not already in the clean diff;
    - **training table** (`dataset.mjs`): positives = those causes; negatives = every cause of the clean pages (natural engine differences: unlabeled, mostly expected, so positive-unlabeled). 82 features per cause (`lib/features.mjs`); planted pages are never trained on;
    - **LightGBM** (`train/train.py`), cross-validated by site. Precision on this table depends on how many bugs were injected, so the comparison is **recall on injected bugs vs false alarms on natural causes**, at equal recall;
    - `lib/gbdt.mjs` evaluates the model from its JSON dump; `train/parity.mjs` checks it against LightGBM's own predictions. `TIER=gbdt` lets the trees decide the tier.

## Scoring

Scoring uses **known answers only** (`score.mjs`):

- **Planted differences** fire in Firefox only (UA-conditional CSS). Recall is checked **in the Report tier**, with a Wilson 95% interval. For every other engine the planted pages act as controls.
- **Control pages** (and planted pages in non-Firefox engines) must produce no Report cause.
- **Catalog test pages** (`14-kb-line-height`, `15-kb-line-height-normal`) may differ between engines, but only as explained Info causes.
- **Design intent:** the planted intent pages (`intent: true`) must produce a violation on the planted element; control pages must produce none.
- **Real sites** are counted (Report causes per page, match rate) but **precision is not measured**: there is no ground truth for them yet. Manual labels were tried in rounds 1–4 and **withdrawn as unreliable**; all label files and label-based figures were removed.
- **Judge vs rules:** planted recall, control Report causes and corpus Report counts per τ, plus each voter's learned accuracy and coverage.

## Results

### Round 5 (2026-11-07): manual labels withdrawn, intent redesigned, realistic planted bugs

Engines: Chrome 155 vs Firefox 153 ESR and WebKitGTK 2.50 on the planted suite (recaptured with the new text-inset field); real and crawled sites reuse the round-4 captures.

- **Manual labels withdrawn** as unreliable: `labels.json`, `labeled-findings.json`, the labeling UI and every label-based figure were removed. Scoring uses known answers only.
- **16 new planted pages**, each imitating a real cross-engine bug class with a `.ff` rule: card CTA no longer pinned (`margin-top: auto`), indented grid cell, toolbar button 4 px low, broken list rhythm, tile that stops stretching, badge overlapping its title, ellipsis missing, line-clamp missing, aspect-ratio missing, navigation wrapping, code block scrolling instead of wrapping, invisible notice, `table-layout` difference, flex `min-width: auto`; plus two layout-heavy **control** pages (cards, dashboard).
- **They found five blind spots**, now fixed:
  - **Truncation with identical boxes** (ellipsis vs clip) was never examined: it is now a cause (`truncated-in-*`, hard).
  - **Text moved inside an unchanged box** (padding, indent) was invisible: the extractor records the text's inset (`ti`); a shift ≥ 3 px beyond what a different text width explains is a cause (`text-shifted`).
  - **Table columns redistributed** (`table-layout`) were blamed on cells and explained away as text advance (K4): when cells change far more than any text (and the table keeps its width), the table is the cause (`columns`).
  - **The judge tier costs recall** (10/24 on the first run, 19/24 after the fixes, vs 24/24 for the rules): the tier stays rule-based (PRD 1.3.0 #4); P ranks.
  - **Intent** missed relations across components (CTAs in different cards) and blamed the wrong element of an overlap: corresponding parts of repeated components are now compared, text columns too, and blame goes to the element that moved relative to its parent.
- **Design intent fires:** 100,961 relations checked; 24 violations on 11 of 71 real-site engine pairs, 5 of them where **Chrome** is the engine breaking the design. Planted intent bugs 6/6, control pages 0.

| Criterion | Target | Result |
|---|---|---|
| Planted bugs reported at the right element | ≥ 90% | **24/24 = 100% [86–100%]** (Wilson 95%) |
| Planted pages within their Report budget | all | **24/24** |
| Report causes on control pages (Firefox, WebKit, plus Chromium/Brave/Edge from round 3) | 0 | **0** |
| KB fixtures: Report / unexplained | 0 / 0 | **0 / 0** |
| Intent: planted intent bugs / violations on controls | ≥ 90% / 0 | **6/6 / 0** |
| Precision on real sites | lower bound ≥ 80% | **not measured**: needs automated ground truth (PRD changelog 1.4.0) |

### Round 4 (2026-11-07): eight concepts in tandem

Engines: Chrome 155 (baseline) vs Firefox 153 ESR and WebKitGTK 2.50, all groups, with causal probes captured in every run. Corpus: 15 planted/control/KB pages, the owner's site, the 10 real sites and a new **unlabeled crawl of 24 sites** (gnu.org timed out in every engine: 23 captured). 329 corpus causes, 118 distinct fingerprints.

**What each concept contributed** (corpus causes, real + own + crawl):

| Concept | Module | Fired on | Notes |
|---|---|---|---|
| 1 Causal probes | `mechanism.mjs` | 329/329 | `layout` 175, `font` 85, `lh` 65, `controls` 4. The K1 fixture's terminal: 692 px in WebKit, 722 px under the `lh` probe, as in Chrome: **the fix is verified by experiment**. Probes cost 0.4 s median per run (max 16 s on the largest page) |
| 2 Perception | `perception.mjs` | 108 with an event | Score 0 on 221, Weber-noticeable size on 82, relation events (alignment, overlap, order) on 9 |
| 3 Self-calibration | `calibrate.mjs` | 23 explained | Advance ratio, line pitch per typography and control sizes, median/MAD per page |
| 4 Consensus | `consensus.mjs` | 238 decided | B-odd 173, **A-odd 17 (Chrome is the odd engine)**, all-differ 48 |
| 5 Novelty | `novelty.mjs` | 103 common | Same fingerprint on ≥ 3 other sites |
| 6 Ink | `ink.mjs` | 263 compared | 124 not observable against the page's noise floor (measured on 55 pages, median 0.97) |
| 7 Judge | `judge.mjs` | all | MAP-EM label model, prior conditional on the rules tier, τ calibrated on planted pages (0.05) |
| 8 Design intent | `intent.mjs` | **0** | No near-miss alignment that the other engines draw exactly. Either rare or the 0.5–3 px window is wrong; unproven |

| Criterion | Target | Result |
|---|---|---|
| Planted bugs reported at the right element | ≥ 9/10 | **10/10** |
| Report causes on control and KB pages (Firefox + WebKit) | 0 | **0** |
| Corpus Report causes (70 engine pairs) | fewer | rules tier **127** → judge **62** (69 demoted, 4 promoted); median 0 per pair, max 14 |
| Analysis time per engine pair | ≤ 60 s | median 0.05 s, p95 3.4 s, max 6.7 s (whole corpus, 140 pairs: 101 s) |

What the round established:

- **Visible overflow is not a hard consequence.** In the first full run, 45 of 103 corpus Report causes were "overflow", which could not be vetoed. For almost all of them, ink, perception and novelty independently said "invisible". Examples: Mozilla's social icons (Kellum image replacement, 9 999 px of deliberately hidden text) and `h1.lh-1` glyphs overflowing a `line-height: 1` box. The extracted overflow *mode* now decides:
  - `hidden`/`clip` → **clipped** (hard, unless the content is pushed beyond a viewport);
  - `auto`/`scroll` → **scrollbar** (hard);
  - `visible` → a soft consequence the judge weighs.

  Planted `11-clip` is now correctly *clipped*.
- **Mechanism tells you why, not whether it matters.** A button row that wraps because of form-control defaults is fully explained by the controls probe, and still visible. An expected mechanism with a visible consequence still matters; that judgement is already in the rules tier (catalog + consequences), so voting against it would count it twice. Mechanism therefore only votes *for* (a difference that survives every probe); its verdict and verified fix go into the report.
- **Rarity is not relevance.** In a small corpus almost everything is rare. Novelty only votes *against* (the same cause on ≥ 3 other sites = baseline engine behaviour).
- **"Which engine is odd" is not "worth reporting".** WebKit's expected line-height rounding is the odd one out too. Consensus feeds the report (17 causes where Chrome is the odd engine) and votes only when every engine differs.
- **Plain EM collapses on small corpora.** First fit: prior 0.02, consequence voter at 0.17 accuracy (below chance). This needed:
  - MAP-EM with Beta priors;
  - accuracies kept ≥ 0.5 (the Dawid–Skene identifiability assumption);
  - a class prior conditional on the rules tier.
- **Correlated voters break the label model's independence assumption.** Perception, ink and novelty tend to vote "invisible" together. EM then rates all three at 0.95 accuracy and pushes the Report prior down to 0.08. Modelling voter dependencies (Snorkel's structure learning) is the fix.

**Verdict:**
- Keep concepts 1–6 as **evidence**: each answers a question a developer asks:
  - why (mechanism, with a verified fix);
  - would anyone notice (perception, ink);
  - which engine is wrong (consensus);
  - is it this page or every page (calibration, novelty).
- Keep the overflow-mode rule.
- The judge is **experimental**: it halves the corpus Report tier, but nothing yet shows that it keeps the right half.
- **Next:**
  - automated ground truth (PRD changelog 1.4.0) to measure precision;
  - then model voter dependencies.

### Round 3 (2026-11-07): causes, tiers, knowledge base

Engines: Chrome 155 (baseline) vs Chromium 156, Brave 157, Edge 157, Firefox 153 ESR and WebKitGTK 2.50 on 15 planted/control/KB pages and synaptech-solutions.com; Chrome vs Firefox on the 10 real sites (recaptured). 0 capture failures.

| Criterion | Target | Result |
|---|---|---|
| Planted bugs reported at the right element (Report tier) | ≥ 9/10 | **10/10**, one cause each |
| Causes on control pages, all 5 compared engines | 0 | **0** |
| KB fixtures: Report causes / unexplained Info | 0 / 0 | **0 / 0** (WebKit: 1 K1 cause on the line-height page; everything else identical) |
| Elements matched: own site / real sites | ≥ 95% / ≥ 85% | **100% / 100%** (min 99.9%) |
| Report causes per page | ≤ ~10 | median **0**, max 2 |

What changed compared with round 1, on the same 10 sites + the owner's site:

- **Reported items: 28 non-Stripe findings → 4 Report causes.**
- HN's 61-line table is explained as K2 (`line-height: normal`), HN's search input as K8 (form-control defaults).
- **The owner's site reports nothing in any engine.** WebKit's line-pitch drift is 3 explained Info causes (K1).
- Stripe's two runs agreed with each other this time and its engines rendered the same state (0 causes). That's one observation, not a fix for A/B variants.

Remaining Report causes: go.dev playground buttons moved; a Wikipedia URL that breaks differently (K5); a fixed-position Tailwind panel 31 px narrower; a Wikipedia menu with different children.

**Verdict:** the Report tier is small enough to read, and recall on planted bugs is intact. Inline elements that shift inside reflowed text are neither "interactive target moved" nor "structural".

### Earlier rounds

| Round | What changed | Key result |
|---|---|---|
| 1 | Chrome 155 vs Firefox 153 on 13 planted/control pages and 10 real sites | Planted 10/10; controls 0; 99.7% matched. Stripe served different A/B variants and timer states per session |
| — | Pixel head-to-head (`pixeldiff.mjs`) | Whole-page SSIM/DSSIM **fails on bug-free controls** (DSSIM 0.027 / 0.045 > 0.02): Skia and Firefox rasterize the same glyphs differently. SSIM on graphics crops is clean on stable pages |
| 2 | Six engines: Chromium, Brave, Edge, WebKitGTK (WebDriver Classic, headless via Weston); own site synaptech-solutions.com | 0 findings on 48 control pages; Blink family pixel-identical; synaptech 100% matched; WebKit's 6 findings are all one mechanism (fractional line-height, 22.75 px laid out as 22 px) |
| 3 (prep) | Research report on precision (`docs/research/`) | Diagnosis: a triage problem, not detection. Report causes ranked by visible consequence; catalog expected engine differences |

**Same-state evidence:** a striking difference in the synaptech screenshots came from the slideshow (masked dynamic content), not from the line-height drift. The report viewer now compares best-matching runs and shades masked regions, so dynamic content can't be mistaken for a finding.

## Lessons → product requirements

| # | Requirement | Evidence |
|---|---|---|
| 1 | **Font parity:** align how engines resolve generic font families; report the resolved fonts | Chrome used Liberation, Firefox Noto, on the same machine (5–16% wider) |
| 2 | **Missing-glyph detection:** warn when page text uses a script with no installed font | CJK: Firefox draws hex boxes, Chrome empty boxes |
| 3 | **Capture all engines at nearly the same moment** | Hacker News re-ranked between sequential captures |
| 4 | **Normalize the root scrollbar and gutter** | Chrome kept a 15 px `scrollbar-gutter: stable` gutter (MDN) |
| 5 | **Group repeated findings** (same structure + same difference → "×N") | HN 32 → 4 findings |
| 6 | **Clock control** (fake timers before page scripts); impossible over WebDriver Classic, so Safari needs a replay proxy | Stripe kept changing after stabilization |
| 7 | **Capture consistency:** freeze timers after stabilization; re-measure after the screenshot | Slideshow advanced between geometry and screenshot (Chrome, WebKit) |
| 8 | **A/B control:** seed cookies/storage; detect diverged page states | Stripe served a random variant per fresh session |
| 9 | **Visibility differences are findings;** transparent elements that take space are not inert | Fading-in Stripe panels were skipped |
| 10 | **`<svg>` is one box;** canvas/WebGL needs pixel comparison of crops | `<use>` bounds differ by API, not rendering |
| 11 | **Relevance needs more than size:** a 2 px misalignment can matter, a 50 px reflow below the fold may not | Round-1 findings |
| 12 | **No whole-page pixel pass/fail across engines;** SSIM only on aligned graphics crops; page DSSIM as a state alarm | Bug-free controls fail DSSIM 0.02 |
| 13 | **Re-measure after the screenshot,** and skip dynamic elements in the graphics comparison | synaptech slideshow |
| 14 | **Branded Blink browsers matter only where their features act;** warm their profiles | Pixel-identical; Brave blocks 0 fresh vs 3 warmed |
| 15 | **WebKit on Linux needs a headless display** and a viewport fit loop; WebKitGTK is a *layout* proxy, not Safari *pixels* | Pop-up windows; 693 px cap; Skia rendering |
| 16 | **Report causes ranked by visible consequence,** with a catalog of expected engine differences (mechanism, explanation, fix) | synaptech: 6 findings = 1 mechanism; research report |
| 17 | **Evidence must show the same page state as the finding:** compare best-matching runs and shade dynamic regions | A slideshow difference looked like a layout finding |
| 18 | **Causal probes per capture** (line-height pinned, one font, neutral controls): name each cause's mechanism and verify its fix by experiment | K1 terminal: 692 → 722 px under the `lh` probe, equal to Chrome |
| 19 | **Overflow counts by mode:** clipped and scrollbar are hard consequences; visible overflow is soft; content beyond a viewport is deliberate hiding | 45/103 corpus Report causes were invisible overflow |
| 20 | **Mechanism is "why", not "whether":** expected mechanisms with visible consequences stay reportable | A control-defaults difference can still wrap a button row |
| 21 | **No manual labels as ground truth:** precision needs automated ground truth (mutations, null pairs, a trained model) | Rounds 1–4 labels withdrawn as unreliable |
| 22 | **Truncation and text position are geometry:** compare the truncation state and the text's inset, not only boxes | Ellipsis vs clip and an indented cell produced no cause before round 5 |
| 23 | **Design intent is relational and page-wide:** relations kept by ≥ 2 engine families, broken by one (Chrome included), across repeated components | Round 4's sibling near-miss detector fired 0 times; round 5's fires on 11 of 71 real-site pairs |

## Known limits

- iframes are not traversed. Open shadow roots are; closed ones can't be.
- Selectors use `:nth-of-type` over *rendered* siblings: labels for people, not guaranteed `querySelector` strings.
- An element moved to a different parent shows up as removed plus added.
- The planted pages were also used to design the rules, so their scores are optimistic. Precision claims need **independent, automated ground truth**.
- The judge's label model assumes voters are independent; perception, ink and novelty are not.
- Real and crawled sites still use round-4 captures (no text inset there), so the text-shift and text-column checks only run on the planted suite until the next crawl.
- WebKitGTK ≠ Safari for pixels, and possibly for line-height (Safari 27 reportedly lays out inline content with subpixel precision; unverified).
- Edge's tracking prevention blocked nothing under headless automation, even with its lists downloaded.
