# website Specification

## Purpose

Defines what omnisnap.org serves to people evaluating or using Omnisnap (a landing page, the user docs, a live cross-engine demo and the JSON Schemas) and when that content is published, so the site always describes a version people can install.

## Requirements

### Requirement: Landing page
The site SHALL serve a landing page at `https://omnisnap.org/` that says what Omnisnap does, shows how to install it (release binary, npm, PyPI) and how to add the GitHub Action, and links to the demo, the docs, the schema list and the GitHub repository.

#### Scenario: A visitor opens the domain
- **WHEN** a browser requests `https://omnisnap.org/`
- **THEN** the response is an HTML page with status 200 that contains the install commands and the Action snippet
- **AND** it links to `/demo/`, `/docs/`, `/schema/` and `https://github.com/GerMP21/Omnisnap`

#### Scenario: The www host redirects
- **WHEN** a browser requests any path on `https://www.omnisnap.org/`
- **THEN** the response is a 301 redirect to the same path on `https://omnisnap.org/`

### Requirement: Versions on the site match the deployed release
Every Omnisnap version string on the landing page and in the docs pages SHALL name the release the site was deployed with.

#### Scenario: Install commands name the deployed release
- **GIVEN** the site was deployed for release `v0.1.0-rc.7`
- **WHEN** a visitor reads an install command or an Action snippet on the landing page or in the docs
- **THEN** it names `v0.1.0-rc.7` (or `0.1.0-rc.7` / `0.1.0rc7` for npm and PyPI), not a later unreleased version

### Requirement: User docs
The site SHALL serve the quickstart, the GitHub Action setup guide and the fork-comments guide as HTML pages under `/docs/`. They are rendered from `docs/quickstart.md`, `docs/github-action-setup.md` and `docs/github-action-fork-comments.md` at the deployed release, with GitHub Flavored Markdown (tables and fenced code included).

#### Scenario: A guide is served
- **WHEN** a browser requests `https://omnisnap.org/docs/quickstart/`
- **THEN** the response is an HTML page with status 200 holding the rendered content of `docs/quickstart.md`, with its tables and code blocks intact

#### Scenario: The docs index lists the guides
- **WHEN** a browser requests `https://omnisnap.org/docs/`
- **THEN** the page links to each of the three guides

### Requirement: Links in the docs pages resolve
A link in a docs page SHALL point at a page that exists. A link to one of the three guides points at its `/docs/` page; a link to any other repository file points at that file on GitHub at the deployed release's tag; a link to a heading points at that heading's anchor on the target page.

#### Scenario: A link between guides
- **GIVEN** `docs/github-action-setup.md` links to `github-action-fork-comments.md`
- **WHEN** a visitor follows that link on the site
- **THEN** they reach the fork-comments page under `/docs/`, not a 404

#### Scenario: A link to a repository file
- **GIVEN** a guide links to a file outside the three guides (for example `../action/README.md`)
- **WHEN** a visitor follows that link on the site
- **THEN** they reach that file on `github.com/GerMP21/Omnisnap` at the deployed release's tag

#### Scenario: A link to a heading
- **GIVEN** a guide links to `#before-you-open-the-pull-request`
- **WHEN** a visitor follows that link
- **THEN** the page scrolls to that heading

### Requirement: Live demo report
The site SHALL serve at `/demo/` a cross-engine report captured during the deploy with the release being deployed, from `https://corrosion-vst.org/`, `https://corrosion-vst.org/wiki/`, `https://synaptech-solutions.com/` and `https://synaptech-solutions.com/es`, on Linux, Windows and macOS hosts. Its screenshots are served next to it.

#### Scenario: The demo shows a real report
- **WHEN** a browser requests `https://omnisnap.org/demo/`
- **THEN** the response is the Omnisnap report viewer with results for the demo pages from more than one host
- **AND** every screenshot the report references is served with status 200

#### Scenario: The demo is fresh
- **WHEN** the site is deployed
- **THEN** the demo is captured again during that deploy, not reused from an earlier one

### Requirement: Only Omnisnap's own and the owner's content is published
The demo SHALL capture only sites owned by the project owner. The site SHALL NOT publish raw captures, probes or findings files from the demo bundle, only the report and the screenshots it references.

#### Scenario: The demo bundle is trimmed
- **WHEN** the demo is published
- **THEN** `/demo/` holds the report and its referenced screenshots, and no capture, probe or findings files

### Requirement: Deploy stops when the demo compares nothing
A deploy SHALL fail, and leave the site as it was, when the demo capture produced no comparison at all. A demo with at least one comparison SHALL be published, with what could not be checked listed in the report.

#### Scenario: Both demo sites are down
- **GIVEN** neither demo site is reachable from the runners
- **WHEN** a deploy runs
- **THEN** the deploy job fails with an error that says the demo compared nothing
- **AND** `https://omnisnap.org/` keeps serving the previous deployment

#### Scenario: One browser is missing
- **GIVEN** WebKitGTK could not be installed on the Linux runner
- **WHEN** a deploy runs
- **THEN** the deploy succeeds and the demo report lists WebKitGTK on Linux as not checked

### Requirement: Schema files and list
The site SHALL serve each JSON Schema at the path its `$id` names (`/schema/<name>/<major>.json`), with `Access-Control-Allow-Origin: *` and `X-Content-Type-Options: nosniff`, and SHALL serve a list of the schemas at `/schema/`.

#### Scenario: A schema keeps its URL
- **WHEN** a client requests `https://omnisnap.org/schema/summary/1.json`
- **THEN** the response is the summary schema with status 200 and both headers

#### Scenario: The schema list moved
- **WHEN** a browser requests `https://omnisnap.org/schema/`
- **THEN** the page links to every published schema

### Requirement: Unknown paths
A request for a path the site does not serve SHALL get a 404 page that links back to the landing page.

#### Scenario: A mistyped path
- **WHEN** a browser requests `https://omnisnap.org/doc/quickstart`
- **THEN** the response has status 404 and its page links to `/`

### Requirement: Deploy on release and on demand
The site SHALL be deployed when a release is published and when a maintainer starts the deploy by hand. It SHALL NOT be deployed by a push to `main`.

#### Scenario: A release is published
- **WHEN** the release workflow has published release `v0.1.0-rc.7` and verified its assets
- **THEN** the site is deployed from tag `v0.1.0-rc.7`, without a manual step

#### Scenario: A manual redeploy
- **WHEN** a maintainer starts the deploy workflow by hand
- **THEN** the site is deployed from the most recent published release's tag, with a fresh demo

#### Scenario: A docs change merged to main
- **WHEN** a change to `docs/quickstart.md` is merged to `main`
- **THEN** `omnisnap.org` does not change until the next release or manual deploy

### Requirement: Deploy needs credentials
When the Cloudflare credentials are not configured, a deploy SHALL skip with a warning instead of failing, as today.

#### Scenario: Credentials missing
- **GIVEN** `CLOUDFLARE_API_TOKEN` or `CLOUDFLARE_ACCOUNT_ID` is not set
- **WHEN** a release is published
- **THEN** the release workflow still succeeds and the deploy reports a warning that it was skipped
