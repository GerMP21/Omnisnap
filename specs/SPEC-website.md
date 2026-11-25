# Spec: website

Module id: `website` · Code: `tools/site/`, `site/`, `src/worker.js`, `wrangler.json`, `.github/workflows/deploy.yml` · Depends on: `bundle-schema`, `viewer`, `ci-action` · Milestone: M2 · Conventions: [CONVENTIONS.md](CONVENTIONS.md)

## Objective

`omnisnap.org` tells a visitor what Omnisnap is, how to install it, and what a report looks like, and it keeps serving the JSON Schemas at their `$id`s. Everything on it describes a version people can install: it is built from a release tag and deployed with that release. Planned in `openspec/changes/add-omnisnap-website/`.

## Behaviour

### Pages

| Path | Content | Source |
|---|---|---|
| `/` | What Omnisnap does, install commands (release binary, npm, PyPI), the Action snippet, links to `/demo/`, `/docs/`, `/schema/` and GitHub | `site/index.html`, with the release's version substituted |
| `/docs/` | An index of the three guides | generated |
| `/docs/quickstart/` | The quick start | `docs/quickstart.md` |
| `/docs/github-action/` | The GitHub Action setup guide | `docs/github-action-setup.md` |
| `/docs/fork-comments/` | Comments on pull requests from forks | `docs/github-action-fork-comments.md` |
| `/demo/` | A cross-engine report and its screenshots | captured during the deploy |
| `/schema/` | A list of the schemas | generated |
| `/schema/<name>/<major>.json` | Each schema at the path its `$id` names, with `Access-Control-Allow-Origin: *` and `X-Content-Type-Options: nosniff` | `schema/*.schema.json` |
| anything else | 404, with a link to `/` | generated |

`www.omnisnap.org` redirects (301) to the same path on `omnisnap.org`.

### Versions

Every Omnisnap version string on `/` and in the docs names the release the site was deployed with: `v<x>` for the binary and the Action, `<x>` without the `v` for npm, and the PEP 440 form for PyPI (`0.1.0rc7`). The landing page substitutes them at build time; the docs carry them because the release process bumps them before the tag.

### Docs

- The guides are rendered with GitHub Flavored Markdown (tables and fenced code included), as on github.com.
- Every link resolves:
  - a link to one of the three guides points at its `/docs/` page;
  - a link to any other repository file points at that file on GitHub at the deployed tag;
  - a link to a heading reaches that heading.

### Demo

- Captured during every deploy, with the release being deployed, on Linux, Windows and macOS hosts, from five pages on the owner's own sites: `https://corrosion-vst.org/`, `https://corrosion-vst.org/play/`, `https://corrosion-vst.org/wiki/`, `https://synaptech-solutions.com/` and `https://synaptech-solutions.com/es`.
- Only the report and the screenshots it references are published. Captures, probes and findings files are not.
- When the capture compared nothing at all, the deploy fails and the live site is unchanged. A partial capture is published, and its report lists what was not checked.

### Deploy

- The site deploys when a release is published (the release workflow runs it after the assets are verified) and when a maintainer starts it by hand (from the newest published release by default, or a given tag). A push to `main` does not deploy.
- Without the Cloudflare credentials, the deploy skips with a warning before the demo capture, and the release still succeeds.

## Commands

```
python3 -m unittest discover tools/site                     # build tests, no network
python3 tools/site/build.py --version <tag> --demo <merged-dir>   # needs GITHUB_TOKEN for the docs
gh workflow run deploy.yml [-f tag=<tag>]                   # manual redeploy
```

## Project structure

- `site/`: the hand-written landing page and the site stylesheet;
- `tools/site/build.py`: writes `_site/`;
- `tools/site/test_build.py` and `tools/site/testdata/`: its tests and fixtures;
- `src/worker.js` and `wrangler.json`: the Worker;
- `.github/workflows/deploy.yml`, called by `release.yml`.

## Code style

- The build uses the Python standard library only. The docs are rendered by GitHub's API (the repository contents endpoint with `Accept: application/vnd.github.html`, at the deployed tag), not by a Markdown package.
- The site uses one stylesheet and no third-party CSS or JavaScript. The demo is the viewer's own `report.html`, unchanged.

## Testing strategy

- **Unit (`tools/site`, no network):**
  - link and anchor rewriting on a saved API response;
  - the docs pages and index, the landing page's version substitution, the schema list and the 404 page;
  - demo trimming, including a path that escapes the bundle;
  - the "compared nothing" check.
- **Routing:** `wrangler dev` on a built site, checked with `curl` before the first deploy.
- **Live:** after each release, the deploy job is green and the pages in the table above answer as specified.

## Boundaries

- **Always:**
  - build from a release tag;
  - keep the schema URLs and headers;
  - publish only the report and its screenshots from the demo bundle.
- **Ask first:**
  - adding a demo page;
  - adding a page that is not generated from the repository;
  - any analytics or third-party script.
- **Never:**
  - capture or publish a site the owner does not own;
  - deploy from `main`;
  - upload anything a visitor provides.

## Success criteria

1. A release publishes the site with no manual step, and the landing page names that release.
2. The three guides render with their tables and code, and none of their links is broken.
3. `/demo/` shows results from more than one host, and every screenshot loads.
4. With both demo sites unreachable, the deploy fails and the previous site keeps serving.
5. `https://omnisnap.org/schema/summary/1.json` keeps its URL and headers.

## Open questions

- The `e2e-action.yml` and `e2e-report.yml` release triggers never fire for releases the workflow token creates. Calling them from `release.yml`, as the deploy is, is a separate change.
