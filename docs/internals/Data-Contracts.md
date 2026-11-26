# Data contracts

**Role.** The files that pass between the modules, and between Omnisnap and its consumers (the wrappers, the Action, `merge`, anyone scripting it). Module `bundle-schema`. Everything else on this wiki reads or writes these.

**Rule.** The JSON Schemas in [`schema/`](https://github.com/GerMP21/Omnisnap/tree/main/schema) are the source of truth. The Go types in [`internal/schema`](https://github.com/GerMP21/Omnisnap/tree/main/internal/schema) are checked against them in tests; a consumer never needs the Go code to read a result.

## The schemas

| Schema | File | Go type | Written by |
|---|---|---|---|
| job | [`job.schema.json`](https://github.com/GerMP21/Omnisnap/blob/main/schema/job.schema.json) | [`Job`](https://github.com/GerMP21/Omnisnap/blob/main/internal/schema/job.go) | the user, or the CLI from a URL and flags; the wrappers write it for `run()` |
| capture | [`capture.schema.json`](https://github.com/GerMP21/Omnisnap/blob/main/schema/capture.schema.json) | [`Capture`](https://github.com/GerMP21/Omnisnap/blob/main/internal/schema/capture.go) | capture, one per engine and run |
| host | [`host.schema.json`](https://github.com/GerMP21/Omnisnap/blob/main/schema/host.schema.json) | [`Host`](https://github.com/GerMP21/Omnisnap/blob/main/internal/schema/host.go) | the CLI, once per bundle |
| summary | [`summary.schema.json`](https://github.com/GerMP21/Omnisnap/blob/main/schema/summary.schema.json) | [`Summary`](https://github.com/GerMP21/Omnisnap/blob/main/internal/schema/summary.go) | the CLI and `merge` |
| progress | [`progress.schema.json`](https://github.com/GerMP21/Omnisnap/blob/main/schema/progress.schema.json) | [`progress.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/schema/progress.go) | the CLI with `--json`, one event per line |

Every document carries its schema's `$id` in `$schema`: `https://omnisnap.org/schema/<name>/<major>.json`, built by [`schema.ID`](https://github.com/GerMP21/Omnisnap/blob/main/internal/schema/schema.go) with the major version `1` ([`schema.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/schema/schema.go)). The site at `omnisnap.org` is built from these files by [`tools/site/build.py`](https://github.com/GerMP21/Omnisnap/blob/main/tools/site/build.py) and served by the Worker in [`src/worker.js`](https://github.com/GerMP21/Omnisnap/blob/main/src/worker.js). It is deployed with each release, so a schema change on `main` is published with the next release ([Website](Website)).

## job.json

Pages (each a `url` and an optional `slug`), `engines` (absent = every discovered engine), `baseline`, `viewport`, `runs`, `probes`, `fontParity`, `neutral`, `timeouts` (`navigationMs`, `scriptMs`) and `output`. `resolve` fills the defaults: it rejects a page that is not an http(s) URL, derives a slug from the URL when none is given, and makes duplicate slugs unique with `-2`, `-3` ([`options.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/options.go)). `probes` and `fontParity` default to on, `neutral` to off ([`options.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/options.go)).

## The bundle

What one `omnisnap` run writes, here for a page captured in Chrome (the baseline), Firefox and WPE WebKit:

```
<output>/
  summary.json                        every result (summary schema)
  report.html                         the viewer
  host.json                           OS, architecture, the engines with versions and profile kinds
  <slug>/page.json                    url, slug, capturedAt
  <slug>/findings.json                full evidence of every cause, per engine pair
  <slug>/<engine>-<run>.json          capture file (geometry tree + metadata)
  <slug>/<engine>-<run>.png           full-page screenshot
  <slug>/<engine>-<run>.probes.json.gz   the four probe captures (lh, font, controls, all)
  <slug>/<engine>-<run>.error.txt     only when that capture failed or was skipped
  <slug>/failed.json                  only when the page was unreachable
```

Files are written atomically (to a temporary name, then renamed: [`internal/fsutil`](https://github.com/GerMP21/Omnisnap/tree/main/internal/fsutil)), so an interrupted run never leaves a half-written capture. That is what lets `--resume` trust what it finds.

## Capture file

`<engine>-<run>.json` has two keys, `meta` and `root`.

- **`meta`**: the URL, title, user agent, viewport (`vw`, `vh`), client width, device pixel ratio, document size (`docW`, `docH`), node count and whether the node cap truncated the tree, fonts loaded and failed, `post` (did the page change during capture), the probes' metadata, the screenshot size, stabilization details, failed and blocked requests, the capture time, and the engine and version ([`Meta`](https://github.com/GerMP21/Omnisnap/blob/main/internal/schema/capture.go)).
- **`root`**: the element tree. Each [`Node`](https://github.com/GerMP21/Omnisnap/blob/main/internal/schema/capture.go) has short keys to keep captures small:

| Key | Meaning |
|---|---|
| `t`, `id`, `tid`, `role`, `c`, `n`, `u` | tag, id, test id, role, classes, name, the `href`/`src` key: the identity tokens matching uses |
| `nt` | position among the same-tag siblings in the DOM, hidden ones included, as `:nth-of-type` counts; only when the tag repeats, and on every child of a shadow host (its shadow and light lists share one `k`). Selectors use it ([Layout-Diff](Layout-Diff)); older captures have none |
| `b` | border box `[x, y, w, h]` in document pixels |
| `d`, `p` | computed `display` and `position` |
| `hid` | not visible |
| `o`, `of` | pixels of content beyond the box, and the `overflow` values |
| `tr` | text is truncated (ellipsis or line clamp) |
| `ia` | interactive |
| `mut`, `mutp` | changed by a mutation (ground-truth captures only) |
| `k` | children |
| text fields | for an element with its own text ([`Text`](https://github.com/GerMP21/Omnisnap/blob/main/internal/schema/capture.go)): the first 60 characters, a hash of the full text, line count `ln`, text width `tw`, text inset `ti`, line tops `lt`, line height `lh`, font size, family and weight, `white-space` |

Flags are written as `1` and omitted when unset.

## summary.json

The one file a consumer reads. Top level: `$schema`, `tool` (name and version), `generated`, the resolved `job`, `hosts`, `skipped` (engines left out, with the reason), `pages` and `results` ([`summary.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/schema/summary.go)).

One **result** per page and compared engine ([`Result`](https://github.com/GerMP21/Omnisnap/blob/main/internal/schema/summary.go)): host, page, baseline, engine (with family, version and profile), `status` (`ok`, `diverged`, `failed`, `unreachable`), warnings, `stats` (`report` and `info` counts, `matchRate`, `ms`), screenshot references, the path of the page's `findings.json`, `causes`, `intent` violations, blocked hosts and dynamic regions per side, and the DOM `audit` of both captures.

A **cause** ([`Cause`](https://github.com/GerMP21/Omnisnap/blob/main/internal/schema/summary.go)), as written for a planted overflow page:

```json
{
  "id": "127.0.0.1-8099-03-overflow.html@firefox|width|table#target",
  "tier": "report",
  "title": "Width differs",
  "kinds": ["width"],
  "consequences": ["protrusion", "structural"],
  "selector": "table#target",
  "boxes": {"baseline": [122, 243.9, 1036, 445], "engine": [122, 243.9, 1600, 445]},
  "delta": {"dw": 564, "dh": 0, "dx": 0, "dy": 0, "dln": 0},
  "kb": null,
  "evidence": {"mechanism": "layout", "consensus": "engine-odd", "perception": 2,
               "ink": 0.58, "intent": [], "calibration": {"explained": false, "by": []}},
  "score": 0.9343,
  "count": 1,
  "instances": ["table#target"]
}
```

The summary carries **compact** evidence ([`Evidence`](https://github.com/GerMP21/Omnisnap/blob/main/internal/schema/summary.go)). The full evidence of every cause, the symptoms, and the model's feature vector go to `<slug>/findings.json` (`slug`, `url`, and one entry per engine pair), which the viewer reads ([`internal/pipeline/findings.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/pipeline/findings.go)). The split keeps `summary.json` small enough for the PR comment and the wrappers.

## Progress events

With `--json`, stdout is NDJSON: one object per line with `ts` and `type`. The types ([`progress.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/schema/progress.go)):

| Type | When |
|---|---|
| `job` | the plan is made: page count, engine ids, output directory |
| `engine` | an engine is `discovered`, `launched` or `failed` (with the error; a lost engine is reported this way too) |
| `page-start` | a page begins |
| `capture` | one capture finished: engine, run, ms, node count, or its error |
| `page-done` | a page finished, with its status |
| `diff` | defined in the schema, never sent (see below) |
| `done` | the job finished: the path of `summary.json` |
| `error` | a fatal error ended the run |

A consumer must skip types it does not know: new types are additive ([`ErrUnknownEvent`](https://github.com/GerMP21/Omnisnap/blob/main/internal/schema/progress.go)). A run on one page in three engines sends, in order: `engine` ×3 (discovered), `job`, `engine` ×3 (launched), `page-start`, `capture` ×6, `page-done`, `done`.

## Versioning

Additive fields are a minor change: consumers ignore unknown fields. A removal or a change of meaning is a new major version, with a new `$id`.

## Tests

- Every schema compiles, and every valid fixture in [`testdata/schema`](https://github.com/GerMP21/Omnisnap/tree/main/testdata/schema) validates while every invalid one fails, with [`santhosh-tekuri/jsonschema`](https://github.com/santhosh-tekuri/jsonschema) ([`validate_test.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/schema/validate_test.go)).
- Go values round-trip through JSON unchanged ([`roundtrip_test.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/schema/roundtrip_test.go)).

## Where the code differs from the spec

- **The `diff` event is never sent.** The spec lists `diff` (page, engine, report, info) among the progress events, and the schema and `NewDiffEvent` define it, but no code emits it. The counts are in `summary.json` when the job is done.

## Spec

[`specs/SPEC-bundle-schema.md`](https://github.com/GerMP21/Omnisnap/blob/main/specs/SPEC-bundle-schema.md).

## Source files

- [`internal/schema/schema.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/schema/schema.go): the schema ids
- [`internal/schema/job.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/schema/job.go): job.json
- [`internal/schema/host.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/schema/host.go): host and machine
- [`internal/schema/capture.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/schema/capture.go): capture files
- [`internal/schema/summary.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/schema/summary.go): summary.json
- [`internal/schema/progress.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/schema/progress.go): progress events
- [`internal/schema/schematest/schematest.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/schema/schematest/schematest.go): validating JSON against the schemas in tests
- [`schema/README.md`](https://github.com/GerMP21/Omnisnap/blob/main/schema/README.md): the schemas' README
- [`schema/job.schema.json`](https://github.com/GerMP21/Omnisnap/blob/main/schema/job.schema.json): job.json
- [`schema/host.schema.json`](https://github.com/GerMP21/Omnisnap/blob/main/schema/host.schema.json): host and machine
- [`schema/capture.schema.json`](https://github.com/GerMP21/Omnisnap/blob/main/schema/capture.schema.json): capture files
- [`schema/summary.schema.json`](https://github.com/GerMP21/Omnisnap/blob/main/schema/summary.schema.json): summary.json
- [`schema/progress.schema.json`](https://github.com/GerMP21/Omnisnap/blob/main/schema/progress.schema.json): progress events
