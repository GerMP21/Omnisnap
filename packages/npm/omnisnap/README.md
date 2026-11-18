# omnisnap (npm)

Thin wrapper around the Omnisnap binary: writes a job, streams progress,
returns the summary. No runtime dependencies, no postinstall.

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
