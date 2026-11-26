# Spec Delta

## Purpose

Defines how Omnisnap versions and publishes its releases, so users know what a version number promises and which version each channel (GitHub release, Action, npm, PyPI, omnisnap.org) installs by default.

## ADDED Requirements

### Requirement: Release tags and their spellings
A release tag SHALL be `vX.Y.Z` for a stable release or `vX.Y.Z-rc.N` for a candidate. Every channel SHALL publish that release under the tag's spelling for that channel: the tag itself for the binary, the Action and `report.yml`; `X.Y.Z[-rc.N]` for npm; `X.Y.Z[rcN]` for PyPI.

#### Scenario: A candidate is spelled per channel
- **WHEN** the tag `v0.2.0-rc.1` is released
- **THEN** the release assets are named `omnisnap_v0.2.0-rc.1_<os>_<arch>`
- **AND** npm publishes `0.2.0-rc.1` and PyPI publishes `0.2.0rc1`

#### Scenario: A stable release is spelled per channel
- **WHEN** the tag `v0.2.0` is released
- **THEN** npm and PyPI both publish `0.2.0` and `omnisnap version` prints `v0.2.0`

### Requirement: A stable release is the default on every channel
A stable release SHALL be the default install on every channel, with no opt-in flag. Its GitHub release SHALL NOT be marked as a prerelease and SHALL be the repository's "Latest" release. On npm, `latest` SHALL name the newest stable release, and a candidate published after a stable release SHALL go to the `next` dist-tag, never to `latest`.

#### Scenario: Plain installs get the stable release
- **GIVEN** `v0.2.0` is released
- **WHEN** a user runs `pip install omnisnap` or `npm i omnisnap` on Linux, Windows or macOS
- **THEN** version `0.2.0` is installed, without `--pre` or a dist-tag

#### Scenario: The GitHub release is Latest
- **WHEN** a user opens the repository's releases
- **THEN** `v0.2.0` is marked "Latest" and is not labelled as a pre-release

#### Scenario: npm has only latest
- **WHEN** someone runs `npm view omnisnap dist-tags`
- **THEN** the output is `{ latest: '0.2.0' }`, with no `next` entry

#### Scenario: A candidate after the stable does not replace it
- **GIVEN** `0.2.0` is `latest` on npm
- **WHEN** a candidate such as `v0.2.1-rc.1` is released
- **THEN** npm publishes `0.2.1-rc.1` under `next`, and `npm i omnisnap` still installs `0.2.0`

### Requirement: A candidate is published as a prerelease
A candidate release SHALL be marked as a prerelease on GitHub, and PyPI SHALL NOT install it without `--pre`.

#### Scenario: A candidate tag is a prerelease
- **WHEN** the tag `v0.2.0-rc.1` is released
- **THEN** its GitHub release is marked as a pre-release
- **AND** `pip install omnisnap`, run without `--pre`, does not install `0.2.0rc1`

### Requirement: Patch releases keep the public surface
Within a `0.Y` line, a patch release (`0.Y.Z` with Z > 0) SHALL NOT change the public surface. That surface is the CLI's flags and exit codes, the bundle and job schemas, the `--json` progress events, the Action and `report.yml` inputs, and the npm and PyPI APIs. It MAY fix bugs and MAY add optional fields, flags or inputs.

#### Scenario: A patch keeps a workflow working
- **GIVEN** a consumer's workflow that works with `report.yml@v0.2.0`
- **WHEN** they change the ref to `report.yml@v0.2.1`
- **THEN** the workflow runs with the same inputs and its exit codes keep their meanings

#### Scenario: A breaking change waits for a minor
- **WHEN** a change would rename or remove a flag, an exit code, a schema field, a progress event or an Action input
- **THEN** it is released in the next minor (`0.3.0`), not in a `0.2.x` patch

### Requirement: Minor releases say how to move
A minor release that changes the public surface SHALL have release notes that name each change and say how to move from the previous minor.

#### Scenario: The first stable release explains the move
- **WHEN** a user reads the GitHub release notes of `v0.2.0`
- **THEN** they name the selector change and the browser cap, and say what a `0.1.0-rc.*` user has to change, if anything

### Requirement: Schema majors are never edited
A breaking change to a bundle or job schema SHALL be published at a new major URL (`https://omnisnap.org/schema/<name>/<major+1>.json`). The file at an existing major URL SHALL only ever gain optional properties.

#### Scenario: An additive schema change keeps the URL
- **WHEN** a release adds an optional property to the capture schema
- **THEN** it is published at `https://omnisnap.org/schema/capture/1.json` and every bundle that validated before still validates

### Requirement: Tags are never moved or reused
A pushed release tag SHALL NOT be moved, deleted to be pushed again, or reused for different content. A withdrawn tag SHALL be retracted in `go.mod` and replaced by a new tag.

#### Scenario: A broken release is replaced
- **WHEN** a release tag turns out to be broken
- **THEN** a new tag is released with the fix, the broken tag is retracted in `go.mod`, and the broken tag still names its original commit

### Requirement: A stable release was first a candidate
A stable release SHALL be tagged on a commit whose code is the code of a released candidate. That candidate's release run, its release self-tests and the network install job SHALL have passed. Only version strings, release notes and the documentation that describes the release status may differ.

#### Scenario: v0.2.0 follows v0.2.0-rc.1
- **GIVEN** `v0.2.0-rc.1`'s release run, release self-tests and the "Install (network)" run on Linux and Windows are green
- **WHEN** `v0.2.0` is tagged
- **THEN** `git diff v0.2.0-rc.1 v0.2.0` touches only the version files, the release notes, and the docs, specs and templates that pin the tag or describe the release status
- **AND** no Go, JavaScript or workflow logic changes between the two tags
