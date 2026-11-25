# Website

**Role.** `omnisnap.org` is the project's public site: a landing page, the user guides, a live demo report and the JSON Schemas. Every page on it describes one release, the one it was built from. Module `website`, built by [`tools/site/build.py`](https://github.com/GerMP21/Omnisnap/blob/main/tools/site/build.py) and served by the `omnisnap-schemas` Cloudflare Worker.

## What it serves

| Path | Content | Built from |
|---|---|---|
| `/` | Landing page | [`site/index.html`](https://github.com/GerMP21/Omnisnap/blob/main/site/index.html), with the release's version substituted |
| `/docs/`, `/docs/<slug>/` | The quick start, the Action setup guide, fork comments | `docs/*.md` at the release tag, rendered by GitHub |
| `/demo/` | A report captured during the deploy, and its screenshots | the `omnisnap-merged` artifact of the deploy run |
| `/schema/`, `/schema/<name>/<major>.json` | The schema list, and each schema at the path its `$id` names | [`schema/`](https://github.com/GerMP21/Omnisnap/tree/main/schema) |
| `/404.html` | Not found, with a link home | generated |
| `/site.css` | The one stylesheet | [`site/site.css`](https://github.com/GerMP21/Omnisnap/blob/main/site/site.css) |

## The build

```
python3 tools/site/build.py --version <tag> [--demo <merged-dir>] [--out _site] [--skip-docs]
```

[`build`](https://github.com/GerMP21/Omnisnap/blob/main/tools/site/build.py) runs these steps in order. It uses the Python standard library only, and any `BuildError` exits with status 1.

1. **Versions.** `versions` turns the tag into its three spellings: `v0.1.0-rc.7` for the binary and the Action, `0.1.0-rc.7` for npm, and `0.1.0rc7` for PyPI (PEP 440). A string that isn't a release tag is an error.
2. **The demo check, first.** With `--demo`, `compared_anything` must find a `summary.json` with at least one result whose status is not `failed` or `unreachable`. Otherwise the build stops before writing anything: "the demo compared nothing".
3. **A clean output.** `_site/` is deleted and recreated, so nothing from an earlier build is uploaded.
4. **Landing page.** `build_landing` substitutes `{{version}}`, `{{npm_version}}` and `{{pypi_version}}` in `site/index.html`; an unknown placeholder is an error. It wraps the result in the shared page template and copies `site/site.css`.
5. **Schemas and 404.** `build_schemas` copies each `schema/*.schema.json` to the path after `omnisnap.org/` in its `$id`, as the previous schema-only site did, and writes the list at `/schema/`.
6. **Docs.** `build_docs` renders each guide in `GUIDES` (described in the next section).
7. **Demo.** `build_demo` copies the merged bundle's `report.html` to `/demo/index.html`, plus only the screenshots its `summary.json` names (`results[].shots.baseline.src` and `.engine.src`), keeping their relative paths. Captures, probes and findings are never published. A screenshot path that is absolute or contains `..` is refused, and so is one the bundle doesn't hold.

Every page except the demo uses `page`: the same header (Demo, Docs, Schemas, GitHub), the stylesheet, and a footer naming the release.

## Rendering the docs

The guides are rendered by GitHub itself, so they look as they do on github.com: `github_renderer` requests `repos/GerMP21/Omnisnap/contents/<path>?ref=<tag>` with `Accept: application/vnd.github.html` and the workflow token. Any response other than 200 fails the build. The HTML then goes through three steps:

- **`article`** keeps the `<article class="markdown-body">` and drops GitHub's file wrapper.
- **`rewrite_links`** rewrites each `<a href>`:
  - a link to another guide becomes its `/docs/<slug>/` page, with the fragment kept;
  - any other relative path becomes the file on GitHub at the tag (`blob/`, or `tree/` for a directory);
  - absolute URLs and `#fragments` are kept as they are;
  - a link to a path that doesn't exist, or one that leaves the repository, fails the build.

  Only `<a>` tags are matched, so a `href="…"` written inside a code block is never touched: GitHub escapes or splits it there.
- **`fix_anchors`** strips GitHub's `user-content-` prefix from `id` and `name`. github.com maps `#x` to `user-content-x` with JavaScript, and the site has no JavaScript, so the ids carry the plain slug and `#x` links work.

The saved response [`tools/site/testdata/github-action-setup.html`](https://github.com/GerMP21/Omnisnap/blob/main/tools/site/testdata/github-action-setup.html) is what the tests run on. It shows the markup these steps rely on: headings with an `<a id="user-content-…" class="anchor">`, tables inside `<markdown-accessiblity-table>` (GitHub's spelling), and code inside `.highlight` with `pl-*` spans.

`--skip-docs` builds without the network, for a local preview. The deploy never passes it.

## Styles

[`site/site.css`](https://github.com/GerMP21/Omnisnap/blob/main/site/site.css) is the only stylesheet, with no third-party CSS. It has light and dark themes (`prefers-color-scheme`). It styles the GitHub markup the guides produce: tables scroll sideways inside their wrapper on a narrow screen, and it colours the common `pl-*` highlight classes. The demo keeps the viewer's own styles.

## Deploy

```
 tag push v*                                  Actions -> Deploy the website
     |                                                 (optional tag)
     v                                                      |
 release.yml: release (test, build, publish, verify)        |
              deploy  --uses-->  deploy.yml  <--------------+
                                   resolve: credentials? tag? site/ at the tag?
                                   demo:    report.yml (Linux, Windows, macOS),
                                            four demo pages, version = tag
                                   site:    checkout tag, download omnisnap-merged,
                                            build.py --version tag --demo merged,
                                            wrangler deploy
```

[`deploy.yml`](https://github.com/GerMP21/Omnisnap/blob/main/.github/workflows/deploy.yml) has two triggers: `workflow_call`, from the `deploy` job of [`release.yml`](https://github.com/GerMP21/Omnisnap/blob/main/.github/workflows/release.yml), and `workflow_dispatch`. A push to `main` does not deploy.

It isn't `on: release` because `release.yml` creates the release with the workflow token, and GitHub starts no workflow from events that token causes. `on: workflow_run` would run the default branch's copy of the workflow instead of the tag's.

- **`resolve`:**
  - Without `CLOUDFLARE_API_TOKEN` and `CLOUDFLARE_ACCOUNT_ID` it warns, and the other jobs skip, before any capture.
  - Otherwise it takes the given tag, or the newest release with a `SHA256SUMS` asset (the same choice `e2e-report.yml` makes, because the "latest release" endpoint leaves out prereleases).
  - A tag without `site/index.html` predates the website build and is refused.
- **`demo`:** calls [`report.yml`](https://github.com/GerMP21/Omnisnap/blob/main/.github/workflows/report.yml) with the four pages and the tag as `version`, the same path consumers use. It needs `pull-requests: write` only because `report.yml` declares it; outside a pull request it posts no comment.
- **`site`:**
  - It runs even if the demo job broke. A missing `omnisnap-merged` artifact counts as a demo that compared nothing.
  - It checks out the tag and builds with `GITHUB_TOKEN` for the docs.
  - [`wrangler.json`](https://github.com/GerMP21/Omnisnap/blob/main/wrangler.json) uploads `_site/` as the Worker's assets.
  - The `deploy-site` concurrency group runs one deploy at a time and never cancels one in progress.

The demo pages are the owner's own sites only: `corrosion-vst.org/`, `/wiki/`, `synaptech-solutions.com/` and `/es`. `corrosion-vst.org/play/` was tried and left out: only Chrome captured it.

[`src/worker.js`](https://github.com/GerMP21/Omnisnap/blob/main/src/worker.js) is unchanged by the website:
- it redirects `www.omnisnap.org` to the apex;
- it adds `Access-Control-Allow-Origin: *` and `nosniff` under `/schema/`;
- it passes everything else to the assets.

Cloudflare's default `html_handling` serves `/docs/quickstart/` from `docs/quickstart/index.html` and redirects `/docs/quickstart` to it. `not_found_handling: "404-page"` serves `404.html`.

## Tests

[`tools/site/test_build.py`](https://github.com/GerMP21/Omnisnap/blob/main/tools/site/test_build.py) needs no network: a fake renderer returns the saved response. Run it with `python3 -m unittest discover tools/site`. It covers:

- version spellings;
- each kind of link, links inside code, links to missing files;
- that every `#` link in the saved guide has a target;
- the docs pages and index, and the nav on every page;
- the landing page's versions;
- every schema at its `$id` path and in the list;
- the 404 page;
- demo trimming, a `../` screenshot path, and the "compared nothing" cases, including that nothing is written then.

## Source files

- [`tools/site/build.py`](https://github.com/GerMP21/Omnisnap/blob/main/tools/site/build.py): builds the site
- [`tools/site/test_build.py`](https://github.com/GerMP21/Omnisnap/blob/main/tools/site/test_build.py): its tests
- [`tools/site/testdata`](https://github.com/GerMP21/Omnisnap/tree/main/tools/site/testdata): a saved GitHub rendering of a guide
- [`site/index.html`](https://github.com/GerMP21/Omnisnap/blob/main/site/index.html): the landing page
- [`site/site.css`](https://github.com/GerMP21/Omnisnap/blob/main/site/site.css): the stylesheet
- [`.github/workflows/deploy.yml`](https://github.com/GerMP21/Omnisnap/blob/main/.github/workflows/deploy.yml): the deploy
- [`.github/workflows/release.yml`](https://github.com/GerMP21/Omnisnap/blob/main/.github/workflows/release.yml): calls the deploy after a release
- [`wrangler.json`](https://github.com/GerMP21/Omnisnap/blob/main/wrangler.json): the Worker, its assets and its domains
- [`src/worker.js`](https://github.com/GerMP21/Omnisnap/blob/main/src/worker.js): the Worker's code
