# Spec: merge

Module id: `merge` · Package: `internal/merge` · Depends on: `bundle-schema`, `viewer` · Milestone: M2 · Conventions: [CONVENTIONS.md](CONVENTIONS.md)

## Objective

Combine bundles captured on several machines (in v1: the CI runners — Linux, Windows and macOS) into **one** bundle and one report, so a pull request gets a single view that includes real Safari (macOS only) and Windows-only browsers next to Linux. Groundwork for the post-v1 capture network (PRD 1.1.0 moved `host` to each result for this).

## Behaviour

```
omnisnap merge <bundle-dir>... -o <merged-dir>
```

- **Input:** 1..n bundles with the same `job` (same pages and viewport; engines may differ by host).
- **Results:** concatenated; each keeps its `host`. Pages are unified by slug + URL.
- **No cross-host diffing in v1:** each host compares its engines with **its own** baseline (fonts and rasterization differ per OS; a Chrome-on-Linux vs Safari-on-macOS comparison would mostly report OS font differences). The merged report shows, per page, every host's pairs.
- **Files:** screenshots and captures are copied (or hard-linked when on the same filesystem) under `<merged>/<host-id>/<slug>/`; paths in the merged `summary.json` are rewritten.
- **Conflicts:** different `job` fields (other than engines) → error naming the field; two bundles from the same host id → error unless `--rename-hosts`; schema major versions differ → error.
- **Output:** a valid bundle (summary, host list, report). Deterministic: the same inputs give byte-identical `summary.json` (results sorted by page, host, engine).
- **PR comment data:** the merged summary includes per page the total Report causes across hosts, ready for `ci-action`.

## Commands

```
go test ./internal/merge/...                                    # unit + golden merges
go run ./cmd/omnisnap merge out-linux out-windows out-macos -o out-merged
```

## Project structure

`internal/merge/{merge.go, paths.go, conflicts.go}`; `testdata/merge/` (three small bundles from different hosts).

## Code style

Per conventions. Merge is a pure function over parsed summaries plus a file-copy plan; copying is a separate step (testable without I/O).

## Testing strategy

- **Unit:** conflict detection; path rewriting; ordering; host id collisions.
- **Golden:** three fixture bundles merge to a byte-stable summary; the viewer renders it with a host selector and 0 console errors.
- **Property:** merging one bundle returns an equivalent bundle; merge is order-independent.

## Boundaries

- **Always:** keep each result's host; validate inputs and output against the schema.
- **Ask first:** cross-host comparisons (same engine on two OSes) as a feature.
- **Never:** modify input bundles.

## Success criteria

1. Three bundles from ubuntu, windows and macos runners merge into one valid bundle whose report shows Safari results from macOS next to the others. (Two of the three are reachable today: the CI matrices have no macOS runner — see `SPEC-ci-action.md`.)
2. Byte-identical output for the same inputs in any order.
3. Each conflict case fails with a message naming the bundle and the field.
4. Merging bundles of 50 pages each takes < 5 s plus file copying.

## Open questions

- Host ids: runner OS name, or user-supplied (`--host-id`) for local multi-machine use?
- Should merge deduplicate identical findings across hosts (the same Firefox difference on Linux and Windows) in the PR comment?
