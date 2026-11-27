# Spec: ci-action

Module id: `ci-action` · Location: `action/` · Depends on: `cli`, `merge` · Milestone: M2 (v1 = M2 released) · Conventions: [CONVENTIONS.md](CONVENTIONS.md)

## Objective

A GitHub Action that runs Omnisnap on Ubuntu and Windows runners for every pull request (macOS is opt-in: hosted macOS runners cost ~10x) and posts **one** merged comment with the cross-engine differences, real Safari included, so developers on Windows and Linux get Safari results without owning a Mac. **Report-only: it never fails the build** (one wrong failure gets a visual gate uninstalled; the gate waits for report-mode noise data).

Success metric (one-pager): the Action stays installed on ≥ 5 outside repositories for a month.

## Behaviour

### Usage

```yaml
# .github/workflows/omnisnap.yml
on: pull_request
jobs:
  omnisnap:
    uses: GerMP21/Omnisnap/.github/workflows/report.yml@v0.2.0-rc.3
    with:
      start: npm ci && npm run build && npx serve -l 3000 dist
      wait-on: http://localhost:3000
      pages: |
        http://localhost:3000/
        http://localhost:3000/pricing
```

A reusable workflow (`report.yml`: the OS jobs + the merge job) plus a composite action (`action/action.yml`) for custom matrices.

### The two ways to use it

```yaml
# 1. the reusable workflow: the OS jobs, the merge and the comment
# .github/workflows/omnisnap.yml
on: pull_request
jobs:
  omnisnap:
    uses: GerMP21/Omnisnap/.github/workflows/report.yml@v0.2.0-rc.3
    with:
      pages: |
        http://localhost:3000/
```

```yaml
# 2. the composite action, in your own matrix
# .github/workflows/omnisnap.yml
on: pull_request
jobs:
  omnisnap:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7
      - uses: GerMP21/Omnisnap/action@v0.2.0-rc.3
        with:
          version: v0.2.0-rc.3
          pages: |
            http://localhost:3000/
```

The composite action is referenced **by path** (`GerMP21/Omnisnap/action@<tag>`): the repository root has no `action.yml`, so `GerMP21/Omnisnap@<tag>` would not resolve.

The reusable workflow checks out two things: the caller's repository (the app under test needs its own code for `start`) and the action itself, beside it. A plain `actions/checkout` inside a reusable workflow brings the **caller's** repository into the workspace — where there is no `action/` directory — so the action is checked out with `repository: ${{ job.workflow_repository }}` and `ref: ${{ job.workflow_sha }}`, the ref the caller pinned the workflow to. That checkout is the whole repository, so the step is `uses: ./omnisnap-action/action`, not `./omnisnap-action` (v0.1.0-rc.2 shipped with the latter, and every capture job failed with "Can't find 'action.yml'").

Source: https://docs.github.com/en/actions/learn-github-actions/contexts#github-context
> "This reusable workflow uses `job.workflow_repository` and `job.workflow_sha` to check out its own source code, rather than the caller's repository. This is useful when a reusable workflow needs to access files co-located with the workflow definition."

### Inputs

Composite action (`action/action.yml`):

| Input | Default | Does |
|---|---|---|
| `version` | `v0.2.0-rc.3` | release tag the binary is downloaded from |
| `pages` | (required) | one URL per line |
| `engines` | every installed | comma-separated engine ids |
| `baseline` | `chrome` | baseline engine |
| `runs` | `2` | captures per engine and page |
| `out` | `omnisnap-out` | bundle directory |
| `start` | — | shell command that starts the app under test |
| `wait-on` | — | URL to wait for before capturing |
| `upload` | `true` | upload the bundle as an artifact |
| `artifact-name` | `omnisnap-<os>` | artifact name |
| `binary` | — | path to an existing binary; skips the release download (self-tests, air-gapped setups) |
| `allow-small-pages` | `false` | drop the under-50-elements bot-wall rule (planted and lab pages) |
| `setup-timeout` | `300` | Linux: seconds the engine package downloads may take before the step skips what it could not install |

The reusable workflow (`report.yml`) adds `macos` (`every` | `labelled` | `never`, default `every`) `artifact-retention-days` (default `7`; the step only prints it — the retention itself is a repository setting) and `artifact-prefix` (default `omnisnap`: the capture artifacts are `<prefix>-<os>`, the merge job downloads only `<prefix>-*-latest` and uploads `<prefix>-merged`; a run that calls `report.yml` more than once gives each call its own prefix, because artifact names are unique per run), and passes `setup-timeout` through to the action.

### Per OS job

1. Download the release binary for the runner and check it against `SHA256SUMS` (skipped when the `binary` input is given).
2. Prepare engines:
   - **Linux** (`action/linux-engines.sh`, backlog B1): the parity fonts (`fonts-liberation fonts-noto-mono`) always, and Weston with the WebKit driver package (`webkitgtk-webdriver`; `webkit2gtk-driver` on images whose archive has no candidate for the new name) when WebKitGTK is wanted: `engines` empty or naming `webkitgtk`. The downloaded `.debs` are cached with `actions/cache`, keyed on the package set, the runner arch, `ImageOS` and `ImageVersion`. An exact hit installs them with `dpkg -i` and downloads nothing; a miss restores the previous image's set by prefix, runs `apt-get update` and downloads with `--download-only` under `setup-timeout` (default 300 s), then installs with `--no-download`. Only the download is bounded, because killing dpkg would leave the package database half configured. Past the budget the step warns and the capture goes on with the engines the image ships (Chrome, Firefox). Only a complete set is saved, pruned to the versions now installed. The Ubuntu mirror served about 40-50 KB/s on 2026-11-20, and a consumer's job spent its whole 30 minutes in the unbounded install it replaced;
   - **macOS:** `sudo safaridriver --enable` (allowed on hosted runners); Safari is the only WebKit here;
   - **Windows:** Chrome, Edge and Firefox from the runner image.
3. Start the user's app (`start`, under `nohup`) and wait for `wait-on` (120 tries, 1 s apart; a timeout is a warning, not a failure).
4. Write `omnisnap-job.json`, then `omnisnap run omnisnap-job.json --json --max-browsers 4` (the flag only when the binary's `--help` lists it: one from before `v0.2.0`, which a pinned `version` can select, rejects it); the NDJSON progress lines are tee'd into the log. Four live browsers, not the CLI's default of 3, so every hosted runner captures in one group: `macos-latest` starts Chrome, Edge, Firefox and Safari, and with 3 Safari ran alone in a second group, about 100 s after the baseline (run 45872276142; owner, 2026-11-26). A self-hosted runner with more engines still captures in groups of four (`SPEC-process.md`, Resources).
5. Upload the bundle as an artifact (`omnisnap-<os>`, plus the progress log and `app.log`). The upload is report-only too: it is retried once (with `overwrite`, in case the first attempt finalized), and if both attempts fail the step warns and the job still succeeds; the merge job then has one bundle fewer. A hosted macOS runner lost DNS for the artifact store mid-upload on 2026-11-20 (`ENOTFOUND`), after every capture had succeeded.
6. Always exit 0.

**Report-only, exactly.** The capture step runs under `bash -e -o pipefail` (GitHub's default for `shell: bash`), so a non-zero `omnisnap` exit (3: every capture failed) would end the step before its code could be read. The step therefore turns `-e` off around the pipeline, reads `${PIPESTATUS[0]}`, emits a warning and exits 0.

Source: https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax#exit-codes-and-error-action-preference
> "By default, fail-fast behavior is enforced using `set -e` for both `sh` and `bash`. When `shell: bash` is specified, `-o pipefail` is also applied to enforce early exit from pipelines that generate a non-zero exit status."

### The macOS policy

The `macos` input: `every` (the default) runs the macOS job, and its Safari capture, on every PR; `labelled` runs it only on PRs labelled `omnisnap:safari`; `never` drops the job.

The choice is made in `strategy.matrix`, not in the job's `if`: `jobs.<job_id>.if` cannot read the matrix context, so GitHub rejected the whole file when the choice lived there. `strategy` can read `github` and `inputs`, and the matrix is built with `fromJSON(case(...))`.

Source: https://docs.github.com/en/actions/reference/workflows-and-actions/contexts#context-availability
> `jobs.<job_id>.if` → `github, needs, vars, inputs`; `jobs.<job_id>.strategy` → `github, needs, vars, inputs`.

Source: https://docs.github.com/en/actions/reference/workflows-and-actions/expressions#case
> "Evaluates predicates in order and returns the value corresponding to the first predicate that evaluates to `true`. If no predicate matches, it returns the last argument as the default value."

Source: https://docs.github.com/en/actions/reference/workflows-and-actions/expressions#contains
> "Returns `true` if `search` contains `item`. If `search` is an array, this function returns `true` if the `item` is an element of the array."

The macOS job is in this repository's own CI matrices (`ci.yml`, `e2e-action.yml`) since 2026-11-19: the account has the runners, and the conformance job enables `safaridriver` there. `safari-smoke.yml` stays the manual one-pager check.

### Merge job

1. Download the artifacts (`actions/download-artifact@v8`, pattern `omnisnap-*`).
2. Get the binary: the same release download as the OS jobs (checksum verified) — the merge job runs on the runner, not in the bundle. It is downloaded under the asset's own name, the one `SHA256SUMS` lists, and renamed only after the check: `sha256sum -c --ignore-missing` skips a file saved under any other name and fails with "no file was verified" (the v0.1.0-rc.2 merge job did that).
3. `omnisnap merge <bundles> --rename-hosts -o merged` (`--rename-hosts` gives a colliding host id a `-2`, `-3` suffix; without it the merge is an error). No bundle at all → a warning and exit 0.
4. `omnisnap comment merged --report-url <run> --run-url <run> > merged/comment.md`.
5. Upload the merged bundle (with `report.html`).
6. Post or update **one** PR comment (same-repo PRs only), identified by the hidden marker `<!-- omnisnap-report -->`: the comment is created if the marker is absent and updated in place otherwise, so a second push never adds a second comment.

Both the OS jobs and the merge job download the binary from the release, so the Action needs a published release; the `binary` input covers only the OS job, and only from inside this repository (the self-test).

The `version` input is the release tag, and the same string names the assets and the download path: `omnisnap_<version>_<os>_<arch>[.exe]` under `releases/download/<version>/`, with `SHA256SUMS` next to them. The reusable workflow's ref is the same tag (`report.yml@v0.2.0-rc.3`), so one tag gives both the workflow a consumer can point at and the binaries it downloads.

The comment is written for someone reviewing the pull request, not for someone who knows Omnisnap's vocabulary: tiers, statuses and kinds become plain sentences, browsers and operating systems are named as people write them ("Safari on macOS"), and the host id is added only when two hosts run the same OS. It carries, in this order:

- **a verdict** as a GitHub alert: `[!WARNING]` "N layout issues on M of K pages checked", `[!TIP]` "No layout issues found on K pages in B browsers", or `[!CAUTION]` "Nothing was compared" when every capture failed; it names the baseline ("compared with Chrome on the same machine");
- **one table row per page**, in the job's order, linked to its URL: `⚠️ N issues in <browsers>`, `✅ no issues in B browsers`, or `❌ not checked`, plus "results may be unreliable" when a browser loaded a different page state (status `diverged`) and "N browsers not checked";
- **per page with issues**, each Report-tier cause **once**, however many browsers and hosts show it (grouped by title and selector), strongest first, at most 5 (`…and N more on this page, in the full report`). Each issue gives its title and selector, **Where** (how far from the top of the page, from the baseline box), **Seen in** (each browser, with how the element differs from the baseline: "12 px wider", "2 more lines", "moved 4 px right"; browsers that differ by the same amount share a line), **What a visitor sees** (the consequences in words), **Why** and **Suggested fix** (from the knowledge-base entry, when one explains it) and **Same cause also affects** (up to 3 other instances). A `diverged` browser gets a note that its differences may not be real; the page's Info-tier causes are counted in one line, not listed;
- **Not checked:** a table of browser, page and reason: engines skipped (`skipped: <reason>`, page `all pages`), captures that failed (`capture failed`) and pages that could not be loaded (`page could not be loaded (unreachable, or a bot check)`);
- links to the full report and the workflow run, and the report-only footer. Over 65,000 characters (GitHub's cap is 65,536) the body is cut at a line boundary with a pointer to the report; the links and the footer are always kept.

### Permissions and forks

`pull-requests: write` and `contents: read` only. Pull requests from forks get a read-only token: the comment is posted by a `workflow_run` follow-up workflow that reads the artifact (documented pattern), never with secrets exposed to fork code. The follow-up is a reusable workflow (`report-comment.yml`, `actions: read` + `pull-requests: write`) called from a file the consumer keeps on the default branch — GitHub only fires `workflow_run` for a file that exists there, and it cannot chain more than three workflow levels. The consumer file is `.github/workflow-templates/omnisnap-fork-comment.yml` (+ `.properties.json`); the guide is `docs/github-action-fork-comments.md`. GitHub's template gallery only reads that folder from an organization's `.github` repository, so in this repository it is an inert copy (`SPEC-ci-action-fork-template.md`).

`report.yml` skips its own comment step when the PR head is another repository; the fork follow-up downloads the `omnisnap-merged` artifact of the report run (`actions/download-artifact@v8` with `run-id` + `github-token`) and upserts the same marker.

## Commands

```
act -j omnisnap                                   # local dry run where possible (Linux job only)
gh workflow run e2e-action.yml                    # the repo's own e2e: the Action on the planted suite, two runners
go test ./internal/cli/... -run Comment           # the comment renderer (markdown snapshot, truncation)
```

## Project structure

`action/action.yml` (composite) · `action/README.md` · `.github/workflows/report.yml` (reusable) · `.github/workflows/report-comment.yml` (fork-safe reusable) · `.github/workflows/e2e-action.yml` (self-test of the composite action) · `.github/workflows/e2e-report.yml` (self-test of the reusable workflow) · `internal/cli/comment.go` (the `omnisnap comment` renderer: a Go subcommand rather than shell, so it is unit-tested) · `.github/workflow-templates/omnisnap-fork-comment.yml` (the consumer template copy) · `docs/github-action-fork-comments.md` (the guide).

## Code style

YAML: no inline scripts longer than a few lines (logic lives in the binary, e.g. `omnisnap comment`); dependencies pinned to a major version tag (`actions/checkout@v7`, `actions/setup-go@v7`, `actions/download-artifact@v8`, `actions/upload-artifact@v7`, `actions/github-script@v8`, `actions/setup-node@v6`). SHA pinning would be stronger against a compromised tag, but it is a digest to re-verify on every bump across seven workflows, and every action here is a first-party GitHub action except the linter, Cloudflare's `wrangler-action` and PyPA's `gh-action-pypi-publish` (`release/v1`, the branch PyPA publishes it on).

## Testing strategy

- **Self-test (`e2e-action.yml`; manual dispatch, and called by every release run (`release.yml`); npm and PyPI wait for it):** it builds the binary from the tag it is given (or the ref the run started from), serves the planted suite with `omnisnap-lab serve` and runs the composite action on `ubuntu-latest` and `windows-latest` with the same five pages. The merge job then asserts that (a) the comment carries the marker, (b) the planted finding `table#target` reaches the comment, and (c) the comment's Not checked table names the part that failed (`| capture failed |`).
- **Reusable-workflow self-test (`e2e-report.yml`; manual dispatch, pull requests touching `report.yml` or `action/`, and called by every release run (`release.yml`) once the release's assets are up; npm and PyPI wait for it):** it calls `report.yml` the way a consumer does, with `artifact-prefix: selftest-report`, with a published release binary (the given tag's, failing early when that release has no `SHA256SUMS`; without a tag, the newest release with its `SHA256SUMS` up: the PR that bumps the `version` default runs before the new tag exists) and two planted pages served by `python -m http.server`, and asserts that the merged report holds three hosts and the comment carries the marker and the planted finding `table#target`. `report.yml` has no `allow-small-pages`, so the bot-wall rule applies: `03-overflow` (82 elements) is captured, `01-card-width` (47) is rejected on every host and shows up in the comment's Not checked table. `e2e-action.yml` still runs the full planted assertions. It exists because `e2e-action.yml` uses `./action` directly and never ran `report.yml`, which let v0.1.0-rc.2 ship with both consumer-breaking bugs above.
- **Fault injection:** the Windows runner is the failing one — nothing listens on port 8099 there, so every capture fails — and the job must still conclude success. This is success criterion 2, asserted on every run.
- **Wall time:** five pages must finish in < 10 minutes (asserted on the Linux runner).
- **Comment rendering:** unit tests of `omnisnap comment` on merged fixtures (markdown snapshot, truncation at the limit, both argument orders).
- **Smoke (first week of M0, one-pager assumption):** a ten-line workflow proves Safari runs unattended on a macOS runner. Still open: no macOS runner is available.

## Boundaries

- **Always:** exit 0; one comment, updated in place; pin dependencies to a known version.
- **Ask first:** a pass/fail mode (deferred until report-mode noise data exists); extra permissions; self-hosted runner support.
- **Never:** expose secrets to fork code; fail the build; upload captures anywhere but the workflow's own artifacts.

## Success criteria

1. On a sample repository, a PR gets one comment with results from Linux (Chrome, Firefox, WebKitGTK), Windows (Chrome, Edge, Firefox) and macOS (Chrome, Firefox, Safari). — **open**: the sample repository does not exist yet.
2. With one OS job failing (fault injection), the workflow still concludes success and the comment says which part failed. — **verified** in the self-test (Windows is the injected failure).
3. Five pages finish in ≤ 10 minutes wall time on hosted runners. — **verified** (asserted on the Linux runner of the self-test).
4. A second push updates the same comment. — **open**: needs the sample repository.
5. Fork PRs get the comment through the `workflow_run` path without secrets. — **open**: the workflows are written, the path has not run.

## Decisions pending review

- The first release is a candidate, not `v1` (2026-11-18): the tag is `v0.1.0-rc.1` and the Action's `version` default follows it. I am not comfortable calling it `v1` yet — the Safari lane, the crawl and the model's role are still open, and a `v1` tag would promise a stable surface I would then have to defend. The cost of a candidate is that consumers pin an rc that will be superseded. The candidate in use is `v0.2.0-rc.3` (2026-11-27), the candidate for `v0.2.0`. It is rc.2's code with two fixes found on omnisnap.org after rc.2's release re-run: a capture whose browser painted none of the page's text (Safari 27.2 on an older macOS runner image) now fails instead of being compared, and the self-tests fail on it (backlog B17); and the report no longer prints a literal `null` in its panes and under the summary (backlog B18). Both are Go or JavaScript changes, which CONVENTIONS freezes between a candidate and its stable release, so the fix is a new candidate (owner's call, ZANZIBAR-58). Before it, `v0.2.0-rc.2` (2026-11-27). It is rc.1's code with two fixes to the Windows network install test, which failed twice on rc.1 (run 46008712513) for reasons outside the install: the runner's own Firefox wrote its registry keys mid-test, and the uninstall ran while the closed browser still held `firefox.exe`, which `Uninstall` now retries. CONVENTIONS freezes Go between a candidate and its stable release, so the fix is a new candidate (owner's call, ZANZIBAR-47). Before it, `v0.2.0-rc.1` (2026-11-26), the first candidate for the first stable release, `v0.2.0` (change `release-v0-2-0`): selectors follow the DOM and are escaped CSS, an element inside a shadow root is named by its host, at most three browsers run at once (`--max-browsers`; the Action passes 4, so macOS captures in one group), the cached Firefox is found on Windows, and the release run calls the self-tests. npm and PyPI publish it as `latest`, since no stable release exists yet. Before it, `v0.1.0-rc.7` (2026-11-25). It has no engine changes from rc.6: it exists because the `v0.1.0-rc.6` tag was cut on the merge of the comment change, before its version bump was merged, so `report.yml@v0.1.0-rc.6` and `action@v0.1.0-rc.6` default `version` to `v0.1.0-rc.5` and download the rc.5 binary (the old comment), and the tag's npm and PyPI files still say rc.5. The rc.6 binary is correct: passing `version: v0.1.0-rc.6` uses it. The tag was not moved (the Go module proxy may have cached it); rc.7 carries the correct defaults, and it is the first release that deploys omnisnap.org (`SPEC-website.md`). npm and PyPI skip rc.6. Before it, `v0.1.0-rc.6` (2026-11-24), the first with the readable PR comment (a verdict, one row per page, each issue once). Before that, `v0.1.0-rc.5` (2026-11-20), the first with the cached, time-bounded Linux setup (backlog B1). Before that, `v0.1.0-rc.4` (2026-11-20). `v0.1.0-rc.1` was withdrawn and is retracted in `go.mod`. So is `v0.1.0-rc.3`: it was tagged on a commit from before the `report.yml` fixes, so consumers pinned to it fail, and Go's module proxy had already cached it, so moving the tag would have broken `go` checksums for anyone who fetched it; `v0.1.0-rc.4` is the same code with the fixes. Its tag was cut before the version defaults were bumped, so `report.yml@v0.1.0-rc.4` and `action@v0.1.0-rc.4` still default `version` to `v0.1.0-rc.2`; that binary is the same capture code (no non-test Go file changed between the two tags), and passing `version: v0.1.0-rc.4` uses the matching one.
- The macOS job runs on an input: `every` (the default), `labelled` (Safari only on PRs labelled `omnisnap:safari`) and `never` (2026-11-17; default changed 2026-11-19). The default was `labelled` while the account had no macOS runners and hosted ones cost about ten times an Ubuntu job; with the runners available, Safari is on by default, because a report without Safari is missing the engine most of the users are on. `labelled` is one word away if the minutes matter more.
- The self-tests are called from `release.yml` (2026-11-25, backlog B5): their `release` triggers never fired, because GitHub starts no workflow from a release the workflow token created, so `v0.1.0-rc.2`, rc.4 and rc.6 shipped bugs they would have caught. `release.yml` calls both with the tag, as it calls the deploy. npm and PyPI wait for both; the GitHub release cannot (the consumer test downloads its binary), and the website does not (a red run is already the signal not to announce). A personal access token or GitHub App that dispatches them was the alternative: one more secret to manage, rejected for the deploy too. One release run now holds three reports, and artifact names are unique per run, hence `report.yml`'s `artifact-prefix`. Change `run-self-tests-on-release`.
- The `binary` input (2026-11-18): the self-test runs the Action before any release exists, and the same input covers air-gapped setups. A second way to do it (a debug flag on the CLI) would have leaked a test hook into the product.
- `allow-small-pages` (2026-11-18): the 50-element bot-wall rule is a live-page heuristic; planted and lab pages are small by design. It is a public flag for something that is really a test concern, but a flag is visible and a hidden environment variable would be worse.

## Open questions

- Should `artifact-retention-days` be applied by the workflow (it would need the API) or stay a repository setting? The setting is less code; the API is more precise.
