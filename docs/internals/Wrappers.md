# Wrappers

**Role.** The npm and PyPI packages: put the right binary on the user's machine, and give a thin API that writes a job, runs the binary, streams its progress and returns the parsed summary. **No capture or diff logic lives in them**, and they have no runtime dependencies. Module `wrappers`, in [`packages/npm`](https://github.com/GerMP21/Omnisnap/tree/main/packages/npm) and [`packages/pypi`](https://github.com/GerMP21/Omnisnap/tree/main/packages/pypi).

## How the binary gets there

| | npm | PyPI |
|---|---|---|
| Packages | `omnisnap` (API, types, the `omnisnap` bin shim) plus five platform packages `@omnisnap/<os>-<arch>` ([`platforms/`](https://github.com/GerMP21/Omnisnap/tree/main/packages/npm/platforms)): `linux-x64`, `linux-arm64`, `darwin-x64`, `darwin-arm64`, `win32-x64`, each holding one binary under `bin/` | one package, built as **five platform wheels** (`py3-none-<tag>`), each holding only its platform's binary under `omnisnap/bin/<key>/` |
| How the right one is chosen | `omnisnap` lists the five platform packages as `optionalDependencies`, and each declares its `os` and `cpu`, so npm installs only the one that matches | pip picks the wheel whose platform tag matches: `manylinux_2_28_x86_64`/`_aarch64`, `macosx_13_0_x86_64`/`_arm64` (Go 1.27 needs macOS 13), `win_amd64` ([`tools/release/wheels.py`](https://github.com/GerMP21/Omnisnap/blob/main/tools/release/wheels.py)) |
| CLI | the [`bin/omnisnap.js`](https://github.com/GerMP21/Omnisnap/blob/main/packages/npm/omnisnap/bin/omnisnap.js) shim runs the binary with the same arguments, and reports a missing binary instead of crashing | the `omnisnap` console script ([`_cli.py`](https://github.com/GerMP21/Omnisnap/blob/main/packages/pypi/omnisnap/_cli.py)) |

**Binary resolution**, in order:

- npm, [`resolveBinary`](https://github.com/GerMP21/Omnisnap/blob/main/packages/npm/omnisnap/index.js): `OMNISNAP_BINARY`, then `@omnisnap/<platform>/bin/omnisnap[.exe]` (resolved through the platform package's `package.json`), then `omnisnap` on `PATH`;
- Python, [`resolve_binary`](https://github.com/GerMP21/Omnisnap/blob/main/packages/pypi/omnisnap/_run.py): `OMNISNAP_BINARY`, then the wheel's `bin/<key>/omnisnap[.exe]`, then `PATH`.

## The API

| npm ([`index.js`](https://github.com/GerMP21/Omnisnap/blob/main/packages/npm/omnisnap/index.js), [`index.d.ts`](https://github.com/GerMP21/Omnisnap/blob/main/packages/npm/omnisnap/index.d.ts)) | Python ([`_run.py`](https://github.com/GerMP21/Omnisnap/blob/main/packages/pypi/omnisnap/_run.py), [`types.py`](https://github.com/GerMP21/Omnisnap/blob/main/packages/pypi/omnisnap/types.py)) | Runs |
|---|---|---|
| `run(options)` | `run(pages, **options)` | `omnisnap run <out>/omnisnap-job.json --json --out <out>` and returns `summary.json` |
| `browsers()` | `browsers()` | `omnisnap browsers --json` |
| `merge(bundles, out)` | `merge(bundles, out)` | `omnisnap merge` |
| `report(bundle)` | `report(bundle)` | `omnisnap report`; returns the HTML path |
| `runBinary(args)`, `resolveBinary()` | `run_binary(args)`, `resolve_binary()` | the lower level |

`run` options map onto the job: `pages`, `engines`, `baseline`, `runs`, `out`, `viewport`, `timeouts`, `probes`, `fontParity` / `font_parity`, plus `allowSmallPages` / `allow_small_pages` (passes `--allow-small-pages`) and an `onProgress` / `on_progress` callback that receives each NDJSON event as it arrives. Lines are buffered until complete, and events of unknown types are ignored. A non-zero exit raises `OmnisnapError` with the exit code and stderr.

The types (`Summary`, `Result`, `Cause`, …) are written by hand in `index.d.ts` and `types.py`; a test keeps them covering every field the summary schema requires.

## Tests

- **npm** ([`api.test.mjs`](https://github.com/GerMP21/Omnisnap/blob/main/packages/npm/omnisnap/test/api.test.mjs)), against a stub binary ([`stub.mjs`](https://github.com/GerMP21/Omnisnap/blob/main/packages/npm/omnisnap/test/stub.mjs)): progress events in order; `allowSmallPages` passes the flag only when set; the platform package's binary is found under `bin/`; a non-zero exit is a typed error; `browsers`, `merge`, `report`; partial and unknown NDJSON lines; the declarations cover the schema's required fields; the `LICENSE` copies match the repository's.
- **Python** ([`test_api.py`](https://github.com/GerMP21/Omnisnap/blob/main/packages/pypi/omnisnap/test/test_api.py)), against a stub: progress in order, `allow_small_pages`, a typed error on a non-zero exit, `browsers`, `merge` and `report`, the version, the types covering the schema's required fields, the `LICENSE.txt` copy.
- **CI, as published** (the `packages` job of [`ci.yml`](https://github.com/GerMP21/Omnisnap/blob/main/.github/workflows/ci.yml), on Linux, Windows and macOS): the binary is staged into the packages, the npm tarballs and the runner's wheel are built and installed into a fresh project and a fresh venv, and `run()` runs the planted suite. [`test/wrappers/check_summary.py`](https://github.com/GerMP21/Omnisnap/blob/main/test/wrappers/check_summary.py) then checks the planted gates on what `run()` returned ([Ground truth and the model](Ground-Truth-and-the-Model)).

## Where the code differs from the spec

Nothing found. Publishing is described in [Release process](Release-Process).

## Spec

[`specs/SPEC-wrappers.md`](https://github.com/GerMP21/Omnisnap/blob/main/specs/SPEC-wrappers.md).

## Source files

- [`packages/npm`](https://github.com/GerMP21/Omnisnap/tree/main/packages/npm): the npm package and its platform packages
- [`packages/pypi`](https://github.com/GerMP21/Omnisnap/tree/main/packages/pypi): the PyPI package
