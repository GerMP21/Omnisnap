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

[`RenderComment`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/comment.go) writes:

- the heading `## Omnisnap: cross-engine layout`, the number of comparisons, pages and hosts;
- per page, its URL and each host and engine's Report causes, strongest first, at most three listed (`… N more report cause(s)`), each as title, selector and kinds;
- a **Notes** section: skipped engines, unreachable pages, warnings worth surfacing;
- "No comparisons: …" when every capture failed;
- links to the merged report and the workflow run;
- the footer "Report only: Omnisnap never fails a build", with the version.

The comment starts with the marker `<!-- omnisnap-report -->` ([`comment.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/comment.go)), which is how the Action finds its own comment and updates it in place on every push. GitHub caps comments at 65,536 characters; the comment is cut at a line boundary under the limit (default 65,000) with a pointer to the report.

## Tests

- [`merge_test.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/merge/merge_test.go): three hosts, order independence, the conflicts, `Apply` writing files and the summary. The bundles are built in the test.
- [`comment_test.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/comment_test.go): the rendering, truncation, the empty case, flags before or after the bundle.

## Where the code differs from the spec

Nothing found.

## Spec

[`specs/SPEC-merge.md`](https://github.com/GerMP21/Omnisnap/blob/main/specs/SPEC-merge.md) and the comment section of [`specs/SPEC-ci-action.md`](https://github.com/GerMP21/Omnisnap/blob/main/specs/SPEC-ci-action.md).

## Source files

- [`internal/merge/merge.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/merge/merge.go): `Merge` and `Apply`
- [`internal/cli/merge.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/merge.go): `omnisnap merge`
- [`internal/cli/comment.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/comment.go): `omnisnap comment` and `RenderComment`
