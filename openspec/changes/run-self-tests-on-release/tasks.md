# Tasks

## 1. Artifact names that do not collide (`report.yml`)

- [ ] 1.1 Add the `artifact-prefix` input to `.github/workflows/report.yml` (string, default `omnisnap`; its description says that no two calls in one run may use prefixes where one starts with the other followed by `-`). Name the capture artifacts `<prefix>-<matrix.os>`. Make the merge job download the pattern `<prefix>-*-latest` and upload `<prefix>-merged` (design decision 4), with a comment tying the pattern to the matrix labels. Verify: `actionlint .github/workflows/report.yml` reports no new findings, and with the input unset, `grep` shows the names `omnisnap-${{ matrix.os }}` and `omnisnap-merged` are produced exactly as before.
- [ ] 1.2 Document the input. Add a row to the inputs table in `docs/github-action-setup.md` and `action/README.md` where `report.yml`'s inputs are listed, mention it in `specs/SPEC-ci-action.md`'s line on the reusable workflow's extra inputs, and add it to `docs/internals/GitHub-Action.md`. Verify: `grep -rn 'artifact-prefix' docs action specs` finds each of them, and `go test ./test/internals/` passes.

## 2. The composite-action self-test is callable (`e2e-action.yml`)

- [ ] 2.1 Add `on: workflow_call` with an optional `tag` input, add the same optional input to `workflow_dispatch`, and remove the `release:` trigger and its comment. Both jobs' `actions/checkout` use `ref: ${{ inputs.tag || github.ref }}` (design decision 2). Verify: `actionlint .github/workflows/e2e-action.yml` reports no new findings, and the file has no `release:` key.
- [ ] 2.2 Rename its artifacts to `selftest-action-<os>`, download `selftest-action-*-latest` in `merge`, and upload `selftest-action-merged`. Verify: `grep -n 'omnisnap-' .github/workflows/e2e-action.yml` finds no artifact name or pattern left (the `omnisnap-out` folder and `omnisnap-lab` are not artifacts).
- [ ] 2.3 Dispatch it by hand on the branch with `tag: v0.1.0-rc.7` (`gh workflow run e2e-action.yml --ref <branch> -f tag=v0.1.0-rc.7`). Verify: the run is green, its checkout log shows the `v0.1.0-rc.7` commit, and `selftest-action-merged` is uploaded; record the run ID here.
- [ ] 2.4 Update `docs/internals/Testing-and-CI.md`'s description of the self-test: its triggers (manual, and called by every release) and its artifact names. Verify: `go test ./test/internals/` passes.

## 3. The reusable-workflow self-test is callable (`e2e-report.yml`)

- [ ] 3.1 Add `on: workflow_call` with an optional `tag` input, add the same optional input to `workflow_dispatch`, remove the `release:` trigger, and keep `pull_request` as it is. In `pick`, use `inputs.tag` when set and the newest release with `SHA256SUMS` otherwise, and fail with `::error::<tag> has no SHA256SUMS` when the given tag's release lacks it. Drop the `EVENT = release` branch. Verify: `actionlint .github/workflows/e2e-report.yml` reports no new findings, and the file has no `release:` key.
- [ ] 3.2 Pass `artifact-prefix: selftest-report` to `report.yml` and download `selftest-report-merged` in `assert`. Update the header comment and the `pick` comment to say where the binary comes from now. Verify: `grep -n 'omnisnap-merged' .github/workflows/e2e-report.yml` finds nothing.
- [ ] 3.3 Dispatch it by hand on the branch twice: once with `tag: v0.1.0-rc.7`, and once without a tag. Verify: both runs are green, `pick` logs `testing with the v0.1.0-rc.7 binary` in both (rc.7 is also the newest), and the merged report holds three hosts. Record the run IDs here.
- [ ] 3.4 Dispatch it once with `tag: v0.0.0-nope`. Verify: `pick` fails with the `has no SHA256SUMS` (or no release) error and no capture job starts.
- [ ] 3.5 Update `docs/internals/Testing-and-CI.md` and `docs/internals/GitHub-Action.md` where they describe this self-test (triggers, how the binary is picked, the artifact prefix). Verify: `go test ./test/internals/` passes.

## 4. The release runs them and the registries wait (`release.yml`)

- [ ] 4.1 Add job `e2e-action` (`uses: ./.github/workflows/e2e-action.yml`, no `needs`, `permissions: contents: read`, `with: tag: ${{ github.event.inputs.tag || github.ref_name }}`). Add job `e2e-report` (`needs: release`, `uses: ./.github/workflows/e2e-report.yml`, `permissions: contents: read, pull-requests: write`, same `tag`). Neither gets `secrets:`. Change `packages` to `needs: [release, e2e-action, e2e-report]` and leave `deploy` on `needs: release` (design decision 1). Update the header comment's job list. Verify: `actionlint .github/workflows/release.yml` reports no new findings (it checks the called workflows' inputs and permissions).
- [ ] 4.2 In the `release` job, add the step that warns when a dispatched run's ref is not the tag (design decision 3): `::warning::the workflows under test are from <ref>, not <tag>; start the run from the tag to test the tag's report.yml`. Verify: actionlint passes, and the step's condition is `github.event_name == 'workflow_dispatch' && github.ref_name != env.TAG`.
- [ ] 4.3 Update `docs/internals/Release-Process.md` section 3. Replace "Then e2e-action.yml and e2e-report.yml run on the prerelease" with the new graph: `e2e-action` beside `release`, `e2e-report` after it, and `packages` after both, while `deploy` does not wait. Add what to do on a red self-test (cut the next candidate; the tag and release stay), and say to start a manual re-run from the tag ("Use workflow from"). Update its `## Source files` line for `release.yml`. Verify: `go test ./test/internals/` passes.
- [ ] 4.4 Update `specs/SPEC-ci-action.md`. Its "Self-test" and "Reusable-workflow self-test" entries say "every prerelease": change them to "called by every release run (`release.yml`); npm and PyPI wait for both". Add a decision-log entry (2026-11, B5: called from `release.yml`, registries gated, website not; why a call and not a token). Remove the "separate change" note from `specs/SPEC-website.md` Out of scope, and update `docs/internals/Website.md` if it names the `e2e-*` triggers. Verify: `grep -rn 'every prerelease' specs docs/internals` finds nothing about the self-tests, and `go test ./test/internals/` passes.

## 5. Integration

- [ ] 5.1 Run `actionlint` on every workflow, and `go test ./test/internals/`. Verify: no new actionlint findings across `.github/workflows/*.yml`, and the wiki test passes.
- [ ] 5.2 Point backlog B5 at this change: set its **Status** in `tasks/backlog.md` to "in progress, OpenSpec change `run-self-tests-on-release`" with the date. Verify: the line is there.
- [ ] 5.3 After the owner pushes `v0.2.0-rc.1` (change `release-v0-2-0`), check the release run. Verify: its graph shows `e2e-action` and `e2e-report` with `packages` after them; both are green; `e2e-report`'s `pick` logged `testing with the v0.2.0-rc.1 binary`; the deploy and the self-test reports were uploaded under distinct names (`omnisnap-merged`, `selftest-report-merged`, `selftest-action-merged`) with no duplicate-name error. Record the run ID in this task and in B5.
- [ ] 5.4 On that same run (or a later candidate's), use "Re-run failed jobs" once on a self-test job, or "Re-run all jobs" if none failed. Verify: the re-run uploads its artifacts without a duplicate-name error; if it does not, add `overwrite: true` to the self-tests' uploads and re-check (design, Risks).

## Workflow follow-up

- Archive the change once 5.3 is ticked, and mark backlog B5 done with the run ID.
