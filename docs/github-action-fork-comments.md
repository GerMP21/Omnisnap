# Comments on pull requests from forks

The report itself works on a fork pull request; only the comment cannot be
posted, because GitHub gives that run a read-only token. A second workflow in
the base repository, triggered by `workflow_run`, picks the report up from the
artifacts and posts it — without ever executing the fork's code.

- Setup in one screen (the file to copy, the one line to change):
  [github-action-setup.md](github-action-setup.md#comments-on-pull-requests-from-forks)
- What the Action does and guarantees: [`../specs/SPEC-ci-action.md`](../specs/SPEC-ci-action.md)

---

## 1. The mechanism

```
fork pushes a branch
        │
        ▼
pull_request run  (read-only token)          ← "Omnisnap report"
   └─ calls report.yml
        ├─ capture (ubuntu-latest)   ──► artifact omnisnap-Linux
        ├─ capture (windows-latest)  ──► artifact omnisnap-Windows
        └─ merge                     ──► artifact omnisnap-merged
                                        (the comment step fails here: read-only)
        │
        │  workflow_run: completed
        ▼
"Omnisnap fork comment" run  (write token)   ← the follow-up
   └─ calls report-comment.yml
        ├─ downloads omnisnap-merged from the previous run
        ├─ renders or reuses merged/comment.md
        └─ creates or updates the marked comment on the pull request
```

The split matters: the capture has to run the contributor's code (it renders
their pages), and the comment needs write access. Keeping them in separate
runs means the untrusted code only ever sees a read-only token, while the
privileged run never sees the untrusted code.

---

## 2. Why the comment does not appear on a fork pull request

> "With the exception of `GITHUB_TOKEN`, secrets are not passed to the runner
> when a workflow is triggered from a forked repository. **The `GITHUB_TOKEN`
> has read-only permissions in pull requests from forked repositories.**"
> — [Events that trigger workflows](https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows#pull_request)

Posting a comment is a write. The capture jobs are unaffected — they read the
code and write into their own workspace — but the merge job's comment step
either fails with `Resource not accessible by integration` or is skipped, and
the run ends without a comment.

The report is not lost: the merge job uploaded `omnisnap-merged` before the
comment step. A later run can download that artifact and post it. That is the
whole trick.

---

## 3. The rules this design rests on

All four are GitHub's documented behaviour, from the
[workflow_run reference](https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows#workflow_run).

**The follow-up must exist on the default branch.**

> "This event will only trigger a workflow run if the workflow file exists on
> the default branch."

A commit on a feature branch does not enable it.

**The follow-up may write even though the pull request run could not.**

> "The workflow started by the `workflow_run` event is able to access secrets
> and write tokens, even if the previous workflow was not."

**At most three levels of chaining.**

> "You can't use `workflow_run` to chain together more than three levels of
> workflows."

The pull request workflow → `report.yml` → the follow-up is the last hop that
can exist.

**The follow-up must not check out the fork's code.**

> "The `pull_request_target` and `workflow_run` workflow triggers, when used
> with the checkout of an untrusted pull request, expose the repository to
> security compromises. These workflows are privileged, which means they share
> the same cache of the main branch with other privileged workflow triggers,
> and may have repository write access and access to referenced secrets."
> — [Security hardening for GitHub Actions](https://docs.github.com/en/actions/security-for-github-actions/security-guides/security-hardening-for-github-actions#mitigating-the-risks-of-untrusted-code-checkout)

So the follow-up does no `actions/checkout`, runs no script from the
repository, and reads only the artifact the previous run uploaded.

---

## 4. The file, line by line

The template ships in the Omnisnap repository at
`.github/workflow-templates/omnisnap-fork-comment.yml`. Copy it to
`.github/workflows/fork-comment.yml` in the base repository.

```yaml
name: Omnisnap fork comment

on:
  workflow_run:                      # run me when another workflow finished
    workflows: ["Omnisnap report"]   # ← the NAME of the workflow that calls report.yml
    types: [completed]               # …when it has completed (successfully or not)

permissions:                         # what this run's token may do
  actions: read                      #   → read another run's artifacts
  pull-requests: write               #   → create or edit a comment

jobs:
  comment:
    if: |                            # run this job only for fork pull requests
      github.event.workflow_run.event == 'pull_request' &&
      github.event.workflow_run.head_repository.full_name != github.repository
    permissions:
      actions: read
      pull-requests: write
    uses: GerMP21/Omnisnap/.github/workflows/report-comment.yml@v0.2.0-rc.3
    with:
      run-id: ${{ github.event.workflow_run.id }}
      branch: ${{ github.event.workflow_run.head_branch }}
```

### 4.1 `workflows: [...]` takes names, not file names

It must equal the `name:` of the workflow **in the base repository** that
calls `report.yml`:

```yaml
# .github/workflows/omnisnap.yml  (base repository)
name: Omnisnap report          # ← this exact string
on: pull_request
jobs:
  omnisnap:
    uses: GerMP21/Omnisnap/.github/workflows/report.yml@v0.2.0-rc.3
```

Case and spacing count. If the caller is named `Layout check`, the list
becomes `["Layout check"]`.

### 4.2 The `if:` guard

| Condition | Effect |
|---|---|
| `github.event.workflow_run.event == 'pull_request'` | ignores runs triggered by a push |
| `github.event.workflow_run.head_repository.full_name != github.repository` | ignores same-repository pull requests, which `report.yml` already comments |

Both paths upsert the same marker, so even a double run would update one
comment rather than create two.

### 4.3 The inputs

| Input | Value | Used for |
|---|---|---|
| `run-id` | the finished report run's id | downloading `omnisnap-merged` **from another run**, which needs both the run id and a token (`actions/download-artifact@v8` with `run-id` + `github-token`) |
| `branch` | the pull request's head branch | finding the pull request number when `pr` is not given |
| `pr` | *(not passed)* | the pull request number, for when two open pull requests share a branch name |
| `artifact` | `omnisnap-merged` (default) | the artifact to download |

### 4.4 What the called workflow does

`.github/workflows/report-comment.yml` (in the Omnisnap repository):

1. downloads `omnisnap-merged` with `continue-on-error: true`, so a report run
   that died before uploading does not fail the follow-up;
2. stops quietly when `merged/summary.json` is missing — nothing to report;
3. renders the comment from `merged/comment.md`, or writes a one-line note
   linking to the run when the file is absent;
4. finds the pull request by number or by head branch among the open ones;
5. upserts the comment: it lists the comments, looks for the marker
   `<!-- omnisnap-report -->`, and updates that one or creates it.

---

## 5. Setup

**Fetch the template.**

```sh
mkdir -p .github/workflows
curl -fsSL https://raw.githubusercontent.com/GerMP21/Omnisnap/v0.2.0-rc.3/.github/workflow-templates/omnisnap-fork-comment.yml \
  -o .github/workflows/fork-comment.yml
```

The `.properties.json` beside it in the template folder is metadata for
GitHub's template gallery and is not needed here.

**Check the one name** — `workflows: ["Omnisnap report"]` against the `name:`
of the caller in your repository.

**Push it to the default branch.**

```sh
git add .github/workflows/fork-comment.yml
git commit -m "Add the fork comment workflow"
git push origin main
```

**Confirm GitHub sees it:** the workflow must appear in the Actions tab's
workflow list and in the code of the default branch.

---

## 6. Test

```sh
git clone https://github.com/<you>/<your-repo>.git
cd <your-repo>
git checkout -b test/omnisnap-fork
```

Make a change that is visible in the rendering (a size, a colour, a margin) —
the comment is about per-engine rendering differences, so a visual change
makes the test meaningful. Push it, then open the pull request **from the
fork to the original repository**.

Two runs appear:

1. **Omnisnap report** — the capture jobs run; the comment step of the merge
   job fails or is skipped, which is expected. On a first-time contributor's
   pull request GitHub may require a maintainer to approve the run.
2. **Omnisnap fork comment** — downloads `omnisnap-merged` and posts the
   comment.

Push another commit to the same branch: the comment is updated in place, not
duplicated.

---

## 7. Troubleshooting

| Symptom | Cause | How to check | Fix |
|---|---|---|---|
| The follow-up never appears in the Actions tab | the file is not on the default branch | open it on GitHub at the default branch | push it to `main` |
| The follow-up runs and skips | the guard: not a pull request, or the head is in the base repository | the job log | expected for pushes and same-repository pull requests |
| The follow-up runs, no comment, job green | `merged/summary.json` missing: the merge job had no bundle, or an earlier job failed | the report run's artifacts | fix the capture first |
| `Resource not accessible by integration` | the follow-up lost `pull-requests: write` | the `permissions:` block | restore it from the template |
| Comment on the wrong pull request | two open pull requests share the head branch name | compare branch names | pass `pr:` explicitly |
| The report run failed at the first step | the release assets are missing (the binary is downloaded from the release) | the first step of the capture job | publish the release, or check `version:` |
| The comment is empty | no page was captured | `omnisnap-merged` → `summary.json` | the comment's Not checked table says why |

---

## 8. Why not `pull_request_target`

> "This event runs in the context of the default branch of the base repository,
> rather than in the context of the merge commit, as the `pull_request` event
> does. This prevents execution of unsafe code from the head of the pull
> request… **Avoid using this event if you need to build or run code from the
> pull request.**"
> — [Events that trigger workflows](https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows#pull_request_target)

`pull_request_target` runs with a write token *and* the default branch's code.
Combining it with the need to build the contributor's pages is the "pwn
request" pattern: the contributor's code would run with write access. The
split above avoids it — the capture keeps the read-only token and no secrets,
the comment never touches the fork's code.

The cost is one file and one extra run.

---

## 9. Design notes

- **The capture runs the fork's code; the comment does not.** The capture has
  to (it renders the pages) and runs with a read-only token and no secrets.
- **Same-repository pull requests are commented by `report.yml` itself**, and
  the guard keeps the follow-up away from them.
- **Runner minutes:** each fork pull request costs two capture jobs plus the
  merge. Restrict the report workflow if the repository takes many fork pull
  requests, or keep the macOS lane off with the `macos` input.
- **Private repositories:** the mechanism is the same, but forks of a private
  repository are only possible for collaborators, so the fork path is rare
  there.
- **The follow-up cannot be tested without a fork** — the point is a pull
  request whose head is in another repository. What can be tested without one
  is that the guard skips a same-repository pull request.

---

## 10. Checklist

- [ ] `.github/workflows/fork-comment.yml` exists in the base repository, on the default branch.
- [ ] Its `workflows:` list matches the `name:` of the workflow that calls `report.yml`.
- [ ] Its `permissions:` block still has `actions: read` and `pull-requests: write`.
- [ ] It contains no `actions/checkout` and runs no script from the repository.
- [ ] A pull request from a fork produced two runs and one comment.
- [ ] A second push to that branch updated the comment instead of adding another.

---

## 11. Where things live

| What | Where |
|---|---|
| The template to copy | [`.github/workflow-templates/omnisnap-fork-comment.yml`](../.github/workflow-templates/omnisnap-fork-comment.yml) |
| The reusable workflow it calls | [`.github/workflows/report-comment.yml`](../.github/workflows/report-comment.yml) |
| The reusable workflow that produces the report | [`.github/workflows/report.yml`](../.github/workflows/report.yml) |
| The spec | [`../specs/SPEC-ci-action-fork-template.md`](../specs/SPEC-ci-action-fork-template.md) |
| The Action's own spec | [`../specs/SPEC-ci-action.md`](../specs/SPEC-ci-action.md) |
| GitHub's rules | [Events that trigger workflows](https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows#workflow_run) |
