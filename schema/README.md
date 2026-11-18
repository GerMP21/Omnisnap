# JSON Schemas

`job.schema.json`, `capture.schema.json`, `summary.schema.json`,
`progress.schema.json` and `host.schema.json` are the contracts of
`SPEC-bundle-schema.md`. Their `$id`s point at the published location:

```
https://omnisnap.org/schema/<name>/1.json
```

The file name and the published path differ because the `$id` carries a major
version. `tools/site/build.py` reads the `$id` and writes the file at that
path, so the schema stays the single source of truth; `wrangler.json` uploads
the result as static assets to the `omnisnap-schemas` Worker, with
`omnisnap.org` and `www.omnisnap.org` as custom domains (`src/worker.js`
redirects `www` to the apex and adds CORS to the schema responses).

Deployment: `.github/workflows/deploy.yml` runs on schema changes to `main`
and needs `CLOUDFLARE_API_TOKEN` and `CLOUDFLARE_ACCOUNT_ID` in the repository
secrets. The zone must be active on the same Cloudflare account, and the
hostnames must have no DNS records yet — a custom domain cannot be created over
an existing CNAME.

Local checks:

```sh
python3 tools/site/build.py     # writes _site/schema/<name>/1.json
node --check src/worker.js
go test ./internal/schema/      # compiles every schema and its fixtures
```
