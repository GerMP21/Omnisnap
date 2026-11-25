# Glossary

The words the pages use, in the sense the code uses them.

| Term | Meaning |
|---|---|
| **Baseline** | The engine every other engine is compared with: a Blink browser, Chrome by default ([`makePlan`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/run.go)). Results are always "baseline vs engine". |
| **Engine** | One browser a job captures with, named by its id (`chrome`, `firefox`, `webkit`, `safari`, `edge`, `brave`…). Several browsers can share an engine family. |
| **Engine family** | `blink`, `gecko` or `webkit`. A cross-engine comparison needs at least two families; design intent and consensus need a third. |
| **Job** | What to capture: pages, engines, baseline, viewport, runs, probes, font parity, timeouts, output. `job.json`, or built from a URL and flags. See [Data contracts](Data-Contracts). |
| **Slug** | A page's directory name in the bundle, derived from its URL unless the job names one; duplicates get `-2`, `-3`. |
| **Bundle** | The output directory: captures, screenshots, probes, `summary.json`, `findings.json`, `host.json`, `report.html`. Everything the report needs, readable offline. |
| **Capture** | One engine's record of one page in one run: `<engine>-<run>.json` (the geometry tree and metadata), `<engine>-<run>.png`, and `<engine>-<run>.probes.json.gz`. |
| **Run** | One of the captures of a page per engine, named `a`, `b`, …; two by default. Every engine's run a is taken before any run b. |
| **Lane** | A sequence of captures that runs in parallel with the other lane: `cdp+bidi` (Blink and Gecko) and `classic` (WebKit) ([`lanes.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/capture/lanes.go)). |
| **Unreachable page** | A page whose baseline run a timed out, failed to navigate, or rendered fewer than 50 elements (a bot wall); nothing else is captured and `failed.json` says why ([`policy.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/capture/policy.go)). |
| **Lost engine** | An engine whose capture crashed the browser and whose retry, in a fresh browser, crashed too or could not start. Its later captures are skipped and name the crash; the other engines go on ([`crash.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/capture/crash.go)). |
| **Probe** | A re-measure of the page with one suspected cause neutralized: `lh` (whole-pixel line heights), `font` (one monospace font with fixed advances, no ligatures), `controls` (form controls with `appearance: none` and fixed padding and border), `all` (the three together). A difference a probe removes has that **mechanism**. |
| **Dynamic mask** | Elements that changed between two runs of the same engine (carousels, clocks, ads). They are left out of causes and shaded in the viewer. |
| **Match** | The pairing of baseline elements with engine elements ([`MatchTrees`](https://github.com/GerMP21/Omnisnap/blob/main/internal/layoutdiff/match.go)). Match rate = matched / baseline elements. |
| **Selector** | How an element is named everywhere (causes, intent, warnings, audit findings): valid CSS of up to five segments, from the nearest id, `body` or `html`, with `:nth-of-type` counted over the DOM's same-tag siblings, hidden ones included ([`selector.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/selector/selector.go)). |
| **Delta** | The measured difference of one matched element: width, height, position, lines, overflow, truncation, visibility, text shift. |
| **Cause** | An element where a difference **starts**, with its kinds (`width`, `wrap`, `only-in-firefox`…). What the report lists. |
| **Effect** | A difference explained by another element's cause: a child reflowing in a narrower parent, a sibling pushed down. Not reported. |
| **Symptom** | An overflow that another element already explains; kept in `findings.json`, not reported as a cause. |
| **Kind** | The type of a cause: `only-in-*`, `hidden-in-*`, `truncated-in-*`, `children-differ`, `children-moved`, `wrap`, `width`, `height`, `position`, `overflow-x`, `overflow-y`, `text-shifted`, `columns`, `intent-*`, `appearance`. |
| **Design intent** | A relation between two elements (shared edge, equal size, equal gap, grid column, no overlap) that two engine families keep and one engine breaks. |
| **KB entry** | A known, expected engine difference in the knowledge base (K1, K2, K4, K5, K8), with an explanation and a fix. |
| **Consequence** | What a cause does to the page a person sees: `presence`, `visibility`, `clipped`, `scrollbar`, `overflow`, `protrusion`, `truncation`, `line-count`, `interactive-moved`, `structural`, `intent-broken`, `content-moved`, `appearance`. Some are **hard** (always Report). |
| **Tier** | `report` (worth your time) or `info` (expected or invisible). Set by rules, not by the model. |
| **Evidence** | Per cause: `mechanism`, `perception`, `calibration`, `consensus`, `ink`, `intent`, `novelty`. Why it happens and whether it shows. |
| **Score** | The tree model's probability that a cause looks like an injected bug rather than natural engine variation. Ranks causes within a tier. |
| **Fingerprint** | A cause's class (kinds, element shape, rounded deltas). The novelty table counts how many crawled sites show each fingerprint. |
| **Ink** | Pixels that differ from a crop's background. Ink agreement between two crops says whether a change is visible, against the page's noise floor. |
| **Planted page** | A small synthetic page with one known bug (or none: a **control** page, or a **KB** fixture). The planted suite is the recall gate. See [Ground truth and the model](Ground-Truth-and-the-Model). |
| **Branded browser** | A Blink browser as shipped with its own blocking (Brave Shields, Edge tracking prevention), captured from a warmed template profile. |
| **Host** | The machine a bundle was captured on (OS, architecture, engines). Merged reports keep one baseline per host. |
