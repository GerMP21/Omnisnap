# JSON Schemas

`job.schema.json`, `capture.schema.json`, `summary.schema.json`,
`progress.schema.json` and `host.schema.json` are the contracts of
`SPEC-bundle-schema.md`. Their `$id`s point at the published location:

```
https://omnisnap.org/schema/<name>/1.json
```

The file name and the published path differ because the `$id` carries a major
version (`.github/workflows/pages.yml` maps one to the other from the `$id`
itself, so the schemas remain the single source of truth). After a schema
change lands on `main`, the workflow republishes the site; `internal/schema`
compiles every schema and its fixtures in `testdata/schema/`.

Local checks:

```sh
go test ./internal/schema/
python3 -m json.tool schema/job.schema.json > /dev/null
```
