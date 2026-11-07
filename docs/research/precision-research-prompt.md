# Deep-research prompt: raising the precision of cross-engine difference detection

Prepared 2026-11-07 from the layout-diff spike (`spikes/layout-diff/`), rounds 1 and 2. Related: [PRD](../prd.md) (amended by its changelog) · [One-pager](../ideas/omnisnap.md) · [Spike README](../../spikes/layout-diff/README.md)

**How to use:** paste everything below the line into a deep-research model with web access. The prompt is self-contained. If the tool accepts files, also attach `spikes/layout-diff/README.md`, `spikes/layout-diff/lib/align.mjs`, `spikes/layout-diff/inject/extract.js`, `spikes/layout-diff/inject/stabilize.js` and `spikes/layout-diff/labels.json`.

---

## Your role and task

You are a research analyst specializing in web rendering engines, automated GUI/visual testing, and the detection of cross-browser inconsistencies (XBIs). Research and recommend **the techniques and the algorithm design that will most improve the precision** of an open-source tool that compares how different browser engines render the same page.

"Precision" here means: **of what the tool reports, how much a frontend developer would actually want to be told about.** The tool already finds real differences reliably. Its problem is that most of what it reports is genuine but not worth a developer's time, and some is wrong.

Be concrete and evidence-based. Every recommendation must say which of the failure cases below it would fix, how it fits into the existing pipeline, what it costs, and how we can verify it on our labeled data. Distinguish clearly between published evidence, production practice, and your own reasoning. Cite sources with links and dates.

## 1. The project: Omnisnap

- **What it is:** an open-source CLI plus SDKs that render a URL in **every browser already installed on the machine** (no browser downloads) and report which elements render differently between engines, with screenshots as evidence.
- **Users:** frontend developers running it locally before committing, and CI pipelines (a GitHub Action posting a report on each PR; report-only first, a pass/fail gate later).
- **v1 headline feature:** an element-level, cross-engine **layout diff** that says *which element* differs and *why*, instead of a pixel heatmap.
- **Architecture (decided):** one **Go** binary (job JSON in, artifact bundle out), shipped to npm and PyPI as thin wrappers. Package must stay under ~15 MB per platform. License Apache-2.0, so dependencies must be license-compatible (no AGPL).
- **Protocols:** Chrome DevTools Protocol for Blink (Chrome, Chromium, Edge, Brave, Opera, Vivaldi); WebDriver BiDi for Gecko (Firefox family); **W3C WebDriver Classic for WebKit** (real Safari via `safaridriver` on macOS, WebKitGTK on Linux). Classic has no network events, no scripts injected before page load, and viewport-only screenshots (full page means scroll-and-stitch).
- **The page-side logic is injected JavaScript** that must run unchanged in all three engines and over all three protocols. The analysis runs in Go on the JSON it returns.
- **Platforms:** Windows, macOS, Linux. Real Safari only exists on macOS (developers on Windows/Linux get it through macOS CI runners).

## 2. The current pipeline

**Capture (per engine, 2 runs each, engines interleaved per page):**
1. Viewport 1280×800, device pixel ratio 1.
2. Normalization:
   - **font parity:** engines that use fontconfig (Gecko, WebKitGTK) are pointed at Blink's fonts for the generic families, because on Linux Blink resolves `sans-serif` to Liberation Sans and Firefox to Noto Sans, a 5–16% width difference;
   - the root scrollbar and its gutter are hidden in every engine;
   - CJK fonts are installed (missing glyphs render differently: hex boxes in Firefox, empty boxes in Chrome).
3. Stabilization, in this order:
   - wait for `document.readyState`;
   - wait for `document.fonts.ready`;
   - stepped scroll through the page to trigger lazy loading;
   - wait for images to load;
   - finish finite animations and freeze infinite ones at t = 0;
   - two `requestAnimationFrame` ticks;
   - **freeze all JS timers**, so geometry and screenshot describe the same moment (added after a slideshow advanced between them).
4. Geometry extraction (injected JS). For every rendered element:
   - identity: tag, id, `data-testid`, role, first classes, accessible-ish name, `href`/`src` (host + path);
   - own text: hash, number of line boxes, horizontal text extent;
   - border box in document coordinates (0.1 px), computed `display` and `position`, hidden flag (visibility/opacity);
   - overflow amounts and overflow modes.

   Open shadow roots are included, iframes are not, and `<svg>` is a single leaf box.
5. A full-page screenshot, then the geometry is **measured again** (consistency check).

**Matching:** a hierarchical tree alignment. Children are aligned with an LCS over identity tokens (id > test id > tag + href/src > tag + name > tag + text hash > tag + classes). Leftovers are paired in order by tag + class, then by tag. On real sites **97.3–100% of elements match**.

**Deltas and tolerances:**
- A size change is reported if it exceeds max(2 px, 1%). For *explaining* other changes an absolute 2 px is used, because a 4 px change on an 800 px box still moves everything below it.
- Position is compared relative to the parent, with a 2 px tolerance (inline boxes get a tolerance of max(2, 0.2 × height)).
- Overflow counts only if it appears or disappears AND changes by more than 4 px.
- Inline-box heights are ignored: the content-area height differs per engine for the same font and doesn't affect layout.

**Cause/effect rules.** Each changed element is classified as a cause or an effect:
- **Width direction:**
  - a child whose width keeps the same offset or proportion to its resized parent is an effect;
  - a shrink-to-fit parent pushed by a non-tracking child is an effect;
  - an element whose own text got wider or narrower is a cause;
  - a share of a resized parent (table cells, flex-grow) is an effect;
  - an element redistributed by a flex/grid sibling is an effect.
- **Boxes:** a box that grew because its children grew, wrapped or moved is an effect.
- **Siblings:** an item that moved because a sibling resized, or because an earlier sibling moved the same way, is an effect.
- **Children moved, none resized:** one cause on the parent (padding, gap, order).
- **Flex/grid rows:** stretching like the siblings is an effect.
- **Inline flow:** only the first changed inline box in a block is a cause.
- **Overflow** explained by a descendant is a symptom, not a cause.
- **Presence:** an element present in one engine only is a cause; extras on both sides under one parent become one "children differ" finding.
- **Visibility:** a flip between visible and hidden is a cause (topmost only).

**Dynamic content:** anything that differs between the two runs of the *same* engine is masked (ads, slideshows, rotating logos). A page-level warning fires when engines rendered different page states.

**Reporting:**
- **Grouping:** findings with the same structure and the same difference are merged ("×29").
- **Line-pitch explanation:** when line spacing differs, the finding says so and is grouped by that mechanism, e.g. "line pitch 22.8 → 22.0 px over 30 lines (fractional line-height rounded differently)".
- **Severity:**
  - 3 = present in only one engine, hidden in one engine, or horizontal overflow;
  - 2 = wrapped differently, vertical overflow, or ≥ 10 px;
  - 1 = everything else.

**Pixel layer:** SSIM only on matched **graphics** elements (img/svg/canvas/video/picture/iframe), cropped at each engine's own position, best offset within ±3 px, flagged below 0.8, skipping dynamic elements. Page-level DSSIM is used only as an alarm for divergent page states.

## 3. How we measured, and the results

**Labels**, given by the project owner for each reported finding:
- **real:** a genuine rendering difference I'd want the tool to tell me about;
- **trivial:** genuine, but not worth a developer's time;
- **false:** not actually different, or the wrong element was blamed.

**Pass criteria (fixed before measuring):**

| Criterion | Target |
|---|---|
| Planted bugs found at the right element | ≥ 9/10 |
| Findings on control pages | 0 |
| Elements matched (own app / real sites) | ≥ 95% / ≥ 85% |
| Findings per page | ≤ ~10 |
| **Reported findings that are real** | **≥ 80%** |

**Results:**
- Planted bugs: **10/10**, each found as a single finding at the right element. Bugs were planted only in Firefox via UA-conditional CSS: width change, wrapping heading, overflowing table, missing banner, padding, flex order swap, font change, clipped text, flex gap, resized SVG.
- **0 findings on 61 bug-free control pages** across five engines.
- Elements matched: 99.7% on real sites, 100% on the owner's site.
- Median findings per page: 0–1.

**Precision is the failure.** Engines used: Chrome 155 (baseline) vs Firefox 153 ESR, Chromium 156, Brave 157, Edge 157 and WebKitGTK 2.50, all on Linux. The ten real sites were MDN, go.dev, python.org, three Bootstrap examples, tailwindcss.com, Hacker News, Wikipedia and Stripe; plus the owner's site, synaptech-solutions.com.

| Set | Real | Trivial | False | Strict precision | Genuine (real + trivial) |
|---|---|---|---|---|---|
| All labeled, incl. Stripe | 21 | 17 | 7 | **46.7%** | 84% |
| Excluding Stripe (28 findings, Appendix A) | 7 | 16 | 5 | **25%** | 82% |
| Owner's site vs WebKit | 1 | 5 | 0 | 17% per finding; **1/1 per cause** | 100% |

Stripe's labels are invalid as a measure of layout. The engines received different A/B variants (randomly assigned per fresh session), and JS timers kept changing the page. The owner described it as "a completely different site".

## 4. Where precision is lost (the failure analysis to research against)

**A. Genuine but not worth reporting: the dominant problem (16 of 28).**
- Line-breaking detail in reference lists (15–50 px).
- Code-block widths (30–36 px).
- 2–4 px heights.
- A 4 px narrower upvote link repeated ×29.
- **Size doesn't separate real from trivial:** real findings exist at 2.1 px and 5.9 px, trivial ones at 49.7 px. Two Wikipedia reference spans were labeled real at 61.7 px and trivial at 49.7 px.

**B. False positives (5 of 28).** Examples:
- an HN table body 32 px taller from line spacing 17.1 → 17.6 px over 61 lines;
- a 9 px narrower HN search input;
- a 3 px taller table cell;
- a 3.9 px wider navbox list item;
- a paragraph flagged as "children-moved" with no visible change.

**C. Context decides relevance, not the measurement.** The same mechanism (fractional line-height rounded differently, compounding over many lines) was labeled:
- **real** for synaptech's terminal block (`#terminal-output`: 30 lines, line-height 22.75 px, laid out at 22 px in WebKit, 24 px shorter inside a panel);
- **false** for Hacker News's main table (61 lines, 32 px taller in Firefox).

Also: the eleven 2–4 px paragraph instances of the synaptech mechanism were trivial while the terminal instance was real. **The unit of reporting matters:** per finding the owner's site scores 1/6; per cause, ranked by impact, 1/1.

**D. Dynamic pages and page state:** Stripe (A/B variant per session, timers). Now detected and warned about, but not solved. True clock control (fake timers installed before page scripts and advanced identically) is possible over CDP and BiDi but **not over WebDriver Classic, i.e. not for real Safari.**

**E. Label noise:** one person, about 45 labels, and similar findings labeled differently. Any method must be robust to this and should come with a plan to grow and validate the label set.

**Already ruled out (with evidence), so don't recommend these as the primary detector:**
- **Whole-page pixel diff across engines.** SSIM/DSSIM over 8×8 windows with the original 0.02 DSSIM threshold: both bug-free Chrome-vs-Firefox control pages FAIL (DSSIM 0.027 / 0.045, 9–12% of windows divergent), and two planted bugs (DSSIM 0.003 / 0.007) fall below that noise. `pixelmatch` with antialiasing detection still leaves 1.4–2.5% of pixels differing on a page with zero layout differences. **Chrome (Skia) and Firefox rasterize the same glyphs from the same font file differently.** (WebKitGTK now also uses Skia and is pixel-close to Chrome, but real Safari uses CoreGraphics.)
- **Raising the size threshold:** see A.
- **Treating branded Blink browsers as extra engines:** Chrome 155, Chromium 156, Brave 157 and Edge 157 were pixel-identical (SSIM 1.0000) on every test page and on the owner's site. They only matter where their product features act, such as content blocking.

## 5. Research questions (answer each explicitly)

**R1. Reporting unit and ranking.** What should the reported unit be: element, cause/mechanism, visual region, or user-facing defect type? How do the best academic and commercial tools group and rank? Propose a ranking (e.g. by impact on the visible composition) and the right metrics: precision@k, per-cause precision, nDCG.

**R2. Relevance / severity modeling.** Which signals predict that a developer cares? Candidates to evaluate, with evidence:
- text truncated or clipped; overlap or occlusion; overflow and horizontal scroll;
- content pushed out of, or blank space created inside, a fixed-size container;
- interactive targets moved, resized or covered; misalignment of elements that were aligned (relative layout);
- position above the fold; visual salience; element area; contrast; repetition across the page.

Design a relevance model that runs in Go without heavy ML runtimes:
- heuristic scoring first;
- then a small learned model (logistic regression, gradient-boosted stumps, learning-to-rank) trainable from 50–500 labels;
- active learning to choose what to label next.

**R3. What to compare.** Absolute geometry vs **relative layout** (alignment and containment relations between siblings and parents, as in X-PERT's "relative layout failures"), vs responsive-layout-failure classes (ReDeCheck: overflow, collision, wrapping, protrusion, small-range), vs perception-based comparison. Which representation produces fewer genuine-but-trivial findings? Is better tree matching worth it (GumTree, RTED/APTED tree edit distance) when we already match 97–100%?

**R4. A knowledge base of expected engine differences.** Catalog the systematic, *expected* Blink/Gecko/WebKit differences:
- UA stylesheet defaults and form controls;
- scrollbars;
- font metrics: ascent/descent, inline content area;
- **fractional line-height rounding**;
- subpixel layout and LayoutUnit precision;
- text shaping and line-breaking (UAX #14, URLs, hyphenation), `text-wrap`, emoji fonts.

How can these be detected (signatures) and downgraded, or turned into a single explained cause with a fix ("use a whole-pixel line-height")? How do commercial tools handle this (e.g. Applitools' Strict / Layout / Content match levels)?

**R5. Determinism and noise floor.** Best practice for making dynamic pages comparable across engines:
- clock control per protocol: CDP virtual time (`Emulation.setVirtualTimePolicy`), BiDi preload scripts with fake timers, what is possible on WebDriver Classic/Safari;
- network record and replay (HAR) so every engine gets identical content;
- seeding A/B experiments through cookies or storage;
- how many repeated runs, and which statistics, to set a per-page noise floor.

**R6. Root-cause explanation.** Techniques for mapping a geometry difference to the CSS responsible: computed-style diffs between engines, cascade tracing, minimal-CSS-change search as in XFix-style automated repair. The goal is "line-height 22.75px → rounded to 22px in WebKit; fix: leading-[23px]".

**R7. Pixel metrics for the graphics layer only.** SSIM vs MS-SSIM vs FLIP vs LPIPS vs butteraugli for cropped images, SVG, canvas and video: accuracy, cost per crop, licenses, availability in pure Go.

**R8. Evaluation methodology.**
- How large a labeled corpus is needed to trust an 80% precision estimate (confidence intervals)?
- Inter-rater agreement with more than one labeler.
- Synthetic bug injection (CSS mutation testing) to measure recall at scale.
- Public XBI or layout-failure datasets from prior work that we could reuse.

**R9. Prior art with numbers.** Academic: WebDiff (2010), CrossCheck (2012), X-PERT (2013), Browserbite, XFix (2017), ReDeCheck (2017) and later responsive-layout work, WebSee and other presentation-failure detection, ML-based visual-bug classification. Industry: Applitools, Percy, Chromatic, Happo, Argos, Lost Pixel, BackstopJS, LambdaTest SmartUI, BrowserStack. For each: what they compare, how they reduce noise, and **reported precision/recall** if any.

## 6. Constraints on proposed solutions

- **Runs inside our architecture:** injected JS that works over CDP, BiDi and WebDriver Classic (no reliance on CDP-only APIs for the core path), plus analysis in Go.
- **Lightweight:**
  - no Python/PyTorch/ONNX in the core (an optional plugin is acceptable);
  - total package under ~15 MB;
  - analysis within seconds per page on a laptop CPU.
- **Explainable:** every reported item needs a selector, a cause in plain words, evidence (crops), and ideally a fix.
- **Deterministic:** the same input gives the same report.
- **Must keep what works:** 10/10 planted recall, 0 findings on control pages, 97%+ matching.
- Apache-2.0-compatible dependencies; Windows/macOS/Linux; real Safari only through macOS.

## 7. Deliverables (required format)

1. **Executive summary:** the top 5–7 recommendations ranked by expected precision gain ÷ cost, each with a confidence level and the failure cases (A–E, Appendix A rows) it addresses.
2. **Technique dossiers**, one per technique, each covering:
   - what it is;
   - evidence (with citations);
   - where it plugs into our pipeline (capture / stabilize / extract / match / classify / group / rank / report);
   - which Appendix A findings it would fix or break;
   - an implementation sketch (injected-JS fields to add, Go logic, data needed);
   - complexity and runtime;
   - risks;
   - **an experiment on our corpus, with a pass criterion**.
3. **Prior-art table:** tool or paper, year, comparison method, noise handling, reported precision/recall, licence/availability.
4. **A relevance model specification:** features (each computable from our data or a named addition), a scoring function for day 1, a learning procedure for later, and how to get to ≥ 80% precision at the chosen reporting unit without losing recall.
5. **A knowledge-base outline of expected engine differences:** a signature, a detection rule and a suggested fix for each entry.
6. **An experiment plan for the next spike round:** ordered, with hypotheses, metrics and stop/go criteria, including how to grow and validate the label set.
7. **What not to do:** with reasons and evidence.
8. **Bibliography:** links and publication dates; mark each claim as published evidence, vendor documentation, or your inference.

## 8. Quality bar

- Prefer primary sources: papers, engine source code and bug trackers (Chromium, Gecko, WebKit), specs, and vendor documentation.
- Give numbers where they exist.
- Say when evidence is weak or missing.
- No generic advice ("use AI", "tune thresholds") unless it comes with a concrete method tied to our data.
- Where you're unsure about current behavior of an engine (versions above), say so and suggest how to verify it.

## Appendix A: every labeled finding, excluding Stripe (28)

Chrome 155 is the baseline; "vs" is the compared engine. Deltas are B minus A, in CSS px (w = width, h = height, x/y = position relative to the parent).

| label | site | vs | kind | element (selector tail) | measured difference |
|---|---|---|---|---|---|
| real | wikipedia | firefox | width | `span#mwUw` (a URL in running text) | lines 1→2, w +144.2, x -5 |
| real | wikipedia | firefox | width | `span#mw-reference-text-cite_note-31` | w -61.7 |
| real | synaptech | webkit | children-moved | `div#terminal-output` | line pitch 26.8 → 26.0 px over 30 lines; h -24 |
| real | wikipedia | firefox | width | `span#mw-reference-text-cite_note-12` | w -15.3 |
| real | go-dev | firefox | children-moved | `…Playground-controls > div.Playground-buttons` | x +5.9 |
| real | wikipedia | firefox | position | `li#mwAUU` | y +2.1 |
| real | python-org | firefox | children-moved | `div#content > div.container > section.main-content` | (children moved; no delta on the element itself) |
| trivial | wikipedia | firefox | width | `span#mw-reference-text-cite_note-36` | w -49.7 |
| trivial | tailwindcss | firefox | width + children-moved | `… pre > code > code` (code sample) | w -36, x +36 |
| trivial | tailwindcss | firefox | width | `div.max-w-screen.overflow-x-hidden > div.fixed` | w -30.7 |
| trivial | wikipedia | firefox | width ×2 | `span#mw-reference-text-cite_note-4` | w -22.4 |
| trivial | wikipedia | firefox | height | `…navbox-subgroup > tbody > tr > th.navbox-group` | h -21.2 |
| trivial | synaptech | webkit | children-moved | `div#how-i-work-right > div.px-6` | line pitch 28.8 → 28.4 px over 12 lines; h -6 |
| trivial | synaptech | webkit | children-moved | `div#home-services-project > ul.divide-y` | line pitch 35.6 → 35.2 px over 16 lines; h -6 |
| trivial | hn | firefox | width ×29 | `a#up_…` (upvote arrow link, every story) | w -4 |
| trivial | synaptech | webkit | height | `div#home-services-automation > ul.divide-y` | line pitch 33.6 → 33.1 px over 8 lines; h -3 |
| trivial | wikipedia | firefox | height | `…vector-pinnable-header-unpin-button` | h +3 |
| trivial | wikipedia | firefox | height | `…vector-pinnable-header-unpin-button` (second instance) | h +3 |
| trivial | synaptech | webkit | height | `div#manifesto-statement > div > p.font-mono.text-xs` | line pitch 19.5 → 19.0 px over 5 lines; h -2.5 |
| trivial | wikipedia | firefox | width ×2 | `th.navbox-title > div.navbar.plainlinks` | w +2.4 |
| trivial | synaptech | webkit | height ×11 | `p.text-sm.text-ink/70` (body paragraphs) | line pitch 22.8 → 22.0 px over 3–6 lines; h -2.3 to -4.5 |
| trivial | wikipedia | firefox | position | `span#mwA74` | x +2.2 |
| trivial | wikipedia | firefox | children-moved ×5 | `a#mwAsQ` | (children moved; no delta on the element itself) |
| false | hn | firefox | children-moved | `tr#bigbox > td > table > tbody` (main story table) | line pitch 17.1 → 17.6 px over 61 lines; h +32 |
| false | hn | firefox | width | `form > input` (search box) | w -9, x +4.5 |
| false | wikipedia | firefox | width | `…navbox-list > div > ul > li` | w +3.9 |
| false | hn | firefox | height | `table#hnmain > tbody > tr > td` | h +3 |
| false | wikipedia | firefox | children-moved | `p#mwnQ` | (children moved; no visible change) |

## Appendix B: finding kinds

| Kind | Meaning |
|---|---|
| width / height | the element's own box changed size and nothing else explains it |
| wrap | same width, different number of text lines |
| position | moved relative to its parent, not explained by siblings or its own growth |
| children-moved | its children moved but none resized (padding, gap, order, line spacing) |
| children-differ | different child elements in each engine |
| only-in-*engine* | element rendered in one engine only |
| hidden-in-*engine* | element visible in one engine, hidden in the other |
| overflow-x / overflow-y | content starts or stops overflowing the box (by more than 4 px) |
