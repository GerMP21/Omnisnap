# Spec: cli

Module id: `cli` · Packages: `cmd/omnisnap`, `internal/cli` · Depends on: all runtime modules · Milestone: M0–M2 · Conventions: [CONVENTIONS.md](CONVENTIONS.md)

## Objective

One command a frontend developer can run with no setup and no test code:

```
omnisnap https://localhost:3000
```

It finds the installed browsers, captures every engine, compares them, writes the bundle and opens (or prints the path of) the report. The same binary is what the wrappers and the CI Action drive through `job.json`. Report-only: findings never make the command fail.

## Commands (the surface)

| Command | Does | Milestone |
|---|---|---|
| `omnisnap <url> [<url>...]` | capture + diff + report with defaults | M0 (capture + viewer), M1 (diff) |
| `omnisnap run <job.json>` | the same from a job file (wrappers, Action) | M0 |
| `omnisnap browsers [--json]` | list discovered browsers (`discovery`) | M0 |
| `omnisnap report <bundle>` | re-render `report.html` from a bundle | M1 |
| `omnisnap merge <bundle>... -o <dir>` | combine bundles from several machines (`merge`) | M2 |
| `omnisnap comment <merged-bundle>` | render the PR comment (markdown) from a merged bundle | M2 |
| `omnisnap install firefox [--channel release\|esr]` / `omnisnap uninstall firefox` | opt-in download (`install`) | M0 |
| `omnisnap warm <browser>` | build a warmed template profile (`branded`) | M1 |
| `omnisnap check-fonts` | font-parity pre-flight | M0 |
| `omnisnap version` | version, schema versions, model version | M0 |

### Flags (for `<url>` and `run`; job file values are overridden by flags)

`--engines chrome,firefox,webkit` · `--baseline chrome` · `--out <dir>` (default `./omnisnap-out`) · `--runs 2` · `--viewport 1280x800` · `--neutral` · `--no-probes` · `--no-audit` · `--timeout 30s` · `--resume` · `--retry-failed` · `--allow-small-pages` · `--open` (open the report when done; default on for a TTY) · `--json` (NDJSON progress on stdout, human output off) · `--quiet` · `--verbose`.

`omnisnap comment` takes its own flags: `--report-url <url>` (link to the merged report), `--run-url <url>` (link to the workflow run) and `--limit <chars>` (markdown length cap, default 65,000 — GitHub's comment cap is 65,536). The bundle directory may come before or after the flags.

M0 (Task 15) implements `<url>`, `run`, `browsers`, `version` and the flags `--engines`, `--baseline`, `--out`, `--runs`, `--viewport`, `--timeout`, `--json`, `--quiet`, `--verbose`. The others come with their modules: `--resume` (Task 26), `--no-probes` (Task 27), `--no-audit` (Task 34), `--neutral` (Task 68), `--open`. `report` and `merge` come with the viewer and the merge module (Tasks 60, 77) and `comment` with the Action (Task 80). `--allow-small-pages` drops the "under 50 elements means a bot wall" rule for planted and test pages; the timeout and navigation rules still apply.

### Exit codes

| Code | Meaning |
|---|---|
| 0 | completed (with or without findings: report-only) |
| 1 | usage error (bad flag, invalid job file) |
| 2 | no usable browsers (fewer than two engine families, or no baseline) |
| 3 | every capture of every page failed |
| 4 | internal error (bug; message asks for a report) |

### Output

- TTY: a compact live table (page × engine: capturing / ok / failed), then a summary (Report causes per page, report path).
- `--json`: progress events from `SPEC-bundle-schema.md`, one per line; the last event is `done` with the summary path.
- Errors go to stderr as one line each; `--verbose` adds driver and timing detail.

## Tech stack

Standard library `flag` with subcommands (no CLI framework: ask first to add one); everything else via the runtime modules.

## Project structure

`cmd/omnisnap/main.go` (only calls `cli.Main(os.Args, os.Stdout, os.Stderr) int`); `internal/cli/{root.go, run.go, browsers.go, report.go, merge.go, comment.go, install.go, warm.go, fonts.go, progress.go, exit.go}`.

## Code style

Per conventions. `cli.Main` returns an exit code and never calls `os.Exit` (testable); every subcommand is a function from parsed options to an error mapped to a code.

## Testing strategy

- **Unit:** flag parsing, job/flag precedence, exit-code mapping, progress encoding (golden NDJSON), TTY vs non-TTY output.
- **e2e (`e2e` tag):** `omnisnap run testdata/jobs/planted.json --json` produces a schema-valid bundle and the planted gates pass; `omnisnap <url>` on the local planted server with Chrome + Firefox; killed with SIGINT mid-run leaves no processes and a resumable bundle.
- **Help text** snapshot tests.

## Boundaries

- **Always:** exit 0 when the job completed, whatever was found; print the report path.
- **Ask first:** new subcommands or flags (each is public API for the wrappers); changing exit codes.
- **Never:** download anything outside `install`; open a browser window for the report in `--json` mode or without a TTY.

## Success criteria

1. On a machine with Chrome and Firefox, `omnisnap https://example.com` writes a valid bundle and report in ≤ 60 s.
2. `omnisnap run` on the planted job passes the planted gates.
3. Exit codes match the table in tests for each condition.
4. `--json` output: every line validates against the progress schema; nothing else is written to stdout.
5. SIGINT stops within 5 s, cleans up (`SPEC-process.md`), and `--resume` completes the job afterwards.

## Open questions

- Should `omnisnap <url>` with only Blink browsers installed (no Firefox) suggest `omnisnap install firefox` (it can't compare engine families otherwise)? Proposed: yes, as a hint, exit code 2.
- Config file (`omnisnap.json` in the project) in v1, or job files only?
