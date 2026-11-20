# Omnisnap Action

Report-only cross-engine layout comparison on GitHub runners. One reusable
workflow (one job per OS — Linux, Windows and macOS, with Safari — plus the merged
report and one PR comment) and this composite action for custom matrices.

> **Known issue: slow Linux setup (2026-11-20).** The Ubuntu job installs
> WebKitGTK, Weston and the parity fonts from the Ubuntu package mirror, which
> can be slow depending on demand (on 2026-11-20 it served about 50 KB/s for
> hours). With `v0.1.0-rc.4` and earlier the
> download has no time limit of its own: it can take 15 minutes or more, or run
> into GitHub's 30-minute job limit, and then the report has no Linux host
> (Windows and macOS still report). On `main` the Action caches the packages
> per runner image and stops downloading after `setup-timeout` (5 minutes),
> then captures with Chrome and Firefox; this ships in the next release. Until
> then, re-run the cancelled Ubuntu job: the comment is updated in place.

**Setting it up in your repository:** [docs/github-action-setup.md](../docs/github-action-setup.md)
— the workflow file, the `start` command per stack, the pages list, and the
fork path.

```yaml
on: pull_request
jobs:
  omnisnap:
    uses: GerMP21/Omnisnap/.github/workflows/report.yml@v0.1.0-rc.4
    with:
      start: npm ci && npm run build && npx serve -l 3000 dist
      wait-on: http://localhost:3000
      pages: |
        http://localhost:3000/
        http://localhost:3000/pricing
```

The job never fails the build. Before a release exists, `binary` points the
action at an executable you built yourself (this is how `e2e-action.yml` tests
it), and `allow-small-pages` drops the under-50-elements bot-wall rule that the
planted pages trip.

On Linux the action installs the parity fonts, plus Weston and the WebKitGTK
driver when WebKitGTK is wanted (`engines` empty, or naming `webkitgtk`). The
downloaded packages are cached per runner image with `actions/cache`: an exact
hit installs them with no download at all, and a miss downloads them under
`setup-timeout` (default 300 s). Past that the step warns, and the capture goes
on with the engines the image ships (Chrome and Firefox). The cache belongs to
your repository: a pull request's first run downloads the packages and its
later pushes reuse them, and pull requests can also read the caches of the
default branch. The logic is in [`linux-engines.sh`](linux-engines.sh)
(unreleased: on `main` only).

For your own matrix, use the composite action directly — it is referenced by
path, because the repository root has no `action.yml`:

```yaml
- uses: GerMP21/Omnisnap/action@v0.1.0-rc.4
  with:
    version: v0.1.0-rc.4
    pages: |
      http://localhost:3000/
```

 Fork PRs get their comment from a `workflow_run`
follow-up that never runs fork code (see [Pull requests from forks](#pull-requests-from-forks)). The macOS job
carries Safari and is on by default (`macos: every`); `labelled` runs it only
on PRs labelled `omnisnap:safari` and `never` drops it.

## Pull requests from forks

A `pull_request` run started by a fork gets a read-only `GITHUB_TOKEN`, so it
cannot post its comment. `report.yml` skips its own comment step when the PR
head is another repository, and the comment comes from a `workflow_run`
follow-up that never checks out or executes fork code.

The follow-up is
[`.github/workflow-templates/omnisnap-fork-comment.yml`](../.github/workflow-templates/omnisnap-fork-comment.yml).
Copy it to `.github/workflows/` in your repository, on the **default branch**
(GitHub only fires `workflow_run` for a file that exists there), and set
`workflows:` to the `name:` of the workflow that calls `report.yml`.

Full explanation — permissions, the three-level chaining limit, the cross-run
artifact download, troubleshooting:
[docs/github-action-fork-comments.md](../docs/github-action-fork-comments.md).

Note that GitHub's template gallery reads `workflow-templates/` from an
organization's `.github` repository; in this project repository the folder is
an inert, versioned copy.
