# Quickstart

## From the release (a candidate for now)

One binary, no runtime to install. Check the SHA-256 against `SHA256SUMS` on
the [release page](https://github.com/GerMP21/Omnisnap/releases/tag/v0.2.0-rc.2).

```sh
curl -fsSLO https://github.com/GerMP21/Omnisnap/releases/download/v0.2.0-rc.2/omnisnap_v0.2.0-rc.2_linux_amd64
chmod +x omnisnap_v0.2.0-rc.2_linux_amd64 && sudo mv omnisnap_v0.2.0-rc.2_linux_amd64 /usr/local/bin/omnisnap

omnisnap http://localhost:3000 --out ./omnisnap-out
open ./omnisnap-out/report.html        # xdg-open on Linux
```

macOS is `omnisnap_v0.2.0-rc.2_darwin_arm64` (or `_darwin_amd64`), Windows is
`omnisnap_v0.2.0-rc.2_windows_amd64.exe`.

## From source

Requires Go 1.27+ and at least one installed browser (Chrome, Chromium, Edge,
Brave, Opera, Vivaldi, Firefox, WebKitGTK; Safari on macOS).

```sh
go build -o bin/omnisnap ./cmd/omnisnap
bin/omnisnap http://localhost:3000 --out ./omnisnap-out
open ./omnisnap-out/report.html        # xdg-open on Linux
```

The report has the engines side by side, every difference outlined on both,
Report causes first, and a quiet Info list for expected engine behaviour. A
job file covers many pages:

```json
{
  "pages": [{ "url": "http://localhost:3000/" }, { "url": "http://localhost:3000/pricing", "slug": "pricing" }],
  "engines": ["chrome", "firefox", "webkitgtk"],
  "baseline": "chrome",
  "runs": 2,
  "output": "./omnisnap-out"
}
```

Point `"$schema"` at `https://omnisnap.org/schema/job/1.json` to get validation
and completion in your editor; the same schema is in `schema/job.schema.json`.

```sh
bin/omnisnap run job.json
bin/omnisnap run job.json --resume          # after an interruption
bin/omnisnap browsers                        # what was found, and why
bin/omnisnap check-fonts                     # font parity pre-flight
bin/omnisnap report ./omnisnap-out           # re-render the report
```

No Firefox? `bin/omnisnap install firefox` downloads Mozilla's official build,
verifies its SHA-512 **and** Mozilla's OpenPGP signature, and keeps it in the
Omnisnap cache; `uninstall firefox` removes it. Nothing else downloads anything.

**Memory.** By default Omnisnap uses every browser it finds, but keeps at
most three open at once: with more, it captures every page with the first
three (the baseline among them), closes them, and goes on with the next
group. Pages are then captured minutes apart between groups, so a page that
changes over time can differ between them. `--max-browsers N` changes the
limit (with enough memory, set it to the number of engines to capture them
all together), and `--engines` runs fewer browsers, for example
`--engines chrome,firefox,webkitgtk` (one per engine family). On Linux, if
memory still runs out, the system closes one of Omnisnap's browsers before
your own programs; Omnisnap retries that capture or reports the engine.

## From a test suite

```js
import { run } from 'omnisnap';
const summary = await run({ pages: ['http://localhost:3000/'], out: './omnisnap-out' });
```

```python
from omnisnap import run
summary = run(pages=["http://localhost:3000/"], out="./omnisnap-out")
```

Both are thin wrappers around the same binary. From this repository:

```sh
cd packages/npm/omnisnap && npm test          # against a stub binary
cd packages/pypi && python -m pytest
```

## In CI

```yaml
on: pull_request
jobs:
  omnisnap:
    uses: GerMP21/Omnisnap/.github/workflows/report.yml@v0.2.0-rc.2
    with:
      start: npm ci && npm run build && npx serve -l 3000 dist
      wait-on: http://localhost:3000
      pages: |
        http://localhost:3000/
        http://localhost:3000/pricing
```

One job per OS (Linux, Windows and macOS — the macOS job carries Safari and
can be limited with the `macos` input), one merged report, one PR comment
(updated in place). Report only: the Action never fails a build. Fork PRs get
their comment from a `workflow_run` follow-up that never runs fork code.

The full setup — the `start` command for your stack, the pages list, the fork
path and the troubleshooting table: [github-action-setup.md](github-action-setup.md).

## Privacy

Captures, reports and browser profiles stay in the output directory and the
Omnisnap cache. There is no service and no upload; `install firefox` is the only
network command, and only when invoked.
