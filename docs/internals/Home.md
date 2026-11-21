# Omnisnap developer wiki

Omnisnap captures a page in every browser engine installed on the machine (Blink, Gecko, WebKit), compares the layout of every element against a Chrome baseline, and writes a self-contained report: which element changed, why, whether anyone would see it, and which engine is the odd one out.

This wiki explains **how it works inside**: how the pieces fit, what each module does with its input, where the code lives, and how to change it. It is for people who work on Omnisnap, not for people who use it.

## Where to read what

| You want… | Read |
|---|---|
| To install and use Omnisnap | [README](https://github.com/GerMP21/Omnisnap/blob/main/README.md), [quick start](https://github.com/GerMP21/Omnisnap/blob/main/docs/quickstart.md), [Action setup](https://github.com/GerMP21/Omnisnap/blob/main/docs/github-action-setup.md) |
| What a module **must** do (requirements, thresholds, success criteria) | The module's spec in [`specs/`](https://github.com/GerMP21/Omnisnap/blob/main/specs/CAPABILITY-MAP.md). Specs are normative. |
| How a module **does** it, and how the modules fit | This wiki |
| What was decided, measured, and what is left | [`tasks/plan.md`](https://github.com/GerMP21/Omnisnap/blob/main/tasks/plan.md), [`tasks/todo.md`](https://github.com/GerMP21/Omnisnap/blob/main/tasks/todo.md), [`tasks/backlog.md`](https://github.com/GerMP21/Omnisnap/blob/main/tasks/backlog.md), [`docs/status-2026-11-19.md`](https://github.com/GerMP21/Omnisnap/blob/main/docs/status-2026-11-19.md) |
| How to contribute (issues, PRs, rules) | [`CONTRIBUTING.md`](https://github.com/GerMP21/Omnisnap/blob/main/CONTRIBUTING.md) |

## How to read this wiki

1. [Architecture](Architecture): one URL's path from discovery to the report, and the module graph.
2. [Data contracts](Data-Contracts): the files that pass between the modules. Everything else reads or writes these.
3. The module pages, in pipeline order (sidebar). Each one has the same sections: role, inputs and outputs, how it works, key files and types, failure handling, tests, where the code differs from the spec, and the spec.
4. [Extending Omnisnap](Extending-Omnisnap) when you are about to change something.

The [Glossary](Glossary) defines the words the pages use (cause, consequence, tier, probe, lane…).

## How this wiki is kept

- **The pages live in the repository**, in [`docs/internals/`](https://github.com/GerMP21/Omnisnap/tree/main/docs/internals), and change in the same pull request as the code they describe. The [wiki workflow](https://github.com/GerMP21/Omnisnap/blob/main/.github/workflows/wiki.yml) mirrors them into the GitHub Wiki on every push to `main` that changes them. **Never edit the wiki on GitHub**: the next publish overwrites it.
- **The rule:** a change updates, in the same change, every page that describes what it touched. To find them, search for the path you changed: `grep -rl 'internal/merge/merge.go' docs/internals/`. Every page except this one and the Glossary ends with a `## Source files` list, so the search finds the pages that explain a file.
- **The check:** `go test ./test/internals/` (part of `go test ./...`, so CI runs it on every pull request). It fails when a linked file or directory no longer exists, when a code link points anywhere but `main` or carries a line anchor, when a non-test Go file under `cmd/`, `internal/`, `tools/` or `assets/` (or a top-level item of `.github/workflows/`, `action/`, `packages/`, `schema/`, `scripts/`, `test/` or `tools/`) is named by no page, when a link between pages or to a heading is broken, and when a page is missing from the sidebar ([Testing and CI](Testing-and-CI#the-wikis-own-check)). It cannot tell whether the prose is still true: that is the rule, and the reviewer's job.
- A statement here comes from reading the code. Where the code and a spec disagree, the page says so under "Where the code differs from the spec".

### Format

- One file per page in `docs/internals/`, named as the wiki names it (`Title-With-Hyphens.md`; the first line is `# Title with hyphens`). `Home.md` is the landing page; `_Sidebar.md` and `_Footer.md` are the sidebar and footer. A new page gets a line in `_Sidebar.md`.
- Links between pages are relative: `[text](Page-Name)` or `[text](Page-Name#heading-anchor)`, with GitHub's anchor rule (lower case; drop everything but letters, digits, spaces, `-` and `_`; spaces become `-`).
- Code links are full URLs to `main`: `https://github.com/GerMP21/Omnisnap/blob/main/<path>` for files, `…/tree/main/<path>` for directories. No line anchors: they drift as `main` changes, so name the function in the link text instead.
- A final `## Source files` list, one line per path with what the page uses it for, and no `## ` heading after it.
- Diagrams are Mermaid, which GitHub renders in wikis and in Markdown files ([creating diagrams](https://docs.github.com/en/get-started/writing-on-github/working-with-advanced-formatting/creating-diagrams)).
