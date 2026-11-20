# Omnisnap Action

Report-only cross-engine layout comparison on GitHub runners. One reusable
workflow (one job per OS — Linux, Windows and macOS, with Safari — plus the merged
report and one PR comment) and this composite action for custom matrices.

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
