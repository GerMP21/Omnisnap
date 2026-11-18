# Omnisnap Action

Report-only cross-engine layout comparison on GitHub runners. One reusable
workflow (Linux, Windows, macOS, merged report, one PR comment) and this
composite action for custom matrices.

```yaml
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

The job never fails the build. Fork PRs get their comment from a `workflow_run`
follow-up that never runs fork code (see [Pull requests from forks](#pull-requests-from-forks)). macOS runs on every PR by default; pass
`macos: labelled` to run Safari only on PRs labelled `omnisnap:safari`.

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
