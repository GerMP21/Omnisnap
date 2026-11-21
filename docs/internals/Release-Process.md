# Release process

How a version gets from a commit to GitHub, npm and PyPI. The GitHub release is automated; the npm and PyPI uploads are run by hand, from the same binaries.

## 1. Choose the version

Release candidates are `vX.Y.Z-rc.N` (npm `X.Y.Z-rc.N`, PyPI `X.Y.Zrc.N`). The version appears in:

- the npm `package.json` files (the main package's `optionalDependencies` pin the platform packages to the same version);
- `packages/pypi/pyproject.toml` and the package's `__init__.py`;
- the `version` default of [`action/action.yml`](https://github.com/GerMP21/Omnisnap/blob/main/action/action.yml) and of [`report.yml`](https://github.com/GerMP21/Omnisnap/blob/main/.github/workflows/report.yml);
- the fork workflow template, the README, the docs and the specs that name it.

A tag that was withdrawn stays withdrawn: the Go module proxy caches every tag it has seen, so [`go.mod`](https://github.com/GerMP21/Omnisnap/blob/main/go.mod) **retracts** it (today `v0.1.0-rc.1` and `v0.1.0-rc.3`) instead of reusing the tag.

## 2. Build the binaries

[`tools/release`](https://github.com/GerMP21/Omnisnap/blob/main/tools/release/main.go) cross-compiles `./cmd/omnisnap`, and never the lab binary, for the five targets: `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, `windows/amd64`. Builds are `-trimpath` with the version stamped through `-ldflags`. It writes `omnisnap_<version>_<os>_<arch>[.exe]` and `SHA256SUMS` (sorted by file name) into `dist/`, after removing an earlier build's files so a stale binary cannot be published with the new ones.

```
go run ./tools/release --version v0.1.0-rc.5 [--dry-run] [--stage]
```

`--stage` copies the binaries into the packages: `packages/npm/platforms/<os>-<arch>/bin/` and `packages/pypi/omnisnap/bin/<key>/` (both ignored by git).

## 3. The GitHub release

Pushing a `v*` tag runs [`release.yml`](https://github.com/GerMP21/Omnisnap/blob/main/.github/workflows/release.yml): the unit tests, `tools/release` for the tag, then `gh release create` with the five binaries and `SHA256SUMS` and generated notes. A tag with a dash is a prerelease. If the release exists already (a re-run), the assets are replaced. A last step checks that the assets download. The release is the whole interface for the Action, which downloads the asset by tag and checks it against `SHA256SUMS`.

Then [`e2e-action.yml`](https://github.com/GerMP21/Omnisnap/blob/main/.github/workflows/e2e-action.yml) and [`e2e-report.yml`](https://github.com/GerMP21/Omnisnap/blob/main/.github/workflows/e2e-report.yml) run on the prerelease ([Testing and CI](Testing-and-CI)).

## 4. npm

From staged binaries (`--stage`):

1. Publish the five platform packages first, each with `npm publish --access public` (scoped packages are private by default), so the main package's `optionalDependencies` can resolve.
2. Then publish `omnisnap`.

Each tarball ships `LICENSE`, listed in its `"files"`; a test keeps the copies identical to the repository's `LICENSE.txt`. `npm whoami` must succeed first: an expired login makes `npm publish` fail.

## 5. PyPI

From staged binaries, [`tools/release/wheels.py`](https://github.com/GerMP21/Omnisnap/blob/main/tools/release/wheels.py) builds one wheel per platform from a copy of `packages/pypi` that holds only that platform's binary, retags it with `wheel tags` (`manylinux_2_28_x86_64` and `_aarch64`, `macosx_13_0_x86_64` and `_arm64`, `win_amd64`), and checks each one: the tag in the file name and in `WHEEL`, exactly one binary and executable, `LICENSE.txt` in the dist-info, under 15 MB. Without the split, `python -m build` would make one universal wheel of every binary (19 MB, tagged for every platform).

```
python tools/release/wheels.py --out dist-py
python -m twine check dist-py/*
python -m twine upload dist-py/*
```

## 6. After the release

- Nothing to do for this wiki: its pages changed with the code, and the [wiki workflow](Testing-and-CI#the-wikis-own-check) publishes them when `main` moves.
- A consumer pinned to `report.yml@<tag>` keeps the old version until they move the ref.

## Spec

[`specs/SPEC-wrappers.md`](https://github.com/GerMP21/Omnisnap/blob/main/specs/SPEC-wrappers.md) (distribution) and [`specs/SPEC-ci-action.md`](https://github.com/GerMP21/Omnisnap/blob/main/specs/SPEC-ci-action.md) (versions and the release interface).

## Source files

- [`tools/release/main.go`](https://github.com/GerMP21/Omnisnap/blob/main/tools/release/main.go): cross-compiles the five binaries and writes SHA256SUMS
- [`tools/release/wheels.py`](https://github.com/GerMP21/Omnisnap/blob/main/tools/release/wheels.py): builds and checks one wheel per platform
- [`.github/workflows/release.yml`](https://github.com/GerMP21/Omnisnap/blob/main/.github/workflows/release.yml): the GitHub release
- [`go.mod`](https://github.com/GerMP21/Omnisnap/blob/main/go.mod): the retracted tags
