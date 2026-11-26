# Omnisnap: backlog

Improvements that are worth doing but are not planned yet. An item here is a
problem with its evidence and the success criteria it would have to meet, not
a design. When an item is picked up, it gets a task in [todo.md](todo.md)
(with acceptance criteria and verification), and a spec change if it changes a
contract.

Plan: [plan.md](plan.md) · Tasks: [todo.md](todo.md) · Specs: [specs/](../specs/CAPABILITY-MAP.md)

---

## B1: Stop downloading the Linux engines on every run, so the Action runs faster on Ubuntu

**Status:** done, Task 90 in [todo.md](todo.md) (2026-11-25). Raised in priority on 2026-11-20: a slow mirror cancelled a consumer's Linux job (below). Done: approaches 1 and 3 and the time bound (`action/linux-engines.sh`). Not taken: approaches 2 and 4.

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

---

## B2: Selectors count only captured siblings, so they can name the wrong element

**Status:** in progress, OpenSpec change `fix-selector-sibling-index` and Task 91 in [todo.md](todo.md), targeted at `v0.2.0` (2026-11-25). Approach 1, plus CSS escaping and the DOM audit's selectors (owner's call). PR #20 (2026-11-26): conformance on every CI engine, Safari included, and every planted cause's selector resolving in the browser (run 45851219534). The review added two fixes: every child of a shadow host records `nt` (a light child after a same-tag shadow child selected nothing), and tag names are escaped. Left: the owner's synaptech-site re-check.

### Problem

A cause's selector uses `:nth-of-type(N)` to tell same-tag siblings apart, and
`segment()` in `internal/layoutdiff/match.go` computes N among the parent's
**captured** children. `extract.js` drops `display: none` elements
(`if (cs.display === 'none') return null;`), so a hidden sibling before the
element is not counted. The selector then reads as valid CSS but points at a
different element: run in the page, it matches the hidden sibling, or a
neighbour, instead of the element that differs.

`Selector()` says it is "not a guaranteed querySelector", but `nth-of-type`
reads as CSS, and people paste it into devtools.

Evidence: synaptech-site's first report (PR 20, run 44777766699, 2026-11-20).
`/work` renders its first project card with `hidden` (it repeats the featured
project), and the comment reported:

- `overflow x` on `div#projects-list > div.project-card:nth-of-type(1) > …`.
  In the page, that selector matches the hidden card (0×0, `display: none`),
  which looked like a false positive on an invisible element. The element that
  overflowed was the first *visible* card, the DOM's second: the site really
  had that overflow (fixed in synaptech-site PR 21).
- `Width differs` on `div.project-card:nth-of-type(6)`, which is the DOM's
  seventh card.

### Candidate approaches (to evaluate, not decided)

1. Record each node's position among its same-tag DOM siblings in
   `extract.js`, hidden ones included, and use that in `segment()`. This
   changes the capture format: `schema/capture.schema.json` and the spec.
2. Keep counting captured siblings, but stop writing it as `nth-of-type`
   (for example `div.project-card[2nd shown]`), so it does not read as CSS.

### Success criteria (proposed)

- For every cause on the planted suite and on a page with hidden siblings,
  `document.querySelector(selector)` in the captured page returns the element
  the cause is about.
- A planted page with a `display: none` sibling before a planted difference
  reports a selector that matches the planted element.
- Matching between engines does not change: the same causes are found on the
  planted suite (the planted gates hold).

### Open questions

- ~~Does the matcher pair nodes across engines by the same captured index? If
  one engine hides an element that another shows, the indices of the siblings
  after it shift, and the pairing could be off as well as the label.~~
  No (2026-11-25): `alignChildren` pairs by identity tokens (LCS anchors), then
  in order by `tag.class` and by tag, never by a position number. An element
  one engine hides becomes unpaired and its siblings still pair, so B2 is a
  labelling bug only (the change's design.md, "Pairing").
- ~~`visibility: hidden` and `opacity: 0` nodes are captured (with `hid`), so they
  already count. Is `display: none` the only case that shifts the index?~~
  In practice yes: `extract.js` also skips `SKIP` tags, nodes past
  `MAX_NODES` and everything below an `<svg>`, but only `display: none` is
  commonly the same tag as a rendered sibling. The fix counts every element
  child, so all of them are covered.

---

## B3: Leave the release candidates with `v0.2.0`, not `v0.1.0` or `v1`

**Status:** in progress, OpenSpec change `openspec/changes/release-v0-2-0` (2026-11-25). The owner's call that day: the release that leaves the candidates is the stable **`v0.2.0`**, cut through one candidate, `v0.2.0-rc.1` (Task 95 in [todo.md](todo.md)). It is not `v0.1.0` (B2 and B6 change what the `0.1.0` candidates previewed) and not `v1`, whose criteria are undecided (Open questions). The 0.x promise is written in `specs/CONVENTIONS.md`, "Versioning".

### Problem

Every release so far is a candidate (`v0.1.0-rc.1` to `v0.1.0-rc.7`). A
candidate costs users something on each channel:

- PyPI: `pip install omnisnap` skips pre-releases, so the README and the
  PyPI README have to say `pip install --pre omnisnap`
  (`packages/pypi/README.md`).
- npm: an rc on `latest` installs, but `npm i omnisnap` saves the exact
  pre-release range, and a stale `next` dist-tag still points at
  `0.1.0-rc.6` from the old manual flow (2026-11-25: every one of the six
  packages has `latest: 0.1.0-rc.7`, `next: 0.1.0-rc.6`).
- The Action: consumers pin `report.yml@v0.1.0-rc.7`, a tag the project has
  said it will supersede.

The reasons the first release was a candidate (the Safari lane, the crawl and
the model's role, `specs/SPEC-ci-action.md` 2026-11-18) are all settled:
Safari runs unattended on `macos-latest` and passes driver conformance
(Tasks 2 and 23); the crawl ran on 457 pages; the model ranks and the rules
set the tier (Task 76). Every checkpoint in [todo.md](todo.md) up to
Checkpoint I is ticked, and main's CI, Walkthrough and Release are green
(2026-11-25). What still stands in the way is listed under "Before
`v0.2.0-rc.1`".

### Why `v0.2.0`, and why not `v1`

A `0.x` version is a final release that still promises little. Under SemVer
and Go's module rules a `v0` major makes no compatibility promise, and npm's
caret treats the minor as the breaking digit below `1.0.0`: `^0.2.0` takes
`0.2.x` patches and never `0.3.0`. So a user who installs `0.2.0` is protected
from the next breaking change, and the project can still make one, in
`0.3.0`.

The jump from the `0.1.0` candidates to `0.2.0` says that this is not the
`0.1.0` they previewed: B2 changes the selector text in every report and
comment, and B6 adds a public flag. Nothing is ever published as `0.1.0`; the
`v0.1.0-rc.*` tags and the `go.mod` retractions stay as they are.

The promise (README status block, `specs/CONVENTIONS.md` "Versioning"):

- `0.2.x` patches fix bugs and never change the CLI's flags or exit codes,
  the bundle schemas, the `--json` progress events, the Action and
  `report.yml` inputs, or the npm and PyPI APIs.
- A minor (`0.3.0`) may change any of them, and its release notes say what
  changed and how to move (`docs/release-notes/<tag>.md`, which `release.yml`
  puts ahead of the generated notes).
- The schema URLs (`/schema/<name>/<major>.json`) keep their own major: a
  breaking schema change is a new `<major>`, not an edit of the old file.

### Before `v0.2.0-rc.1` (blocking)

The four changes merged on `main`:

1. **B4** (`fix-windows-firefox-install`): `omnisnap install firefox` fails on
   Windows. A final release must not ship a documented command that does not
   work on one of its three systems.
2. **B5** (`run-self-tests-on-release`): the release self-tests never ran on a
   release. `v0.2.0` is the release most people will pin, and nothing tested
   `report.yml@<tag>` with the tag's own binary before consumers did.
3. **B2** (`fix-selector-sibling-index`): selectors that can name the wrong
   element. It changes the selector text, so it goes in before the promise.
4. **B6** (`cap-live-browsers`): every browser stays open for the whole job.
   It adds `--max-browsers`.

Then `v0.2.0-rc.1`, to see the Windows install and the release self-tests
green on the runners, and `v0.2.0` from the same code plus the version bump.
A red candidate is fixed on `main` and becomes `-rc.2`; its tag is never
moved.

### At `v0.2.0` (the release itself)

The tooling already handles a tag without a dash (checked 2026-11-25):
`release.yml` passes `--prerelease` only for a dashed tag;
`tools/release`'s `packageVersions` and `tools/site/build.py`'s `versions`
turn `v0.2.0` into `0.2.0` for both npm and PyPI; the packages job publishes
with `--tag latest`; `deploy.yml` and `e2e-report.yml` pick the newest release
with `SHA256SUMS`, prereleases or not. What changes is the wording:

- Bump every version file to `0.2.0` (Release-Process step 1; the `--stage`
  check refuses the tag otherwise).
- `README.md`: the status block becomes the `0.x` promise above; drop `--pre`
  from the pip line and the "pip skips release candidates" sentence; the
  milestone table's "**M2 = v1**" row and "`v1` waits for the weekly network
  job and a stable public surface" no longer hold (M2 shipped as `0.2.0`).
- `packages/pypi/README.md`: drop `--pre` and its sentence.
- `specs/SPEC-ci-action.md` decision log: record the decision (stable
  `0.2.0`, not `0.1.0`, not `v1`) and the reasons.
- `docs/release-notes/v0.2.0.md`: the move from `0.1.0-rc.*` (the selector
  change, the browser cap, the Windows install fix).
- `docs/status-2026-11-19.md` is a dated report and stays as it is.

### Housekeeping (any time before `v0.2.0`)

- Remove the stale `next` dist-tag on the six npm packages, by hand with the
  owner's login (the trusted publisher has no dist-tag permission by
  design):
  `for p in omnisnap @omnisnap/linux-x64 @omnisnap/linux-arm64 @omnisnap/darwin-x64 @omnisnap/darwin-arm64 @omnisnap/win32-x64; do npm dist-tag rm "$p" next; done`.
- Archive the OpenSpec changes `add-developer-wiki` and `add-omnisnap-website`
  (done 2026-11-25).

### After `v0.2.0`, in `0.3.0` (not blocking)

- **B7**, the spec questions that would change a public schema or flag.

### Success criteria (proposed)

- `pip install omnisnap` (no `--pre`) and `npm i omnisnap` install `0.2.0` on
  Linux, Windows and macOS, and `omnisnap version` prints `v0.2.0`.
- The GitHub release `v0.2.0` is not marked as a prerelease and is GitHub's
  "Latest".
- `npm view omnisnap dist-tags` shows only `latest: '0.2.0'`.
- `omnisnap.org` names `v0.2.0` (deployed by the release).
- The release run, the release self-tests (B5) and "Install (network)" on
  Linux and Windows (B4) are green for the candidate, and the release run for
  the stable tag.
- The README states the `0.x` promise, and no user-facing doc says `--pre`.

### Boundaries

- **Always:** cut the stable from code that already passed as a candidate;
  keep the `go.mod` retractions.
- **Ask first:** calling anything `v1`; changing a public surface in a
  `0.2.x` patch.
- **Never:** move or reuse a tag (the Go module proxy caches them); publish
  `0.2.0` to npm or PyPI by hand outside the release workflow.

### Open questions

- What makes `v1`? Candidates to choose from, not decided: a stretch of
  `0.x` minors without a breaking change; real consumers beyond the owner's
  own sites; B7's questions answered; specific features (iframes, more than
  one viewport, a resizable WebKitGTK, see the specs' open questions). When
  it is decided, it goes into [todo.md](todo.md) as a checkpoint with
  criteria, so `v1` is not decided by feel.
- Should `report.yml` consumers be told to pin `@v0.2` (a moving tag) or keep
  pinning exact tags? A moving tag means moving tags, which this project has
  avoided so far.

---

## B4: `omnisnap install firefox` fails on Windows

**Status:** in progress, OpenSpec change `fix-windows-firefox-install` and Task 92 in [todo.md](todo.md) (2026-11-25). Blocks `v0.2.0` (B3). PR #20 (2026-11-26): Firefox now unpacks on `windows-latest` (86 MB to 341 MB). The network job then failed at discovery on two more Windows-only bugs, both fixed: the cache source required an execute bit, which Windows files never have, and discovery's file system (`os.DirFS`) could not read drive-letter paths at all, so the install cache and `PATH` sources had never worked on a real Windows machine. The network job is green on Linux and Windows since (run 45860965902). Left: the owner's install without admin rights, and the Windows unit log showing the 7z fixture and each extractor (needs a verbose CI step).

### Problem

`omnisnap install firefox` downloads Mozilla's Firefox, verifies it and
unpacks it into the cache without running an installer
(`specs/SPEC-install.md`). On Windows the unpack step leaves no `firefox.exe`,
so the command fails after the download and the signature check have
succeeded.

Evidence: the weekly network job's first scheduled run, "Install (network)",
run 45198060633 (2026-11-23, `main` at 595b146):

- `install ESR (ubuntu-latest)`: green.
- `install ESR (windows-latest)`: `TestInstallESRFromMozilla` fails after
  4.17 s with
  `network_test.go:27: install: the unpacked archive has no firefox.exe`.

The error comes from `findFile` in `internal/install/install.go`, called by
`finalize` after `unpackArchive` (`internal/install/unpack_windows.go`) ran
`msiexec /a <msi> /qn TARGETDIR=<dest>` without an error.

This is the only unticked box in [todo.md](todo.md) (Task 37, "Weekly network
job"), and the README's milestone table still says the job "first runs on
2026-11-23".

### Why the tests did not catch it

- The unit test builds its own MSI with WiX, with `firefox.exe` in an
  embedded cabinet (`internal/install/fixture_windows_test.go`), so it proves
  that `msiexec /a` extracts *that* MSI. It says nothing about the shape of
  Mozilla's.
- The only earlier run against Mozilla's real MSI (by hand, 2026-11-18)
  failed earlier, at the signature, and the fixes since then (the pinned key,
  `checksumFor` splitting names on spaces) were each found one step at a
  time. 2026-11-23 is the first run that got as far as unpacking.

### Likely cause (to confirm)

Mozilla's Windows MSI is understood to be a wrapper around the regular
`Firefox Setup <v>.exe` installer, which it runs as a custom action, rather
than an MSI whose file table holds Firefox's files. An administrative install
copies the file table, so it would leave the setup `.exe` (or nothing usable)
in `TARGETDIR`, never `firefox.exe`. This is a hypothesis: confirm it by
listing what `msiexec /a` left in the destination on `windows-latest` before
choosing a fix.

### Candidate approaches (to evaluate, not decided)

1. **Download the `.exe` installer and extract it** instead of the MSI. The
   `.exe` is a 7-Zip self-extracting archive; its `core/` folder is the
   browser. Needs a 7z reader: the runner images ship `7z`, user machines
   may not; a pure-Go 7z reader is a new dependency (CONVENTIONS: standard
   library first).
2. **Run the installer with an install directory and no shortcuts or
   registry**, e.g. its `/ExtractDir=<dir>` option if Mozilla's setup still
   supports it, which only extracts. Must be checked against
   SPEC-install.md's "without running an installer"; it would need a spec
   change if it counts as running one.
3. **Keep the MSI, extract the embedded setup with `msiexec /a`, then apply 1
   or 2 to that `.exe`.** Same tools as 1 or 2 with an extra step; only worth
   it if the MSI carries something the `.exe` download does not (it should
   not).
4. **Use the `.zip` builds** if Mozilla publishes one for the channel (it does
   for some nightly and unsigned builds, not for release or ESR as far as
   known). Simplest if it exists; check `archive.mozilla.org` first.

Whatever is chosen also settles the open question in
`docs/status-2026-11-19.md` ("`omnisnap install` on Windows relies on
`msiexec /a`: accept it, or download the `.exe` and extract with 7-Zip when
present?").

### Success criteria (proposed)

- "Install (network)" is green on `windows-latest` and `ubuntu-latest`:
  install ESR, discover it through the cache source, launch it through
  `process`, capture a planted page, uninstall leaving nothing.
- The Windows unit fixture is in the same format as Mozilla's download, so
  the unit test would have caught this.
- `omnisnap install firefox` (release channel, not only ESR) works on a
  Windows machine without admin rights, and leaves no registry key, Start
  menu entry or uninstaller entry (check `HKCU\Software\Mozilla` and
  "Apps & features").
- The signature check still covers exactly the file that is unpacked.
- Task 37's last box is ticked with the run ID; the README's milestone row is
  updated.

### Boundaries

- **Always:** verify the download against Mozilla's signed `SHA512SUMS`
  before unpacking; dispatch "Install (network)" by hand on the fix's branch
  rather than waiting for Monday.
- **Ask first:** adding a dependency (a 7z reader); a spec change to
  "without running an installer".
- **Never:** run an installer that writes to the registry or Program Files;
  download from anywhere but Mozilla's archive.

### Open questions

- Is the hypothesis right? (Check the extracted tree first.)
- Should the weekly job also run on a pull request that touches
  `internal/install/`? It is the only test of the real download.
- macOS is not in the network job's matrix (cost). Is one manual macOS run
  per release enough for the `.dmg` path?

---

## B5: The release self-tests never run on a release

**Status:** in progress, OpenSpec change `run-self-tests-on-release` and Task 93 in [todo.md](todo.md) (2026-11-25). Blocks `v0.2.0` (B3). Owner's call: npm and PyPI wait for the self-tests, the deploy does not. Both self-tests green when dispatched by hand; left: their first run on a real tag, `v0.2.0-rc.1`.

### Problem

`e2e-report.yml` (calls `report.yml` like a consumer, with a published
release binary) and `e2e-action.yml` (runs the composite action on the planted
suite) are meant to run on every release candidate. Both listen for
`on: release` (`types: [published, prereleased, created]`). `release.yml`
creates the release with the workflow's `GITHUB_TOKEN`, and GitHub starts no
workflow from events that token causes, so neither ever fires on a release.

Evidence:

- `v0.1.0-rc.1` fired no self-test run at all; it was dispatched by hand
  (comment in `e2e-action.yml`, 2026-11-19).
- The website deploy hit the same rule and was fixed by calling `deploy.yml`
  from `release.yml` as a reusable workflow (#15, rc.7 deployed that way on
  2026-11-25).

Why it matters: `v0.1.0-rc.2` shipped with two bugs that broke every consumer
(`./omnisnap-action` instead of `./omnisnap-action/action`, and the merge job
saving the binary under a name `SHA256SUMS` does not list), and `v0.1.0-rc.4`
and rc.6 shipped with `version` defaults that pointed at an older release.
`e2e-report.yml` exists to catch exactly that class, but only runs on pull
requests that touch `report.yml` or `action/`, never on the tag itself.

### Candidate approaches (to evaluate, not decided)

1. **Call both from `release.yml`**, as the deploy is: give them
   `workflow_call` with a `tag` input, and add jobs `needs: release`.
   `e2e-report.yml` must run after the assets are published (it downloads
   the binary); `e2e-action.yml` builds its own binary, so it can run in
   parallel with `release`.
2. **Dispatch them from `release.yml` with a PAT or a GitHub App token**, so
   the events count. Adds a secret to manage; rejected for the deploy for the
   same reason.
3. **Gate the release on them:** run the self-tests on the tag *before* the
   release job publishes, and publish only if they pass. Stronger, but
   `e2e-report.yml` tests the published binary, so it would need the
   `binary` input or a draft release.

Also drop the dead `release:` triggers once they are called, so the files do
not promise something that never happens.

### Success criteria (proposed)

- A tag push runs both self-tests without a manual step, against that tag's
  `report.yml` and that tag's binary; the release run's graph shows them.
- A self-test failure makes the release run red (the release itself is
  already published at that point under approach 1; the red run is the
  signal to cut the next rc instead of announcing).
- The packages job (npm and PyPI) runs only after the self-tests pass, so a
  consumer-breaking tag is not published to the registries. (Decide whether
  this ordering is wanted: it delays the registries by the self-test's
  10-20 minutes.)
- `docs/internals/GitHub-Action.md`, `Testing-and-CI.md`, `Release-Process.md`
  and `specs/SPEC-ci-action.md` describe the new trigger.

### Boundaries

- **Always:** keep `workflow_dispatch` on both, for a manual re-check.
- **Ask first:** adding a token secret (approach 2); holding the registries
  back on the self-tests.
- **Never:** let a self-test failure delete or move a published tag.

### Open questions

- Should the deploy wait for the self-tests too, so `omnisnap.org` never
  advertises a release that fails its own consumer test?
- `e2e-report.yml` runs macOS only when asked; is a release the place to
  always include it?

---

## B6: Every browser stays open for the whole job, and the memory adds up

**Status:** in progress, OpenSpec change `openspec/changes/cap-live-browsers`
and Task 94 in [todo.md](todo.md) (2026-11-25), in `v0.2.0` (through
`v0.2.0-rc.1`). It adds `--max-browsers` (default 3) and the Linux
out-of-memory score. Measured: eight engines peak at 1,816 MB with at most 3
live browsers. The review (2026-11-26) fixed a Ctrl-C between groups that
exited 0 with a summary. Left: the OOM test on the Linux CI driver job and
`e2e-action.yml` dispatched by hand.

### Problem

`specs/SPEC-process.md` asks for a configurable cap on concurrently live
browsers (default 3). It is not implemented: every engine's browser is
launched once and stays open until the job ends, so a job with every
discovered engine keeps all of them in memory at once.

Evidence: on the owner's Arch Linux machine (2026-11-25), running
`omnisnap <url>` with every discovered engine (Brave, Chrome, Chromium, Edge,
Firefox, Firefox ESR, WebKitGTK, WPE: eight) closed the owner's own Firefox,
without session restore. The code rules out a direct kill (process groups,
`--no-remote`, temporary profiles), and the same URL with
`--engines chrome,firefox` did not reproduce it, so the likely cause is the
kernel's out-of-memory killer choosing the largest process: the user's
Firefox. Since then `README.md` and `docs/quickstart.md` carry a note about
memory and point at `--engines`, and `SPEC-process.md` marks the cap as not
implemented.

### Candidate approaches (to evaluate, not decided)

1. **The spec's cap:** at most N live browsers; an engine's browser is closed
   when its captures for the job are done, and the next one starts. Changes
   the order captures run in (lanes, interleaving: `SPEC-capture.md`), which
   matters for run pairing; check the planted gates.
2. **Raise omnisnap's browsers' `oom_score_adj`** on Linux (write
   `/proc/<pid>/oom_score_adj` for each launched browser), so the kernel kills
   a browser Omnisnap started, which is restarted or reported, instead of the
   user's. Cheap, Linux only, does not reduce the memory.
3. **A memory check before launch:** warn (or lower the cap) when free memory
   is below an estimate per engine.

1 and 2 are complementary.

### Success criteria (proposed)

- With every engine on a machine with 8 GB, the peak resident memory of
  Omnisnap's browsers stays below a stated bound (measure today's first).
- The planted gates hold with the cap on (24 of 24, no false report on the
  controls), and the run pairing is unchanged.
- On Linux, under memory pressure, the OOM killer picks one of Omnisnap's
  browsers before the user's (approach 2), and the crash policy reports it.
- `SPEC-process.md`'s "Not implemented yet" line goes; the README note says
  what the cap does.

### Boundaries

- **Ask first:** changing the default engine set or the cap's default.
- **Never:** touch processes Omnisnap did not start.

### Open questions

- Is the cap a flag (`--max-browsers`), a job field, or both? Adding either is
  additive, so it fits a patch release. (Answered by the change: a flag.)
- Does a closed-and-relaunched browser change captures (cache, fonts warm-up)
  enough to matter for the comparison?

---

## B7: Spec questions that would change a public surface

**Status:** proposed (2026-11-25). Retargeted to `0.3.0` the same day (B3):
each answer either lands in `0.3.0` or is recorded as "additive, later".

### Problem

After `v0.2.0`, a `0.2.x` patch must not change the CLI, the schemas, the
progress events, the Action's inputs or the wrappers' APIs (B3's promise).
Several open questions in the specs would change one of them, so each one
answered late costs a breaking minor. Collected 2026-11-25 from the specs'
Open questions and `docs/status-2026-11-19.md`:

| Question | Where | Surface it changes |
|---|---|---|
| Warnings as a first-class progress event, so `--json` consumers see them | status report | progress schema |
| `findings.json` gains the full evidence (perception events, ink) | status report | findings schema |
| Host ids on merge: runner OS name, or `--host-id` | `SPEC-merge.md` | CLI flag, summary hosts |
| Merge deduplicates the same finding across hosts in the PR comment | `SPEC-merge.md` | comment format |
| A config file (`omnisnap.json`) or job files only | `SPEC-cli.md` | CLI |
| With only Blink browsers, suggest `omnisnap install firefox` with exit code 2 | `SPEC-cli.md` | exit codes |
| Pages under 50 elements count as bot walls, or only with blocked requests or a challenge title | status report, `SPEC-capture.md` | which pages are compared; the comment's Not checked table |
| Discovery lists browsers it cannot drive (Safari without `--enable`) with a note | `SPEC-discovery.md` | `browsers --json` output |
| Viewports beyond 1280×800 | `SPEC-capture.md` | job schema, summary |
| WebKitGTK as a "same-engine only" engine, or a CLI change | `SPEC-capture-webkit-viewport.md` | CLI, summary |
| ~~Selectors that count hidden siblings (B2)~~ answered: in `0.2.0` (B2) | backlog B2 | capture schema or selector text |
| An interrupt (Ctrl-C) exits 4, the code for "a bug, please report it": give interrupts their own code (130, as shells do)? Found 2026-11-26 | `SPEC-cli.md` | exit codes |
| `labels.json` carries cause ids | `SPEC-ground-truth-mutant-policy.md` | lab output (not public: can wait) |

### What to do

For each row: decide now, or decide that it will be added in a backward-
compatible way later (a new optional field, a new flag), and record which in
the spec's Open questions (struck through with the answer, as the specs do
today). The ones decided "now" become tasks for `0.3.0`.

### Success criteria (proposed)

- Every row has a recorded answer in its spec.
- The rows answered "breaking" ship together in `0.3.0`, with release notes
  that say how to move from `0.2.x`.

### Not in this item

Questions that change no public surface (clock control, A/B variants, a move
detector, iframes, the viewer's diff overlay, Playwright and pytest
integrations): they can be answered any time, additively.

---

## B8: The report self-test passes when Windows captures nothing

**Status:** proposed (2026-11-26, from the pre-release review of PR #20). Not blocking `v0.2.0`.

### Problem

`.github/workflows/e2e-report.yml`'s assert job checks that the merged report
has three hosts and that the planted finding `table#target` reached the
comment. That finding can come from the Linux or the macOS bundle alone, and
`e2e-action.yml` injects a fault on Windows on purpose. So a release binary
that fails every capture on Windows still passes both self-tests, and npm and
PyPI publish it (B5 made them wait for exactly these gates).

### Success criteria (proposed)

- The assert job fails unless the Windows host contributed a successful
  capture of `03-overflow` (from its bundle's `summary.json`).
- A dispatched run with a binary that cannot capture on Windows is red.

---

## B9: Windows Defender can make the Firefox install's last rename fail

**Status:** proposed (2026-11-26, from the pre-release review of PR #20). Not blocking `v0.2.0`; check during `fix-windows-firefox-install` task 6.2.

### Problem

`internal/install/install.go` moves the freshly extracted `core\` folder into
the cache with one `os.Rename`. On real Windows machines an antivirus scan of
the new files commonly holds them open for a moment, and the rename then fails
with "Access is denied". The hosted runners did not hit it. Cleanup is right
(the temp directory and the download are removed); the user just gets a failed
install and has to run the command again.

### Success criteria (proposed)

- The rename retries for a short, bounded time on `ERROR_ACCESS_DENIED` and
  `ERROR_SHARING_VIOLATION`, and then fails with the original error.
- A unit test simulates a rename that fails twice and then succeeds.

---

## B10: The install's free-space check is skipped on a first install

**Status:** proposed (2026-11-26, from the pre-release review of PR #20). Existed before `v0.2.0`; not blocking it.

### Problem

`install.Firefox` checks free space with `freeDisk(cache)` before the cache
directory exists. On a first install, `statfs` (or `GetDiskFreeSpaceExW`)
fails on the missing path, the error is ignored, and the "needs about N times
the download" check never runs: on a nearly full disk the install fails
part-way through unpacking instead of up front. The test replaces `freeDisk`,
so it does not see this.

### Success criteria (proposed)

- The check measures the nearest existing parent of the cache directory.
- A test with a cache path whose parents do not exist yet still gets a
  free-space reading.

---

## B11: A kill between unpacking and the marker leaves an install nothing can see or remove

**Status:** proposed (2026-11-26, from the pre-release review of PR #20). Existed before `v0.2.0`; not blocking it.

### Problem

The install writes `omnisnap-source.json` last, so discovery never lists a
half install (right). But if the process is killed after the browser is moved
into `firefox/<version>/` and before the marker is written:

- `omnisnap install firefox` says "already installed";
- discovery never lists it, because it has no marker;
- `omnisnap uninstall firefox` says no Firefox is installed and removes
  nothing.

Leftover `install-*` temp directories also survive `uninstall`, which removes
only `firefox/` and `downloads/`. The window is short.

### Success criteria (proposed)

- "Already installed" requires the marker; a version directory without one is
  removed and installed again.
- `uninstall` also removes `install-*` leftovers.

---

## B12: A class value with a non-breaking space becomes two classes in the selector

**Status:** proposed (2026-11-26, from the pre-release review of PR #20). Existed before `v0.2.0`; rare; not blocking it.

### Problem

HTML splits `class` on ASCII whitespace only, so `class="btn primary"`
is one class. `extract.js`'s `norm()` (JavaScript `\s`) and
`internal/selector`'s `strings.Fields` both split on Unicode spaces too, so
the selector says `.btn.primary`, which matches nothing.

### Success criteria (proposed)

- Both sides split classes on ASCII whitespace only (an `extract.js` change:
  ask first), and the selector escapes the non-breaking space.
- A selector test and a conformance case with such a class.

---

## B13: With many engines, the design-intent check floods a planted page with Report causes

**Status:** proposed (2026-11-26). Found while measuring B6; not caused by the cap, and not blocking `v0.2.0`.

### Problem

The design-intent check (`SPEC-layout-diff.md`, step 6) uses the captures of
a third engine family. On the owner's machine (2026-11-25), an eight-engine
job on the planted suite (Brave, Chrome, Chromium, Edge, Firefox, Firefox ESR,
WebKitGTK, WPE; `--max-browsers 3`) gave every engine **24 `intent-size`
Report causes** on `28-table-layout`, whose Report budget is 3. The same page
in the three-engine run (Chrome, Firefox, WebKitGTK) has one `columns` cause.
The captures themselves are the same geometry within 0.5 px in both runs, so
the extra causes come from the check seeing more third-family captures, not
from the pages or the grouping.

Both the CLI and the Action capture every discovered engine by default, so a
developer with many browsers installed gets this on a table layout. On the
hosted runners the default stays at three working engines (Chromium and Edge
fail to launch on Ubuntu, backlog B1), which is why CI does not show it.

Evidence: change `cap-live-browsers`, task 3.7's first run.

### Success criteria (proposed)

- An eight-engine run of the planted suite keeps every planted page within
  its Report budget, with recall unchanged (24/24).
- The three-engine planted gates and golden parity are unchanged.

### Open questions

- Should the intent check use one capture per third family (the first, or a
  consensus) instead of all of them?
- Should several engines of the same family count once in the consensus?
