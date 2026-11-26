# Tasks

## 1. Wiki set up (owner steps first)

- [x] 1.1 The owner enables the Wiki (`gh repo edit GerMP21/Omnisnap --enable-wiki`) and saves a placeholder `Home` page in the web UI. Verify: `gh api repos/GerMP21/Omnisnap --jq .has_wiki` prints `true`, and `git ls-remote https://github.com/GerMP21/Omnisnap.wiki.git` lists a branch.
- [x] 1.2 Check the wiki conventions this design assumes against GitHub's docs, and record each source URL with the passage it supports in the `Home` page's editing notes: file name becomes the page title, `Home.md` is the landing page, `_Sidebar.md` and `_Footer.md` are shown on every page, Mermaid renders, and the git repository exists only after the first page is saved. Verify: every assumption has a quoted source, or the design is corrected before any page is written.
- [x] 1.3 Clone the wiki repository to `../Omnisnap.wiki`, and record the pinned commit: the current `main` SHA and its release tag. Verify: `git -C ../Omnisnap.wiki log -1` shows the placeholder page, and `git rev-parse main` matches the SHA recorded for the footer.
- [x] 1.4 Write the link checker in the session scratchpad. For every `blob/<sha>/<path>#L<n>` link, it checks that the path exists at `<sha>` and that `n` is within the file; it also reports wiki links to pages that don't exist. Verify: it fails on a page with one deliberately broken link of each kind, and passes once they are removed.

## 2. Start pages

- [x] 2.1 Write `_Sidebar.md`, with the five groups and 24 pages from design.md, and `_Footer.md`, naming the pinned SHA and its tag. Verify: the link checker reports every sidebar entry as a missing page, and nothing else. They are created in the next tasks.
- [x] 2.2 Write `Home.md`: what the wiki is for, how it relates to the specs (the specs are normative), README and `docs/`, how to read it, and the editing notes from 1.2. Verify: the link checker passes for the page.
- [x] 2.3 Write `Architecture.md`, from `cmd/omnisnap/main.go`, `internal/cli/run.go` and `internal/pipeline/`. It has the pipeline diagram (URL → discovery → process → drivers → capture → layout diff → appearance → triage → audit → summary.json → viewer), the module dependency diagram from `specs/CAPABILITY-MAP.md`, and the split between `omnisnap` and `omnisnap-lab`. Verify: both Mermaid diagrams render in the wiki preview, and the link checker passes.
- [x] 2.4 Write `Repository-Layout.md`, one line per top-level directory and per `internal/` package, and `Glossary.md` (cause, effect, kind, consequence, tier, evidence, probe, mask, lane, bundle, baseline, engine family, planted page, KB entry). Verify: every directory in `ls` and `ls internal` appears exactly once, and the link checker passes.

## 3. Contracts and the capture side

- [x] 3.1 Write `Data-Contracts.md`, from `internal/schema/`, `schema/` and `SPEC-bundle-schema.md`. It covers `job.json`, the bundle layout, `<engine>-<run>.json` captures, probes files, `summary.json`, `<slug>/findings.json`, NDJSON events, and schema versioning with the `omnisnap.org` `$id`s. Verify: every file name in the page appears in a bundle produced by `go run ./cmd/omnisnap run` on a planted job; every event type matches `internal/cli/progress.go`; the link checker passes.
- [x] 3.2 Write `Discovery.md` (`internal/discovery/`: per-OS sources, family and channel, the 300 ms budget) and `Process-Management.md` (`internal/process/`: Job Objects, process groups, temp profiles, template copies, the sweeper, headless display). Each page has its "Where the code differs from the spec" section. Verify: each page names the tests that back its claims (for example, `TestTerminatingOmnisnapKillsEveryBrowser`), and the link checker passes.
- [x] 3.3 Write `Drivers.md`, from `internal/drivers/{cdp,bidi,classic,wsrpc,replaytest}`: what each protocol client does, which engine uses it, screenshots (native or scroll-and-stitch), and the conformance suite. Verify: the engine-to-protocol table matches `internal/drivers/conformance_test.go`, and the link checker passes.
- [x] 3.4 Write `Capture-Pipeline.md`, from `internal/capture/` and `assets/inject/`: normalization, stabilization, extraction, probes, two runs, parallel lanes, the re-measure, and the crash retry (`captureRetrying`, `Lost`). It includes the lane diagram. Verify: the diagram's states match `lanes.go` and `crash.go`, and the link checker passes.
- [x] 3.5 Write `Branded-Browsers.md` (`internal/branded/`, `omnisnap warm`, blocked-request attribution, `--neutral`) and `Install.md` (`internal/install/`: download, signature check, cache, uninstall). Verify: the commands quoted match `omnisnap --help`, and the link checker passes.

## 4. The analysis side

- [x] 4.1 Write `Layout-Diff.md`, from `internal/layoutdiff/`: run pairing and masks, matching, deltas and the `Tol*` constants, root causes versus effects, table columns, design intent, and warnings. Verify: every constant and cause kind in the page is found in the code by grep at the pinned SHA, and the link checker passes.
- [x] 4.2 Write `Appearance-Diff.md`, from `internal/appearance/`: registration, SSIM with the ±3 px search, ink masks, and the noise floor. Verify: the thresholds match `ssim.go` and `ink.go`, and the link checker passes.
- [x] 4.3 Write `Triage.md`, from `internal/triage/` and `evidence/`: the knowledge base, consequences, the tier rules and their two exceptions, each kind of evidence, the model score, novelty, and grouping. Its "differs from the spec" section covers the knowledge base being Go code, not data in `assets/kb/`. Verify: the KB ids, consequence names and evidence names match the code by grep, and the link checker passes.
- [x] 4.4 Write `DOM-Audit.md`, from `internal/domaudit/`: the seven checks, their thresholds and severities, and `--no-audit`. Verify: the severities match `checks.go`, and the link checker passes.

## 5. Output, CI and quality

- [x] 5.1 Write `CLI-and-Orchestration.md`, from `internal/cli/`: subcommands, how job and flag options combine, `restartable` engines, the progress events, and exit codes. Its "differs from the spec" section covers `--no-probes` and `--open`. Verify: the flags listed match `omnisnap run --help`, the exit codes match `exit.go`, and the link checker passes.
- [x] 5.2 Write `Viewer.md` (`internal/viewer/`: template, app, how findings are drawn) and `Merge-and-PR-Comment.md` (`internal/merge/`, `omnisnap comment`, host ids). Verify: the link checker passes, and the page's description of the viewer matches a report generated from a planted bundle.
- [x] 5.3 Write `GitHub-Action.md`, from `action/`, `.github/workflows/report.yml`, `report-comment.yml` and `docs/github-action-fork-comments.md`: the job graph, the Linux engine setup, merging, and the fork path. It links to the user docs rather than repeating the inputs. Verify: the job names match the workflow files, and the link checker passes.
- [x] 5.4 Write `Wrappers.md`, from `packages/npm` and `packages/pypi`: binary resolution, platform packages and wheels, the API, and the drift tests. Verify: the function and option names match `index.d.ts` and `_run.py`, and the link checker passes.
- [x] 5.5 Write `Ground-Truth-and-the-Model.md`, from `internal/groundtruth/`, `cmd/omnisnap-lab/`, `tools/train.py`, `assets/model/` and `docs/status-2026-11-19.md`: planted pages, mutation testing, the crawl, training, the parity test, the gate result, and why the model only ranks. Verify: the gate numbers match the status note, and the link checker passes.
- [x] 5.6 Write `Testing-and-CI.md` (test levels, goldens, conformance, browser tags, and each workflow in `.github/workflows/` with its trigger) and `Release-Process.md` (`tools/release/`, `wheels.py`, npm and PyPI publishing, `go.mod` retractions, and the step to re-pin the wiki after a release). Verify: every workflow file appears once with a trigger that matches its `on:`, and the link checker passes.
- [x] 5.7 Write `Extending-Omnisnap.md`: how to add a knowledge-base entry, a cause kind, an audit check, an engine or browser, and a planted page. Each recipe names its files, tests and spec to update. Verify: each recipe's file list exists at the pinned SHA, and the link checker passes.

## 6. Integration and hand-off

- [x] 6.1 Run the link checker over the whole clone, and preview every page locally with a Markdown renderer that supports Mermaid. Verify: there are 0 broken links, every sidebar entry resolves, and the diagrams render.
- [x] 6.2 Collect every code-versus-spec difference found while writing pages into one list for the owner. Nothing in the main repository is changed. Verify: each difference names its page, spec section and code location.
- [x] 6.3 Only if the owner approves the README link (design.md, Open Questions): add one "Developer wiki" line to `README.md`'s Repository section. Verify: the link resolves after the wiki is pushed.
- [x] 6.4 Hand over to the owner: the clone path, the pinned SHA, the difference list, and the `git -C ../Omnisnap.wiki push` command. The owner reviews and pushes. Verify: the owner confirms the wiki is live.

## 7. Move the pages into the repository

- [x] 7.1 Copy the 26 pages into `docs/internals/`: links between pages become relative (`Page-Name`), code links go to `blob/main` or `tree/main`, line anchors are removed. Verify: no link to `/wiki/`, `0259ece` or `#L` remains.
- [x] 7.2 Write `test/internals/internals_test.go`, a port of Corrosion-VST's wiki test plus the `main`-only and no-line-anchor checks. Verify: each check fails on a deliberately broken page (bad page link, bad anchor, missing file, line anchor, non-`main` ref, unnamed Go file, missing sidebar entry, a section after "Source files") and passes once restored.
- [x] 7.3 Give every page but Home, Glossary, `_Sidebar` and `_Footer` a final `## Source files` list, so every non-test Go file and every top-level item the test covers is named. Verify: `go test ./test/internals/` passes.
- [x] 7.4 Add `scripts/publish-wiki.sh` and `.github/workflows/wiki.yml`. Verify: on a scratch clone of the wiki, the script removes a stale page, makes the wiki identical to `docs/internals/`, and makes no commit on a second run.
- [x] 7.5 Rewrite the upkeep text: Home's "How this wiki is kept", `_Footer.md`, Release-Process step 6, Repository-Layout and Testing-and-CI; add rule 7 to `CONTRIBUTING.md`, a section to `CLAUDE.md`, and the README row. Verify: the test passes.
- [x] 7.6 The owner commits, merges to `main`, and sees the wiki workflow publish. Verify: the wiki matches `docs/internals/` and its footer says it is published from the repository.

## Workflow follow-up

- Archive the change after the owner has pushed the wiki and reviewed it.
