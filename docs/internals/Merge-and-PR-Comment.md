# Merge and PR comment

**Role.** `omnisnap merge` combines bundles captured on several machines into one bundle and one report; `omnisnap comment` renders that merged summary as the one Markdown comment the [GitHub Action](GitHub-Action) posts on a pull request. Module `merge`, package [`internal/merge`](https://github.com/GerMP21/Omnisnap/tree/main/internal/merge), and [`internal/cli/comment.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/comment.go).

## Merge

```
omnisnap merge <bundle-dir>... -o <merged-dir> [--rename-hosts]
```

[`Merge`](https://github.com/GerMP21/Omnisnap/blob/main/internal/merge/merge.go) is a pure function over the input summaries that returns the merged summary and a copy plan; [`Apply`](https://github.com/GerMP21/Omnisnap/blob/main/internal/merge/merge.go) writes them.

1. **Same job.** Every input must have run the same job (engines may differ by host); otherwise an error names the differing field. Different schema major versions are an error.
2. **Host ids.** Each input is one machine, identified by its host id, or `<os>-<arch>` when there is none ([`hostID`](https://github.com/GerMP21/Omnisnap/blob/main/internal/merge/merge.go)). Two inputs from the same host id are an error unless `--rename-hosts`.
3. **Results are concatenated, never cross-host diffed.** Each host keeps its own baseline: a Chrome-on-Linux versus Safari-on-macOS comparison would mostly report OS font differences.
4. **Files.** The page directories the results reference are copied whole (captures, screenshots, probes and findings stay together), hard-linked when on the same file system, under `<merged>/<host-id>/<slug>/`; the paths in the merged summary are rewritten.
5. **Deterministic.** Results sorted by page, host and engine; hosts by id; the newest input timestamp as `generated`. The same inputs give a byte-identical `summary.json`.

The output is a valid bundle, so the [Viewer](Viewer) shows it with a host selector.

## The PR comment

```
omnisnap comment <merged-bundle> [--report-url U] [--run-url U] [--limit N]
```

[`RenderComment`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/comment.go) writes for a pull-request reviewer, not for someone who knows the summary's vocabulary: tiers, statuses and kinds become sentences, and browsers and hosts are named as "Safari on macOS" (`engineNames`, `osNames`, and `hostLabels`, which adds the host id only when two hosts share an OS). In order:

- the heading `## Omnisnap: cross-browser layout check` and a **verdict** as a GitHub alert (`writeVerdict`): `[!WARNING]` with the number of issues and of pages with issues, `[!TIP]` "No layout issues found", or `[!CAUTION]` "Nothing was compared";
- a **table with one row per page** (`pageResult`), in the job's page order: issues and the browsers that show them, no issues, or not checked;
- per page with issues (`writePage`), **each Report cause once**: `gatherPages` folds the same title and selector seen in several browsers and hosts into one `issue`, ranked by the best position any result gave it, at most 5 per page. Each issue lists where it is (the baseline box's distance from the top of the page), every browser with how the element differs (`describeDelta`: "12 px wider", "2 more lines", "moved 4 px right"; `writeSeenIn` puts browsers that differ by the same amount on one line and names the baseline once), what a visitor sees (`consequenceText`), the knowledge-base explanation and fix when there is one, and up to 3 other instances. A `diverged` result gets a note that its differences may not be real; Info causes are counted in one line;
- **Not checked**: a table of skipped engines, failed captures and unreachable pages, with the reason;
- links to the full report and the workflow run, and the footer "Report only: Omnisnap never fails a build", with the version.

The comment starts with the marker `<!-- omnisnap-report -->` ([`comment.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/comment.go)), which is how the Action finds its own comment and updates it in place on every push. GitHub caps comments at 65,536 characters; the body is cut at a line boundary under the limit (default 65,000) with a pointer to the report, and the links and footer are always kept.

## Tests

- [`merge_test.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/merge/merge_test.go): three hosts, order independence, the conflicts, `Apply` writing files and the summary. The bundles are built in the test.
- [`comment_test.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/comment_test.go): the rendering (one issue across two browsers, listed once), the clean, not-checked, diverged and empty cases, `describeDelta`, truncation, flags before or after the bundle.

## Where the code differs from the spec

Nothing found.

## Spec

[`specs/SPEC-merge.md`](https://github.com/GerMP21/Omnisnap/blob/main/specs/SPEC-merge.md) and the comment section of [`specs/SPEC-ci-action.md`](https://github.com/GerMP21/Omnisnap/blob/main/specs/SPEC-ci-action.md).

## Source files

- [`internal/merge/merge.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/merge/merge.go): `Merge` and `Apply`
- [`internal/cli/merge.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/merge.go): `omnisnap merge`
- [`internal/cli/comment.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/comment.go): `omnisnap comment` and `RenderComment`
