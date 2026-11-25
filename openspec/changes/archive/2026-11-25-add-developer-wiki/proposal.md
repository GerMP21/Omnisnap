# Proposal

## Why

Omnisnap's internals are documented in three places today: the module specs, which say what each module must do; the code; and the task list and status notes, which record what was decided and measured. None of them explains how the pieces fit together. A new contributor reads 22 specs, 18 internal packages and 90 tasks before they can answer "where does a cause get its tier?" or "what happens when Firefox crashes mid-job?".

Now that v0.1.0-rc.5 is public, with an Action, npm and PyPI packages, and outside contributors invited by `CONTRIBUTING.md`, the project needs one developer-facing explanation of its inner workings.

## What Changes

- Turn on the GitHub Wiki for `GerMP21/Omnisnap` and write a developer wiki there, in English. The wiki covers:
  - the architecture, from discovery to the report;
  - every runtime module: what it does, its key types and files, its tolerances, and its failure handling;
  - the data contracts;
  - the ground-truth suites and the tree model;
  - testing and CI, and the release process;
  - how to extend Omnisnap: a knowledge-base entry, a cause kind, an audit check, an engine.
- The pages live in the main repository, in `docs/internals/`, and are reviewed in pull requests with the code. A workflow (`.github/workflows/wiki.yml`, with `scripts/publish-wiki.sh`) mirrors them into the GitHub Wiki on every push to `main` that changes them. A Go test (`test/internals/`) keeps them in step with the repository. This follows the setup of `GerMP21/Corrosion-VST`. *Revised:* the first version wrote the pages only in the wiki repository; the owner then asked for them to move into the repository.
- Each page:
  - links to code on `main`, without line anchors;
  - ends with a `## Source files` list of the files it explains;
  - links to the spec that is normative for each module.

  The wiki explains how the specs are carried out. It does not restate their requirements as new rules.
- Where the code and a spec disagree, the page says what the code does and names the difference. Two are known today: the `--no-probes` and `--open` flags, and the knowledge base being Go code rather than versioned data.
- One `docs/internals/` row in `README.md`'s Repository section, and the upkeep rule in `CONTRIBUTING.md` (rule 7) and `CLAUDE.md`.

## Capabilities

### New Capabilities

None. The wiki is documentation; it adds no system behavior. The change sets `skip_specs: true`.

### Modified Capabilities

None.

## Impact

- **Repository settings:** the Wiki feature is enabled (`has_wiki` is `false` today). The wiki's git repository only exists after the first page is created in the GitHub web UI.
- **Main repository:** `docs/internals/` (26 pages), `scripts/publish-wiki.sh`, `.github/workflows/wiki.yml`, `test/internals/internals_test.go`, and the README, `CONTRIBUTING.md` and `CLAUDE.md` lines. No product code or spec changes.
- **Publishing:** the workflow pushes to the wiki with the workflow token (`contents: write`) when `main` changes. Edits made on GitHub are overwritten by the next publish.
- **Maintenance:** every change updates the pages that describe what it touched, in the same pull request. `go test ./...` fails when a page links a path that no longer exists or when a source file is named by no page; whether the prose is still true is the reviewer's job.
