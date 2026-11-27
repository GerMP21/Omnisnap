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

The schemas are part of the website (`specs/SPEC-website.md`), which is built
from a release tag. Deployment: `release.yml` runs `.github/workflows/deploy.yml`
once a release's assets are verified, and it can be run by hand (Actions →
Deploy the website); a push to `main` does not deploy, so a schema change goes
live with the next release. It needs `CLOUDFLARE_API_TOKEN` and
`CLOUDFLARE_ACCOUNT_ID` in the repository secrets and warns and skips, rather
than failing the release, until they exist. The zone must be active on the same
Cloudflare account, and the hostnames must have no DNS records yet — a custom
domain cannot be created over an existing CNAME. The Worker must not also be
connected to this repository in Cloudflare (Workers Builds), which would deploy
every push.

Local checks:

```sh
python3 tools/site/build.py --version v0.2.0-rc.3 --skip-docs   # writes _site/, schemas at schema/<name>/1.json
python3 -m unittest discover tools/site
node --check src/worker.js
go test ./internal/schema/      # compiles every schema and its fixtures
```
