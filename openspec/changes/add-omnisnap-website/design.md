# Design

## Context

See proposal.md for the motivation and specs/website/spec.md for the required behavior. What shapes the approach:

- **The site today.**
  - `tools/site/build.py` writes `_site/`: each `schema/*.schema.json` copied to the path its `$id` names, a generated `index.html` (the schema list) and a `404.html`.
  - `wrangler.json` uploads `_site/` as static assets of the `omnisnap-schemas` Worker, with `omnisnap.org` and `www.omnisnap.org` as custom domains, `run_worker_first: true` and `not_found_handling: "404-page"`.
  - `src/worker.js` redirects `www` to the apex, passes every path outside `/schema/` straight to the assets, and adds CORS and `nosniff` headers under `/schema/`.
  - `deploy.yml` runs on pushes to `main` that touch `schema/`, `src/`, `tools/site/` or `wrangler.json`. It warns and skips when the Cloudflare secrets are missing.
- **Releases come from the workflow token.** `release.yml` runs on a `v*` tag push and creates the release with `GH_TOKEN: ${{ github.token }}`. GitHub does not start workflows from events caused by that token, except `workflow_dispatch` and `repository_dispatch`, so a `release: published` listener never fires. `e2e-action.yml` already records the symptom: "the v0.1.0-rc.1 release fired no run at all".
  Source: https://docs.github.com/en/actions/security-for-github-actions/security-guides/automatic-token-authentication#using-the-github_token-in-a-workflow
- **The capture machinery already exists as a reusable workflow.**
  - `report.yml` runs the capture on Linux, Windows and macOS, merges the bundles and uploads the `omnisnap-merged` artifact (`report.html` included).
  - It posts a comment only on `pull_request` events.
  - `e2e-report.yml` already calls it, after a `pick` job that finds the newest release whose `SHA256SUMS` is up.
  - It declares `contents: read` and `pull-requests: write`, so every caller up the chain must grant both.
- **The viewer references only screenshots.** `report.html` holds the summary as a JSON data island and loads only `shot.src` images (bundle-relative, `<host>/<slug>/…png`). Captures, probes and findings files are never fetched by the page.
- **No tests cover `tools/site/` today**, and CI runs no Python for it.

## Goals / Non-Goals

**Goals:**
- One build script produces the whole site from one checkout (the release tag) plus one downloaded artifact (the demo).
- The parts that can break without anyone noticing are checked by unit tests that need no network: link rewriting, anchor fixing, demo trimming and the "nothing compared" check.
- The deploy can be re-run from the Actions tab without a new release.

**Non-Goals:**
- A site generator, a theme, a JavaScript framework or a search box.
- Changing the viewer. The demo is whatever `report.html` the released binary writes.
- Changing `report.yml`'s behavior for consumers.
- Fixing the `e2e-*` release triggers (follow-up; see Open Questions).

## Decisions

### 1. Build everything from the release tag

The deploy checks out the tag being deployed and runs that tag's `tools/site/build.py` with that tag's `site/`, `docs/` and `schema/`. The site therefore always matches one release, landing page included.

The first deploy is the first release that contains this change. A manual redeploy of an older tag that has no `site/` folder fails with a clear error ("this release predates the website build; publish a newer release").

*Alternative:* take the build tooling and landing page from the workflow's own commit and only the content from the tag. Rejected: two sources per deploy make "what is live?" hard to answer, and a landing page edit could go live before its release.

### 2. Who triggers the deploy

```
 tag push v*
     |
     v
 release.yml
   job release:  test -> build -> gh release create -> assets downloadable
   job deploy:   needs: release
                 uses: ./.github/workflows/deploy.yml
                 with: tag
                 secrets: inherit
                 permissions: contents: read, pull-requests: write
     |
     v
 deploy.yml   (on: workflow_call(tag) and workflow_dispatch(tag, optional))
   job resolve:  pick the tag (input, else the newest release whose
                 SHA256SUMS is up, as e2e-report.yml does); check the
                 Cloudflare secrets -> outputs tag, configured
   job demo:     if configured
                 uses: ./.github/workflows/report.yml
                 with: version = tag, pages = the five demo URLs
   job site:     if configured; needs demo (runs even if demo reported
                 failures: report.yml's jobs always conclude success)
                 checkout tag -> download omnisnap-merged
                 -> "compared nothing?" -> fail
                 -> build.py --version tag --demo merged --out _site
                 -> wrangler deploy
```

- `release.yml` calls the deploy as a job, rather than with `gh workflow run deploy.yml`. A called workflow shows up in the same run, its failure is visible on the release, and it needs no extra token. The release's top-level `contents: write` is narrowed to what the deploy chain needs on the calling job.
- The credential check runs before the capture, so a repository without Cloudflare secrets spends no runner minutes on a demo it can't publish. Without credentials, `resolve` warns and the other jobs skip; the release run stays green.
- Nesting is release → deploy → report: three levels, within GitHub's limit for reusable workflows.
  Source: https://docs.github.com/en/actions/sharing-automations/reusing-workflows#nesting-reusable-workflows
- `push` to `main` is removed from `deploy.yml`'s triggers. The concurrency group becomes `deploy-site`, `cancel-in-progress: false`: a release deploy shouldn't be cancelled by a manual one started a minute later.

*Alternatives:*
- **`on: release` in `deploy.yml`.** It never fires, as explained above.
- **`on: workflow_run` of `release.yml`.** It works, but runs from the default branch's copy of `deploy.yml`, not the tag's, which breaks decision 1.
- **A personal access token for `gh release create`.** It would make `release` events fire, but adds a long-lived secret to fix a trigger.

### 3. Demo pages and the "compared nothing" check

The owner chose five pages: `https://corrosion-vst.org/`, `https://corrosion-vst.org/play/`, `https://corrosion-vst.org/wiki/`, `https://synaptech-solutions.com/` and `https://synaptech-solutions.com/es`. They live in one list in `deploy.yml`, passed to `report.yml`'s `pages` input. `/es` is the same layout with Spanish text, where engines tend to break lines differently. `report.yml` has no `allow-small-pages`, so the 50-element bot-wall rule applies; synaptech's home page has about 780 elements.

"Compared nothing" means `merged/summary.json` is missing, or none of its `results` has a status other than `failed` or `unreachable`. In that case the `site` job fails with `::error::the demo compared nothing` before running `wrangler`, so the live site is untouched (spec: Deploy stops when the demo compares nothing). The check lives in `build.py` (a function with a unit test) and the workflow calls it.

*Alternative:* deploy the docs and keep the previous demo. Rejected by the owner: it needs state carried between releases (a demo saved as a release asset), and recovering from an outage is a click on "redeploy" anyway.

### 4. Publishing the demo

`build.py --demo <merged-dir>` copies `report.html` to `_site/demo/index.html`, plus every screenshot path named in `summary.json` (`results[].shots.baseline.src` and `.engine.src`), keeping their relative paths under `_site/demo/`. Nothing else from the bundle is copied (spec: Only Omnisnap's own and the owner's content is published). A path that escapes the bundle (`..`, absolute) is an error: the paths come from a file the build didn't write.

*Alternative:* copy the whole merged bundle. Rejected: it publishes captures and probes (page DOM and text), which is more than the report shows, and it's larger.

### 5. Rendering the docs with GitHub's API

`build.py` sends each guide to `POST https://api.github.com/markdown` with `{"text": …, "mode": "gfm", "context": "GerMP21/Omnisnap"}`, using `urllib` from the standard library and the workflow token (`GITHUB_TOKEN`). The response is HTML. It is wrapped in the site's page template (header, nav, footer) and written to `_site/docs/<slug>/index.html`:

| Source | Page |
|---|---|
| `docs/quickstart.md` | `/docs/quickstart/` |
| `docs/github-action-setup.md` | `/docs/github-action/` |
| `docs/github-action-fork-comments.md` | `/docs/fork-comments/` |

`/docs/index.html` lists the three guides. A request failure or non-200 response fails the build, so the site is never deployed with a missing page.

Post-processing on the returned HTML:
- **Links.** Each `href` in the API output is rewritten. Code blocks can't contain a raw `href="…"` because the API escapes their quotes, so a regex on `<a … href="…">` is safe. The rules:
  - `<guide>.md[#x]` becomes `/docs/<slug>/[#x]`;
  - any other relative path, resolved against `docs/`, becomes `https://github.com/GerMP21/Omnisnap/blob/<tag>/<path>`, or `tree/` for a directory;
  - absolute URLs and `#anchors` are left as they are.
- **Anchors.** GitHub prefixes heading ids with `user-content-` and fixes links with JavaScript on github.com. The build strips the prefix from `id` and `name` attributes so plain `#x` links work.
  Note: the exact heading markup is checked against a real API response in task 2.1 before the rewrite is written.

*Alternatives:*
- **python-markdown or markdown-it-py.** A new dependency, and output that differs from what contributors preview on GitHub.
- **A site generator.** A new toolchain for three pages.
- **Rendering in the browser.** Pages would be empty without JavaScript.

### 6. One stylesheet, no third-party CSS

`site/site.css` styles the landing page, the docs (including the GitHub markup: tables, `pre`, `.highlight` and a few `pl-*` syntax classes) and the schema list, in light and dark (`prefers-color-scheme`). The viewer keeps its own styles. Shared header and footer markup lives in one template in `build.py`, so every page has the same nav: Demo, Docs, Schemas, GitHub.

*Alternative:* bundle `github-markdown-css` (MIT). Rejected: it styles only the docs, needs a `THIRD-PARTY-NOTICES.md` entry, and would look unlike the landing page.

### 7. Version strings on the landing page

`site/index.html` writes the version as `{{version}}` (and `{{npm_version}}`, `{{pypi_version}}`); `build.py --version <tag>` substitutes them (`v0.1.0-rc.7`, `0.1.0-rc.7`, `0.1.0rc7`). The docs need no substitution: the release process bumps every version in `docs/` before the tag (Release-Process step 1), and the docs are taken from that tag.

### 8. Paths and the Worker

New output: `/index.html`, `/docs/…/index.html`, `/demo/index.html` and screenshots, `/schema/index.html`, `/site.css`, `/404.html`. Cloudflare's default `html_handling` (`auto-trailing-slash`) serves `/docs/quickstart/` from `docs/quickstart/index.html` and redirects `/docs/quickstart` to it, so `src/worker.js` doesn't change. `/schema/index.html` also gets the CORS and `nosniff` headers, which is harmless. Task 5.2 checks this with `wrangler dev` before the first deploy.
Source: https://developers.cloudflare.com/workers/static-assets/routing/advanced/html-handling/

## Risks / Trade-offs

- [A demo site is down or slow at release time] → The deploy fails only when nothing at all was compared. Recovery: fix the site, press redeploy. Partial results are published with their gaps listed.
- [corrosion-vst.org could not be inspected during planning (the sandbox's network blocked it); one of its three pages might be under 50 elements, behind a bot check, or (`/play/`) built around a canvas or audio element whose layout says little] → That page then shows as "not checked" or with few findings, and the deploy still succeeds on the other pages. Task 6.2 reviews the first real demo and swaps the page if needed.
- [The demo shows real differences on the owner's own sites, in public] → Intended: it's the product working. The owner reviews the first demo before announcing the site (task 6.2).
- [Each release costs a macOS job (~10-20 min for five pages, two runs each)] → Releases are infrequent; the credential check skips the capture where nothing would be deployed.
- [GitHub's Markdown API changes its markup or is down] → The build fails rather than publishing a broken page; the anchor rewrite is covered by a test on a saved response.
- [Docs fixes wait for a release] → Accepted by the owner; the manual redeploy covers urgent fixes once they are in a release.
- [The release run turns red when only the deploy failed] → Accepted: the release itself is published and verified before the deploy job starts, and the red job is the signal to fix the site.
- [Schema changes are published only on release] → The schemas change additively and only together with a binary that writes the new fields, which also ships in a release.

## Migration Plan

1. Merge the change to `main`. Nothing deploys: `push` is no longer a trigger.
2. The next release (expected `v0.1.0-rc.7`) runs the deploy from `release.yml` and replaces the schema-only site. The schema URLs are unchanged throughout.
3. Rollback: re-run the deploy by hand for an earlier tag that has the website build, or revert the change and push to `main` with the old `deploy.yml` restored.

## Open Questions

- The `e2e-action.yml` and `e2e-report.yml` release triggers have the same token problem and probably never run on releases. Fixing them (call them from `release.yml` too) is a separate change.
