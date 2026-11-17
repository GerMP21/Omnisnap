# Omnisnap

Status (2026-11-08): **v1 specified and planned**: 17 module specs in [specs/](../../specs/CAPABILITY-MAP.md), 86 tasks in [tasks/](../../tasks/plan.md). The layout-diff spike (rounds 1–6) validated detection; precision on real sites is not measured yet. · License: Apache-2.0 · Related: [PRD](../prd.md) (amended by its changelog) · [Idea Refinement](../idea-refinement.md) · [Spike README](../../spikes/layout-diff/README.md) · [Scientific grounding](../SCIENTIFIC_GROUNDING.md)

## Problem Statement
How might we tell a frontend developer exactly which elements render differently across the engines their users actually run, in seconds, using only the browsers already installed and no test code?

## Recommended Direction
**The Inspector, with a report-only CI Action.**

`omnisnap <url>` finds every browser on the machine and captures each engine after the page has settled. It then compares *element geometry* (sizes, positions, line wraps, overflow) against the Blink render, reporting each difference at the element where it starts. Each difference comes with what a developer needs to act on it:
- why it happens (causal probes, with the fix verified when the mechanism is known);
- whether anyone would see it;
- which engine is the odd one out;
- which design rule it breaks.

A side-by-side viewer outlines each finding on every engine's screenshot. Branded browsers run as shipped, from prepared ("warmed") template profiles, and their differences come with a cause: the requests their built-in blockers stopped.

The same job runs in a GitHub Action on Ubuntu, Windows and macOS runners, giving Windows and Linux developers real Safari. The Action posts one merged report on the PR and never fails the build. Report mode measures real-world noise; the pass/fail gate ships only once those numbers say it's safe.

**Why this direction**
- Element-level layout comparison is a capability no tool ships. Pixel diffs break at the first reflow, and existing tools compare each browser only with its own past screenshots.
- A short demo of one command showing every engine is what wins OSS adoption.
- Report-only CI delivers Safari without the false failures that get visual gates switched off.

**Architecture:** a single Go binary (`job.json` in → artifact bundle out). The injected JS does the real work. npm and PyPI packages are thin wrappers around the binary. Nothing is downloaded by default; `omnisnap install firefox` is an explicit opt-in. The repository lives in the author's own GitHub organization. Licensed Apache-2.0.

## Key Assumptions to Validate
- [ ] **Layout diff is mostly signal.** *Detection validated, precision open (2026-11-08):*
  - 24/24 planted bugs found at the right element, including 16 realistic cross-engine bug patterns added in round 5;
  - 0 Report causes on bug-free control pages in five engines;
  - design intent catches 6/6 planted intent bugs, with no false alarm;
  - elements matched: 100% on the owner's site, ≥ 99.9% on real sites.

  Precision on real sites is not measured: manual labels were withdrawn as unreliable. It will be measured with automated ground truth: realistic bugs injected into real pages, and a tree model compared with the rules at equal recall. The round-6 training run is paused until drive C: has free space (task 76).
- [ ] **Branded quirks reproduce from warmed template profiles.** *Half validated:* Brave yes (fresh profile blocked 0, warmed 3). Edge no: its lists are a downloadable component, and it blocked nothing under headless automation even after downloading them. Next: test Edge headed on a Windows runner (task 68).
- [ ] **Safari runs unattended on GitHub macOS runners.** Not tested yet: a ten-line smoke workflow in week one (task 2). WebKitGTK on Linux works headless as a WebKit *layout* proxy (it renders with Skia, so not Safari *pixels*).
- [ ] **Developers will actually run it.** The report-only Action stays installed on ≥ 5 outside repos for a month; the README demo turns into stars and installs.
- [ ] **Browser discovery stays under 300 ms** on Windows and Linux. The slow parts are Flatpak, Snap and reading version numbers (tasks 30–32).

## MVP Scope
| Step | Scope |
|---|---|
| **Spike** (throwaway, done) | Rounds 1–6 in `spikes/layout-diff/`. What it hands over:<ul><li>the injected scripts: settling, extraction, causal probes, bug injection;</li><li>the matching, root-cause, triage and design-intent rules;</li><li>the planted suite (31 pages);</li><li>mutation testing and the tree-model training pipeline.</li></ul> |
| **M0**: capture + viewer | Go core:<ul><li>browser discovery (Windows registry; Linux .desktop files, PATH, Flatpak, Snap; macOS mdfind);</li><li>CDP / BiDi / WebDriver Classic drivers;</li><li>capture discipline: font parity, two runs, parallel engine lanes, failure policy, resume;</li><li>process and profile cleanup;</li><li>`job.json` → bundle;</li><li>side-by-side viewer and single-engine DOM audit;</li><li>Safari smoke test on a macOS runner;</li><li>opt-in `omnisnap install firefox`.</li></ul>Tasks 1–38. |
| **M1**: layout diff | Layout diff ported to Go (matching, root causes, design intent). On top of it:<ul><li>appearance checks on matched crops of graphics elements;</li><li>triage: the catalog of expected engine differences, visible consequences, Report/Info tiers, evidence;</li><li>the tree-model score, used for ranking;</li><li>branded-browser causes (template profiles + blocked-request list);</li><li>the planted suite as CI gates.</li></ul>Tasks 39–76. |
| **M2**: CI report | GitHub Action across 3 OS runners, `omnisnap merge`, PR comment, report-only. npm and PyPI packages ship here. **v1 = M2 released.** Tasks 77–86. |

## Not Doing (and Why)
- **Pass/fail gate:** one wrong failure gets the Action uninstalled. It waits for noise data from report mode.
- **Whole-page pixel/SSIM diff across engines:** breaks at the first reflow, and bug-free pages fail it from rasterization noise alone. Pixels are only compared on matched element crops.
- **Two native implementations:** one Go core; Node and Python are thin wrappers.
- **Neutralizing branded browsers by default:** that erases the quirks we're trying to catch. `--neutral` is opt-in.
- **Bundling or silently downloading browsers:** only the explicit `omnisnap install firefox`.
- **Per-browser quirk handlers up front:** one adapter per engine family. A branded browser gets its own handler only when a real bug demands it. Arc is dropped (maintenance mode); Tor Browser is out (it disables remote automation by design).
- **An LLM or vision model in the product:** the only learned part is a small local tree model (LightGBM), trained on automatically generated ground truth. The MCP layer (variation 6) comes after v1, built on the JSON output.
- **Manual labels as ground truth:** withdrawn as unreliable (PRD 1.4.0). Ground truth comes from planted pages, injected bugs and null pairs.
- **Clock control and A/B seeding in v1:** deferred until report mode shows how often engines render different page states.
- **Multi-machine capture network (variation 7):** after v1. M2's job spec, bundles and merge command lay its groundwork.
- **Mobile browsers:** they need simulators or devices; a different problem.
- **GitLab, Bitbucket, self-hosted runners:** GitHub first.

## Open Questions
- **Go module path:** `github.com/<the author's org>/omnisnap`. The org name is to be filled in before task 1.
- **Register `omnisnap.org`, and claim `omnisnap` on npm and PyPI** (task 85). `omnisnap.org` was registered on 2026-11-17; npm and PyPI remain. Registry lookups (RDAP) on 2026-11-06 had shown no registration for `.org`. `.com`, `.net` and `.app` are taken, and `.dev` is a parked Hostinger page. Publish placeholder releases before announcing; that also confirms npm accepts the name. Platform packages can use `omnisnap-<os>-<arch>` names, so no npm scope is needed.
- **The tree model's role in v1:** ranking only, or also deciding the tier? Task 76 decides, once training has data.
- **Safari on every PR, or only on labeled ones?** macOS runners cost more (task 79).
- **DOM audit in v1:** proposed as informational only, with checks promoted one by one.
