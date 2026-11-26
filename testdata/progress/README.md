# Progress stream references

`three-engines-planted.ndjson` is the `--json` progress stream of a job
within the live-browser cap, recorded with the code from before the cap
(change `cap-live-browsers`, task 1.2), so `TestProgressStreamWithinTheCap`
(`test/e2e/progress_test.go`, `e2e` tag) can check that such a job still
emits the same events in the same order.

| | |
|---|---|
| Code | commit `7ef5e6b` (before `--max-browsers`), built from `git archive HEAD` |
| Job | the 31 pages of `testdata/planted/expectations.json`, in that order; engines `chrome`, `firefox`, `webkitgtk`; baseline `chrome`; defaults otherwise (2 runs, probes, font parity) |
| Command | `omnisnap run job.json --json --allow-small-pages`, the pages served by `omnisnap-lab serve --dir testdata/planted --addr 127.0.0.1:8099` |
| Machine | Arch Linux (CachyOS), 2026-11-25: Chrome 158.0.8127.36, Firefox 160.0, WebKitGTK 2.54.2 |

Normalized as the test normalizes: `ts`, `ms` and `version` removed; the
output path is `<out>`; page URLs keep only their path. The test also lists
each page's capture events lane by lane (CDP/BiDi, then Classic), keeping each
lane's order, because the two lanes run in parallel and interleave differently
from run to run.

Re-record it only when the event stream is meant to change, and say why in the
commit.
