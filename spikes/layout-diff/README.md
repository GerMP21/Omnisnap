# Spike: cross-engine layout diff

**Question:** can comparing element geometry between browser engines report the differences a developer actually wants to know about?

**Status (2026-11-07, round 3):**
- **Detection works.** 10/10 planted bugs are found at the right element; bug-free control pages produce nothing; 97–100% of elements are matched.
- **Precision is the open problem.** The research report and round 3 move it from per-element findings to **causes in two tiers** (Report / Info), checked against a **catalog of expected engine differences**. See [Results](#results) and the PRD changelog 1.2.0 ([docs/prd.md](../../docs/prd.md)).

> **Throwaway code.** `puppeteer-core` and the small WebDriver client stand in for the future Go drivers. What survives is listed below.

| File | Role | Fate |
|---|---|---|
| `inject/stabilize.js` | Settles the page: fonts, lazy content, animations, then **freezes JS timers** | Embedded unchanged in the Go core |
| `inject/extract.js` | Element geometry, line boxes, typography, interactivity, truncation | Embedded unchanged in the Go core |
| `lib/align.mjs` | Element matching across engines and root-cause analysis | **Ported to Go** |
| `lib/triage.mjs` | Catalog of expected engine differences (the knowledge base, "KB"), visible consequences, Report/Info tiers, impact | **Ported to Go** |
| `lib/webdriver.mjs` | Minimal W3C WebDriver Classic client (WebKit/Safari), headless through Weston | Reference for the Go client |
| `planted/pages.mjs`, `labels.json`, `labeled-findings.json` | Pages with known answers, human labels, and the measurements they judged | **Regression suite** |
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
| `npm run capture:planted` / `capture:own` / `capture:real` | Captures each page group in all installed engines (real sites in Chrome + Firefox only), 2 runs each, engines interleaved per page |
| `npm run diff` | Matches elements, finds root causes, triages them into Report/Info causes. Writes `out/findings.json` and `out/report.html` |
| `npm run score` | Checks the pass criteria (Wilson intervals on precision) |
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

## Labels and scoring

- **Labels:**
  - **real:** worth reporting;
  - **trivial:** genuine, but not worth a developer's time;
  - **false:** not actually different, or the wrong element was blamed.

  Label per cause in `out/report.html`, then click **Export labels.json** and save the file here. Labels given to individual findings in earlier rounds still count: a cause is real if any of its instances is real.
- **`labeled-findings.json`:** every labeled finding with the measurements it was judged on. It keeps labels reproducible after `out/` (which is gitignored) is regenerated.
- **`score.mjs`:**
  - Recall is checked **in the Report tier**.
  - Precision is measured on **Report-tier causes**, with a **Wilson 95% interval**. It passes only if the *lower bound* is ≥ 80%.
  - Real causes demoted to Info are counted.
  - Pages whose engines rendered different page states (Stripe) are excluded from precision, because they measure the state gap, not layout.
- **Planted differences** fire in Firefox only (UA-conditional CSS). For every other engine the planted pages act as controls.
- **Catalog test pages** (`14-kb-line-height`, `15-kb-line-height-normal`) may differ between engines, but only as explained Info causes.

## Results

### Round 3 (2026-11-07): causes, tiers, knowledge base

Engines: Chrome 155 (baseline) vs Chromium 156, Brave 157, Edge 157, Firefox 153 ESR and WebKitGTK 2.50 on 15 planted/control/KB pages and synaptech-solutions.com; Chrome vs Firefox on the 10 real sites (recaptured). 0 capture failures.

| Criterion | Target | Result |
|---|---|---|
| Planted bugs reported at the right element (Report tier) | ≥ 9/10 | **10/10**, one cause each |
| Causes on control pages, all 5 compared engines | 0 | **0** |
| KB fixtures: Report causes / unexplained Info | 0 / 0 | **0 / 0** (WebKit: 1 K1 cause on the line-height page; everything else identical) |
| Elements matched: own site / real sites | ≥ 95% / ≥ 85% | **100% / 100%** (min 99.9%) |
| Report causes per page | ≤ ~10 | median **0**, max 2 |
| **Report causes that are real** (Wilson 95%) | lower bound ≥ 80% | **2/3 = 67% [21–94%]**, plus 1 unlabeled → PENDING |
| Real causes demoted to Info | few | **4** |

What changed compared with round 1, on the same 10 sites + the owner's site:

- **Reported items: 28 non-Stripe findings → 4 Report causes.**
- **False positives in the Report tier: 5 → 0.** All seven false-labeled findings land in Info, most of them explained:
  - HN's 61-line table → K2 (`line-height: normal`);
  - HN's search input → K8 (form-control defaults);
  - the rest have no visible consequence.
- **The owner's site reports nothing in any engine.** WebKit's line-pitch drift is 3 explained Info causes (K1).
- **The cost: 4 human-real items now sit in Info.** The research report predicted exactly these, as items no consequence feature can separate from the trivial ones:
  - python.org's main section (explained as K1, Firefox's fractional line-height);
  - `li#mwAUU` (y +2.1 px);
  - two reference texts that break their lines at different positions without changing their line count. Two others with the same mechanism were labeled *trivial*, so this is label noise or a missing signal.
- Stripe's two runs agreed with each other this time and its engines rendered the same state (0 causes). That's one observation, not a fix for A/B variants.

Remaining Report causes:
- go.dev playground buttons moved (real);
- a Wikipedia URL that breaks differently (K5, real);
- a fixed-position Tailwind panel 31 px narrower (trivial);
- a Wikipedia menu with different children (new, unlabeled).

**Verdict:** the Report tier now has no false positives and is small enough to read, and recall on planted bugs is intact. **80% precision can't be demonstrated with 3 labeled Report causes.** The next step is the label programme: ≥ 2 raters, cause-level labels on ~40 new sites, and labels collected on same-state evidence. Tuning on these labels any further would be overfitting. Two rules were adjusted *in principle* during this round, not to match a label: inline elements that shift inside reflowed text are neither "interactive target moved" nor "structural".

### Earlier rounds

| Round | What changed | Key result |
|---|---|---|
| 1 | Chrome 155 vs Firefox 153 on 13 planted/control pages and 10 real sites | Planted 10/10; controls 0; 99.7% matched; **precision 51% (20/39)**. Stripe's labels are invalid: different A/B variants and timer states |
| — | Pixel head-to-head (`pixeldiff.mjs`) | Whole-page SSIM/DSSIM **fails on bug-free controls** (DSSIM 0.027 / 0.045 > 0.02): Skia and Firefox rasterize the same glyphs differently. SSIM on graphics crops is clean on stable pages |
| 2 | Six engines: Chromium, Brave, Edge, WebKitGTK (WebDriver Classic, headless via Weston); own site synaptech-solutions.com | 0 findings on 48 control pages; Blink family pixel-identical; synaptech 100% matched; WebKit's 6 findings are all one mechanism (fractional line-height, 22.75 px laid out as 22 px) |
| 3 (prep) | Research report on precision (`docs/research/`) | Diagnosis: a triage problem, not detection. Report causes ranked by visible consequence; catalog expected engine differences; 28 labels cannot prove 80% (95% interval 13–43%) |

**Label correction:** the synaptech terminal was first labeled real. In the same page state (WebKit run b), the line-height drift is barely visible. The striking difference in the screenshot that was labeled came from the slideshow, which was masked. It was relabeled **trivial**. The report viewer now compares best-matching runs and shades masked regions, so dynamic content can't be mistaken for a finding.

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
| 11 | **Relevance needs more than size:** real findings at 2 px, trivial ones at 50 px | Round-1 labels |
| 12 | **No whole-page pixel pass/fail across engines;** SSIM only on aligned graphics crops; page DSSIM as a state alarm | Bug-free controls fail DSSIM 0.02 |
| 13 | **Re-measure after the screenshot,** and skip dynamic elements in the graphics comparison | synaptech slideshow |
| 14 | **Branded Blink browsers matter only where their features act;** warm their profiles | Pixel-identical; Brave blocks 0 fresh vs 3 warmed |
| 15 | **WebKit on Linux needs a headless display** and a viewport fit loop; WebKitGTK is a *layout* proxy, not Safari *pixels* | Pop-up windows; 693 px cap; Skia rendering |
| 16 | **Report causes ranked by visible consequence,** with a catalog of expected engine differences (mechanism, explanation, fix) | synaptech: 6 findings = 1 mechanism; research report |
| 17 | **Evidence must show the same page state as the finding:** compare best-matching runs and shade dynamic regions | The terminal label was given to a slideshow difference |

## Known limits

- iframes are not traversed. Open shadow roots are; closed ones can't be.
- Selectors use `:nth-of-type` over *rendered* siblings: labels for people, not guaranteed `querySelector` strings.
- An element moved to a different parent shows up as removed plus added.
- The planted pages and early labels were also used to design the rules, so their scores are optimistic. Precision claims need **new, independent labels**.
- WebKitGTK ≠ Safari for pixels, and possibly for line-height (Safari 27 reportedly lays out inline content with subpixel precision; unverified).
- Edge's tracking prevention blocked nothing under headless automation, even with its lists downloaded.
