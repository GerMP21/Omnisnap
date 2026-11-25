# Tasks

## 1. Before building

- [ ] 1.1 The owner confirms the current site deploys: `CLOUDFLARE_API_TOKEN` and `CLOUDFLARE_ACCOUNT_ID` are set, and the latest `deploy.yml` run ran `wrangler deploy` (not the "skipping" warning). The only run so far (2026-11-18) skipped for missing credentials, which the owner has since added, yet the site serves the schemas, so it was deployed some other way. The owner checks the `omnisnap-schemas` Worker's Settings → Build in the Cloudflare dashboard, and disconnects any connected Git repository (Workers Builds), so `release.yml` is the only deployer. Verify: `curl -sI https://omnisnap.org/schema/summary/1.json` returns 200 with `access-control-allow-origin: *` and `x-content-type-options: nosniff`.
- [ ] 1.2 Capture the demo pages once with the `v0.1.0-rc.6` binary (`omnisnap <the URLs>`) and record, per page, whether it was captured or rejected (bot-wall rule, under 50 elements, unreachable) and how many Report causes it produced. Verify: all five are captured; any that is not is raised with the owner, and the page list in design.md decision 3 is updated before task 5.1. *Owner's trial so far:* `corrosion-vst.org/play/` was captured only in Chrome and was dropped (four pages remain); the other four are awaiting the owner's confirmation.
- [x] 1.3 Write `specs/SPEC-website.md` (normative, in the project's spec format) from `openspec/changes/add-omnisnap-website/specs/website/spec.md`, and add its row to `specs/CAPABILITY-MAP.md`. Verify: every requirement in the OpenSpec delta has a counterpart in the spec, and `go test ./test/internals/` passes.

## 2. Docs rendering

- [x] 2.1 Render `docs/github-action-setup.md` once with `gh api -H "Accept: application/vnd.github.html" repos/GerMP21/Omnisnap/contents/docs/github-action-setup.md` and save the response as `tools/site/testdata/github-action-setup.html`. Record the heading and anchor markup it uses, and correct design.md decision 5 if it differs from the `user-content-` prefix assumed there. Verify: the fixture is committed, and design.md matches it.
- [x] 2.2 Add the renderer to `tools/site/build.py`: one function that fetches a guide rendered as HTML from GitHub's contents API at the tag, with `urllib` and `GITHUB_TOKEN`, and fails the build on any non-200 response. The rest of the build takes it as a parameter, so tests can pass a fake. Verify: `python3 -m unittest discover tools/site` passes a test where the fake returns an error and the build exits non-zero.
- [x] 2.3 Add the link and anchor rewrite (design decision 5): guide `.md` links become `/docs/<slug>/`, other relative links become GitHub URLs at the tag (`blob/` for files, `tree/` for directories), absolute URLs and `#x` are left as they are, and `user-content-` is stripped from `id` and `name`. Verify: unit tests on the 2.1 fixture cover each link kind, a heading anchor, and an `href="…"` inside a code block that stays untouched.
- [x] 2.4 Write the three docs pages and `/docs/index.html` with the shared page template (nav: Demo, Docs, Schemas, GitHub). Verify: a unit test builds into a temp dir with the fake renderer and finds `docs/quickstart/index.html`, `docs/github-action/index.html`, `docs/fork-comments/index.html`, and an index linking all three.
- [x] 2.5 Create `docs/internals/Website.md` (role, build steps, docs rendering, `## Source files` with `tools/site/build.py`) and add it to `_Sidebar.md`. Verify: `go test ./test/internals/` passes.

## 3. Landing page, styles, schema list

- [x] 3.1 Write `site/index.html` (what Omnisnap does, install commands for the binary, npm and PyPI, the Action snippet, links to `/demo/`, `/docs/`, `/schema/` and GitHub) using `{{version}}`, `{{npm_version}}` and `{{pypi_version}}`. Add `--version <tag>` to `build.py` to substitute them. Verify: a unit test builds with `--version v0.1.0-rc.7` and finds `v0.1.0-rc.7`, `0.1.0-rc.7` and `0.1.0rc7` in `_site/index.html`, and no `{{` left.
- [x] 3.2 Write `site/site.css` (landing page, docs including tables, `pre`, `.highlight` and the common `pl-*` classes, schema list; light and dark via `prefers-color-scheme`), copied to `/site.css`. Verify: Playwright screenshots of the built landing page and a docs page, served with `python3 -m http.server`, at 390 px and 1280 px wide in both colour schemes, show no horizontal scroll and readable tables.
- [x] 3.3 Move the schema list to `_site/schema/index.html` and give `404.html` the shared template with a link to `/`. Verify: unit tests find every schema at its `$id` path, a list linking each one, and a 404 page linking `/`.
- [x] 3.4 Document the landing page, styles and schema list in `docs/internals/Website.md` (add `site/index.html` and `site/site.css` to its `## Source files`), add `site/` to `Repository-Layout.md`, and add the `omnisnap.org` link near the top of `README.md`. Verify: `go test ./test/internals/` passes.

## 4. The demo

- [x] 4.1 Add `--demo <merged-dir>` to `build.py`: copy `report.html` to `_site/demo/index.html` and only the screenshots named in `summary.json` (`results[].shots.baseline.src` and `.engine.src`), keeping their relative paths; any path that is absolute or contains `..` is an error. Verify: unit tests on a fixture bundle that also holds capture, probe and findings files find only the report and its screenshots in the output, and the build exits non-zero on a `../` path.
- [x] 4.2 Add the "compared nothing" check: missing `summary.json`, or no result with a status other than `failed` or `unreachable`, prints `the demo compared nothing` and exits non-zero. Verify: unit tests cover a missing summary, all results failed or unreachable (exit 1), and one `ok` result among failures (exit 0).
- [x] 4.3 Document the demo (pages, trimming, the "compared nothing" rule) in `docs/internals/Website.md`. Verify: `go test ./test/internals/` passes.

## 5. Workflows

- [x] 5.1 Rewrite `.github/workflows/deploy.yml` (design decision 2):
  - triggers `workflow_call` (input `tag`) and `workflow_dispatch` (optional `tag`), no `push`;
  - job `resolve` picks the tag (input, else the newest release with `SHA256SUMS`) and checks the Cloudflare secrets;
  - job `demo` calls `./.github/workflows/report.yml` with the four demo pages and `version: tag`;
  - job `site` checks out the tag, fails clearly if it has no `site/`, downloads `omnisnap-merged`, runs the 4.2 check, builds with `--version` and `--demo` (`GITHUB_TOKEN` set) and runs `wrangler deploy`;
  - concurrency group `deploy-site` without cancel-in-progress.

  Verify: `actionlint` reports nothing new for the file.
- [x] 5.2 Check the built site's routing with `wrangler dev` before any deploy. Verify with `curl`: `/docs/quickstart` redirects to `/docs/quickstart/`, `/docs/quickstart/` is 200, `/schema/summary/1.json` is 200 with both headers, `/demo/` is 200, `/doc/quickstart` is 404 with the 404 page, and `www.` redirects (or is confirmed by reading `src/worker.js` if `wrangler dev` cannot simulate the host).
- [x] 5.3 Add the `deploy` job to `.github/workflows/release.yml`: `needs: release`, `uses: ./.github/workflows/deploy.yml` with the tag, `secrets: inherit`, permissions `contents: read` and `pull-requests: write`. Verify: `actionlint` passes, and the job graph in the file has `deploy` after the assets check.
- [ ] 5.4 Run `python3 -m unittest discover tools/site` in `ci.yml` (Linux job), and add `tools/site/` and `site/` to the paths that matter for CI if it filters on paths. Verify: the CI job runs the site tests on this branch's pull request.
- [x] 5.5 Update the docs for the new trigger:
  - `schema/README.md`, `specs/SPEC-bundle-schema.md` and `specs/CONVENTIONS.md`: the Worker deploys on release and by hand, not on `main`;
  - `docs/internals/Testing-and-CI.md`: the `deploy.yml` row;
  - `docs/internals/Release-Process.md`: section 3 says the release deploys the site, with a link to the Website page;
  - `docs/internals/Data-Contracts.md`: where the schemas are published from;
  - `docs/internals/Website.md`: the deploy flow, with `deploy.yml`, `release.yml` and `wrangler.json` in its `## Source files`.

  Verify: `grep -rn "push.*main\|changes to .main." schema/README.md specs/SPEC-bundle-schema.md specs/CONVENTIONS.md` finds no deploy-on-main wording, and `go test ./test/internals/` passes.

## 6. First release

- [ ] 6.1 After merge, the owner bumps the version to `v0.1.0-rc.7` (Release-Process step 1) and pushes the tag. Verify:
  - the release run's `deploy` job is green;
  - the spec scenarios hold on the live site: `curl` checks of `/`, `/docs/quickstart/`, `/demo/`, `/schema/`, `/schema/summary/1.json` (headers), a mistyped path (404) and `www.` (301);
  - every screenshot in `/demo/` loads;
  - the landing page names `v0.1.0-rc.7`.
- [ ] 6.2 The owner reviews the live demo before announcing the site, and swaps a page in `deploy.yml` if it shows nothing useful or something they don't want public. Verify: the owner's go-ahead, or a follow-up commit changing the page list and a manual redeploy.

## Workflow follow-up

- Archive the change once 6.1 and 6.2 are done.
- Open a separate change for the `e2e-action.yml` and `e2e-report.yml` release triggers, which never fire for the same token reason (design.md, Open Questions).
