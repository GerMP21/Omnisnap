# Spec Delta

## Purpose

Defines what is proven about a release tag before its packages reach npm and PyPI: both self-tests run on every release against that tag's own workflow, action and binary, and their result decides whether the registries are published, without ever touching the tag.

## ADDED Requirements

### Requirement: Every release runs both self-tests
Every run of the release workflow for a tag SHALL run the composite-action self-test and the reusable-workflow self-test, with no manual step, whether the run was started by the tag push or by hand for an existing tag.

#### Scenario: A tag push runs both self-tests
- **WHEN** a `v*` tag is pushed and the release workflow runs for it
- **THEN** the same release run includes the composite-action self-test and the reusable-workflow self-test, visible in that run's graph

#### Scenario: A manual re-run of a tag runs them too
- **GIVEN** the release for `v0.2.0-rc.1` already exists
- **WHEN** the owner starts the release workflow by hand with tag `v0.2.0-rc.1`
- **THEN** both self-tests run again in that run, against `v0.2.0-rc.1`

### Requirement: The self-tests test the tag
The composite-action self-test SHALL build the binary from the tag's commit and run the tag's composite action. The reusable-workflow self-test SHALL call the tag's `report.yml` with the binary published in that tag's GitHub release, never the newest other release.

#### Scenario: The consumer test uses the tag's own binary
- **WHEN** the reusable-workflow self-test runs as part of the release of `v0.2.0-rc.1`
- **THEN** every capture job and the merge job download `omnisnap_v0.2.0-rc.1_<os>_<arch>` and check it against that release's `SHA256SUMS`

#### Scenario: The action test builds from the tag
- **GIVEN** the release workflow was started by hand from `main` for an older tag
- **WHEN** the composite-action self-test runs
- **THEN** it builds the binary from the tag's commit, not from `main`

### Requirement: The registries wait for the self-tests
The npm and PyPI publish SHALL start only after both self-tests of the same release run have succeeded. If either fails or is cancelled, nothing is published to npm or PyPI by that run, and the run concludes as failed.

#### Scenario: A self-test fails
- **GIVEN** the reusable-workflow self-test fails for `v0.2.0-rc.1`
- **WHEN** the release run finishes
- **THEN** no `0.2.0-rc.1` package was published to npm or PyPI by that run
- **AND** the release run is marked as failed

#### Scenario: Both self-tests pass
- **WHEN** both self-tests succeed for `v0.2.0-rc.1`
- **THEN** the npm and PyPI publish runs for `0.2.0-rc.1` in the same release run

### Requirement: The GitHub release and the website do not wait
The GitHub release SHALL be published before the reusable-workflow self-test runs, because that test downloads the release's binary. The website deploy SHALL keep starting once the release's assets are verified, independently of the self-tests' result.

#### Scenario: The website deploys while a self-test fails
- **GIVEN** the composite-action self-test fails for a tag
- **WHEN** the release's assets were verified
- **THEN** the website deploy for that tag still runs to its own conclusion

### Requirement: A failure never changes the tag or the release
A self-test failure SHALL NOT delete, move or re-point the tag. It SHALL NOT delete the GitHub release or its assets, and SHALL NOT change the release's prerelease or latest flags.

#### Scenario: A failed self-test leaves the release as published
- **GIVEN** a self-test fails for `v0.2.0-rc.1`
- **WHEN** the release run has finished
- **THEN** the tag `v0.2.0-rc.1` points at the same commit as before the run
- **AND** the GitHub release `v0.2.0-rc.1` and its assets are unchanged

### Requirement: The self-tests stay runnable on their own
Each self-test SHALL keep a manual trigger, so it can be re-checked without a release. The reusable-workflow self-test SHALL keep running on pull requests that change `report.yml`, its own workflow or `action/`. Neither SHALL declare a trigger on release events.

#### Scenario: Manual re-check without a release
- **WHEN** the owner dispatches the reusable-workflow self-test by hand without a tag
- **THEN** it runs with the newest published release whose `SHA256SUMS` is up, as before this change

#### Scenario: No dead release trigger
- **WHEN** a reader opens either self-test workflow
- **THEN** it declares no `release:` trigger

### Requirement: Reports from one run do not mix
When `report.yml` is called more than once in the same workflow run, each call SHALL be able to name its artifacts with a prefix of its own, so that no upload collides and each merge reads only its own capture bundles. Without the prefix, the artifact names SHALL be the same as before this change.

#### Scenario: Demo and consumer test in one release run
- **GIVEN** one release run calls `report.yml` for the website demo with the default prefix and for the reusable-workflow self-test with its own prefix
- **WHEN** both calls upload their bundles and merge them
- **THEN** no artifact upload fails on a duplicate name
- **AND** the demo's merged report holds only the demo pages and the self-test's holds only the planted pages

#### Scenario: Existing consumers are unchanged
- **WHEN** a consumer calls `report.yml` without the prefix input
- **THEN** the capture artifacts are named `omnisnap-<os>` and the merged report `omnisnap-merged`, as before
