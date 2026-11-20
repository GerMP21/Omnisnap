# omnisnap (npm)

Thin wrapper around the Omnisnap binary: writes a job, streams progress,
returns the summary. No runtime dependencies, no postinstall.

## API

```js
import { run, browsers, merge, report } from 'omnisnap';

const summary = await run({
  pages: ['http://localhost:3000/'],
  engines: ['chrome', 'firefox'],
  out: './omnisnap-out',
  onProgress: (e) => console.log(e.type),
});
console.log(summary.results.map((r) => `${r.engine.engine}: ${r.stats.report} to report`));
```

The binary comes from the platform package (`@omnisnap/<os>-<arch>`), then
`OMNISNAP_BINARY`, then `PATH`. Errors are `OmnisnapError` with `code` and
`stderr`; findings never throw.

## Command line

The package also installs the `omnisnap` command, using the same binary:

```sh
npm install -g omnisnap                    # on your PATH
npx omnisnap version                       # or from a project that has it as a dependency
```

```sh
omnisnap browsers                          # the engines found, with versions and paths
omnisnap http://localhost:3000 http://localhost:3000/pricing
omnisnap http://localhost:3000 --engines chrome,firefox --runs 1
omnisnap run job.json --json               # a job file; NDJSON progress on stdout
omnisnap install firefox                   # add Gecko when no Firefox is installed
omnisnap report omnisnap-out               # re-render a bundle's report.html
```

Each run writes a bundle to `./omnisnap-out` (`--out` to change it), with a
self-contained `report.html` to open in a browser. Findings never fail a run:
the exit code is `0` with or without differences, `1` for a usage error, `2`
when no Blink baseline (Chrome, Edge, Chromium, Brave) is available, `3` when
every capture failed and `4` for an internal error. Every command and flag:
[the main README](https://github.com/GerMP21/Omnisnap#commands).
