# Omnisnap

**See exactly which elements render differently in every browser engine your users run, with one command, using the browsers already installed.**

> **Status: the Go core exists, no release yet.** Capture, drivers, discovery,
> the layout diff, triage, the viewer, `install` and `merge` are implemented and
> tested against the spike's golden captures (see
> [`docs/status-2026-11-18.md`](docs/status-2026-11-18.md) for what is verified
> and what remains: appearance parity, branded browsers, the lab crawl, the CI
> Action and the wrappers' publishing). Build it with
> [`docs/quickstart.md`](docs/quickstart.md):
>
> ```sh
> go build -o bin/omnisnap ./cmd/omnisnap && bin/omnisnap http://localhost:3000
> ```

## Why

Cross-browser visual testing today means one of two things:
- **pixel diffs**, which flag everything after the first line that wraps differently;
- tools that compare each browser **with its own past screenshots**, so they never notice that Safari and Chrome disagree right now.

Omnisnap compares **element geometry** across engines instead: sizes, positions, line wraps, overflow, truncation. Each difference is reported at the element where it starts, not at every element it pushes down the page. On top of that, Omnisnap tells you:

- **why it happens:** the page is re-measured with suspected causes neutralized (fractional line-heights, fonts, form-control defaults), and when the difference disappears, the fix is verified;
- **whether anyone would see it:** visible consequences (clipped content, a new scrollbar, a broken alignment) are separated from expected engine differences nobody notices;
- **which engine is wrong:** a third engine family decides which one is the odd one out, and Chrome can be the one at fault;
- **which design rule it breaks:** alignments, equal heights and even spacing that the other engines keep.

## How it will work

```
omnisnap http://localhost:3000
```

1. **Finds every browser on the machine**: Chrome, Edge, Brave, Opera, Vivaldi, Chromium, Firefox, Safari, WebKitGTK. Nothing is downloaded unless you ask (`omnisnap install firefox`).
2. **Captures each engine** after the page has settled (fonts, lazy content, animations), twice, so dynamic content is recognized and ignored.
3. **Compares every engine with the Chrome baseline** and triages the differences into **Report** (worth your time) and **Info** (expected engine behaviour, explained).
4. **Writes a self-contained report**: screenshots side by side with synced scrolling, every finding outlined on both, with the explanation and the fix when known.

Branded browsers run **as shipped**, from prepared profiles, so Brave's Shields or Edge's tracking prevention act as they do for your users. Their differences are explained by the requests they blocked.

In CI, a **GitHub Action** runs the same job on Ubuntu, Windows and macOS runners. That gives Windows and Linux developers **real Safari**. The results arrive as **one comment on the pull request**. The Action is **report-only: it never fails your build.**

npm and PyPI packages will wrap the same binary for use from test suites.

## Principles

- **Your browsers, as installed.** No bundled engines, no silent downloads.
- **Nothing leaves your machine.** Captures and reports stay in a local folder; there is no service.
- **No test code.** A URL is enough; a `job.json` file covers many pages.
- **Report, don't block.** A wrong failure gets a visual gate switched off; a pass/fail mode waits until report mode has shown how noisy real projects are.
- **Every claim has a known answer.** Precision and recall are measured against planted bugs, realistic bugs injected into real pages, and bug-free control pages, not opinions.

## What the prototype showed

Measured on the prototype ([details](spikes/layout-diff/README.md#results)):

| Check | Result |
|---|---|
| Planted bugs found at the right element | **24/24**, including 16 realistic cross-engine bug patterns |
| Report causes on bug-free control pages, five engines | **0** |
| Design-intent violations caught (planted) / false alarms on controls | **6/6** / **0** |
| Elements matched between engines, real sites | **≥ 99.9%** |
| Precision on real sites | **not measured yet**: being measured with injected bugs and a small local tree model |

## Roadmap

| Milestone | Delivers | Tasks |
|---|---|---|
| **M0**: capture + viewer | One Go binary: browser discovery on Windows, macOS and Linux; CDP, WebDriver BiDi and WebDriver Classic drivers; capture; side-by-side viewer; single-engine layout audit; `omnisnap install firefox` | 1–38 |
| **M1**: layout diff | Cross-engine comparison, design intent, appearance checks on graphics, triage with explanations and fixes, branded-browser causes | 39–76 |
| **M2 = v1**: CI report | GitHub Action on three OSes, merged PR comment, npm and PyPI packages | 77–86 |

## Repository

| Path | Contents |
|---|---|
| [`docs/`](docs/) | [One-pager](docs/ideas/omnisnap.md), [PRD](docs/prd.md) (amended by its changelog), [idea refinement](docs/idea-refinement.md), [scientific grounding](docs/SCIENTIFIC_GROUNDING.md), [precision research](docs/research/) |
| [`specs/`](specs/CAPABILITY-MAP.md) | The 17 module specs of v1 and their shared [conventions](specs/CONVENTIONS.md) |
| [`tasks/`](tasks/plan.md) | Implementation plan and the task list |
| [`spikes/layout-diff/`](spikes/layout-diff/README.md) | The throwaway prototype (Node.js): its results, lessons, and the pieces the Go core keeps |

### Trying the prototype

The prototype runs on Linux with Node 20+ and the browsers you have installed. Setup and commands are in its [README](spikes/layout-diff/README.md#setup-linux-tested-on-almalinux-10--wsl2). For example:

```
cd spikes/layout-diff
npm install
npm run planted      # capture the planted pages in every installed engine, compare, score
```

It is research code: expect rough edges, Linux only, and no stable interface.

## Contributing

Not open for code contributions yet; the Go core starts with [task 1](tasks/todo.md). Issues with real cross-browser layout bugs you've hit are welcome: they become planted test pages.

## License

[Apache License 2.0](LICENSE.txt)
