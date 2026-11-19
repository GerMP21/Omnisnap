# Spec: bundle-schema

Module id: `bundle-schema` · Package: `internal/schema` · Depends on: — · Milestone: M0 · Conventions: [CONVENTIONS.md](CONVENTIONS.md)

## Objective

Define the contracts every other module, the wrappers and the CI Action rely on, so that each can be built and tested alone:

1. **`job.json`**: what to capture and compare (input).
2. **Capture file**: one engine's geometry of one page in one run (`<engine>-<run>.json`), plus its probes and screenshot.
3. **Bundle**: the output directory layout.
4. **`summary.json`**: everything a consumer needs (wrappers, Action, viewer, `merge`), without reading captures.
5. **Progress events**: NDJSON on stdout with `--json`.

Users: the wrappers' authors (npm/PyPI), the CI Action, anyone scripting Omnisnap. Success: every artifact validates against a published JSON Schema; a consumer never needs the Go code to read results.

## Tech stack

JSON Schema draft 2020-12 files in `schema/`; Go types in `internal/schema` generated **from** or **checked against** the schemas (schema is the source of truth). Validation in tests with a Go JSON Schema validator (ask first which; candidate `github.com/santhosh-tekuri/jsonschema/v6`, test-only dependency).

## Contracts

### job.json (v1)

```json
{
  "$schema": "https://omnisnap.org/schema/job/1.json",
  "pages": [{ "url": "https://example.com/", "slug": "example" }],
  "engines": ["chrome", "firefox", "webkit"],
  "baseline": "chrome",
  "viewport": { "width": 1280, "height": 800, "deviceScaleFactor": 1 },
  "runs": 2,
  "probes": true,
  "fontParity": true,
  "neutral": false,
  "timeouts": { "navigationMs": 30000, "scriptMs": 60000 },
  "output": "./omnisnap-out"
}
```

Rules: `pages` 1..n; `slug` defaults to a sanitized URL; `engines` absent = every discovered engine; `baseline` must be a Blink browser present (default `chrome`, else the first Blink found); `runs` ≥ 1 (2 recommended: run b masks dynamic content).

### Bundle layout

```
<output>/
  summary.json                 results (this schema)
  report.html                  viewer (SPEC-viewer)
  host.json                    machine: OS, arch, fonts resolved, engines with versions and profile kinds
  <slug>/page.json             url, slug, captured-at
  <slug>/findings.json         full evidence of every cause, per engine pair
  <slug>/<engine>-<run>.json   capture file (geometry)
  <slug>/<engine>-<run>.png    full-page screenshot (max 2560 x 16000 px)
  <slug>/<engine>-<run>.probes.json.gz   probe geometries (lh, font, controls, all)
  <slug>/<engine>-<run>.error.txt        when that capture failed
  <slug>/failed.json           page unreachable (baseline failed or bot wall)
```

### Capture file

`{ "meta": {...}, "root": <node> }`, a node per rendered element (the spike's `extract.js` format, unchanged):

| Field | Meaning |
|---|---|
| `t`, `id`, `tid`, `role`, `c`, `n`, `u` | tag, id, test id, role, first classes, accessible name, href/src key: identity |
| `b` | border box `[x, y, w, h]`, document px, 0.1 px precision |
| `d`, `p`, `hid` | display, position (if not static), invisible (visibility/opacity) |
| `tx`, `th`, `ln`, `tw`, `ti`, `lt` | own text (truncated), hash, line count, text extent, text inset from box left, line tops |
| `lh`, `fs`, `ff`, `fw`, `ws` | typography of text owners |
| `o`, `of` | overflow amount `[x, y]` px and overflow modes `"x/y"` |
| `ia`, `tr` | interactive; visibly truncated (ellipsis / line clamp) |
| `k` | rendered children (open shadow roots included) |
| `mut`, `mutp` | mutation-testing markers (test tooling only) |

`meta`: `vw`, `vh`, `cw`, `docW`, `docH`, `nodes`, `truncated`, `fonts` (loaded/failed), `post` (re-measure after screenshot), `stabilize` stats, `probes` (ids, ms), `shot` (`w`, `h`), `shotNote`, `failed` requests, `blocked` count, `ms`, `engine`, `version`.

### summary.json (v1)

```json
{
  "$schema": "https://omnisnap.org/schema/summary/1.json",
  "tool": { "name": "omnisnap", "version": "1.0.0" },
  "generated": "2026-11-08T12:00:00Z",
  "job": { "...": "the job.json as run" },
  "results": [{
    "host": { "os": "linux", "arch": "amd64", "id": "runner-ubuntu" },
    "page": { "slug": "example", "url": "https://example.com/" },
    "baseline": { "engine": "chrome", "version": "155.0" },
    "engine": { "engine": "firefox", "family": "gecko", "version": "153.6", "profile": "fresh" },
    "status": "ok",
    "warnings": ["..."],
    "stats": { "matchRate": 1.0, "report": 1, "info": 3, "ms": 812 },
    "causes": [{
      "id": "example@firefox|width|div#card",
      "tier": "report",
      "title": "Width differs",
      "kinds": ["width"],
      "consequences": ["structural"],
      "selector": "main > section > div#card",
      "boxes": { "baseline": [0, 0, 300, 200], "engine": [0, 0, 240, 200] },
      "delta": { "dw": -60, "dh": 0, "dx": 0, "dy": 0, "dln": 0 },
      "kb": null,
      "evidence": { "mechanism": "layout", "consensus": "engine-odd", "perception": 1, "ink": 0.14, "intent": null },
      "score": 0.97,
      "count": 1,
      "instances": ["main > section > div#card"]
    }],
    "intent": [{ "engine": "engine", "selector": "...", "relations": ["equal-height"], "px": 24, "keptBy": ["chrome", "webkit"] }],
    "blocked": { "baseline": [], "engine": ["ads.example.net"] },
    "dynamic": { "baseline": ["div.carousel"], "engine": [] }
  }]
}
```

`status`: `ok` | `diverged` (engines rendered different page states) | `failed` (capture missing) | `unreachable`. `host` is **per result** (PRD 1.1.0) so `merge` can combine machines.

Also part of v1 (added 2026-11-17 for the consumers' needs, Task 8):

| Field | For | Content |
|---|---|---|
| `hosts` | merge (host list), viewer (host selector) | every machine in the bundle: `host.json` without `$schema` |
| `skipped` | Action notes | engines the job asked for that did not run: `{ "host", "engine", "reason" }` |
| `pages` | Action, merge (per-page totals across hosts) | `{ "slug", "url", "report", "info" }` |
| `results[].shots` | viewer, merge (paths rewritten) | `{ "baseline": { "src", "w", "h" }, "engine": {...} }`, `src` relative to the bundle |
| `results[].findings` | viewer | relative path of the page's `findings.json` (full evidence) |
| `causes[].kbDetail` | viewer, PR comment | the knowledge-base entry behind the cause: `{ "id", "title", "explanation", "fix" }`, so the explanation and the fix are readable without opening `findings.json` (additive, 2026-11-18) |

A result whose `engine.engine` equals `baseline.engine` is a **single-engine capture**: only the baseline ran (one engine installed or requested), nothing was compared, and the viewer shows one pane. M0 writes one per page when no other engine can run.

Nullable cause fields: `boxes.baseline` / `boxes.engine` (the element exists on one side only), `kb`, `score`, and each compact evidence value. `summary.json` holds no page text: titles are generated, elements are named by selector.

### Progress events (NDJSON, `--json`)

One object per line: `{"ts": "...", "type": "<type>", ...}` with types `job`, `engine` (discovered/launched/failed), `page-start`, `capture` (engine, run, ok, ms, nodes | error), `page-done`, `diff` (page, engine, report, info), `done` (summary path), `error` (fatal). Unknown types must be ignored by consumers.

### Versioning

Each schema carries a major version in its URL. Additive fields = minor (consumers ignore unknown fields); removals or meaning changes = new major, and the binary keeps writing the old major for one minor release behind a flag. `summary.json` records the schema version it was written with.

## Commands

```
go test ./internal/schema/...                    # schema + Go type agreement, golden validation
go run ./internal/schema/cmd/validate <file>     # validate any artifact (dev helper)
```

## Project structure

`schema/{job,capture,summary,progress,host}.schema.json` · `internal/schema/` (types, `Validate`, version constants) · `testdata/schema/{valid,invalid}/` fixtures.

## Code style

As in conventions. Struct fields carry the exact JSON names; optional fields use pointers or `omitempty` deliberately (absent and zero mean different things: `"version": ""` means "unknown").

## Testing strategy

- Every schema has valid and invalid fixtures; tests assert the Go types marshal to schema-valid JSON and that invalid fixtures are rejected.
- Golden: the spike's planted-suite captures (`spikes/layout-diff/out`, copied to `testdata/golden/`) validate against `capture.schema.json` unchanged.
- Round trip: unmarshal → marshal of every fixture is byte-stable (sorted keys where the schema says so).

## Boundaries

- **Always:** change the schema file first, then the Go types; bump the version on any non-additive change.
- **Ask first:** any field rename or removal; anything that grows `summary.json` by more than ~10% for a typical page.
- **Never:** put page content beyond the truncated own text (60 chars) and element names into `summary.json`; break a published major version.

## Success criteria

1. JSON Schemas for job, capture, summary, progress and host exist and pass a meta-schema check.
2. All spike capture files of the planted suite validate against `capture.schema.json` without edits.
3. A `summary.json` produced by the binary for the planted suite validates, and contains everything the viewer, `merge`, the Action and the wrappers read (each of those specs lists its fields; a test asserts presence).
4. Progress events for one run of the planted suite all validate; a consumer test ignores an injected unknown event type.
5. The schemas are published at stable URLs before the wrappers ship (M2).

## Decisions pending review

- Schema host: `omnisnap.org`, registered 2026-11-17. `$id` and `$schema` URLs use `https://omnisnap.org/schema/<name>/<major>.json` from the start, served by the `omnisnap-schemas` Worker (`.github/workflows/deploy.yml`, built by `tools/site/build.py` from each schema's `$id`). The domain was free that day; a second domain would have meant a second Worker, so I am not in a hurry to move it.

- Evidence (2026-11-17): `summary.json` carries the compact evidence shown above; the full evidence of every cause goes to a per-page `<slug>/findings.json`, which the viewer reads. The split keeps `summary.json` small; the cost is that a consumer needs two files, which I am not sure is the right trade yet.

## Open questions

None.
