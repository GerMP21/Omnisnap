# Release process

How a version gets from a commit to GitHub, npm and PyPI. Pushing a tag does all of it: the GitHub release, then npm and PyPI from the same binaries, and the website. The only manual step is the version bump before the tag.

## 1. Choose the version

A stable release is `vX.Y.Z` (npm and PyPI `X.Y.Z`) and a release candidate `vX.Y.Z-rc.N` (npm `X.Y.Z-rc.N`, PyPI `X.Y.ZrcN`). What each number promises is in [`specs/CONVENTIONS.md`](https://github.com/GerMP21/Omnisnap/blob/main/specs/CONVENTIONS.md), "Versioning". A stable release is cut from a candidate's code: once the candidate's release run, its self-tests and "Install (network)" are green, the stable tag goes on that code plus the version bump, the release notes and the status wording. Nothing else changes between the two tags. A red candidate is not fixed in place; the fix goes to `main` and becomes the next candidate.

The version appears in:

- the npm `package.json` files (the main package's `optionalDependencies` pin the platform packages to the same version);
- `packages/pypi/pyproject.toml`, the package's `__init__.py`, and the test that pins it (`packages/pypi/omnisnap/test/test_api.py`, `test_version_exposed`; CI's `packages` job fails on a bump that misses it);
- the `version` default of [`action/action.yml`](https://github.com/GerMP21/Omnisnap/blob/main/action/action.yml) and of [`report.yml`](https://github.com/GerMP21/Omnisnap/blob/main/.github/workflows/report.yml);
- the fork workflow template, the README, `action/README.md` (its examples and inputs), the docs and the specs that name it.

`tools/release --stage` (which the release workflow runs) refuses a tag the package files were not bumped for, and names each file: [`checkPackageVersions`](https://github.com/GerMP21/Omnisnap/blob/main/tools/release/main.go). The `v0.1.0-rc.6` tag was cut before its bump and shipped rc.5's package versions and Action default; the check makes that a failed build instead.

A tag that was withdrawn stays withdrawn: the Go module proxy caches every tag it has seen, so [`go.mod`](https://github.com/GerMP21/Omnisnap/blob/main/go.mod) **retracts** it (today `v0.1.0-rc.1` and `v0.1.0-rc.3`) instead of reusing the tag.

## 2. Build the binaries

[`tools/release`](https://github.com/GerMP21/Omnisnap/blob/main/tools/release/main.go) cross-compiles `./cmd/omnisnap`, and never the lab binary, for the five targets: `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, `windows/amd64`. Builds are `-trimpath` with the version stamped through `-ldflags`. It writes `omnisnap_<version>_<os>_<arch>[.exe]` and `SHA256SUMS` (sorted by file name) into `dist/`, after removing an earlier build's files so a stale binary cannot be published with the new ones.

```
go run ./tools/release --version v0.2.0-rc.1 [--dry-run] [--stage]
```

`--stage` copies the binaries into the packages: `packages/npm/platforms/<os>-<arch>/bin/` and `packages/pypi/omnisnap/bin/<key>/` (both ignored by git).

## 3. The GitHub release

Pushing a `v*` tag runs [`release.yml`](https://github.com/GerMP21/Omnisnap/blob/main/.github/workflows/release.yml) on the tag (also when started by hand: it checks out the tag, not the branch it was started from): the unit tests, `tools/release --stage` for the tag, then `gh release create` with the five binaries and `SHA256SUMS`. A tag with a dash is a prerelease. The notes are GitHub's generated ones. When the tagged commit has `docs/release-notes/<tag>.md`, its text goes ahead of them: write one for a release that users have to act on (a minor that changes the public surface always has one, saying how to move), and none for a candidate with nothing to say. If the release exists already (a re-run), the assets are replaced and the notes are left as they are, edits made on GitHub included. A last step checks that the assets download. The release is the whole interface for the Action, which downloads the asset by tag and checks it against `SHA256SUMS`.

The rest of the run:

```
release ----+--> deploy       the website, for the tag
            +--> e2e-report   report.yml as a consumer calls it, with the release's binary
e2e-action  ----------------  the composite action, built from the tag (starts with release)
            |
            v
packages    npm and PyPI, only once release, e2e-action and e2e-report have passed
```

On npm a stable version is published as `latest`. A candidate is `latest` only while the package's `latest` on npm is not a stable version (a new package, or only candidates so far); after the first stable it goes to `next`, so `npm i omnisnap` keeps installing the stable ([`specs/CONVENTIONS.md`](https://github.com/GerMP21/Omnisnap/blob/main/specs/CONVENTIONS.md), "Versioning"). The trusted publisher cannot change dist-tags, so a stale `next` is removed by hand.

The two self-tests ([Testing and CI](Testing-and-CI)) are jobs of `release.yml` that call [`e2e-action.yml`](https://github.com/GerMP21/Omnisnap/blob/main/.github/workflows/e2e-action.yml) and [`e2e-report.yml`](https://github.com/GerMP21/Omnisnap/blob/main/.github/workflows/e2e-report.yml) with the tag. **On a red self-test** the tag stays off npm and PyPI, and the run is red. The tag and the GitHub release stay exactly as published: never move or delete them. Fix the cause on `main` and cut the next candidate, and do not announce the red one. A self-test that failed for a flaky reason can be re-run alone, and `packages` after it; its steps skip versions already on the registries.

**A manual re-run** (Actions → Release → Run workflow) tests the workflow files of the ref it is started from: GitHub reads the called workflows (`e2e-report.yml`, `report.yml`, `deploy.yml`) from the run's own ref, not from the `tag` input. So set "Use workflow from" to the tag itself; the `release` job warns when the two differ.

The `packages` job publishes to npm and PyPI (sections 4 and 5). The `deploy` job does not wait for the self-tests; it calls [`deploy.yml`](https://github.com/GerMP21/Omnisnap/blob/main/.github/workflows/deploy.yml) for the tag: it captures the demo and publishes [the website](Website), so `omnisnap.org` describes the new release. It is a job of `release.yml` rather than a `release: published` workflow, because GitHub starts no workflow from a release the workflow token created. When the demo compares nothing (both demo sites down), that job fails and the site stays on the previous release; the release itself is already published. Fix the cause and run the deploy again by hand.

## 4. npm

The `packages` job of `release.yml` publishes with npm's **trusted publishing**: the job has `id-token: write`, and npm accepts its OpenID Connect token instead of a stored token, and adds a provenance statement. It needs npm 11.5.1 or later on Node 22.14 or later, so the job installs Node 24 and npm 11. One-time setup: on npmjs.com, each of the six packages names `GerMP21/Omnisnap` and the workflow file `release.yml` as its trusted publisher (there is no organisation-wide setting).
Source: https://docs.npmjs.com/trusted-publishers/

It uses the binaries the release job built and staged (`--stage`), passed on as the `staged-binaries` artifact (artifacts drop file modes, so the job restores them):

1. The five platform packages first, each with `npm publish --access public --tag <tag>` (scoped packages are private by default), so the main package's `optionalDependencies` can resolve. `<tag>` is `latest`, or `next` for a candidate after a stable (section 3).
2. Then `omnisnap`.

A version already on npm is skipped (`npm view`), so a re-run of the job publishes only what is missing. Each tarball ships `LICENSE`, listed in its `"files"`; a test keeps the copies identical to the repository's `LICENSE.txt`.

By hand (a registry outage, or a first package that has no trusted publisher yet): `go run ./tools/release --version <tag> --stage`, `npm login`, then the two steps above, with `--tag next` for a candidate once a stable is out.

## 5. PyPI

From staged binaries, [`tools/release/wheels.py`](https://github.com/GerMP21/Omnisnap/blob/main/tools/release/wheels.py) builds one wheel per platform from a copy of `packages/pypi` that holds only that platform's binary, retags it with `wheel tags` (`manylinux_2_28_x86_64` and `_aarch64`, `macosx_13_0_x86_64` and `_arm64`, `win_amd64`), and checks each one: the tag in the file name and in `WHEEL`, exactly one binary and executable, `LICENSE.txt` in the dist-info, under 15 MB. Without the split, `python -m build` would make one universal wheel of every binary (19 MB, tagged for every platform).

```
python tools/release/wheels.py --out dist-py
python -m twine check dist-py/*
python -m twine upload dist-py/*        # by hand only; the workflow uploads with trusted publishing
```

The `packages` job builds the wheels the same way, in a virtual environment, and uploads them with [`pypa/gh-action-pypi-publish`](https://github.com/pypa/gh-action-pypi-publish) through PyPI's trusted publishing, with `skip-existing` so a re-run uploads only what is missing. One-time setup: the PyPI project `omnisnap` names `GerMP21/Omnisnap` and `release.yml` as a trusted publisher (Manage → Publishing).
Source: https://docs.pypi.org/trusted-publishers/using-a-publisher/

## 6. After the release

- Nothing to do for this wiki: its pages changed with the code, and the [wiki workflow](Testing-and-CI#the-wikis-own-check) publishes them when `main` moves.
- A consumer pinned to `report.yml@<tag>` keeps the old version until they move the ref.

## Spec

[`specs/SPEC-wrappers.md`](https://github.com/GerMP21/Omnisnap/blob/main/specs/SPEC-wrappers.md) (distribution) and [`specs/SPEC-ci-action.md`](https://github.com/GerMP21/Omnisnap/blob/main/specs/SPEC-ci-action.md) (versions and the release interface).

## Source files

- [`tools/release/main.go`](https://github.com/GerMP21/Omnisnap/blob/main/tools/release/main.go): cross-compiles the five binaries, writes SHA256SUMS, stages them, and checks the package versions
- [`tools/release/wheels.py`](https://github.com/GerMP21/Omnisnap/blob/main/tools/release/wheels.py): builds and checks one wheel per platform
- [`.github/workflows/release.yml`](https://github.com/GerMP21/Omnisnap/blob/main/.github/workflows/release.yml): the GitHub release, both self-tests, then npm and PyPI; and the website deploy
- [`.github/workflows/e2e-action.yml`](https://github.com/GerMP21/Omnisnap/blob/main/.github/workflows/e2e-action.yml): the composite-action self-test, called by every release
- [`.github/workflows/e2e-report.yml`](https://github.com/GerMP21/Omnisnap/blob/main/.github/workflows/e2e-report.yml): the reusable-workflow self-test, called by every release
- [`go.mod`](https://github.com/GerMP21/Omnisnap/blob/main/go.mod): the retracted tags
