# Proposal

## Why

A job keeps one browser per engine open from the first page to the last, so a job with every discovered engine holds all of them in memory at once. On the owner's Arch Linux machine (2026-11-25), `omnisnap <url>` with eight engines (Brave, Chrome, Chromium, Edge, Firefox, Firefox ESR, WebKitGTK, WPE) closed the owner's own Firefox without session restore. The kernel's out-of-memory killer chose the largest process, which was the user's browser and not one of Omnisnap's. `specs/SPEC-process.md` already asks for a cap on live browsers (default 3) and marks it "not implemented yet" (backlog B6). The owner wants it in the stable `v0.2.0`, through `v0.2.0-rc.1`, so the first stable release does not risk a user's own applications.

## What Changes

- **Omnisnap's browsers die first under memory pressure (Linux).** Every process Omnisnap launches (browsers, their drivers, the private Weston), and every process those start, runs with the highest out-of-memory score. If memory runs out, the kernel kills one of Omnisnap's browsers before any process the user started, and the existing crash policy retries the capture or reports the engine as lost. Omnisnap's own process and processes it did not start are never touched. Windows and macOS have no equivalent knob and keep today's behaviour.
- **At most N live browsers per job (default 3).** When a job has more engines than the cap, its engines run in consecutive groups of at most N live browsers. The baseline is always in the first group. Each group captures every page, its browsers close, and the next group starts. A job with N engines or fewer behaves exactly as today. Engines that fail to launch don't count toward the cap.
- **New flag `--max-browsers N`** on `omnisnap <url>` and `omnisnap run`, a positive integer, default 3. It is a property of the machine, not of the job, so the job file schema does not change. **BREAKING (behaviour, `0.2.0`):** a job with more than three engines now captures them in groups. Live pages can drift between groups, so a user who needs today's all-at-once order passes `--max-browsers` with the engine count.
- **The memory notes change.** The memory notes in `README.md` and `docs/quickstart.md` say what the cap does instead of only pointing at `--engines`. `SPEC-process.md` loses its "Not implemented yet" line.

## Capabilities

### New Capabilities

- `browser-concurrency`: how many browsers a job keeps alive at once, the order engines are captured in when there are more engines than the cap, and how Omnisnap's browsers are ranked against the user's processes under memory pressure.

### Modified Capabilities

None. There are no OpenSpec specs yet (`openspec list --specs` is empty). The normative project specs are updated in the same change: `specs/SPEC-process.md` (Resources), `specs/SPEC-capture.md` (Runs, lanes and timing) and `specs/SPEC-cli.md` (flags).

## Impact

- **Code:**
  - `internal/process`: the Linux launch hook sets the out-of-memory score for the launched tree.
  - `internal/cli/run.go`: the browser-per-engine-for-the-whole-job block becomes a loop over engine groups.
  - `internal/capture`: `RunPage` is reused per group, and page-level status is decided once, across groups.
  - `internal/cli/options.go`: the new flag.
- **Public surface:**
  - A new CLI flag, additive.
  - A change in capture order for jobs with more than three engines (`--json` progress events keep their schema; their order changes for those jobs only).
  - No job, capture, summary or progress schema change.
- **CI and the Action:** `ubuntu-latest` discovers five engines today (Chrome, Chromium, Edge, Firefox, WebKitGTK). Chromium and Edge fail to launch there, and failed launches don't count toward the cap, so the Linux job keeps three live browsers and one group. Whether the Action should expose the flag is an open question in design.md.
- **Docs:**
  - `README.md` (flags table, memory note), `docs/quickstart.md` (memory note).
  - `specs/SPEC-process.md`, `specs/SPEC-capture.md`, `specs/SPEC-cli.md`.
  - The developer wiki pages that describe the touched files (`Process-Management`, `CLI-and-Orchestration`, `Capture-Pipeline`, and any other `grep -rl` finds).
  - `tasks/backlog.md` B6 points at this change.
- **Out of scope:** a memory check before launch (backlog approach 3), cgroup limits, and changing which engines are discovered by default.
