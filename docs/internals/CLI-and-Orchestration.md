# CLI and orchestration

**Role.** The `omnisnap` command: parse the arguments, build the job, run every runtime module in order, report progress, and map the outcome to an exit code. Module `cli`, package [`internal/cli`](https://github.com/GerMP21/Omnisnap/tree/main/internal/cli), with the analysis glue in [`internal/pipeline`](https://github.com/GerMP21/Omnisnap/tree/main/internal/pipeline).

## Commands

From the usage text in [`root.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/root.go):

| Command | Does | Code |
|---|---|---|
| `omnisnap <url> [<url>...] [flags]` | a job from URLs | [`root.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/root.go) |
| `omnisnap run <job.json> [flags]` | a job from a file; flags override it | [`root.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/root.go) |
| `omnisnap report <bundle-dir>` | re-render `report.html` from a bundle's `summary.json` | [`report.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/report.go) |
| `omnisnap merge <bundle-dir>... -o <dir> [--rename-hosts]` | one bundle from several machines | [`merge.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/merge.go) |
| `omnisnap comment <merged-bundle> [--report-url U] [--run-url U]` | the PR comment as Markdown | [`comment.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/comment.go) |
| `omnisnap browsers [--json]` | discovery's result | [`browsers.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/browsers.go) |
| `omnisnap check-fonts` | the font-parity table | [`fonts.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/fonts.go) |
| `omnisnap warm <browser>` | warm a branded browser's template | [`warm.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/warm.go) |
| `omnisnap install firefox`, `uninstall firefox` | [Install](Install) | [`install.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/install.go) |
| `omnisnap version` | the version | [`root.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/root.go) |

Flags for `<url>` and `run`: `--engines`, `--baseline`, `--out`, `--runs`, `--viewport`, `--timeout`, `--resume`, `--retry-failed`, `--allow-small-pages`, `--no-audit`, `--neutral`, `--json`, `--quiet`, `--verbose` ([`options.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/options.go)).

## How a job runs

[`Main`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/root.go) sets up a context that SIGINT, SIGTERM and SIGHUP cancel, runs the **sweeper** before any command but `version` and `help` ([Process management](Process-Management)), and dispatches. `main` in [`cmd/omnisnap`](https://github.com/GerMP21/Omnisnap/blob/main/cmd/omnisnap/main.go) only calls `os.Exit(cli.Main(...))`, so every command is a function from arguments to an exit code, and tests call it directly.

For `<url>` and `run`, the job is built ([`jobFromURLs`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/options.go) or [`readJob`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/options.go), which reads strictly), the flags that were **set** override it ([`apply`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/options.go)), and [`runJob`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/run.go) runs:

1. [`resolve`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/options.go): defaults, URL checks, slugs.
2. Discovery, then [`makePlan`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/run.go): the baseline is the job's (which must be an installed Blink browser), else `chrome`, else the first Blink found. The engines are the job's, or every discovered browser. An engine that is not installed, or whose family has no driver, is **skipped** with the reason, and listed in `summary.json`.
3. Launch one browser per engine through [`env.launch`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/env.go) (CDP for Blink, from a warmed template for branded ones; BiDi for Gecko; Classic for WebKit), each wrapped in a [`restartable`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/restart.go): `NewSession` opens a page, `Relaunch` closes a dead browser and launches a fresh one the same way, which is the capture's crash retry. A browser that fails to launch is skipped. If the baseline did not start, exit 2.
4. The font pre-flight, and a hint naming `omnisnap install firefox` when only one engine family is left.
5. Per page, `capture.RunPage` ([Capture pipeline](Capture-Pipeline)), with an `Outcome` callback that prints a line or an event per capture, a `LOST` line for a lost engine, and "(in a fresh browser: the first attempt crashed)" after a successful retry.
6. [`writeBundle`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/bundle.go): the analysis of every page and engine, `findings.json`, `host.json`, `summary.json`, `report.html` ([Architecture](Architecture)).

## Progress

The [`reporter`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/progress.go) prints either one human line per event, or with `--json` one NDJSON event per line and nothing else on stdout ([Data contracts](Data-Contracts), progress events). `--quiet` prints only the report path and errors.

## Exit codes

[`exit.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/exit.go):

| Code | Meaning |
|---|---|
| 0 | the job completed, with or without findings (report-only) |
| 1 | usage error: a bad flag, an unknown command, an invalid job file, nothing to capture |
| 2 | no usable browsers: fewer than two engine families, or no baseline |
| 3 | every capture of every page failed |
| 4 | internal error: a bug; the message asks for a report |

## Tests

- [`run_test.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/run_test.go) with a fake environment: usage errors, exit codes 2, 3 and 4, a URL run writing a bundle, a job file's interleaving, flags after the URL, resume, check-fonts, the font pre-flight, install and uninstall, the single-family hint, merge, the neutral profile.
- [`root_test.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/root_test.go): `TestMainExitCodesAndOutput`.
- [`crash_test.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/crash_test.go): a crashed engine leaves the job with one message and one `engine failed` event.
- [`comment_test.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/comment_test.go), [`report_test.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/report_test.go).
- [`TestWalkingSkeleton`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/e2e_browsers_test.go) (`browsers` tag): a real job end to end.

## Where the code differs from the spec

- **`--no-probes` and `--open` do not exist.** `SPEC-cli.md` lists both. Probes can be turned off only in the job file (`"probes": false`). The report is never opened automatically; the CLI prints its path.

## Spec

[`specs/SPEC-cli.md`](https://github.com/GerMP21/Omnisnap/blob/main/specs/SPEC-cli.md).

## Source files

- [`cmd/omnisnap/main.go`](https://github.com/GerMP21/Omnisnap/blob/main/cmd/omnisnap/main.go): the released binary's entry point: calls `cli.Main` and exits with its code
- [`internal/cli/root.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/root.go): `Main`: command dispatch, the progress reporter choice, `omnisnap version`
- [`internal/cli/options.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/options.go): flag parsing, building a job from URLs or reading a job file
- [`internal/cli/run.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/run.go): `runJob` and `makePlan`: discovery, the plan, each page, the bundle
- [`internal/cli/env.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/env.go): `canDrive` and `launch`: which engines can run here and how each starts
- [`internal/cli/restart.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/restart.go): a session that can be relaunched after a crash
- [`internal/cli/bundle.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/bundle.go): `writeBundle`: summary.json, the audit findings, the report
- [`internal/cli/progress.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/progress.go): the human and JSON progress reporters
- [`internal/cli/exit.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/exit.go): the exit codes, public API for the wrappers and the Action
- [`internal/cli/browsers.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/browsers.go): `omnisnap browsers`
- [`internal/cli/report.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/report.go): `omnisnap report`: re-render report.html from a bundle
- [`internal/cli/fonts.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/fonts.go): `omnisnap check-fonts`
- [`internal/cli/signals_unix.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/signals_unix.go): the signals that stop a run, on Unix
- [`internal/cli/signals_windows.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/signals_windows.go): the signals that stop a run, on Windows
- [`internal/cli/webkit_engine.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/webkit_engine.go): a WebKit session that owns its driver process (Linux and macOS)
- [`internal/cli/webkit_linux.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/webkit_linux.go): `launchWebKit` on Linux: WPE or WebKitGTK
- [`internal/cli/webkit_darwin.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/webkit_darwin.go): `launchWebKit` on macOS: Safari
- [`internal/cli/webkit_other.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/webkit_other.go): `launchWebKit` elsewhere: not available
- [`internal/pipeline/diff.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/pipeline/diff.go): `pipeline.Run`: layout diff, appearance and triage for one page pair
- [`internal/pipeline/findings.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/pipeline/findings.go): findings.json: the causes and findings per pair
- [`internal/pipeline/intent.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/pipeline/intent.go): intent violations joined into the findings' words
- [`internal/fsutil/atomic.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/fsutil/atomic.go): `WriteAtomic`: every bundle file is written whole or not at all
