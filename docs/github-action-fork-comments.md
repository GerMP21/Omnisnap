# Comments on pull requests from forks

`report.yml` posts its comment from the same job that renders it. That works
for pull requests opened from a branch in your repository, and **not** for
pull requests from forks: GitHub gives a `pull_request` run started by a fork a
read-only `GITHUB_TOKEN`, so the comment step fails or is skipped.

The fix is a second workflow triggered by `workflow_run`. It runs in your
repository, with your token, after the report finishes — and it never checks
out or executes the fork's code.

This page explains the mechanism, the steps, and what to check when no comment
appears. For the one-paragraph version, see
[`action/README.md`](../action/README.md#pull-requests-from-forks).

## How it works

```
fork pushes  ──►  pull_request run (read-only token)
                    └─ calls report.yml  ──►  captures, merge, uploads
                                                  │
                                                  ▼  workflow_run: completed
                              fork-comment.yml (your default branch)
                                └─ calls report-comment.yml: downloads the
                                   artifact, upserts the marked comment
```

Three rules come from GitHub's documentation and shape the design:

1. **The follow-up must exist on the default branch.** *"This event will only
   trigger a workflow run if the workflow file exists on the default branch."*
   ([workflow_run](https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows#workflow_run))
2. **The follow-up may write even though the PR run could not.** A
   `workflow_run` workflow *"is able to access secrets and write tokens, even if
   the previous workflow was not."* That is exactly why it must not run
   untrusted code: no `actions/checkout`, no `uses: ./action`, no scripts from
   the repository under test.
3. **At most three levels of chaining.** *"You can't use `workflow_run` to
   chain together more than three levels of workflows."* The caller → report →
   comment chain is the last hop that can run.

The artifact is fetched from a different run, so the download needs both
`run-id` and `github-token`
([download-artifact](https://github.com/actions/download-artifact#download-artifacts-from-other-workflow-runs-or-repositories)).

## Before you start

- The workflow in your repository that calls `report.yml` has a `name:` — the
  `workflow_run.workflows` list matches **workflow names**, not file names. The
  README example names it `Omnisnap report`.
- You can push to the default branch (that is where this file has to live).

## Steps

1. Copy both [`.github/workflow-templates/omnisnap-fork-comment.yml`](../.github/workflow-templates/omnisnap-fork-comment.yml)
   and its `.properties.json` (the metadata only matters for the template
   gallery; the `name`/`description` are harmless in a plain repository).
2. Put the `.yml` at `.github/workflows/fork-comment.yml` in your repository,
   on the default branch.
3. Edit `workflows: ["Omnisnap report"]` to match your caller workflow's name.
4. Commit and push. Nothing runs until the next `workflow_run` completes.

The workflow as shipped:

```yaml
name: Omnisnap fork comment

on:
  workflow_run:
    workflows: ["Omnisnap report"]
    types: [completed]

permissions:
  actions: read
  pull-requests: write

jobs:
  comment:
    if: |
      github.event.workflow_run.event == 'pull_request' &&
      github.event.workflow_run.head_repository.full_name != github.repository
    permissions:
      actions: read
      pull-requests: write
    uses: GerMP21/Omnisnap/.github/workflows/report-comment.yml@v1
    with:
      run-id: ${{ github.event.workflow_run.id }}
      branch: ${{ github.event.workflow_run.head_branch }}
```

### Permissions

| Scope | Why |
|---|---|
| `actions: read` | download the `omnisnap-merged` artifact from the report run |
| `pull-requests: write` | create or update the comment |
| `contents: read` (implicit to the token) | none of the steps read the repository; no checkout happens |

A reusable workflow cannot raise the permissions of its caller, so the calling
job grants them explicitly.

### Why the guard checks `head_repository`

Same-repository pull requests are already commented by `report.yml` itself.
The `head_repository.full_name != github.repository` condition keeps the
follow-up to forks, so the two paths never race for the same comment. Both
paths upsert the same hidden marker (`<!-- omnisnap-report -->`), so an
accidental double run would update one comment, not create two.

## What the called workflow does

[`.github/workflows/report-comment.yml`](../.github/workflows/report-comment.yml)
is a reusable workflow (`on: workflow_call`) with four inputs: `run-id` (the
report run to download), `pr` (number, optional), `branch` (used to find the
PR when `pr` is empty) and `artifact` (default `omnisnap-merged`). It:

1. downloads `omnisnap-merged` with `continue-on-error`, so a report that
   failed before uploading does not fail the follow-up;
2. stops quietly when `merged/summary.json` is absent;
3. renders `merged/comment.md` when present, otherwise a one-line note with a
   link to the run;
4. finds the open PR by number or by head branch, then creates or updates the
   comment carrying the marker.

## Troubleshooting

| Symptom | Likely cause |
|---|---|
| Nothing runs after the report | The file is not on the default branch, or `workflows:` does not match the caller's `name:` exactly (case and spacing matter). |
| The follow-up runs but skips | The report run was not from a `pull_request` event (e.g. a push), or the head repository is this repository. |
| No comment, job green | `merged/summary.json` was missing: the merge job had no bundle, or the report failed earlier. Check the report run's artifacts. |
| `Resource not accessible by integration` | The calling job lacks `pull-requests: write` (a fork PR's own token cannot grant it; that is what this workflow is for). |
| Comment on the wrong PR | Two open PRs share the head branch name. Pass the number explicitly: the reusable workflow accepts `pr` as well as `branch`. |

## About `.github/workflow-templates/`

GitHub's template gallery reads `workflow-templates/` from an **organization's**
`.github` repository, with a `.properties.json` per template
([creating workflow templates](https://docs.github.com/en/actions/how-tos/reuse-automations/create-workflow-templates)).
Inside this project repository the folder is an inert, versioned copy: the
files are not executed (only `.github/workflows/` is), and the gallery does not
read them. If Omnisnap later gets an org-level `.github` repository, the same
two files work there unchanged.
