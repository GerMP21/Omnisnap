# Design

## Context

See proposal.md for the motivation. What shapes the approach:

- **The normative sources.** The specs are normative: `specs/CAPABILITY-MAP.md` and 22 `SPEC-*.md`. The code lives in 18 packages under `internal/`, plus `cmd/omnisnap`, `cmd/omnisnap-lab`, `tools/`, `action/`, `packages/` and `.github/workflows/`. Decisions and measurements are in `tasks/plan.md`, `tasks/todo.md`, `tasks/backlog.md` and `docs/status-2026-11-19.md`.
- **Pages in the repository, mirrored to the wiki.** The owner first chose the GitHub Wiki only, then asked for the pages to move to `docs/internals/` as in `GerMP21/Corrosion-VST`: the repository holds the pages, a workflow publishes them, and a test checks them. The decisions below are the revised ones.
- **The wiki is off.** It is disabled on the repository (`has_wiki: false`). GitHub creates the wiki's git repository (`https://github.com/GerMP21/Omnisnap.wiki.git`) only after the first page is saved in the web UI.
- **Wiki conventions** (to be confirmed at the start of implementation, task 1.2):
  - each page is a Markdown file at the repository root, and its file name with hyphens becomes its title;
  - `Home.md` is the landing page;
  - `_Sidebar.md` and `_Footer.md` are shown on every page;
  - Mermaid diagrams render in wikis.

  Sources:
  - https://docs.github.com/en/communities/documenting-your-project-with-wikis/about-wikis
  - https://docs.github.com/en/communities/documenting-your-project-with-wikis/creating-a-footer-or-sidebar-for-your-wiki
  - https://docs.github.com/en/get-started/writing-on-github/working-with-advanced-formatting/creating-diagrams
- **Local sessions don't push.** Under `CLAUDE.md`, nothing is pushed from a local session. The agent drafts and commits nothing; the owner reviews and pushes.

## Goals / Non-Goals

**Goals:**
- A reader can follow one page from a URL to a finding in the report, and then open the module page for any step.
- Every statement about the code can be checked from the page: a link to the file, a spec section, or a measured result.
- Pages say what the code does today, including where it differs from a spec.

**Non-Goals:**
- **User documentation.** Install, commands, job files and the Action's inputs stay in `README.md`, `docs/` and `action/README.md`. The wiki links to them.
- **Restating the specs.** Thresholds and rules are quoted only where a page needs them to explain a mechanism. The spec stays the authority, and each module page links to it.
- **Changing code, specs or tasks.** A difference found while writing a page is reported to the owner, not fixed in this change.
- **Editing on GitHub.** The wiki is a published copy; the next publish overwrites edits made there.

## Decisions

### Page set and order

The wiki has 24 pages, grouped in the sidebar the way data flows through Omnisnap:

| Group | Pages |
|---|---|
| Start | `Home`, `Architecture`, `Repository-Layout`, `Glossary` |
| Contracts | `Data-Contracts` (job.json, the bundle, captures, summary.json, NDJSON events, schema versioning) |
| Capture side | `Discovery`, `Process-Management`, `Drivers`, `Capture-Pipeline`, `Branded-Browsers`, `Install` |
| Analysis side | `Layout-Diff`, `Appearance-Diff`, `Triage`, `DOM-Audit` |
| Output | `Viewer`, `CLI-and-Orchestration`, `Merge-and-PR-Comment`, `GitHub-Action`, `Wrappers` |
| Quality | `Ground-Truth-and-the-Model`, `Testing-and-CI`, `Release-Process`, `Extending-Omnisnap` |

Each module page has the same sections:
- **Role**, with its place in the pipeline;
- **Inputs and outputs**;
- **How it works**, in the order the code runs;
- **Key files and types**;
- **Failure handling**;
- **Tests**;
- **Where the code differs from the spec**;
- **Spec**, linked.

*Alternatives considered:*
- **One page per `internal/` package.** It mirrors the code, but it would split capture across `capture`, `pipeline` and parts of `cli`, and the reader asks about steps, not packages.
- **A few long pages.** These are harder to link to from issues, and GitHub's wiki sidebar suits short pages.

### Code links go to `main`, without line anchors

Code links are absolute: `https://github.com/GerMP21/Omnisnap/blob/main/<path>` for files, `…/tree/main/<path>` for directories. A relative link would not work once the page is published, because the wiki is a different repository. Links between pages are relative (`[text](Page-Name)`), which resolves in the wiki.

Line anchors are not used: on `main` they drift silently with every change. Link texts name the function or type instead, and the test rejects `#L…` anchors.

*As built:* the first version pinned every link to commit `0259ece` with line anchors, which suited a wiki no CI read. Once the pages change with the code in the same pull request, links to `main` are the ones that stay true.

### Diagrams in Mermaid

Diagrams use Mermaid, which GitHub renders. Three diagrams:
- the pipeline flow, on `Architecture`;
- the module dependency graph, from `CAPABILITY-MAP.md`;
- the capture lane, including the crash retry and the `Lost` engine, on `Capture-Pipeline`.

*Alternative considered:* image files. They would need a separate editor, they don't show diffs, and they are not readable in the raw Markdown.

### Written from the code

Each module page is written by reading its package and tests at the pinned commit, with the spec open next to it. A statement that comes only from the spec, and that the code doesn't confirm, is not written as fact.

The two known differences go on their pages:
- `--no-probes` and `--open` on `CLI-and-Orchestration`;
- the knowledge base as Go code on `Triage`.

### How the pages are checked

`test/internals/internals_test.go` runs with `go test ./...`, so CI checks the pages on every pull request. It fails when:
- a page other than Home, Glossary, `_Sidebar` and `_Footer` has no final `## Source files` list of repository links;
- a code link names a missing path, points at a ref other than `main`, or carries a line anchor;
- a non-test Go file under `cmd/`, `internal/`, `tools/` or `assets/`, or a top-level item of `.github/workflows/`, `action/`, `packages/`, `schema/`, `scripts/`, `test/` or `tools/`, is named by no page;
- a link between pages, or to a heading, does not resolve;
- a page is missing from the sidebar.

It is a port of Corrosion-VST's `tests/internals_wiki.rs`, plus the `main`-only and no-line-anchor checks. The scratchpad checker of the first version is no longer needed.

### The workflow publishes

`.github/workflows/wiki.yml` runs on every push to `main` that touches `docs/internals/`, the script or itself, and by hand. It clones `Omnisnap.wiki.git` with the workflow token, runs `scripts/publish-wiki.sh` (remove every file, copy `docs/internals/*.md`, commit only if something changed) and pushes. The wiki then holds exactly the pages of that commit. The pages follow `main`, not `dev`, because their code links do.

## Risks / Trade-offs

- **[Drift.** Prose can go stale after code changes.]
  → The rule in `CONTRIBUTING.md` and `CLAUDE.md`, the `## Source files` lists that make `grep -rl <path> docs/internals/` find the pages, and the test for everything mechanical.
- **[Pages read in the repository.]** On github.com's Code tab, relative page links (`Page-Name`, no `.md`) do not resolve; they are written for the wiki. → The README points readers to the published wiki.
- **[A spec-only statement, written as fact.]** → The written-from-the-code decision, and the per-page "Where the code differs from the spec" section.
- **[Duplicating README content.]** → User topics link to `README.md` and `docs/` instead of being repeated.
- **[Wiki behavior assumed from memory.]** (sidebar, Mermaid, first-page creation.) → Task 1.2 checks each one against GitHub's docs before any page is written.

## Migration Plan

1. The owner enables the wiki: `gh repo edit GerMP21/Omnisnap --enable-wiki`, or Settings → Features → Wikis.
2. The owner creates `Home` in the web UI, so the wiki's git repository exists.
3. The agent clones the wiki repository and writes the pages.
4. The owner reviews and pushes.

**Rollback:** the wiki's own history keeps every version of every page. Turning the feature off hides the wiki without deleting it.

## Open Questions

None. The README link was added with the move to `docs/internals/`.
