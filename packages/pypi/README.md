# omnisnap (PyPI)

Thin wrapper around the Omnisnap binary: writes a job, streams progress,
returns the summary. No runtime dependencies, no download at install time.

```python
from omnisnap import run, browsers

summary = run(pages=["http://localhost:3000/"], engines=["chrome", "firefox"],
              out="./omnisnap-out", on_progress=lambda e: print(e["type"]))
print([(r["engine"]["engine"], r["stats"]["report"]) for r in summary["results"]])
```

The binary comes from the wheel for this platform, then `OMNISNAP_BINARY`, then
`PATH`. Errors are `OmnisnapError` with `code` and `stderr`; findings never
raise.
