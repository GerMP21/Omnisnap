# Design

## Context

See proposal.md for the motivation and specs/release-verification/spec.md for the required behavior. What shapes the approach:

- **`release.yml` today.**
  - Runs on a `v*` tag push, or by hand with a `tag` input.
  - Job `release` tests, builds with `tools/release --stage`, publishes the GitHub release with `github.token`, and checks the assets download.
  - Then two jobs run side by side, both `needs: release`. `deploy` calls `./.github/workflows/deploy.yml`, granting `contents: read` and `pull-requests: write`, with `secrets: inherit`. `packages` publishes npm and PyPI from the `staged-binaries` artifact.
- **Workflow-token events start nothing.** GitHub starts no workflow from events caused by `GITHUB_TOKEN`, except `workflow_dispatch` and `repository_dispatch`. So the `on: release` triggers in `e2e-action.yml` and `e2e-report.yml` never fire for a release this workflow creates. The deploy hit the same rule and was fixed by calling it (#15, rc.7).
  Source: https://docs.github.com/en/actions/security-for-github-actions/security-guides/automatic-token-authentication#using-the-github_token-in-a-workflow
- **`e2e-action.yml`.**
  - It checks out with no `ref` and builds `./cmd/omnisnap`.
  - It runs `uses: ./action` with `binary:` on `ubuntu-latest`, `windows-latest` and `macos-latest`. Windows is the fault-injection runner.
  - It uploads `omnisnap-<os>`. Its `merge` job downloads the pattern `omnisnap-*`, asserts, and uploads `omnisnap-merged`.
  - It declares `contents: read`.
- **`e2e-report.yml`.**
  - A `pick` job chooses the binary: the release's tag on a `release` event, otherwise the newest release with `SHA256SUMS`.
  - It then calls `./.github/workflows/report.yml` with two planted pages served by `python -m http.server` from the caller's checkout, and `assert` downloads `omnisnap-merged`.
  - It declares `contents: read` and `pull-requests: write`, because `report.yml` declares both.
- **`report.yml`.**
  - It names capture artifacts `omnisnap-<matrix.os>` (passed to the action as `artifact-name`). The merge job downloads `omnisnap-*` and uploads `omnisnap-merged`.
  - It checks the action out at `job.workflow_sha`.
  - `macos` defaults to `every`, so a call without it includes macOS.
- **Artifact names are unique per workflow run** (upload-artifact v4). Reusable workflows called from one run share that run's artifacts. `deploy.yml` already calls `report.yml` inside every release run, and its `site` job downloads `omnisnap-merged` by name.
  Source: https://github.com/actions/upload-artifact#breaking-changes ("Uploading to the same named Artifact multiple times")
- **A local `uses: ./.github/workflows/x.yml` is read from the caller's commit.** On a tag push that is the tag. On a manual dispatch it is the ref the run was started from, which may differ from the `tag` input.
  Source: https://docs.github.com/en/actions/sharing-automations/reusing-workflows#calling-a-reusable-workflow

## Goals / Non-Goals

**Goals:**
- The two existing self-tests run inside every release run, against the tag, with no new secret.
- npm and PyPI are published only after both pass.
- Each self-test still works standalone (dispatch, and pull requests for `e2e-report.yml`), with the same assertions as today.

**Non-Goals:**
- New assertions in the self-tests. They test what they test today.
- Holding back the GitHub release or the website.
- Changing what `report.yml` does for consumers who do not set the new input.

## Decisions

### 1. Call the self-tests from `release.yml`, as the deploy is

```
 tag push v*  (or dispatch with tag)
     |
     +--> release (test, build, gh release create, assets ok)
     |       |
     |       +--> deploy      (uses deploy.yml)          [unchanged]
     |       |
     |       +--> e2e-report  (uses e2e-report.yml, tag)
     |                 |
     +--> e2e-action  (uses e2e-action.yml, tag)  -----+
                       |                               |
                       +-------------+-----------------+
                                     v
                     packages  needs: [release, e2e-action, e2e-report]
```

- **`e2e-action`** has no `needs`. It builds its own binary from the tag, so it starts with `release` and is usually done before it.
- **`e2e-report`** `needs: release`. It downloads the published binary, so the assets must be up and verified first.
- **`packages`** keeps its default `if: success()`. A failed or cancelled self-test skips it, and the run concludes failed.
- **`deploy`** keeps `needs: release` only (owner's decision).

*Alternatives:* a PAT or GitHub App token that dispatches the self-tests (backlog approach 2). That adds a secret to manage, and the same option was rejected for the deploy. Gating the GitHub release itself (approach 3) would need a draft release or the `binary` input in `report.yml`'s merge job, which only works inside this repository. The registries are the part consumers install from without thinking, so they are what is held back.

### 2. A `tag` input on both self-tests

Both gain `on: workflow_call` with `inputs.tag` (string, default `""`). `workflow_dispatch` gets the same optional input.

- **`e2e-action.yml`:** `actions/checkout` uses `ref: ${{ inputs.tag || github.ref }}` in both jobs, so it builds and runs the tag's action and binary also when `release.yml` was dispatched from another branch. The `merge` job builds the binary from the same ref.
- **`e2e-report.yml`:** `pick` uses `inputs.tag` when set and the newest release otherwise (dispatch without a tag, pull requests). With a tag, `pick` checks that the release has a `SHA256SUMS` asset and fails with `::error::<tag> has no SHA256SUMS` otherwise, so a missing asset is a clear failure, not a 404 deep in a capture job. The `github.event_name == release` branch goes with the trigger.
- `release.yml` passes `tag: ${{ github.event.inputs.tag || github.ref_name }}`, the same expression its other jobs use.

### 3. What a manual re-run tests

On a tag push, every file involved comes from the tag. On a dispatch, the called workflow files (`e2e-report.yml`, `report.yml`) come from the ref the run was started from. The binary, the checked-out action in `e2e-action.yml` and the planted pages come from the tag. `report.yml`'s action checkout uses `job.workflow_sha`, so it follows the workflow files.

Decision: keep that behavior and document it. Release-Process says to start a re-run with "Use workflow from" set to the tag itself, which makes every part the tag's. `release.yml` adds a step to the `release` job that prints `::warning::` when `github.event_name == workflow_dispatch` and `github.ref_name != TAG` ("the workflows under test are from <ref>, not <tag>; start the run from the tag to test the tag's report.yml").

*Alternative:* call `GerMP21/Omnisnap/.github/workflows/e2e-report.yml@<tag>`. Rejected: `uses:` cannot take an expression, so the ref cannot be the input.

### 4. Artifact names: a prefix on `report.yml`, fixed names in `e2e-action.yml`

One release run can hold three reports: the deploy's demo (`report.yml`), `e2e-report.yml` (`report.yml` again) and `e2e-action.yml`. With today's names all three upload `omnisnap-ubuntu-latest` and so on. The second upload of a name fails, and each merge's `omnisnap-*` pattern would read the others' bundles.

- **`report.yml`** gains `artifact-prefix` (string, default `omnisnap`). Capture artifacts are `<prefix>-<matrix.os>`, and the merge job downloads `<prefix>-*-latest` (only the capture artifacts, never a `-merged`) and uploads `<prefix>-merged`. The default keeps today's names, so `deploy.yml` and consumers need no change.
- **`e2e-report.yml`** passes `artifact-prefix: selftest-report`, and `assert` downloads `selftest-report-merged`.
- **`e2e-action.yml`** uses `selftest-action-<os>`, downloads `selftest-action-*-latest`, and uploads `selftest-action-merged`. It is internal, so no input is needed.
- **No prefix starts with another followed by `-`.** `omnisnap-*` must not match a self-test's artifacts. `selftest-report` and `selftest-action` meet this; the input's description says so.

*Alternative:* serialize the calls. Rejected: artifact names are unique for the whole run, not only while jobs overlap, so serializing does not help. A second alternative, deriving the prefix from `github.job`, is not available inside a called workflow's `with:`.

### 5. Permissions and secrets

- `e2e-report` (job in `release.yml`) grants `contents: read` and `pull-requests: write`. A called workflow cannot be granted less than it declares, and outside a pull request `report.yml` posts no comment.
- `e2e-action` grants `contents: read`.
- Neither gets `secrets:`. They use only `github.token`. `release.yml`'s top level keeps `contents: write` for the `release` job.

### 6. Time and macOS

- Both self-tests keep macOS. `e2e-action.yml` has it in its matrix, and `e2e-report.yml` inherits `report.yml`'s `macos: every`. Safari is the lane most likely to break on a new runner image, and releases are infrequent, so a release is the place to pay for it.
- Job timeouts stay as they are (30 minutes per capture job, 10 for `e2e-action`'s merge, 5 for `assert`). The registries wait for the slower of the two, about 10-20 minutes after `release`.

## Risks / Trade-offs

- [The GitHub release is public before its consumer test passes] → The red run is the signal, and the registries are held back. Release-Process says: on a red self-test, cut the next candidate and do not announce. The tag and release are never touched (spec).
- [Re-running only the failed jobs in the same run may upload an artifact name a previous attempt already used] → Re-runs get a new attempt. If upload-artifact refuses the name in the same run, the uploads gain `overwrite: true`. Check this with one "Re-run failed jobs" on the `v0.2.0-rc.1` run before relying on it (task 4.2).
- [The `-*-latest` download pattern assumes the runner labels end in `-latest`] → `report.yml` sets its own matrix (`ubuntu-latest`, `windows-latest`, `macos-latest`), so the pattern and the matrix live in the same file. A comment there ties them together.
- [The new `report.yml` input is a public surface] → It is optional with a default that keeps today's names, so it is additive. It lands in `v0.2.0`, whose release notes list it.
- [The registries wait longer, and a flaky self-test blocks them] → `packages` can be re-run alone once the self-test passes on a re-run. Its steps already skip versions that are on the registry.

## Migration Plan

1. Land the change on `main` before `v0.2.0-rc.1`.
2. Before tagging, dispatch `e2e-action.yml` and `e2e-report.yml` by hand on the branch with `tag: v0.1.0-rc.7`. This exercises the new input paths without a release. `actionlint` checks the call wiring (inputs and the called files).
3. The real trigger is first exercised by the owner's `v0.2.0-rc.1` tag push. That run must show both self-tests in its graph and `packages` starting after them.
4. **Rollback:** revert the workflow changes. No tag, release or package depends on them.
