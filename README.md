# Omnisnap

**See exactly which elements render differently in every browser engine your users run, with one command, using the browsers already installed.**

```sh
omnisnap http://localhost:3000
```

Omnisnap finds the browsers on your machine, captures the page in each engine
(Blink, Gecko, WebKit), compares the **geometry of every element** against a
Chrome baseline, and writes a self-contained report: the engines side by
side, every difference outlined on both, with the reason and the fix when it
is known.

See a live report, the guides and the install options at **[omnisnap.org](https://omnisnap.org)**.

> **Status: release candidate `v0.1.0-rc.6`.** Capture, drivers, discovery,
> the layout diff, appearance checks, triage, the viewer, `install`, `merge`,
> the npm and PyPI wrappers and the GitHub Action are implemented. The driver
> conformance suite passes on Linux, Windows and macOS (Chrome, Edge, Firefox,
> WebKitGTK, Safari), and the planted gates find 24 of 24 planted bugs with no
> false report on the control pages. It is a candidate: interfaces may still
> change before `v1`. What is verified, and what is still open, is in
> [`tasks/todo.md`](tasks/todo.md).

## Contents

- [Install](#install)
- [Quick start](#quick-start)
- [The report and the bundle](#the-report-and-the-bundle)
- [Commands](#commands)
- [Job files](#job-files)
- [Browsers and platforms](#browsers-and-platforms)
- [From a test suite (npm, PyPI)](#from-a-test-suite-npm-pypi)
- [In CI (GitHub Action)](#in-ci-github-action)
- [Why geometry, not pixels](#why-geometry-not-pixels)
- [What it detects](#what-it-detects)
- [How it works](#how-it-works)
- [Principles](#principles)
- [Measured results](#measured-results)
- [Roadmap](#roadmap)
- [Repository](#repository)
- [Contributing](#contributing) · [License](#license)

## Install

Omnisnap is one Go binary with no runtime dependencies. Pick a channel:

**The release binary** (Linux, macOS, Windows):

```sh
# Linux x64; the other assets are listed below
curl -fsSLO https://github.com/GerMP21/Omnisnap/releases/download/v0.1.0-rc.6/omnisnap_v0.1.0-rc.6_linux_amd64
curl -fsSLO https://github.com/GerMP21/Omnisnap/releases/download/v0.1.0-rc.6/SHA256SUMS
sha256sum -c --ignore-missing SHA256SUMS
chmod +x omnisnap_v0.1.0-rc.6_linux_amd64 && sudo mv omnisnap_v0.1.0-rc.6_linux_amd64 /usr/local/bin/omnisnap
omnisnap version
```

| Platform | Asset |
|---|---|
| Linux x64 / arm64 | `omnisnap_v0.1.0-rc.6_linux_amd64` / `_linux_arm64` |
| macOS Apple silicon / Intel (macOS 13+) | `omnisnap_v0.1.0-rc.6_darwin_arm64` / `_darwin_amd64` |
| Windows x64 (Windows 10+) | `omnisnap_v0.1.0-rc.6_windows_amd64.exe` |

Every asset is listed with its SHA-256 in `SHA256SUMS` on the
[release page](https://github.com/GerMP21/Omnisnap/releases/tag/v0.1.0-rc.6).

**npm** (Node 20+) and **PyPI** (Python 3.10+): the binary for your platform
comes with the package, no download at install time. pip skips release
candidates unless asked, hence `--pre`:

```sh
npm install -g omnisnap                 # the omnisnap command, on your PATH
npm install --save-dev omnisnap         # the JavaScript API for a project's tests (npx omnisnap for the command)
pip install --pre omnisnap              # the Python API and the omnisnap command, in the active environment
```

**From source** (Go 1.27+):

```sh
git clone https://github.com/GerMP21/Omnisnap && cd Omnisnap
go build -o bin/ ./cmd/omnisnap
bin/omnisnap version
```

## Quick start

Serve your site, then point Omnisnap at it:

```sh
omnisnap browsers                       # what was found, its version and where
omnisnap http://localhost:3000 http://localhost:3000/pricing
open omnisnap-out/report.html           # xdg-open on Linux, start on Windows
```

It captures each page twice in every engine it found, compares them, and
prints where the report is. To choose engines:

```sh
omnisnap http://localhost:3000 --engines chrome,firefox,safari
```

No second engine family? `omnisnap install firefox` adds Gecko (see
[Browsers and platforms](#browsers-and-platforms)).

## The report and the bundle

`report.html` is self-contained (no server, no network): a summary table of
every page and engine, then per page the baseline and the engine side by side
with synced scrolling, each difference outlined on both. Differences are
triaged into **Report** (worth your time, listed first) and **Info** (expected
engine behaviour, explained). A single-engine **layout audit** also lists
problems that exist in one engine alone (clipped text, overlapping or
off-screen controls, horizontal scroll), unless `--no-audit`.

Everything behind the report stays in the output directory (the *bundle*):

```
omnisnap-out/
  report.html                  the viewer
  summary.json                 every result: status, causes, tiers, warnings
  host.json                    the machine: OS, fonts resolved, engines and versions
  <page>/page.json             url, slug, captured-at
  <page>/findings.json         the full evidence of every cause
  <page>/<engine>-<run>.json   geometry of every rendered element
  <page>/<engine>-<run>.png    full-page screenshot
  <page>/<engine>-<run>.error.txt   why a capture failed
  <page>/failed.json           the page was unreachable (baseline failed or a bot wall)
```

`summary.json`, the capture files and the progress stream follow published
JSON Schemas ([`schema/`](schema/README.md)), so other tools can read a
bundle.

## Commands

```
omnisnap <url> [<url>...] [flags]      capture and compare these pages
omnisnap run <job.json> [flags]        the same, from a job file
omnisnap browsers [--json]             list the browsers found
omnisnap check-fonts                   font-parity pre-flight across engines
omnisnap report <bundle-dir>           re-render a bundle's report.html
omnisnap merge <bundle-dir>... -o <dir> [--rename-hosts]   one report from several machines
omnisnap comment <merged-bundle> [--report-url U] [--run-url U]   the PR comment as Markdown
omnisnap warm <browser>                prepare Brave's or Edge's blocking profile
omnisnap install firefox [--channel release|esr] [--version v]
omnisnap uninstall firefox [--channel release|esr]
omnisnap version
```

Flags for `<url>` and `run` (they override the job file):

| Flag | Default | Meaning |
|---|---|---|
| `--engines a,b,c` | every discovered engine | engines to capture |
| `--baseline id` | `chrome` | the engine every other one is compared with (a Blink browser) |
| `--out dir` | `./omnisnap-out` | output directory |
| `--runs n` | `2` | captures per engine and page; the second run reveals dynamic content |
| `--viewport WxH` | `1280x800` | viewport |
| `--timeout d` | `30s` | navigation timeout |
| `--resume` | | skip pages already captured to the end |
| `--retry-failed` | | with `--resume`, retry pages marked unreachable |
| `--allow-small-pages` | | do not treat a page under 50 elements as a bot wall (test and fixture pages) |
| `--no-audit` | | skip the single-engine layout audit |
| `--neutral` | | fresh profiles, branded blocking disabled where possible |
| `--json` | | NDJSON progress on stdout, nothing else |
| `--quiet` / `--verbose` | | only the report path and errors / more detail on errors |

**Exit codes.** Findings never fail a run: Omnisnap reports, it does not gate.

| Code | Meaning |
|---|---|
| `0` | completed, with or without findings |
| `1` | usage: bad flag, unknown command, invalid job file |
| `2` | no usable baseline: no Blink browser installed, or it did not start |
| `3` | every capture of every page failed |
| `4` | an internal error (a bug: the message asks for a report) |

**Failures.** If the baseline cannot load a page (a timeout, a network error,
or under 50 elements, which is usually a bot wall), the page is marked
unreachable and the other engines skip it. If a browser crashes, Omnisnap
starts it again and retries that capture once. If it crashes again, that
engine sits out the rest of the job with one message, and the others go on.

## Job files

A job file covers many pages and pins the settings:

```json
{
  "$schema": "https://omnisnap.org/schema/job/1.json",
  "pages": [
    { "url": "http://localhost:3000/" },
    { "url": "http://localhost:3000/pricing", "slug": "pricing" }
  ],
  "engines": ["chrome", "firefox", "safari"],
  "baseline": "chrome",
  "runs": 2,
  "viewport": { "width": 1280, "height": 800 },
  "timeouts": { "navigationMs": 30000 },
  "output": "./omnisnap-out"
}
```

```sh
omnisnap run job.json
omnisnap run job.json --resume          # after an interruption
```

Only `pages` is required. The `$schema` URL gives editors validation and
completion; the schema itself is [`schema/job.schema.json`](schema/job.schema.json).

## Browsers and platforms

Discovery reads the registry (Windows), `.desktop` files, `PATH`, Flatpak and
Snap (Linux), and app bundles (macOS). It never launches a browser to find
it.

| Engine | Browsers | Protocol |
|---|---|---|
| Blink | Chrome, Chromium, Edge, Brave, Opera, Vivaldi | Chrome DevTools Protocol |
| Gecko | Firefox, Firefox ESR | WebDriver BiDi |
| WebKit | Safari (macOS), WebKitGTK and WPE MiniBrowser (Linux) | WebDriver Classic |

- **Safari** needs its driver enabled once: `sudo safaridriver --enable`.
- **WebKitGTK** runs on a private, headless Weston display (install `weston`);
  WPE needs no display.
- **Branded browsers run as shipped**: Brave's Shields and Edge's tracking
  prevention act as they do for your users, from a prepared profile
  (`omnisnap warm brave`). Differences they cause are explained by the
  requests they blocked. `--neutral` turns that off.
- **No Firefox?** `omnisnap install firefox` downloads Mozilla's official
  build, verifies its SHA-512 and Mozilla's OpenPGP signature on
  `SHA512SUMS`, and keeps it in the Omnisnap cache; `uninstall firefox`
  removes it. It is the only command that downloads anything, and only when
  you run it.
- **Memory.** Omnisnap keeps one instance of each browser it uses open for the
  whole run, and by default it uses every browser it finds. On a machine with
  many browsers installed, that can use a lot of memory alongside your own
  applications; if memory runs low, the system may close other programs. Use
  `--engines` to run fewer browsers, for example
  `--engines chrome,firefox,webkitgtk` (one per engine family).

Supported OSes: Linux (x64, arm64), macOS 13 or newer (Apple silicon, Intel),
Windows 10 or newer (x64).

## From a test suite (npm, PyPI)

Both wrappers are thin: they write a job file, run the binary, stream its
progress events and return the parsed `summary.json`. No logic is
reimplemented, so both behave like the CLI.

```js
import { run } from 'omnisnap';

const summary = await run({
  pages: ['http://localhost:3000/', 'http://localhost:3000/pricing'],
  engines: ['chrome', 'firefox'],
  out: './omnisnap-out',
  onProgress: (event) => console.log(event.type),
});
for (const r of summary.results) {
  console.log(`${r.page.slug} ${r.engine.engine}: ${r.stats.report} to report`);
}
```

```python
from omnisnap import run

summary = run(
    pages=["http://localhost:3000/", "http://localhost:3000/pricing"],
    engines=["chrome", "firefox"],
    out="./omnisnap-out",
    on_progress=lambda event: print(event["type"]),
)
for r in summary["results"]:
    print(r["page"]["slug"], r["engine"]["engine"], r["stats"]["report"], "to report")
```

Also `browsers()`, `merge(bundles, out)` and `report(bundle)`. Options:
`engines`, `baseline`, `runs`, `out`, `viewport`, `timeouts`, `probes`,
`fontParity`, and `allowSmallPages` / `allow_small_pages` for pages under 50
elements (snake_case in Python). A non-zero exit raises a typed error carrying the exit code and
stderr; findings do not. Set `OMNISNAP_BINARY` to use a binary of your own.
The TypeScript declarations ship with the npm package.

## In CI (GitHub Action)

The reusable workflow runs the job on Ubuntu, Windows and macOS runners. That
gives Linux and Windows developers real Safari. It merges the three bundles
into one report and posts one comment on the pull request, updated in place
on every push. It is **report-only: every job succeeds, whatever it finds.**

> **Slow Linux setup: use `v0.1.0-rc.5` or later.** The Ubuntu job installs
> WebKitGTK, Weston and the parity fonts from the Ubuntu package mirror, which
> can be slow depending on demand (on 2026-11-20 it served about 50 KB/s). Up to
> `v0.1.0-rc.4` that download had no time limit of its own: it could take 15
> minutes or more, or run into GitHub's 30-minute job limit and leave the report
> without a Linux host. From `v0.1.0-rc.5` the Action caches the packages per
> runner image, so a later run on the same image downloads nothing, and stops
> downloading after `setup-timeout` (300 s by default): past it the job warns
> and captures with Chrome and Firefox. A pull request's first run still pays
> for the download, so on a slow day that run can lose WebKitGTK.

Create `.github/workflows/omnisnap.yml`:

```yaml
name: Omnisnap report          # the fork follow-up matches this name

on: pull_request

permissions:
  contents: read
  pull-requests: write         # to post the comment

jobs:
  omnisnap:
    uses: GerMP21/Omnisnap/.github/workflows/report.yml@v0.1.0-rc.6
    with:
      start: npm ci && npm run build && npx serve -l 3000 dist
      wait-on: http://localhost:3000
      pages: |
        http://localhost:3000/
        http://localhost:3000/pricing
```

| Input | Default | What it does |
|---|---|---|
| `pages` | (required) | one URL per line, as the runner sees them |
| `start` | | the command that builds and serves your site; the Action backgrounds it, so do not end it with `&` |
| `wait-on` | | a URL polled until the server answers |
| `engines` | every installed | comma-separated engine ids |
| `baseline` | `chrome` | the engine the others are compared with |
| `runs` | `2` | captures per engine and page |
| `macos` | `every` | `labelled` runs the macOS job only on PRs labelled `omnisnap:safari`; `never` drops it |
| `version` | `v0.1.0-rc.6` | the release the binary is downloaded from |
| `setup-timeout` | `300` | Linux: seconds the engine package downloads may take |

Pull requests from forks get their comment from a follow-up workflow that
never runs fork code. The setup, a `start` command for each common stack and
the troubleshooting table are in
[`docs/github-action-setup.md`](docs/github-action-setup.md) and
[`docs/github-action-fork-comments.md`](docs/github-action-fork-comments.md).
The composite action alone (`GerMP21/Omnisnap/action`) is described in
[`action/README.md`](action/README.md).

## Why geometry, not pixels

Cross-browser visual testing today means one of two things:

- **pixel diffs**, which flag everything after the first line that wraps differently;
- tools that compare each browser **with its own past screenshots**, so they never notice that Safari and Chrome disagree right now.

Omnisnap compares **element geometry** across engines instead: sizes,
positions, line wraps, overflow, truncation. Each difference is reported at
the element where it starts, not at every element it pushes down the page. On
top of that, Omnisnap tells you:

- **why it happens:** the page is measured again with suspected causes neutralized (fractional line-heights, fonts, form-control defaults); when the difference disappears, the cause is confirmed;
- **whether anyone would see it:** visible consequences (clipped content, a new scrollbar, a broken alignment) are separated from engine differences nobody notices;
- **which engine is wrong:** a third engine family decides which one is the odd one out, and Chrome can be the one at fault;
- **which design rule it breaks:** alignments, equal heights and even spacing that the other engines keep.

## What it detects

Every element is compared with the Chrome baseline. The full rules and
thresholds are in the specs linked under each list.

**Layout differences** (cause kinds):

| Kind | Meaning |
|---|---|
| `only-in-<engine>` | the element exists in one engine only |
| `hidden-in-<engine>` | visible in one engine, hidden in the other |
| `truncated-in-<engine>` | text cut by an ellipsis or line clamp in one engine only |
| `width`, `height` | the box changed size (≥ 2 px and ≥ 1%) |
| `position` | the box moved relative to its parent (≥ 2 px) |
| `wrap` | the text takes a different number of lines |
| `overflow-x`, `overflow-y` | overflow appears or disappears and changes by more than 4 px |
| `text-shifted` | the text moved inside a box that did not change |
| `children-differ` | the element's children do not match across engines |
| `children-moved` | all the children moved: the parent is the cause |
| `columns` | a table split its columns differently at the same total width |
| `intent-*` | a design relation the other engines keep is broken: shared edges, equal sizes, equal gaps, grid columns, no overlap (needs a third engine family) |

Source: [SPEC-layout-diff.md](specs/SPEC-layout-diff.md).

**Appearance:** `appearance`, for images, SVG, canvas, video, iframes and
background images. It is used when the geometry matches but the pixels
differ (SSIM below 0.90).
Source: [SPEC-appearance-diff.md](specs/SPEC-appearance-diff.md).

**DOM audit** (problems in a single engine; no comparison needed):

| Check | Meaning |
|---|---|
| `page-horizontal-scroll` | the document is wider than the viewport |
| `clipped-text` | text cut off by `overflow: hidden` or `clip` |
| `truncated-text` | text with an ellipsis or a line clamp (info) |
| `overlap-interactive` | two interactive elements overlap by ≥ 4 px |
| `offscreen-interactive` | a visible interactive element lies outside the page |
| `zero-size-interactive` | an interactive element with text but no width or height |
| `protrudes-parent` | an element sticks out of its parent and covers a sibling |

Source: [SPEC-dom-audit.md](specs/SPEC-dom-audit.md).

**Page warnings:**
- the viewports differ;
- the node cap was hit;
- the page changed during capture;
- the engines rendered different page states;
- web fonts failed differently;
- requests were blocked, for example by a branded browser's blocker (Brave, Edge).

### What it takes into account

**Avoiding false alarms:**
- **Two runs per engine.** What changes between them is dynamic (carousels, clocks, ads) and is masked.
- **Tolerances:**
  - 2 px for sizes and positions;
  - 1% for reported sizes;
  - 20% of the height for inline tops.

  Inline box heights are ignored, because each engine measures the same font's content area differently.
- **Cause, not effect.** When a parent narrows, its children reflow, and only the parent is reported. The same applies to boxes that grow because a child grew and to siblings pushed along. In inline text flow, only the first changed box counts.
- **A knowledge base of normal engine differences:**
  - K1: fractional line-height rounded differently;
  - K2: `line-height: normal` resolved differently;
  - K4: text advance differs;
  - K5: a long token (URL) breaks differently;
  - K8: form-control default sizes.
- **Deliberate hiding:** text moved a viewport out of sight on purpose (`text-indent: 100%`, `-9999px`) is not reported as clipped.
- **Inline images:** line-box metrics around an inline image differ between Blink and Firefox, without anything visible changing. These causes go to Info.
- **Branded browsers:** a difference caused by a blocked request is linked to the block (`blockedBy`).

**Consequences:**
- **Hard** (always Report): presence, visibility, clipped, scrollbar, protrusion beyond the viewport, truncation.
- **Soft:** visible overflow, line count, an interactive element moved, a structural change (≥ 8 px and ≥ 10%), broken intent, content moved inside its box.

**Evidence on each cause:**

| Evidence | Meaning |
|---|---|
| `mechanism` | which neutralizing probe removes the difference: `lh`, `font`, `controls`, `all`; `layout` if none does; `author-font` when the page asks each engine for a different font |
| `perception` | whether the eye notices it: alignment broken, overlap created, order flipped, spacing changed; sizes ≥ 8% and ≥ 2 px |
| `calibration` | whether the page's own systematic engine differences predict the change |
| `consensus` | which engine is the odd one out (needs a third engine family) |
| `ink` | whether the change is visible in the pixels, against the page's own noise floor |
| `intent` | the relations kept by the other engines and broken here |
| `novelty` | on how many other crawled sites the same pattern occurs (the shipped table holds 300 patterns from a 457-site crawl) |

**Tier and ranking:**
- **Report** holds a hard consequence, or a consequence the knowledge base does not explain. Everything else is **Info**.
- A tree model (LightGBM) gives each cause a `score` that ranks causes within a tier; it does not set the tier.
- Repeated findings merge into one cause. Causes are ranked by tier, then score, then impact.

Source: [SPEC-triage.md](specs/SPEC-triage.md).

## How it works

1. **Discovery** finds every browser on the machine, with its version, without launching it.
2. **Capture**: each engine loads the page at the same viewport, waits until it has settled (fonts, lazy content, animations), and records the geometry of every element and a full-page screenshot. Two runs per engine let dynamic content (carousels, ads) be recognized and masked.
3. **Layout diff**: elements are matched across engines, and each difference is traced to the element where it starts, with the rule that explains it.
4. **Appearance**: images, SVG and canvas are compared in pixels where geometry cannot see a difference.
5. **Triage** sorts causes into Report and Info with the evidence: probes, the third engine, visible consequences, a knowledge base of known engine differences.
6. **The report** is written into the bundle, readable offline.

## Principles

- **Your browsers, as installed.** No bundled engines, no silent downloads.
- **Nothing leaves your machine.** Captures and reports stay in a local folder; there is no service.
- **No test code.** A URL is enough; a job file covers many pages.
- **Report, don't block.** A wrong failure gets a visual gate switched off; a pass/fail mode waits until report mode has shown how noisy real projects are.
- **Every claim has a known answer.** Precision and recall are measured against planted bugs, realistic bugs injected into real pages, and bug-free control pages, not opinions.

## Measured results

The planted suite is 31 pages: 24 with a known cross-engine bug, plus control,
knowledge-base and native form-control pages that must stay quiet. CI runs it
on every pull request.

| Check | Result |
|---|---|
| Planted bugs found at the right element, Report tier | **24/24** |
| Report causes on bug-free control pages | **0** |
| Design-intent violations caught (planted) / false alarms on controls | **6/6** / **0** |
| Same suite through the npm and PyPI packages, installed as published | Linux 24/24, Windows and macOS 23/24 (live captures on each runner's fonts); 0 on controls |
| Driver conformance: Chrome, Edge, Firefox, WebKitGTK, Safari | passes on all three OSes |
| Diff + triage, largest golden page / 10,000 elements | 72 ms / 62 ms |
| Elements matched between engines, real sites (prototype) | **≥ 99.9%** |
| Precision on real sites | **not measured yet**: needs a crawl with injected bugs |

## Roadmap

The current release is the candidate `v0.1.0-rc.6`.

| Milestone | Delivers | State |
|---|---|---|
| **M0**: capture + viewer | Browser discovery on three OSes; CDP, WebDriver BiDi and WebDriver Classic drivers; capture; viewer; single-engine audit; `install firefox` | done; one check left: the weekly network job that installs Firefox, captures and uninstalls first runs on 2026-11-23 |
| **M1**: layout diff | Cross-engine comparison, design intent, appearance, triage with explanations, branded-browser causes | done; the rules set the Report/Info tier and the tree model ranks causes within it |
| **M2 = v1**: CI report | GitHub Action on Linux, Windows and macOS, merged PR comment, npm and PyPI packages | done; Action, npm and PyPI published as release candidates |

`v1` waits for the weekly network job and a stable public surface.

Next, from the [backlog](tasks/backlog.md):

- **B1:** a faster Linux engine setup in the Action. The cache and a time bound are in; two approaches are still to try.
- **B2:** selectors count only captured siblings, so a `:nth-of-type()` can name the wrong element.
- Two spec items not implemented yet:
  - the `--no-probes` and `--open` flags;
  - knowledge-base entries as versioned data with the engine versions they were validated on.

The task list with every check and what backs it is [`tasks/todo.md`](tasks/todo.md).

## Repository

| Path | Contents |
|---|---|
| [`cmd/omnisnap/`](cmd/omnisnap) | the CLI entry point; the code is under [`internal/`](internal) |
| [`action/`](action/README.md), [`.github/workflows/report.yml`](.github/workflows/report.yml) | the composite action and the reusable report workflow |
| [`packages/`](packages) | the npm wrapper and its platform packages, the PyPI wrapper |
| [`schema/`](schema/README.md) | the five JSON Schemas of the bundle contracts, published at `https://omnisnap.org/schema/` |
| [`docs/`](docs/) | [quickstart](docs/quickstart.md), [Action setup](docs/github-action-setup.md), [one-pager](docs/ideas/omnisnap.md), [PRD](docs/prd.md), [scientific grounding](docs/SCIENTIFIC_GROUNDING.md), [precision research](docs/research/) |
| [`docs/internals/`](docs/internals/Home.md) | the developer wiki: how every module works inside, published to the [GitHub Wiki](https://github.com/GerMP21/Omnisnap/wiki) |
| [`specs/`](specs/CAPABILITY-MAP.md) | the module specs and their shared [conventions](specs/CONVENTIONS.md) |
| [`tasks/`](tasks/plan.md) | the implementation plan and the task list |
| [`testdata/planted/`](testdata/planted) | the planted pages and their expectations |
| [`tools/release/`](tools/release) | release builds, staging for the packages, the platform wheels |
| [`spikes/layout-diff/`](spikes/layout-diff/README.md) | the throwaway Node.js prototype: results, lessons, and the pieces the Go core keeps |
| [`THIRD-PARTY-NOTICES.md`](THIRD-PARTY-NOTICES.md) | licenses of the vendored development tooling |

Build and test from a checkout:

```sh
go build ./... && go test ./...                              # unit tests
go test -tags browsers ./internal/drivers/... -run Conformance   # against your browsers
go test -tags e2e ./test/e2e/... -run Planted                # the planted gates
```

[CONTRIBUTING.md](CONTRIBUTING.md) has the full list of checks.

## Contributing

Contributions are welcome, and [CONTRIBUTING.md](CONTRIBUTING.md) covers the
workflow: where to post, the checks to run, the rules the codebase lives by,
and how a significant change is planned before it is coded. Participation is
governed by the [Code of Conduct](CODE_OF_CONDUCT.md).

Reports of real cross-browser layout bugs you have hit are especially useful:
they become planted test pages. Questions and ideas go to
[Discussions](https://github.com/GerMP21/Omnisnap/discussions).

## License

[GNU Affero General Public License v3](LICENSE.txt), or (at your option) any
later version. The vendored agent skills keep their own licenses; see
[THIRD-PARTY-NOTICES.md](THIRD-PARTY-NOTICES.md).
