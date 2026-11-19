# Spec: wrappers

Module id: `wrappers` · Locations: `packages/npm/`, `packages/pypi/` · Depends on: `cli`, `bundle-schema` · Milestone: M2 · Conventions: [CONVENTIONS.md](CONVENTIONS.md)

## Objective

Ship Omnisnap where frontend developers and test suites already are: `npm install omnisnap` and `pip install omnisnap`. Both are **thin wrappers** around the one Go binary (PRD 1.1.0): they write `job.json`, run the binary, stream its NDJSON progress, and return the parsed `summary.json`. No logic is reimplemented, so both stay small and identical in behaviour.

## Behaviour

### Packaging

- **npm:** `omnisnap` (JS API + `bin` shim) with platform packages as `optionalDependencies`: `@omnisnap/linux-x64`, `@omnisnap/linux-arm64`, `@omnisnap/darwin-arm64`, `@omnisnap/darwin-x64`, `@omnisnap/win32-x64` (each containing only the binary, with `os` and `cpu` fields). No `postinstall` download.
- **PyPI:** `omnisnap` platform wheels (`manylinux_2_28_x86_64`, `manylinux_2_28_aarch64`, `macosx_11_0_arm64`, `macosx_10_15_x86_64`, `win_amd64`) with the binary inside; a pure-Python API; a console entry point.
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

Also `browsers()`, `merge(bundles, out)`, `report(bundle)`. Errors: a typed error carrying the exit code and stderr; a non-zero exit is an error, findings are not.

### Types

TypeScript declarations and Python `TypedDict`s are **generated** from the JSON Schemas (`SPEC-bundle-schema.md`) at build time; a CI check fails when they drift.

### Test-runner helpers (optional, M2)

`omnisnap/playwright` and a pytest plugin that run Omnisnap after a test suite on the pages it visited: in scope only if the spec's open question is answered yes.

## Commands

```
cd packages/npm && npm test && npm pack                 # API tests against a stub binary, then the real one
cd packages/pypi && python -m pytest && python -m build
go run ./tools/release ...                               # builds binaries and stages them into the platform packages
```

## Project structure

`packages/npm/{omnisnap/, platforms/<os-arch>/}`, `packages/pypi/{omnisnap/, pyproject.toml}`, `packages/types/` (generated from `schema/`), `tools/release/`.

## Code style

TypeScript (ESM, Node ≥ 20, no runtime dependencies) and Python (≥ 3.10, typed, no runtime dependencies). Both resolve the binary path the same way: platform package first, then `OMNISNAP_BINARY` override, then `PATH`.

## Testing strategy

- **Unit:** job serialization; NDJSON parsing (partial lines, unknown event types ignored); error mapping; binary resolution; using a stub binary that replays recorded progress and summaries.
- **Integration (CI, Linux and Windows):** install the packed tarball / wheel into a fresh project, run against the local planted server, assert the planted gates on the returned summary. The platform binaries are not published yet, so the CI job points the wrapper at the binary it just built (`OMNISNAP_BINARY`).
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
- npm scope `@omnisnap` availability for the platform packages.
