# Proposal

## Why

`omnisnap.org` is the project's domain, but a visitor who opens it sees a bare list of five JSON Schema links: nothing says what Omnisnap is, how to install it, or what a report looks like. The user documentation exists only as Markdown on GitHub, and nobody can see a real cross-engine report without installing the tool. The domain and its Cloudflare Worker are already in place, so the missing parts are the content and a deploy that keeps that content in step with what users can install.

## What Changes

- **Landing page at `/`.** A hand-written page that says what Omnisnap does, how to install it (binary, npm, PyPI), how to add the GitHub Action, and links to the demo, the docs, GitHub and the schemas. It replaces today's generated schema list as the home page.
- **User docs at `/docs/`.** `docs/quickstart.md`, `docs/github-action-setup.md` and `docs/github-action-fork-comments.md` become web pages, rendered at build time by GitHub's API, which returns each file at the release tag as HTML (GitHub Flavored Markdown, the same rendering as on github.com). Links between the guides point at their `/docs/` pages; links to other repository files point at GitHub. The README and the developer wiki stay on GitHub.
- **Live demo at `/demo/`.** A real `report.html` with its screenshots, captured fresh at every deploy. The capture covers five pages of the owner's own sites: `corrosion-vst.org` (home, `/play/`, `/wiki/`) and `synaptech-solutions.com` (home, `/es`), on Linux, Windows and macOS, with the release being deployed. It goes through the same reusable workflow consumers use (`report.yml`).
- **Schema list at `/schema/`.** Today's list moves from `/` to `/schema/`. The schema files keep their URLs (`/schema/<name>/<major>.json`) and their headers.
- **Deploy on release, not on push to `main`.** The site is built and deployed when a release is published (`release.yml` calls the deploy as its last job) and from a manual "redeploy" button (`workflow_dispatch`). It no longer deploys on pushes to `main`. **BREAKING (for maintainers):** a schema or docs change on `main` reaches `omnisnap.org` with the next release or a manual redeploy, not on merge.
- **A deploy with no demo stops.** If the demo capture compares nothing at all (both sites down, or every capture failed), the deploy fails and the site stays as it was. Partial results (a browser skipped, one page unreachable) are published, with the gaps listed in the report.

## Capabilities

### New Capabilities

- `website`: what `omnisnap.org` serves (landing page, user docs, live demo, schema list and schema files), how its content is produced, and when and how it is deployed.

### Modified Capabilities

None. There are no OpenSpec specs yet. The normative `specs/SPEC-bundle-schema.md` and `specs/CONVENTIONS.md` say the schema Worker is deployed by `deploy.yml` on changes to `main`. Their wording is updated in this change to say it deploys on release; the schema URLs and contents don't change.

## Impact

- **Code and tooling:**
  - `tools/site/build.py` gains the landing page, the docs rendering and the demo copy.
  - A new `site/` folder holds the hand-written landing page and the site stylesheet.
  - `src/worker.js` is expected to stay unchanged; the design checks its routing against the new paths.
- **Workflows:**
  - `.github/workflows/deploy.yml` changes trigger: `workflow_call` and `workflow_dispatch` replace `push` to `main`. It calls `report.yml` for the demo and needs `pull-requests: write` to satisfy the called workflow's permissions, though no comment is posted outside a pull request.
  - `.github/workflows/release.yml` calls the deploy after the release assets are verified.
- **Cost:** each deploy runs the three capture jobs (macOS included), about 10-20 minutes and some macOS runner minutes. Releases are infrequent.
- **External services:**
  - GitHub's Markdown API, with the workflow token.
  - The two demo sites must be reachable from GitHub's runners at deploy time.
  - Cloudflare as today.
- **Docs:**
  - `schema/README.md`, `specs/SPEC-bundle-schema.md` and `specs/CONVENTIONS.md` (deploy trigger), and `README.md` (a link to the site).
  - The wiki pages that name the changed files (`Testing-and-CI`, `Repository-Layout`, `Release-Process`, `Data-Contracts`), and a new wiki page for the site (`CONTRIBUTING.md` rule 7).
- **Out of scope:**
  - Opening a user's own bundle in the browser.
  - Moving the README or the wiki to the site.
  - The `e2e-action.yml` and `e2e-report.yml` release triggers, which have the same token problem described in the design. They are noted as a follow-up.
