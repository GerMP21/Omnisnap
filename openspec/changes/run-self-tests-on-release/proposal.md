# Proposal

## Why

The two release self-tests never run on a release. `e2e-report.yml` (calls `report.yml` the way a consumer does, with a published binary) and `e2e-action.yml` (runs the composite action on the planted suite) listen for `on: release`, but `release.yml` creates the release with the workflow's `GITHUB_TOKEN`, and GitHub starts no workflow from events that token causes. `v0.1.0-rc.1` fired no self-test at all, and every candidate since has relied on a manual dispatch or on nothing. That is how `v0.1.0-rc.2` shipped two bugs that broke every consumer, and rc.4 and rc.6 shipped stale `version` defaults (backlog B5). `v0.2.0` is the release most people will pin, so it must not reach npm and PyPI before its own consumer test has passed against the tag.

## What Changes

- **The release runs both self-tests.** `release.yml` calls `e2e-action.yml` and `e2e-report.yml` as reusable workflows, the way it already calls `deploy.yml`. They test the tag's own `report.yml`, composite action and binary, both on a tag push and on a manual re-run of a tag.
- **The registries wait for the self-tests.** The `packages` job (npm and PyPI) starts only after both self-tests pass. A failure leaves the registries untouched and the release run red. The GitHub release is still published first, because `e2e-report.yml` downloads its binary from it. The website deploy does not wait.
- **A failure never touches the tag.** A self-test failure does not delete, move or edit the tag or the GitHub release. The red run is the signal to cut the next candidate.
- **The dead triggers go.** The `release:` triggers are removed from both self-tests. `workflow_dispatch` stays on both for a manual re-check, and `e2e-report.yml` keeps its `pull_request` trigger.
- **Called workflows no longer share artifact names.** In one release run, the deploy's demo and `e2e-report.yml` both call `report.yml`, and `e2e-action.yml` uploads under the same `omnisnap-<os>` and `omnisnap-merged` names. Artifact names must be unique within a run, so the uploads would collide and each merge would download the others' bundles. `report.yml` gains an optional `artifact-prefix` input (default `omnisnap`, so existing callers are unchanged), and the self-tests use names of their own.

## Capabilities

### New Capabilities

- `release-verification`: what must be proven about a tag before its packages reach the registries. This covers which self-tests run on every release, against what, in what order relative to the release, the registries and the website, and what a failure does and does not do.

### Modified Capabilities

None. There are no OpenSpec specs yet. The normative `specs/SPEC-ci-action.md` ("Self-test" and "Reusable-workflow self-test" say "every prerelease"; the `report.yml` inputs list) and `specs/SPEC-website.md` (the out-of-scope note about the `e2e-*` triggers) are updated in this change to match.

## Impact

- **Workflows:**
  - `.github/workflows/release.yml`: two new jobs (`e2e-action`, `e2e-report`), and `packages` needs both.
  - `.github/workflows/e2e-action.yml`: `workflow_call` with a `tag` input; checks out the tag; artifact names of its own; no `release:` trigger.
  - `.github/workflows/e2e-report.yml`: `workflow_call` with a `tag` input; `pick` uses the tag when given; passes `artifact-prefix`; no `release:` trigger.
  - `.github/workflows/report.yml`: new optional `artifact-prefix` input.
- **Public surface:** one new, optional `report.yml` input with a default that keeps today's names. This is additive.
- **Time and cost:**
  - npm and PyPI are published about 10-20 minutes later than today, after the slower of the two self-tests.
  - Each release runs two more three-OS matrices (both include `macos-latest`: `e2e-action.yml` in its matrix, `e2e-report.yml` through `report.yml`'s `macos: every` default).
  - Releases are infrequent.
- **Docs:**
  - `docs/internals/Release-Process.md`, `Testing-and-CI.md`, `GitHub-Action.md` and `Website.md`, which name the changed workflows (`CONTRIBUTING.md` rule 7).
  - `docs/github-action-setup.md` and `action/README.md` (the new input).
  - `specs/SPEC-ci-action.md` and `specs/SPEC-website.md`.
  - `tasks/backlog.md` B5's status.
- **Out of scope:**
  - Gating the GitHub release itself on the self-tests (backlog approach 3).
  - A PAT or GitHub App token (approach 2).
  - Making the deploy wait.
  - Running the network install job on releases.
