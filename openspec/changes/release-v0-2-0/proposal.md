# Proposal

## Why

Every Omnisnap release so far is a candidate (`v0.1.0-rc.1` to `v0.1.0-rc.7`), and each channel makes users pay for that. `pip install omnisnap` skips pre-releases, so every guide says `--pre`. npm still carries a stale `next` dist-tag (`0.1.0-rc.6`) next to `latest`. Consumers pin `report.yml@v0.1.0-rc.7`, a tag the project has said it will replace.

The reasons the first release was a candidate are settled:
- Safari runs unattended and passes driver conformance (Tasks 2 and 23).
- The crawl ran on 457 pages.
- The model ranks causes and the rules set the tier (Task 76).

What is left is backlog B4 (the Windows install), B5 (release self-tests), and the two fixes the owner wants in the first stable release: B2 (selectors) and B6 (live browsers). The owner's call (2026-11-25): the release that leaves the candidates is **`v0.2.0`**, not `v0.1.0` and not `v1`. It is cut through one candidate, `v0.2.0-rc.1`.

## What Changes

- **The release path.**
  - `v0.2.0-rc.1` is cut once these four changes are merged: `fix-windows-firefox-install` (B4), `run-self-tests-on-release` (B5), `fix-selector-sibling-index` (B2) and `cap-live-browsers` (B6).
  - When its release run, its release self-tests and the Windows network install are green, `v0.2.0` is tagged from the same commit plus the version bump.
- **Why 0.2.0 and not 0.1.0.** The jump says that `0.2.0` is not the `0.1.0` the candidates previewed: B2 changes the selector text in every report and comment, and B6 adds a public option. There is no `v0.1.0` stable. The `v0.1.0-rc.*` tags stay as they are, and the `go.mod` retractions stay.
- **A written 0.x compatibility promise**, in the README status block and `specs/CONVENTIONS.md`:
  - `0.2.x` patches never change the public surface. That surface is the CLI's flags and exit codes, the bundle schemas, the `--json` progress events, the Action and `report.yml` inputs, and the npm and PyPI APIs.
  - The next minor (`0.3.0`) may change any of them, and its release notes say what changed and how to move.
  - A breaking schema change gets a new `/schema/<name>/<major>.json`. The existing file is never edited.
- **Release notes that say how to move.** `release.yml` puts a hand-written notes file for the tag (`docs/release-notes/<tag>.md`) ahead of GitHub's generated notes when one exists. `v0.2.0` ships one covering the move from `0.1.0-rc.*`: the selector change and the new browser cap.
- **Wording at the release.**
  - The README status block becomes the 0.x promise.
  - `--pre` and its "pip skips release candidates" sentence go from the README and the PyPI README.
  - The milestone table's "**M2 = v1**" row and the line "`v1` waits for…" are rewritten: M2 shipped as `0.2.0`.
  - The decision log in `specs/SPEC-ci-action.md` records the decision.
  - Every pinned `v0.1.0-rc.7` in the docs, specs and templates moves to the new tag (Release-Process step 1).
- **Housekeeping before `v0.2.0`.**
  - Remove the stale npm `next` dist-tag from the six packages, by hand, with the owner's login.
  - Archive the OpenSpec change `add-developer-wiki`.
  - Finish and archive `add-omnisnap-website`.
  - Set backlog B1 to done.
  - Retarget B7 to `0.3.0`.

## Capabilities

### New Capabilities

- `release-versioning`: how Omnisnap versions and publishes its releases. This covers the candidate and stable tags and how each channel spells them, the 0.x compatibility promise, where a release is published and how a stable release is marked, and the rules for tags and schema majors.

### Modified Capabilities

None. `openspec/specs/` is empty. The normative text lives in `specs/CONVENTIONS.md` and `specs/SPEC-ci-action.md`, which this change updates.

## Impact

- **Workflows:** `.github/workflows/release.yml` (the release-notes file). Its prerelease logic, and the `latest` tag on npm, already handle a tag without a dash (checked 2026-11-25).
- **Version files** (Release-Process step 1):
  - the six npm `package.json` files
  - `packages/pypi/pyproject.toml` and `__init__.py`
  - the `version` defaults in `action/action.yml` and `report.yml`
  - the fork workflow template
  - every doc and spec that pins the tag
- **Docs:**
  - `README.md` and `packages/pypi/README.md`
  - `docs/quickstart.md`, `docs/github-action-setup.md` and `docs/github-action-fork-comments.md`
  - `specs/CONVENTIONS.md`, `specs/SPEC-ci-action.md` and `specs/SPEC-ci-action-fork-template.md`
  - `docs/internals/Release-Process.md` and `GitHub-Action.md`
  - `tasks/backlog.md`
  - `docs/status-2026-11-19.md` is a dated report and is left unchanged.
- **External:**
  - npm: the dist-tag cleanup, by hand.
  - The GitHub release becomes "Latest".
  - `omnisnap.org` names `v0.2.0` (deployed by the release).
- **Depends on:** the four changes listed under "The release path". This change's own work is the release itself and its wording.
- **Out of scope:** what earns `v1`, a moving `@v0.2` tag, and backlog B7 (retargeted to `0.3.0`).
