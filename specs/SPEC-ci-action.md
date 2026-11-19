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
    uses: GerMP21/Omnisnap/.github/workflows/report.yml@v1
    with:
      start: npm ci && npm run build && npx serve -l 3000 dist &
      wait-on: http://localhost:3000
      pages: |
        http://localhost:3000/
        http://localhost:3000/pricing
```

A reusable workflow (`report.yml`: the OS jobs + the merge job) plus a composite action (`action/action.yml`) for custom matrices.

### Inputs

Composite action (`action/action.yml`):

| Input | Default | Does |
|---|---|---|
| `version` | `v1` | release tag the binary is downloaded from |
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

The reusable workflow (`report.yml`) adds `macos` (`labelled` | `every` | `never`, default `labelled`) and `artifact-retention-days` (default `7`; the step only prints it — the retention itself is a repository setting).

### Per OS job

1. Download the release binary for the runner and check it against `SHA256SUMS` (skipped when the `binary` input is given).
2. Prepare engines:
   - **Linux:** `apt-get install weston fonts-liberation fonts-noto-mono` (font parity) and the WebKit driver package (`webkitgtk-webdriver`; `webkit2gtk-driver` on older images);
   - **macOS:** `sudo safaridriver --enable` (allowed on hosted runners); Safari is the only WebKit here;
   - **Windows:** Chrome, Edge and Firefox from the runner image.
3. Start the user's app (`start`, under `nohup`) and wait for `wait-on` (120 tries, 1 s apart; a timeout is a warning, not a failure).
4. Write `omnisnap-job.json`, then `omnisnap run omnisnap-job.json --json`; the NDJSON progress lines are tee'd into the log.
5. Upload the bundle as an artifact (`omnisnap-<os>`, plus the progress log and `app.log`).
6. Always exit 0.

**Report-only, exactly.** The capture step runs under `bash -e -o pipefail` (GitHub's default for `shell: bash`), so a non-zero `omnisnap` exit (3: every capture failed) would end the step before its code could be read. The step therefore turns `-e` off around the pipeline, reads `${PIPESTATUS[0]}`, emits a warning and exits 0.

Source: https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax#exit-codes-and-error-action-preference
> "By default, fail-fast behavior is enforced using `set -e` for both `sh` and `bash`. When `shell: bash` is specified, `-o pipefail` is also applied to enforce early exit from pipelines that generate a non-zero exit status."

### The macOS policy

The `macos` input: `labelled` (the default) runs the macOS job only on PRs labelled `omnisnap:safari`; `every` runs it on every PR; `never` drops the job.

The choice is made in `strategy.matrix`, not in the job's `if`: `jobs.<job_id>.if` cannot read the matrix context, so GitHub rejected the whole file when the choice lived there. `strategy` can read `github` and `inputs`, and the matrix is built with `fromJSON(case(...))`.

Source: https://docs.github.com/en/actions/reference/workflows-and-actions/contexts#context-availability
> `jobs.<job_id>.if` → `github, needs, vars, inputs`; `jobs.<job_id>.strategy` → `github, needs, vars, inputs`.

Source: https://docs.github.com/en/actions/reference/workflows-and-actions/expressions#case
> "Evaluates predicates in order and returns the value corresponding to the first predicate that evaluates to `true`. If no predicate matches, it returns the last argument as the default value."

Source: https://docs.github.com/en/actions/reference/workflows-and-actions/expressions#contains
> "Returns `true` if `search` contains `item`. If `search` is an array, this function returns `true` if the `item` is an element of the array."

macOS is out of this repository's own CI matrices (`ci.yml`, `e2e-action.yml`): hosted macOS runners cost about ten times an Ubuntu job and the account could not get them (2026-11-18). `safari-smoke.yml` is the manual path; Tasks 2 and 23 stay open.

### Merge job

1. Download the artifacts (`actions/download-artifact@v8`, pattern `omnisnap-*`).
2. Get the binary: the same release download as the OS jobs (checksum verified) — the merge job runs on the runner, not in the bundle.
3. `omnisnap merge <bundles> --rename-hosts -o merged` (`--rename-hosts` gives a colliding host id a `-2`, `-3` suffix; without it the merge is an error). No bundle at all → a warning and exit 0.
4. `omnisnap comment merged --report-url <run> --run-url <run> > merged/comment.md`.
5. Upload the merged bundle (with `report.html`).
6. Post or update **one** PR comment (same-repo PRs only), identified by the hidden marker `<!-- omnisnap-report -->`: the comment is created if the marker is absent and updated in place otherwise, so a second push never adds a second comment.

Both the OS jobs and the merge job download the binary from the release, so the Action needs a published release; the `binary` input covers only the OS job, and only from inside this repository (the self-test).

The comment carries:

- per page and host: the engine against its baseline, the status and the Report/Info counts;
- up to 3 Report-tier causes per result (title, selector, kinds) and how many more there are;
- **Notes:** engines skipped (`host: engine skipped: reason`) and pages that failed or were unreachable (`host/page: failed`, `host/page: unreachable`);
- links to the merged report and the workflow run;
- the report-only footer, and a truncation at 65,000 characters (GitHub's cap is 65,536) at a line boundary with a pointer to the report.

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

`action/action.yml` (composite) · `action/README.md` · `.github/workflows/report.yml` (reusable) · `.github/workflows/report-comment.yml` (fork-safe reusable) · `.github/workflows/e2e-action.yml` (self-test) · `internal/cli/comment.go` (the `omnisnap comment` renderer: a Go subcommand rather than shell, so it is unit-tested) · `.github/workflow-templates/omnisnap-fork-comment.yml` (the consumer template copy) · `docs/github-action-fork-comments.md` (the guide).

## Code style

YAML: no inline scripts longer than a few lines (logic lives in the binary, e.g. `omnisnap comment`); dependencies pinned to a major version tag (`actions/checkout@v7`, `actions/setup-go@v7`, `actions/download-artifact@v8`, `actions/upload-artifact@v7`, `actions/github-script@v8`). The original rule was a commit SHA — SHA pinning is still open (see Open questions). `ci.yml` still uploads its artifact with `actions/upload-artifact@v4`; align it with the action workflows.

## Testing strategy

- **Self-test (`e2e-action.yml`; manual dispatch and every prerelease):** it builds the binary from the checkout, serves the planted suite with `omnisnap-lab serve` and runs the composite action on `ubuntu-latest` and `windows-latest` with the same five pages. The merge job then asserts that (a) the comment carries the marker, (b) the planted finding `table#target` reaches the comment, and (c) the Notes name the part that failed.
- **Fault injection:** the Windows runner is the failing one — nothing listens on port 8099 there, so every capture fails — and the job must still conclude success. This is success criterion 2, asserted on every run.
- **Wall time:** five pages must finish in < 10 minutes (asserted on the Linux runner).
- **Comment rendering:** unit tests of `omnisnap comment` on merged fixtures (markdown snapshot, truncation at the limit, both argument orders).
- **Smoke (first week of M0, one-pager assumption):** a ten-line workflow proves Safari runs unattended on a macOS runner. Still open: no macOS runner is available.

## Boundaries

- **Always:** exit 0; one comment, updated in place; pin dependencies to a known version.
- **Ask first:** a pass/fail mode (deferred until report-mode noise data exists); extra permissions; self-hosted runner support.
- **Never:** expose secrets to fork code; fail the build; upload captures anywhere but the workflow's own artifacts.

## Success criteria

1. On a sample repository, a PR gets one comment with results from Linux (Chrome, Firefox, WebKitGTK) and Windows (Chrome, Edge, Firefox); with `macos: every` or the `omnisnap:safari` label, macOS (Chrome, Firefox, Safari) joins them. — **open**: the sample repository does not exist yet.
2. With one OS job failing (fault injection), the workflow still concludes success and the comment says which part failed. — **verified** in the self-test (Windows is the injected failure).
3. Five pages finish in ≤ 10 minutes wall time on hosted runners. — **verified** (asserted on the Linux runner of the self-test).
4. A second push updates the same comment. — **open**: needs the sample repository.
5. Fork PRs get the comment through the `workflow_run` path without secrets. — **open**: the workflows are written, the path has not run.

## Decisions

- Each project chooses when the macOS job runs, through an input (2026-11-17; default changed 2026-11-18): `labelled` (the default) runs Safari only on PRs labelled `omnisnap:safari`, `every` opts back into every PR and `never` drops the job, since hosted macOS runners are billed at a higher rate.
- The macOS choice lives in `strategy.matrix` (2026-11-18): `jobs.<job_id>.if` cannot read the matrix context, so GitHub rejected the file.
- The `binary` input (2026-11-18): the self-test runs the Action before any release exists, and the same input covers air-gapped setups.
- `allow-small-pages` (2026-11-18): the 50-element bot-wall rule is a live-page heuristic; planted and lab pages are small by design.
- Report-only is implemented, not assumed: the capture step reads the binary's exit code and ends with 0 (2026-11-18).

## Open questions

- Comment content when 0 causes everywhere: a short "no differences" line, or no comment?
- Should the dependencies be pinned by commit SHA (the original rule) instead of a major version tag?
- Should `artifact-retention-days` be applied by the workflow (it would need the API) or stay a repository setting?
- Should `ci.yml` move to `actions/upload-artifact@v7` (the version the Action workflows use)?
