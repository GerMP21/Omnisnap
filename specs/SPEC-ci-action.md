# Spec: ci-action

Module id: `ci-action` · Location: `action/` · Depends on: `cli`, `merge` · Milestone: M2 (v1 = M2 released) · Conventions: [CONVENTIONS.md](CONVENTIONS.md)

## Objective

A GitHub Action that runs Omnisnap on Ubuntu, Windows and macOS runners for every pull request and posts **one** merged comment with the cross-engine differences, real Safari included, so developers on Windows and Linux get Safari results without owning a Mac. **Report-only: it never fails the build** (one wrong failure gets a visual gate uninstalled; the gate waits for report-mode noise data).

Success metric (one-pager): the Action stays installed on ≥ 5 outside repositories for a month.

## Behaviour

### Usage

```yaml
# .github/workflows/omnisnap.yml
on: pull_request
jobs:
  omnisnap:
    uses: omnisnap/omnisnap/.github/workflows/report.yml@v1
    with:
      start: npm ci && npm run build && npx serve -l 3000 dist &
      wait-on: http://localhost:3000
      pages: |
        http://localhost:3000/
        http://localhost:3000/pricing
```

A reusable workflow (three OS jobs + a merge job) plus a composite action `omnisnap/omnisnap@v1` for custom matrices.

### Per OS job

1. Download the release binary for the runner (checksum verified); cache it.
2. Prepare engines:
   - **Linux:** install fonts (Liberation, Noto, DejaVu) for font parity; WebKitGTK (`webkit2gtk` driver package) and Weston or Xvfb;
   - **macOS:** `sudo safaridriver --enable` (allowed on hosted runners); Safari is the only WebKit here;
   - **Windows:** Chrome, Edge and Firefox from the runner image.
3. Start the user's app (`start`), wait for `wait-on`.
4. `omnisnap run job.json --json --out omnisnap-<os>`; progress lines become log groups.
5. Upload the bundle as an artifact (retention configurable, default 7 days).
6. Always exit 0 (failures are reported, not raised).

### Merge job

Download the three artifacts, `omnisnap merge`, upload the merged bundle (with `report.html`), and post or update **one** PR comment (identified by a hidden marker; never a second comment):

- per page: Report causes per engine and host, top 3 causes with title, engine and selector;
- links: the merged report artifact, each OS job;
- notes: engines skipped (e.g. Safari unavailable), pages unreachable, runs that failed.

### Permissions and forks

`pull-requests: write` and `contents: read` only. Pull requests from forks get a read-only token: the comment is posted by a `workflow_run` follow-up workflow that reads the artifact (documented pattern), never with secrets exposed to fork code.

## Commands

```
act -j omnisnap                                   # local dry run where possible (Linux job only)
gh workflow run e2e.yml                           # the repo's own e2e: runs the Action on the planted suite on three runners
```

## Project structure

`action/action.yml` (composite), `.github/workflows/report.yml` (reusable), `action/scripts/` (comment rendering: small Go subcommand `omnisnap comment <merged>` rather than shell, so it's tested), `.github/workflows/e2e.yml` (self-test).

## Code style

YAML: pinned action versions by commit SHA; no inline scripts longer than a few lines (logic lives in the binary, e.g. `omnisnap comment`).

## Testing strategy

- **Self-test on every release candidate:** the Action runs against the planted suite served from the runner on all three OSes; asserts the merged comment contains the expected planted findings and that the job conclusion is success even when one OS job's capture fails (fault-injected).
- **Comment rendering:** unit tests of `omnisnap comment` on merged fixtures (markdown snapshot, length limits: GitHub's 65,536-character cap → truncation with a link).
- **Smoke (first week of M0, one-pager assumption):** a ten-line workflow proves Safari runs unattended on a macOS runner.

## Boundaries

- **Always:** exit 0; one comment, updated in place; pin dependencies by SHA.
- **Ask first:** a pass/fail mode (deferred until report-mode noise data exists); extra permissions; self-hosted runner support.
- **Never:** expose secrets to fork code; fail the build; upload captures anywhere but the workflow's own artifacts.

## Success criteria

1. On a sample repository, a PR gets one comment with results from Linux (Chrome, Firefox, WebKitGTK), Windows (Chrome, Edge, Firefox) and macOS (Chrome, Firefox, Safari).
2. With one OS job failing (fault injection), the workflow still concludes success and the comment says which part failed.
3. Five pages finish in ≤ 10 minutes wall time on hosted runners.
4. A second push updates the same comment.
5. Fork PRs get the comment through the `workflow_run` path without secrets.

## Open questions

- Hosted macOS runners are billed at a higher rate: run macOS only on PRs labeled `omnisnap:safari`, or always?
- Comment content when 0 causes everywhere: a short "no differences" line, or no comment?
