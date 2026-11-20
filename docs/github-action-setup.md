# Setting up the Omnisnap Action in your repository

The Action captures your pages in every engine installed on the runner, compares
them and posts **one** comment on the pull request with the differences. It is
**report-only**: it never fails your build, and nothing leaves the runner except
the capture artifacts of that workflow run.

Two steps: [the workflow on a pull request](#the-workflow-on-a-pull-request),
then [comments on pull requests from forks](#comments-on-pull-requests-from-forks).
- What the Action does and what it guarantees: [`../specs/SPEC-ci-action.md`](../specs/SPEC-ci-action.md)
- The inputs, one by one: [`../action/README.md`](../action/README.md)
- The fork mechanism in depth: [github-action-fork-comments.md](github-action-fork-comments.md)

## Before you start

- **A published release.** The Action downloads its binary from the GitHub
  release (`releases/download/<version>/…`). Until the candidate
  `v0.1.0-rc.4` is published, every job fails at the download step. There is
  no way around it from a consumer repository: the `binary` input that lets the
  self-test use a locally built binary only works from inside the Omnisnap
  repository.
- **A site that can be served on the runner.** You tell the Action how to
  build and start it; it does not know your stack.
- **No secrets.** The capture jobs need none, and no secret is exposed to the
  code under test.

> The tables below are the **starting point**, not a recipe that is guaranteed
> to work: they are written for the versions the hosted runner images ship
> (Node 22, Python 3.12, Ruby 3.2, PHP 8.3), and your project may pin others.

## The workflow on a pull request

Create `.github/workflows/omnisnap.yml` in your repository:

```yaml
name: Omnisnap report          # the fork follow-up matches this string exactly

on: pull_request

permissions:
  contents: read
  pull-requests: write         # to post the comment; a called workflow cannot
                                # raise the permissions its caller grants

jobs:
  omnisnap:
    uses: GerMP21/Omnisnap/.github/workflows/report.yml@v0.1.0-rc.4
    with:
      start: npm ci && npm run build && npx serve -l 3000 dist
      wait-on: http://localhost:3000
      pages: |
        http://localhost:3000/
        http://localhost:3000/pricing
```

Two details that are easy to get wrong:

- **Do not end `start` with `&`.** The Action runs it with `nohup bash -c "…"`
  in the background itself and kills it when the job ends. A trailing `&`
  leaves the server running past the job.
- **The `name:` matters later.** The fork follow-up matches on it, character
  for character.

### The inputs of the reusable workflow

| Input | Default | What it does |
|---|---|---|
| `pages` | (required) | one URL per line, as the runner sees them |
| `start` | — | the shell command that builds and starts your site |
| `wait-on` | — | the URL the Action polls until the server answers (120 tries, one second apart) |
| `engines` | every installed | comma-separated engine ids (`chrome`, `chromium`, `edge`, `firefox`, `webkitgtk`, `wpe`, `safari`) |
| `baseline` | `chrome` | the engine every other one is compared against |
| `runs` | `2` | captures per engine and page |
| `version` | `v0.1.0-rc.4` | the release tag the binary is downloaded from |
| `macos` | `every` | `every` runs the macOS job (and its Safari capture) on every pull request; `labelled` runs it only on pull requests labelled `omnisnap:safari`; `never` drops it |
| `artifact-retention-days` | `7` | printed in the log; the retention itself is a repository setting |
| `setup-timeout` | `300` | Linux: seconds the engine package downloads may take; past it the job captures without the engines it could not install (unreleased: on `main` only) |

### What the runner gives you

The hosted Ubuntu image (24.04) ships, among others:

| Runtime | Version | Package manager |
|---|---|---|
| Node.js | 22.23.4 | npm 10.9.9, Yarn 1.22.22 |
| Python | 3.12.3 | pip 24.0, Miniconda 26.9.4 |
| Ruby | 3.2.3 | RubyGems 3.4.20 |
| PHP | 8.3.6 | Composer 2.11.2 |

The Windows image ships Node and Python as well. Two consequences:

- **A `start` command that works on your machine usually works here**, as long
  as it does not depend on a version you do not have. If your project needs
  Node 18 or Python 3.9, the build will fail on the runner.
- **You cannot add setup steps to the capture job.** The reusable workflow owns
  that job, so `actions/setup-node` or `actions/setup-python` cannot run before
  it. Two ways out: bootstrap the version inside `start` (nvm and pyenv are on
  the image), or use [the composite action in your own matrix](#a-custom-matrix-instead),
  where the build is a step of your job and you control everything before it.

### The `start` command, by stack

The command runs under `bash` on both runners (Git Bash on Windows), so keep it
portable: use `python` rather than `python3` if you need an interpreter, and
avoid absolute POSIX paths.

#### JavaScript and TypeScript

| Project | `start` | `wait-on` |
|---|---|---|
| Vite | `npm ci && npm run build && npx vite preview --port 3000` | `http://localhost:3000` |
| Vite (static output) | `npm ci && npm run build && npx serve -l 3000 dist` | `http://localhost:3000` |
| Create React App | `npm ci && npm run build && npx serve -s -l 3000 build` | `http://localhost:3000` |
| Next.js (app router) | `npm ci && npm run build && npx next start -p 3000` | `http://localhost:3000` |
| Next.js (pages router) | `npm ci && npm run build && npx next start -p 3000` | `http://localhost:3000` |
| Next.js static export | `npm ci && npm run build && npx serve -l 3000 out` | `http://localhost:3000` |
| Remix | `npm ci && npm run build && npx remix-serve build/server/index.js` (or `npm run start`) | `http://localhost:3000` |
| SvelteKit | `npm ci && npm run build && node build/index.js` | `http://localhost:3000` |
| Nuxt | `npm ci && npm run build && node .output/server/index.mjs` | `http://localhost:3000` |
| Angular | `npm ci && npm run build && npx http-server dist -p 3000` | `http://localhost:3000` |
| Astro | `npm ci && npm run build && npx serve -l 3000 dist` | `http://localhost:3000` |
| Gatsby | `npm ci && npm run build && npx serve -l 3000 public` | `http://localhost:3000` |
| Eleventy | `npm ci && npm run build && npx serve -l 3000 _site` | `http://localhost:3000` |
| Docusaurus | `npm ci && npm run build && npx serve -l 3000 build` | `http://localhost:3000` |
| VitePress | `npm ci && npm run build && npx vitepress preview --port 3000` | `http://localhost:3000` |
| Plain static files | `python -m http.server 3000 --directory public` | `http://localhost:3000` |

Notes:

- `npm ci` needs a `package-lock.json`; use `npm install` if you do not commit
  one. `npm ci` is faster and reproducible.
- For a single-page app with client-side routing, serve with the SPA
  fallback (`npx serve -s`) or the deep links will 404 and the capture sees an
  error page.
- `npx serve` and `npx vite preview` download their package on first use; that
  is fine on the runner, but it costs a few seconds.
- Check the directory your build writes to: Angular's standalone application
  outputs to `dist/<project>/browser`, Gatsby to `public`, Eleventy to
  `_site`, and the server frameworks to `.next`, `.output` or `build`.

#### Python web apps

| Project | `start` | `wait-on` |
|---|---|---|
| Flask | `pip install -r requirements.txt && flask --app app run --port 3000 --host 0.0.0.0` | `http://localhost:3000` |
| Flask (gunicorn) | `pip install -r requirements.txt && gunicorn --bind 0.0.0.0:3000 app:app` | `http://localhost:3000` |
| Django | `pip install -r requirements.txt && SECRET_KEY=dev DJANGO_SETTINGS_MODULE=app.settings python manage.py migrate --noinput && SECRET_KEY=dev DJANGO_SETTINGS_MODULE=app.settings python manage.py runserver 0.0.0.0:3000` | `http://localhost:3000` |
| Django (gunicorn) | `pip install -r requirements.txt && SECRET_KEY=dev DJANGO_SETTINGS_MODULE=app.settings python manage.py migrate --noinput && SECRET_KEY=dev DJANGO_SETTINGS_MODULE=app.settings gunicorn --bind 0.0.0.0:3000 config.wsgi:application` | `http://localhost:3000` |
| FastAPI | `pip install -r requirements.txt && uvicorn main:app --host 0.0.0.0 --port 3000` | `http://localhost:3000` |
| Starlette | `pip install -r requirements.txt && uvicorn app:app --host 0.0.0.0 --port 3000` | `http://localhost:3000` |
| Pyramid | `pip install -r requirements.txt && gunicorn --bind 0.0.0.0:3000 app:app` | `http://localhost:3000` |
| Bottle | `pip install -r requirements.txt && gunicorn --bind 0.0.0.0:3000 app:app` | `http://localhost:3000` |
| Tornado | `pip install -r requirements.txt && python -m tornado.app --port 3000 app` | `http://localhost:3000` |
| Sanic | `pip install -r requirements.txt && sanic app.app --host 0.0.0.0 --port 3000` | `http://localhost:3000` |
| aiohttp | `pip install -r requirements.txt && python -m aiohttp.web -H 0.0.0.0 -P 3000 app` | `http://localhost:3000` |
| Streamlit | `pip install -r requirements.txt && streamlit run app.py --server.port 3000 --server.address 0.0.0.0` | `http://localhost:3000` |
| Gradio | `pip install -r requirements.txt && gradio app.py --server-name 0.0.0.0 --server-port 3000` | `http://localhost:3000` |

Python static sites and docs:

| Project | `start` | `wait-on` |
|---|---|---|
| Pelican | `pip install pelican markdown && pelican content -o output && python -m http.server 3000 --directory output` | `http://localhost:3000` |
| MkDocs | `pip install -r requirements.txt && mkdocs build && python -m http.server 3000 --directory site` | `http://localhost:3000` |
| Sphinx | `pip install -r requirements.txt && sphinx-build -b html docs build && python -m http.server 3000 --directory build` | `http://localhost:3000` |

Notes:

- **Bind to `0.0.0.0`.** A server that listens only on `127.0.0.1` is still
  reachable, but a development server that picks its own host may not be.
- **Environment variables go inline**: `start` runs through `bash -c`, so
  `SECRET_KEY=dev DJANGO_SETTINGS_MODULE=app.settings gunicorn …` works. Give
  the app throwaway values — the capture never writes anything back.
- **A database is a problem.** The reusable workflow defines no services, so
  there is no Postgres or Redis. Use SQLite, an in-memory database, or a
  migration-free code path. If the page needs a real database, use
  [the composite action in your own matrix](#a-custom-matrix-instead) and add
  a service container there.
- `python manage.py migrate` is safe to include: SQLite creates the file on the
  runner, and the runner is discarded afterwards.

#### Ruby, PHP and static generators

| Project | `start` | `wait-on` |
|---|---|---|
| Rails | `bundle install && SECRET_KEY_BASE=dev RAILS_ENV=development bin/rails server -p 3000 -b 0.0.0.0` | `http://localhost:3000` |
| Jekyll | `bundle install && bundle exec jekyll serve --port 3000 --host 0.0.0.0` | `http://localhost:3000` |
| Hugo | `hugo --minify && hugo server -p 3000 --bind 0.0.0.0` | `http://localhost:3000` |
| Zola | `zola build && zola serve --port 3000` | `http://localhost:3000` |
| Laravel | `composer install --no-interaction && php artisan serve --host 0.0.0.0 --port 3000` | `http://localhost:3000` |
| Plain PHP | `php -S 0.0.0.0:3000 -t public` | `http://localhost:3000` |

Rails and Laravel need their environment files to exist (`config/database.yml`
with a SQLite adapter, a `.env`); commit a development-friendly version or
create them in the `start` command.

#### Pinning a runtime version

If your project needs a version the image does not have, bootstrap it inside
`start`:

```yaml
      # Node 20 on an image that ships 22
      start: source $NVM_DIR/nvm.sh && nvm install 20 && nvm use 20 && npm ci && npm run build && npx serve -l 3000 dist
```

`NVM_DIR` is `/usr/local/nvm` on the Ubuntu image. For Python, `pyenv` or
`uv` work the same way. Keep in mind that the same command also runs on
Windows, where nvm and pyenv are not installed — so the portable answer for a
project with two runtimes is [your own matrix](#a-custom-matrix-instead), with
one job per operating system.

### The two-minute budget

`wait-on` is polled 120 times, one second apart: **the build is inside
`start`, so it shares that budget**. A Rails or Django build that takes three
minutes leaves the server not yet up when the wait gives up — the Action warns,
the capture runs against a dead port and every page comes back failed.

If your build is slow, use the composite action in your own matrix, where the
build is its own step and the wait starts against a server that is already
running:

```yaml
jobs:
  omnisnap:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7
      - uses: actions/setup-node@v5
        with:
          node-version: 20
      - run: npm ci && npm run build
      - uses: GerMP21/Omnisnap/action@v0.1.0-rc.4
        with:
          version: v0.1.0-rc.4
          start: npx serve -l 3000 dist
          wait-on: http://localhost:3000
          pages: |
            http://localhost:3000/
```

### The `pages` list

- One URL per line, absolute, as the runner sees them (`http://localhost:3000/…`).
- Start with **three to five** pages. The wall time grows with pages × engines
  × runs, and the hosted runners have a 30-minute job timeout.
- Use real content pages. A page with fewer than 50 DOM elements is treated as
  a bot wall and comes back `unreachable`; the `--allow-small-pages` escape
  hatch exists, but a page with content is the real fix.
- The same list on every OS job: the merge step requires one job per host.
- Pointing the list at your public URLs captures the live site instead of your
  branch. That is a legitimate smoke test, but it will not see your changes.
- Paths, not just the home page: `http://localhost:3000/pricing` is worth more
  than a second copy of `/`.

### A custom matrix instead

The composite action can be used directly, in your own matrix, and it takes the
same inputs as the workflow plus `out`, `upload`, `artifact-name`, `binary` and
`allow-small-pages`:

```yaml
- uses: GerMP21/Omnisnap/action@v0.1.0-rc.4
  with:
    version: v0.1.0-rc.4
    pages: |
      http://localhost:3000/
```

It is referenced **by path**: the repository root has no `action.yml`, so
`GerMP21/Omnisnap@<tag>` would not resolve.

With your own matrix you control everything before the action runs — the
runtime version, the build step, a database service — and you can give each
operating system its own `start` command.

## What happens when you open a pull request

Three jobs appear in the Actions tab:

```
capture (ubuntu-latest)     → artifact omnisnap-Linux
capture (windows-latest)    → artifact omnisnap-Windows
merge and comment           → artifact omnisnap-merged + the comment on the PR
```

Every engine installed on the runner is captured on each runner — Chrome,
Firefox and WebKitGTK on Linux; Chrome, Edge and Firefox on Windows — which is
the point: the comment shows where your page renders differently per engine.

The comment is one markdown block, updated in place on every push. The hidden
marker `<!-- omnisnap-report -->` is what identifies it, so a second push never
adds a second comment. It carries:

- per page: the engine against its baseline, the status, and up to three
  Report-tier causes with their selector;
- **Notes**: engines that were skipped, pages that failed or were unreachable;
- links to the merged report artifact and to the workflow run;
- the report-only footer.

## Before you open the pull request

Run the same thing locally: start your site with the same command, then point
the binary at the same URLs.

```sh
omnisnap run job.json --json     # job.json lists the same pages
```

The local run writes the same bundle and the same report the comment will
summarize, so you can see what the Action will say before spending runner
minutes on it.

## How to test it

1. Put the workflow on a branch and open a pull request.
2. Watch the three jobs. The comment lands when the merge job finishes.
3. **Push another commit to the same branch.** The run repeats and the comment
   is *updated* — exactly one comment, not two.
4. If a page shows as failed, open the `omnisnap-Linux` artifact: `summary.json`
   says what happened and `omnisnap-progress.ndjson` has the progress lines.

## What usually bites

> **Known issue: slow Linux setup (2026-11-20).** The Ubuntu job installs
> WebKitGTK, Weston and the parity fonts from the Ubuntu package mirror, and the
> mirror has been very slow (about 50 KB/s). With `v0.1.0-rc.4` and earlier the
> download has no time limit of its own: it can take 15 minutes or more, or run
> into GitHub's 30-minute job limit, and then the report has no Linux host
> (Windows and macOS still report). On `main` the Action caches the packages
> per runner image and stops downloading after `setup-timeout` (5 minutes),
> then captures with Chrome and Firefox; this ships in the next release. Until
> then, re-run the cancelled Ubuntu job: the comment is updated in place.


- **The build fails on the runner** (missing environment variable, wrong
  version, no lockfile). The Action does not fail the job: it warns, the page
  is reported as failed and the comment says so. That is the report-only
  contract, but it means a broken build looks like a layout finding — read the
  job log before believing the comment.
- **Every page is `unreachable`.** The server did not answer `wait-on` inside
  two minutes (wrong port, the build wrote elsewhere, the process exited), or
  the pages have too few elements.
- **Nothing is captured.** Check that the download step found the release
  assets; the tag in `version` and the release tag must be the same string.
- **The site works locally and not on the runner.** The usual suspects are a
  missing environment variable, a pinned runtime version the image does not
  have, and a database the runner does not have.

## Comments on pull requests from forks

A pull request opened from a fork gets a read-only `GITHUB_TOKEN`, so the job
that posts the comment cannot write. The workaround is a second workflow in
**your** repository, triggered by `workflow_run` after the report finishes; it
runs with your token and never checks out or executes the fork's code.

### 1. Get the template

```sh
mkdir -p .github/workflows
curl -fsSL https://raw.githubusercontent.com/GerMP21/Omnisnap/v0.1.0-rc.4/.github/workflow-templates/omnisnap-fork-comment.yml \
  -o .github/workflows/fork-comment.yml
```

### 2. Fix the one line it depends on

```yaml
on:
  workflow_run:
    workflows: ["Omnisnap report"]     # ← must equal your caller's `name:`
    types: [completed]
```

If your caller workflow is named something else (say `Layout check`), change
this string to match exactly — case and spacing included.

### 3. Push it to your default branch

This is the hard requirement: GitHub only fires `workflow_run` for a workflow
file that exists on the default branch. Pushing it on a branch does nothing.

The permissions it needs are already in the template:

```yaml
permissions:
  actions: read
  pull-requests: write
```

A called reusable workflow cannot raise the permissions its caller grants, so
keep that block as it is.

### 4. Test it

1. Fork your repository (GitHub → Fork).
2. In the fork, branch off the default branch and make a small, visible change —
   a CSS tweak is ideal, because the comment is about rendering differences.
3. Open a pull request from the fork to the original repository.
4. Two runs appear: **Omnisnap report** (the capture, with the read-only
   token) and then **Omnisnap fork comment** (the follow-up). The follow-up
   downloads the `omnisnap-merged` artifact from the first run and upserts the
   same marked comment.
5. The comment appears even though the first run could not post it.

### When no comment appears

| Symptom | Cause |
|---|---|
| The follow-up never runs | `fork-comment.yml` is not on the default branch, or `workflows:` does not match the caller's `name:` exactly |
| The follow-up runs and skips | The report run was not from a `pull_request` event, or the head repository *is* your repository (that path is commented by `report.yml` itself) |
| No comment, job green | `merged/summary.json` was missing: the merge job had no bundle, or an earlier job failed — check the report run's artifacts |
| `Resource not accessible by integration` | The follow-up lost `pull-requests: write`; check the permissions block |
| Comment on the wrong pull request | Two open pull requests share the head branch name; the reusable workflow also accepts an explicit `pr:` number |

The mechanism, the GitHub rules it rests on and the artifact download are
explained in [github-action-fork-comments.md](github-action-fork-comments.md) —
including why the capture job may run the fork's code with a read-only token
while the comment job never does.
