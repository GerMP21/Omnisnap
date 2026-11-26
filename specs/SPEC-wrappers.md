# Spec: wrappers

Module id: `wrappers` · Locations: `packages/npm/`, `packages/pypi/` · Depends on: `cli`, `bundle-schema` · Milestone: M2 · Conventions: [CONVENTIONS.md](CONVENTIONS.md)

## Objective

Ship Omnisnap where frontend developers and test suites already are: `npm install omnisnap` and `pip install omnisnap`. Both are **thin wrappers** around the one Go binary (PRD 1.1.0): they write `job.json`, run the binary, stream its NDJSON progress, and return the parsed `summary.json`. No logic is reimplemented, so both stay small and identical in behaviour.

## Behaviour

### Packaging

- **npm:** `omnisnap` (JS API + `bin` shim) with platform packages as `optionalDependencies`: `@omnisnap/linux-x64`, `@omnisnap/linux-arm64`, `@omnisnap/darwin-arm64`, `@omnisnap/darwin-x64`, `@omnisnap/win32-x64` (each containing only the binary, at `bin/omnisnap[.exe]`, with `os` and `cpu` fields). No `postinstall` download.
- **PyPI:** `omnisnap` platform wheels (`py3-none-manylinux_2_28_x86_64`, `py3-none-manylinux_2_28_aarch64`, `py3-none-macosx_13_0_arm64`, `py3-none-macosx_13_0_x86_64`, `py3-none-win_amd64`), each with only its own binary inside (`omnisnap/bin/<os-arch>/`); a pure-Python API; a console entry point. `tools/release/wheels.py` builds them from the staged binaries: one wheel from `packages/pypi` would hold every platform's binary under one universal tag. The macOS tags say 13.0 because Go 1.27 binaries need "macOS 13 Ventura or newer" (https://go.dev/wiki/MinimumRequirements); 10.15/11.0 tags would let pip install a binary that cannot start (corrected 2026-11-20).
- Size per platform package < 15 MB (PRD distribution target).
- Versions: wrapper version = binary version.

### API (same shape in both; PRD §10.2/§10.3 kept as the wrapper surface)

```ts
import { run } from 'omnisnap';

const summary = await run({
  pages: [{ url: 'http://localhost:3000/' }],
  engines: ['chrome', 'firefox', 'webkit'],
  out: './omnisnap-out',
  onProgress: (event) => console.log(event.type),
});
console.log(summary.results.map((r) => `${r.engine.engine}: ${r.stats.report} to report`));
```

```python
from omnisnap import run

summary = run(pages=["http://localhost:3000/"], engines=["chrome", "firefox"], out="./omnisnap-out",
              on_progress=lambda e: print(e["type"]))
```

`allowSmallPages: true` / `allow_small_pages=True` passes `--allow-small-pages`: planted and test pages are under 50 elements by design, and without it the binary takes them for bot walls (26 of the 31 planted pages came back unreachable through `run()`, 2026-11-20).

Also `browsers()`, `merge(bundles, out)`, `report(bundle)`. Errors: a typed error carrying the exit code and stderr; a non-zero exit is an error, findings are not.

### Types

TypeScript declarations and Python `TypedDict`s are **generated** from the JSON Schemas (`SPEC-bundle-schema.md`) at build time; a CI check fails when they drift.

### Test-runner helpers (optional, M2)

`omnisnap/playwright` and a pytest plugin that run Omnisnap after a test suite on the pages it visited: in scope only if the spec's open question is answered yes.

## Commands

```
cd packages/npm && npm test && npm pack                 # API tests against a stub binary, then the real one
cd packages/pypi && python -m pytest && python -m build
go run ./tools/release --version <tag> --stage          # builds binaries and stages them into the platform packages (refuses a tag the packages were not bumped for)
python tools/release/wheels.py --out dist-py            # the five platform wheels, checked (tag, one binary, size)
# release.yml's `packages` job publishes both with trusted publishing (no stored token); a stable version is npm `latest`; a candidate is `latest` only while the package has no stable version, `next` after (CONVENTIONS "Versioning")
```

## Project structure

`packages/npm/{omnisnap/, platforms/<os-arch>/}`, `packages/pypi/{omnisnap/, pyproject.toml}`, `packages/types/` (generated from `schema/`), `tools/release/`.

## Code style

TypeScript (ESM, Node ≥ 20, no runtime dependencies) and Python (≥ 3.10, typed, no runtime dependencies). Both resolve the binary path the same way: the `OMNISNAP_BINARY` override first, then the platform package (npm) or the wheel's own binary (PyPI), then `PATH`.

## Testing strategy

- **Unit:** job serialization; NDJSON parsing (partial lines, unknown event types ignored); error mapping; binary resolution; using a stub binary that replays recorded progress and summaries.
- **Integration (CI, Linux, Windows and macOS):** install the packed tarball / wheel into a fresh project, run against the local planted server, assert the planted gates on the returned summary. The CI `packages` job stages the runner's binary into its platform package and platform wheel, installs them with no override (as published), runs `run()` over the 31 planted pages in Chrome and Firefox and checks the summary against SPEC-ground-truth.md's gates 1 and 2: Report-tier recall ≥ 90%, every page within its budget, 0 Report causes on controls and KB pages (`test/wrappers/check_summary.py`; a miss is named, and the bundles are uploaded). These are live captures on each runner's fonts, so a single miss is expected there (2026-11-20: `07-font` on Windows, `05-padding` on macOS) where the Linux goldens hold 24/24.
- **Type drift check:** regenerate types from `schema/` and diff.

## Boundaries

- **Always:** keep logic in the binary; keep zero runtime dependencies.
- **Ask first:** a `postinstall` script of any kind; new public API.
- **Never:** download the binary or browsers at install time; reimplement capture or diff logic.

## Success criteria

1. `npm install omnisnap` and `pip install omnisnap` on Windows, macOS and Linux need no compiler and no network beyond the registry; the API returns a valid summary for the planted job.
2. Each platform package < 15 MB.
3. Types generated from the schemas, with a passing drift check.
4. Progress events reach the callback in order, live (not only at the end).
5. Names `omnisnap` claimed on npm and PyPI with placeholder releases before the announcement (one-pager open question).

## Open questions

- Test-runner integrations (Playwright, pytest) in v1, or after? Proposed: after v1; the plain API covers them.
- ~~npm scope `@omnisnap` availability for the platform packages~~ Answered: the platform packages are published under `@omnisnap` (`v0.1.0-rc.2` onward).
