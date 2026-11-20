# Omnisnap: backlog

Improvements that are worth doing but are not planned yet. An item here is a
problem with its evidence and the success criteria it would have to meet, not
a design. When an item is picked up, it gets a task in [todo.md](todo.md)
(with acceptance criteria and verification), and a spec change if it changes a
contract.

Plan: [plan.md](plan.md) · Tasks: [todo.md](todo.md) · Specs: [specs/](../specs/CAPABILITY-MAP.md)

---

## B1: A lighter Linux engine setup, so the Action runs faster on Ubuntu

**Status:** proposed (2026-11-20)

### Problem

On `ubuntu-latest` the composite action spends about half its time installing
engines, not capturing. Its Linux step runs `apt-get update` and then installs
Weston, the parity fonts and the WebKitGTK driver with their dependencies,
from the network, on every run (`action/action.yml`, "Prepare engines
(Linux)"). Every consumer pays this on every pull request, in wall time and in
billed runner minutes.

Evidence: Report workflow self-test, run 44764175911 (2026-11-20; two planted
pages, `runs: 1`):

| | Ubuntu | Windows | macOS |
|---|---|---|---|
| Whole action step | 3 min 13 s | 43 s | 2 min 4 s |
| Prepare engines (Linux) | **92.9 s** | n/a | n/a |
| Capture and compare | 97.4 s | | |

The install also pulls recommended packages that capture may not need: the log
shows `xdg-desktop-portal`, `xdg-desktop-portal-gtk` and GStreamer plugins
being set up.

### What already exists

CI's driver conformance job installs the same packages, plus
`libwebkitgtk-6.0-4`, in **18 s** (CI run 44764175896, the same day), because
it caches the downloaded `.deb` files with `actions/cache`, keyed on the runner
image (`.github/workflows/ci.yml`, "Cache the Linux engine packages"). The
action does not cache anything.

### Candidate approaches (to evaluate, not decided)

1. **Cache the `.deb` files in the action**, as CI does: `actions/cache` keyed
   on `ImageOS` and `ImageVersion`. It is the proven option here. The cache
   belongs to the consumer's repository, so the first run in each repository
   still pays the full download.
2. **`--no-install-recommends`**, after checking what WebKitGTK needs to
   capture with the same fonts and media as today.
3. **Skip the step when WebKitGTK is not requested.** The step runs even when
   `engines` leaves `webkitgtk` out, so a consumer who drops it still pays for
   the install.
4. **A prebuilt container image with the engines installed.** This is the
   largest change: a composite action cannot choose the job's container, so it
   would only apply to the reusable workflow.

### Success criteria (proposed)

- On `ubuntu-latest`, "Prepare engines (Linux)" takes 20 s or less on a cache
  hit (92.9 s today), measured in the Report workflow self-test.
- A cache miss is no slower than today plus the cache save (10 s or less).
- The same engines are discovered, and the self-test's assertions still pass,
  including the planted finding `table#target`. The Linux planted gates hold
  (font parity).
- With `engines` set without `webkitgtk`, no `apt-get install` runs.
- Report-only still holds: a cache restore or save failure warns and never
  fails the job.

### Boundaries

- **Always:** measure before and after in the self-test, with the run IDs in
  the commit; keep packages from the runner's own Ubuntu archive.
- **Ask first:** dropping an engine from the default Linux set; building or
  publishing a container image.
- **Never:** install from third-party PPAs or unpinned download URLs; let the
  cache change which engine versions are captured without a test noticing.

### Open questions

- Is `apt-get update` needed on a cache hit? Skipping it saves time, but a
  stale index can name package versions the mirror no longer has.
- Two more Linux costs show up in the same run's capture step. Do they belong
  in this item, or in their own?
  - Chromium and Edge are discovered but never start on the hosted image.
    Chromium crashes before DevTools is ready, and Edge's SUID sandbox helper is
    misconfigured. That costs about 14 s per run, and both show up as noise in
    the PR comment.
  - The first page started 29 s after WebKitGTK reported it had launched.
