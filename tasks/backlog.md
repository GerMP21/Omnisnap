# Omnisnap: backlog

Improvements that are worth doing but are not planned yet. An item here is a
problem with its evidence and the success criteria it would have to meet, not
a design. When an item is picked up, it gets a task in [todo.md](todo.md)
(with acceptance criteria and verification), and a spec change if it changes a
contract.

Plan: [plan.md](plan.md) · Tasks: [todo.md](todo.md) · Specs: [specs/](../specs/CAPABILITY-MAP.md)

---

## B1: Stop downloading the Linux engines on every run, so the Action runs faster on Ubuntu

**Status:** proposed (2026-11-20). Raised in priority the same day: a slow mirror cancelled a consumer's Linux job (below).

### Problem

On `ubuntu-latest` the composite action spends about half its time setting up
engines, and most of that is downloading them. Its Linux step runs
`apt-get update` and then downloads and installs Weston, the parity fonts and
the WebKitGTK driver with their dependencies, from the network, on every run
(`action/action.yml`, "Prepare engines (Linux)"). Every consumer pays this on
every pull request, in wall time and in billed runner minutes.

Evidence: Report workflow self-test, run 44764175911 (2026-11-20; two planted
pages, `runs: 1`):

| | Ubuntu | Windows | macOS |
|---|---|---|---|
| Whole action step | 3 min 13 s | 43 s | 2 min 4 s |
| Prepare engines (Linux) | **92.9 s** | n/a | n/a |
| Capture and compare | 97.4 s | | |

The download is the bulk of it. CI installs the same packages in 18 s when the
`.deb` files come from its cache (below), so about 75 s of the 92.9 s is
fetching packages and the rest is unpacking and configuring them. The install
also downloads recommended packages that capture may not need: the log shows
`xdg-desktop-portal`, `xdg-desktop-portal-gtk` and GStreamer plugins being set
up.

It can also cost the whole Linux report. On the first real consumer
(synaptech-site, PR 20, run 44777766699, 2026-11-20, twelve pages), the
Ubuntu mirror served packages at about 40-50 KB/s: `gstreamer1.0-plugins-good`
(2.3 MB) took 48 s, and the download was still on `libjavascriptcoregtk`
(8 MB) when the job hit GitHub's 30-minute limit. The step ran for 1808 s and
was cancelled before any capture started, so that PR's report had no Linux
host. The step has no time bound of its own, so a slow mirror always runs into
the job limit rather than failing fast.

### What already exists

CI's driver conformance job installs the same packages, plus
`libwebkitgtk-6.0-4`, in **18 s** (CI run 44764175896, the same day), because
it caches the downloaded `.deb` files with `actions/cache`, keyed on the runner
image (`.github/workflows/ci.yml`, "Cache the Linux engine packages"). The
action does not cache anything.

### Candidate approaches (to evaluate, not decided)

Each one cuts what is downloaded per run.

1. **Cache the `.deb` files in the action**, as CI does: `actions/cache` keyed
   on `ImageOS` and `ImageVersion`. It is the proven option here. The cache
   belongs to the consumer's repository, so the first run in each repository
   still pays the full download.
2. **Download less:** `--no-install-recommends`, after checking what WebKitGTK
   needs to capture with the same fonts and media as today.
3. **Skip the step when WebKitGTK is not requested.** The step runs even when
   `engines` leaves `webkitgtk` out, so a consumer who drops it still pays for
   the install.
4. **A prebuilt container image with the engines installed.** The image is
   still pulled, but in one layer download from a registry instead of dozens of
   packages from the Ubuntu mirror. This is the largest change: a composite
   action cannot choose the job's container, so it would only apply to the
   reusable workflow.

### Success criteria (proposed)

- On `ubuntu-latest`, a run with a cache hit downloads no engine package: the
  apt log shows no `Get:` line for them.
- "Prepare engines (Linux)" takes 20 s or less on a cache hit (92.9 s today),
  measured in the Report workflow self-test.
- A cache miss is no slower than today plus the cache save (10 s or less).
- The same engines are discovered, and the self-test's assertions still pass,
  including the planted finding `table#target`. The Linux planted gates hold
  (font parity).
- With `engines` set without `webkitgtk`, no `apt-get install` runs.
- Report-only still holds: a cache restore or save failure warns and never
  fails the job.
- The step has its own time bound, well under the job limit (proposed: 5
  minutes). Past it, the action warns, skips the engines it could not
  install, and still captures with the engines the image already has (Chrome
  and Firefox), so a slow mirror loses WebKitGTK, not the whole Linux host.

### Boundaries

- **Always:** measure before and after in the self-test, with the run IDs in
  the commit; keep packages from the runner's own Ubuntu archive.
- **Ask first:** dropping an engine from the default Linux set; building or
  publishing a container image.
- **Never:** install from third-party PPAs or unpinned download URLs; let the
  cache change which engine versions are captured without a test noticing.

### Not in this item

The same run's capture step has two Linux costs that are not downloads. They
can become their own items:

- Chromium and Edge are discovered but never start on the hosted image.
  Chromium crashes before DevTools is ready, and Edge's SUID sandbox helper is
  misconfigured. That costs about 14 s per run, and both show up as noise in the
  PR comment.
- The first page started 29 s after WebKitGTK reported it had launched.

### Open questions

- `apt-get update` downloads the package indexes on every run too. Is it needed
  on a cache hit? Skipping it saves that download, but a stale index can name
  package versions the mirror no longer has.
- How long do the 75 s split between `apt-get update` and the package
  downloads? The run log's `Fetched … in …` lines would say; measure it before
  choosing between the approaches.
