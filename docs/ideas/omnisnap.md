# Omnisnap

Status: direction confirmed (2026-11-06); layout-diff spike rounds 1–3 done (2026-11-07): detection validated, precision still open. See PRD changelog 1.2.0 and [the spike README](../../spikes/layout-diff/README.md). · License: Apache-2.0 · Related: [PRD](../prd.md) (amended by its changelog) · [Idea Refinement](../idea-refinement.md)

## Problem Statement
How might we tell a frontend developer exactly which elements render differently across the engines their users actually run, in seconds, using only the browsers already installed and no test code?

## Recommended Direction
**The Inspector, with a report-only CI Action.**

`omnisnap <url>` finds every browser on the machine and captures each engine after the page has settled. It then compares *element geometry* (sizes, positions, line wraps, overflow) against the Blink render, reporting each difference at the element where it starts. A side-by-side viewer outlines each finding on every engine's screenshot. Branded browsers run as shipped, from prepared ("warmed") template profiles, and their differences come with a cause: the requests their built-in blockers stopped.

The same job runs in a GitHub Action on Ubuntu, Windows and macOS runners, giving Windows and Linux developers real Safari. The Action posts one merged report on the PR and never fails the build. Report mode measures real-world noise; the pass/fail gate ships only once those numbers say it's safe.

**Why this direction**
- Element-level layout comparison is a capability no tool ships. Pixel diffs break at the first reflow, and existing tools compare each browser only with its own past screenshots.
- A short demo of one command showing every engine is what wins OSS adoption.
- Report-only CI delivers Safari without the false failures that get visual gates switched off.

**Architecture:** a single Go binary (`job.json` in → artifact bundle out). The injected JS does the real work. npm and PyPI packages are thin wrappers around the binary. Nothing is downloaded by default; `omnisnap install firefox` is an explicit opt-in. Licensed Apache-2.0.

## Key Assumptions to Validate
- [ ] **Layout diff is mostly signal.** *Partly validated (2026-11-07):* planted bugs, matching and controls pass; precision fails at the finding level (51% of labeled findings real) and is being re-measured at the cause level (Report/Info tiers). Spike: Chrome vs. Firefox on your app, ~10 real sites and ~10 pages with planted bugs. Pass:
  - ≥95% of elements matched on your app (≥85% on real sites);
  - ≥80% of findings are real;
  - ≥9/10 planted bugs found at the right element.
- [ ] **Branded quirks reproduce from warmed template profiles.** *Half validated (2026-11-07):* Brave yes (fresh profile blocked 0, warmed 3); Edge no (its lists are a downloadable component, and it blocked nothing under headless automation even after downloading them). Brave with a fresh profile vs. a warmed one: count requests blocked with `ERR_BLOCKED_BY_CLIENT`. Same test for Edge (is its list built in or downloaded as a component?).
- [ ] **Safari runs unattended on GitHub macOS runners.** A ten-line smoke-test workflow, in the first week of M0.
- [ ] **Developers will actually run it.** The report-only Action stays installed on ≥5 outside repos for a month; the README demo turns into stars and installs.
- [ ] **Browser discovery stays under 300 ms** on Windows and Linux. The slow parts are Flatpak, Snap and reading version numbers.

## MVP Scope
| Step | Scope |
|---|---|
| **Spike** (throwaway) | puppeteer-core with the installed Chrome and Firefox. Builds `extract.js`, element matching + root-cause diff, and a labeled set of test pages. Only those three things are kept. |
| **M0**: capture + viewer | Go core: browser discovery (Windows registry; Linux .desktop files, PATH, Flatpak, Snap; macOS mdfind), CDP / BiDi / WebDriver Classic drivers, page settling, process cleanup (Job Objects, process groups), `job.json` → bundle. Synced side-by-side viewer + DOM audit, macOS-runner smoke test, opt-in `omnisnap install firefox`. |
| **M1**: layout diff | Algorithm ported to Go; per-element crops (with SSIM on matched crops for appearance-only differences); `summary.json`; branded-browser causes (template profiles + blocked-request list). The labeled pages become regression tests. |
| **M2**: CI report | GitHub Action across 3 OS runners, `omnisnap merge`, PR comment, report-only. npm and PyPI packages ship here. **v1 = M2 released.** |

## Not Doing (and Why)
- **Pass/fail gate:** one wrong failure gets the Action uninstalled. It waits for noise data from report mode.
- **Whole-page pixel/SSIM diff across engines:** breaks at the first reflow. SSIM is only used on matched element crops.
- **Two native implementations:** one Go core; Node and Python are thin wrappers.
- **Neutralizing branded browsers by default:** that erases the quirks we're trying to catch. `--neutral` is opt-in.
- **Bundling or silently downloading browsers:** only the explicit `omnisnap install firefox`.
- **Per-browser quirk handlers up front:** one adapter per engine family. A branded browser gets its own handler only when a real bug demands it. Arc is dropped (maintenance mode); Tor Browser is out (it disables remote automation by design).
- **Embedded AI/vision model:** the MCP layer (variation 6) comes after v1, built on the JSON output.
- **Multi-machine capture network (variation 7):** after v1. M2's job spec, bundles and merge command lay its groundwork.
- **Mobile browsers:** they need simulators or devices; a different problem.
- **GitLab, Bitbucket, self-hosted runners:** GitHub first.

## Open Questions
- **Register `omnisnap.org`, and claim `omnisnap` on npm, PyPI and GitHub.** Registry lookups (RDAP) on 2026-11-06 showed no registration for `.org`. `.com`, `.net` and `.app` are taken, and `.dev` is a parked Hostinger page. npm, PyPI and GitHub show no "omnisnap" yet. Publish placeholder releases before announcing; that also confirms npm accepts the name (it can reject names too close to existing packages).
- **Fonts for the spike on AlmaLinux 10 WSL2:** only 8 font families are installed. Add a realistic set (Liberation, DejaVu, Noto) plus Firefox before running, or fallback fonts will dominate the results.
