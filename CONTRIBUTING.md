# Contributing to Omnisnap

Thank you for your interest in Omnisnap! This guide covers where to post, how
to build and test, the rules the codebase lives by, and how to send a pull
request. Issues, pull requests, discussions, code, comments and documentation
are in English.

By participating, you agree to follow the [Code of Conduct](CODE_OF_CONDUCT.md).

## Where to post

| You have… | Post it as… |
|---|---|
| A confirmed, reproducible bug | A [Bug report](https://github.com/GerMP21/Omnisnap/issues/new?template=bug_report.yml) |
| A concrete feature or improvement to propose | A [Feature proposal](https://github.com/GerMP21/Omnisnap/issues/new?template=feature_proposal.yml) |
| A question, a measurement or threshold discussion, a false positive or negative on a real page, a triage explanation that is wrong, or any change that isn't small | A post in [Discussions](https://github.com/GerMP21/Omnisnap/discussions) |

When you post, say which kind of feedback it is: a capture or driver failure
(with the engine and version), a diff problem (two elements matched that aren't
the same element, or a finding reported at the wrong element), a tier problem
(a real difference listed as Info, or an expected engine difference listed as
Report), a wrong or missing cause, an appearance or ink measurement issue, or a
proposal.

Measured claims beat opinions here. A job file, the bundle it produced and the
page (or a reduced copy of it) turn "Safari looks off" into something that can
be verified and fixed.

## Before you start

- Search existing issues, pull requests and Discussions to avoid duplicates.
- Check the planned work. What you have in mind may already be there:
  - [`tasks/plan.md`](tasks/plan.md) records the architecture decisions and the
    phase/risk order; [`tasks/todo.md`](tasks/todo.md) is the task list, with
    acceptance criteria and verification steps per task.
  - [`specs/`](specs/CAPABILITY-MAP.md) holds the 17 module specs and the shared
    [conventions](specs/CONVENTIONS.md). Anything that contradicts a spec needs
    that spec changed in the same contribution.
  - [`docs/status-2026-11-18.md`](docs/status-2026-11-18.md) says what is
    verified, what still needs a runner, a crawl or a registry, and which
    known limitations are deliberate.

  If it's already covered, there's no need to post. If it's related but not
  fully covered, mention the task or spec by name.
- [`docs/quickstart.md`](docs/quickstart.md) covers building, running against a
  local site, and the job file. Some surprises are documented behaviour.
- For any change that isn't small, start a discussion first, so your effort
  fits the project's direction before you invest time in it.

## Basic requirements

Every contribution, whether AI-assisted or not, must meet these requirements.
Pull requests that don't meet them may be sent back for changes or closed.

1. **Bug reports include the relevant information.** The bug report form asks
   for it. A bug that can't be reproduced can't be fixed.
2. **Pull requests explain why the change exists.** Describe the problem it
   solves or the need it addresses, not just what changed. The diff already
   shows *what*; only you can explain *why*. Link the related issue, discussion
   or task.
3. **Code is commented where it matters.** Explain the reasoning behind
   non-obvious decisions, engine workarounds and edge cases, and document
   exported items with Go doc comments. Avoid comments that only restate what
   the code does.
4. **Code has been double-checked.** Review your own diff before requesting a
   review: remove debug output, commented-out code and unrelated changes, and
   make sure the checks in [Build and test](#build-and-test) pass locally.
5. **Changes include tests.** New features need tests covering the expected
   behaviour and relevant edge cases. Bug fixes need a test that fails without
   the fix and passes with it. When the fix comes from a real page that can't be
   committed, add a planted fixture that reproduces the same geometry. If a
   change can't reasonably be tested (for example, documentation only), explain
   why in the pull request.
6. **Documentation is updated.** Update the README when users see the change,
   `docs/quickstart.md` when the commands or the job file change, the module
   spec when the design changes, and `docs/status-2026-11-18.md` when what is
   verified changes. All in the same pull request.

## Rules the codebase lives by

- **Standard library first.** Allowed without asking: `golang.org/x/sys`
  (Windows Job Objects, registry), `github.com/coder/websocket` (CDP/BiDi
  transport) and `github.com/dmitryikh/leaves` (tree model). Anything else is a
  discussion before it is a dependency, and it needs a license compatible with
  the AGPL v3: Apache-2.0, MIT, BSD and ISC are; GPL-2.0-only is not.
- **The injected JavaScript is pinned.** `assets/inject/stabilize.js`,
  `extract.js` and `probes.js` are byte-identical to the spike's copies, and a
  test compares their hashes against a manifest. Change them only through a
  reviewed change that updates the spike too; `mutate.js` is lab-only and must
  never be reachable from `cmd/omnisnap`.
- **The schema is the contract.** The JSON Schemas in `schema/` are the source
  of truth, Go types are checked against them, and their `$id`s are published at
  `https://omnisnap.org/schema/`. Changes are additive; a breaking change needs
  a new major version, not an edit to `1.json`.
- **Every threshold has evidence.** Numbers that came from the spike (tolerances,
  limits, costs) are named constants with a comment pointing at the measurement.
  Don't tune a threshold to make one page pass: say which pages it was checked
  against and what the false-positive and false-negative effect is.
- **Report, don't block.** The GitHub Action is report-only: it must not fail a
  build (see `specs/SPEC-ci-action.md`). A pass/fail mode is deferred until
  report mode has produced noise data on real projects; a contribution doesn't
  enable it.
- **Nothing leaves the machine.** Captures, bundles and reports stay in the
  output directory and the OS temp directory. Never use a user's real browser
  profile, never download an engine or model without an explicit command, and
  never commit third-party page content outside `testdata/` fixtures the
  project owns.
- **Clean up.** Every process and temporary profile a test starts must be gone
  when it finishes; `go test -race ./...` is required before committing
  anything that touches concurrency.
- **The vendored skills are tooling, not code.** `.agents/skills/` and
  `.claude/skills/` are third-party development aids pinned in
  `skills-lock.json`; their licenses are in
  [THIRD-PARTY-NOTICES.md](THIRD-PARTY-NOTICES.md). Don't edit them by hand, and
  don't let them override the rules above.

## Build and test

You need:

- **Go 1.27.4 or newer**, the version in `go.mod`.
- **`golangci-lint` v2.15.0**, the version CI pins
  (`golangci-lint run ./...` must report 0 issues).
- **At least one installed browser** for anything beyond the unit tests: Chrome,
  Chromium, Edge, Brave, Opera, Vivaldi, Firefox, WebKitGTK, or Safari on macOS.
- **Node.js 20+** only for the planted-fixture check and the prototype in
  `spikes/`. It is not needed to build or run Omnisnap.

| Check | Command |
|---|---|
| Build | `go build -o bin/omnisnap ./cmd/omnisnap` |
| Unit tests (no browsers, no network) | `go test ./...` |
| Race detector | `go test -race ./...` |
| Lint | `golangci-lint run ./...` |
| Planted fixtures match the spike | `node spikes/layout-diff/tools/export-planted.mjs --check` |
| Driver conformance (one suite, every installed engine family) | `go test -tags browsers ./internal/drivers/... -run Conformance -count=1` |
| Integration (real engines) | `go test -tags browsers ./internal/...` |
| End-to-end (the planted suite through the binary) | `go test -tags e2e ./test/e2e/...` |
| Ground-truth tooling | `go run ./cmd/omnisnap-lab` (`serve`, `sites`, `crawl`, `mutate`, `diff`, `dataset`) |
| Lane costs and wall time | `OMNISNAP_COST=1` on the capture tests that measure them |

Notes on the environment-specific lanes:

- **Linux WebKitGTK** needs the driver and MiniBrowser
  (`webkitgtk-webdriver` + `libwebkitgtk-6.0-4`, previously
  `webkit2gtk-driver`), and the conformance lane runs them under a private
  Weston display. The WPE port (`OMNISNAP_WEBKIT=wpe`) is the cheaper headless
  alternative. See `specs/SPEC-capture-webkit-viewport.md`.
- **`OMNISNAP_NO_SANDBOX=1`** exists because Ubuntu 24.04+ denies unprivileged
  user namespaces. Chrome's sandbox crashes before DevTools opens, and
  WebKitGTK's bubblewrap sandbox fails with `bwrap: Creating new namespace
  failed`, which the driver reports only as a session that ended without a
  reply. The switch drops Chrome's `--no-sandbox` and sets
  `WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS=1` for the WebKit lanes. CI sets it
  on Linux; the product never sets it.
- **Golden geometry** for Blink and Gecko is asserted to 0.1 px for boxes and
  2 px for inline vertical metrics. WebKit's goldens predate the local font
  stack and reflow by 6–27 px, so its leg is opt-in with
  `OMNISNAP_GOLDEN_WEBKIT=1`.
- **Font parity matters.** Different Liberation/Noto installs move line boxes;
  `bin/omnisnap check-fonts` reports what the machine has before you conclude
  that a diff is an engine difference.

CI runs, on every pull request and on `main`:

| Job | Runners | What |
|---|---|---|
| `test` | Ubuntu, Windows | build, `omnisnap version`, `go test ./...`, planted-fixture check (Linux), lint |
| `driver conformance` | Ubuntu, Windows | `go test -tags browsers ./internal/drivers/... -run Conformance`, and the viewer render on Linux |

macOS is not in the matrix: hosted macOS runners are billed at roughly ten
times the Linux rate. Safari is covered by the manual `safari-smoke.yml`
workflow. A pull request from a fork may wait for the maintainer's approval
before CI starts, and the schema deploy (`deploy.yml`) only runs on `main`, so
it may be skipped on your pull request.

## Planning significant changes

Omnisnap is built spec-first: the design comes before the code, and the
specification is a reviewed artifact, not a comment in a pull request.

1. **Discuss first.** Describe the problem, who it affects, what already exists
   in the codebase, and the alternatives you considered. A feature proposal can
   follow once the idea has a shape.
2. **Amend the design of record.** Update the module spec in
   `specs/SPEC-<id>.md` (or add one and register it in
   [`specs/CAPABILITY-MAP.md`](specs/CAPABILITY-MAP.md)), and add the work to
   [`tasks/todo.md`](tasks/todo.md) with acceptance criteria and a verification
   step per criterion. Decisions that change the architecture or the risk order
   go in [`tasks/plan.md`](tasks/plan.md).
3. **Get feedback on the plan before implementing it**, as a draft pull request
   that contains only the planning artifacts. Reviewing a plan takes minutes;
   discovering after weeks of work that an implementation went in the wrong
   direction costs everyone far more.
4. **Implement against the approved plan.** Tick a task's boxes only when they
   are verified, record the before and after numbers where the task asks for
   them, and note in the spec wherever the implementation differs from the
   design.

Small fixes, such as typos, documentation tweaks or contained bug fixes, don't
need a planning artifact. The `openspec/` directory is initialized but unused;
the planning artifacts above are the ones that count.

## Submitting a pull request

`main` is the maintainer's branch: contributions arrive through pull requests
from your fork, and the maintainer merges.

1. Fork the repository, and create a branch in your fork from `main`.
2. Open your pull request against `main`.
3. Keep each pull request focused on a single logical change. Don't mix
   refactoring or formatting changes with behaviour changes.
4. Follow the [commit message convention](#commit-messages). Signing off your
   commits is [strongly recommended](#signing-off-your-commits-dco).
5. Make sure your change meets the [basic requirements](#basic-requirements).
6. Write the description in your own words: why the change is needed, what
   changed, how users will notice it, the exact commands you ran and what they
   reported, what's out of scope, and whether AI tools were used.

To keep your branch up to date with `main`:

```bash
git remote add upstream https://github.com/GerMP21/Omnisnap.git
git fetch upstream
git rebase upstream/main
```

### Commit messages

Write the subject line in the imperative mood ("Add", "Fix", not "Added" or
"Fixes"), in sentence case, without a trailing period, and under 72
characters. If the commit implements a task, add its number:

```
Fix the ink crop on clipped overflow boxes (Task 49)
Warm Brave and Edge profiles before capture (Task 65)
```

Use the body to explain what was wrong and what the commit changes, so the
history makes sense without opening the pull request. Add measurements when
they matter. If the commit breaks existing behaviour, say so explicitly in the
body.

### Signing off your commits (DCO)

Signing off is not mandatory, but it is strongly recommended. A sign-off
certifies, under the
[Developer Certificate of Origin](https://developercertificate.org/), that you
wrote the contribution or otherwise have the right to submit it under the
project's license. For AI-assisted work, it's also your recorded statement that
you take responsibility for the code's origin.

Sign off with the `-s` flag:

```
git commit -s -m "Fix the ink crop on clipped overflow boxes (Task 49)"
```

This adds a `Signed-off-by: Your Name <your.email@example.com>` line using your
Git configuration. To sign off your last commit after the fact, run
`git commit --amend --no-edit --signoff`.

**Commits made by AI tools.** Some tools commit under their own identity. A
sign-off should come from the commit's author, so re-author those commits under
your name and sign them off in one pass, then update your branch:

```
git rebase --exec "git commit --amend --no-edit --reset-author --signoff" upstream/main
git push --force-with-lease
```

Only rewrite your own branch this way. The tool's `Co-Authored-By:` trailer is
kept, so it remains credited. Do this before writing the pull request
description, because rewriting changes the commit hashes. A sign-off added by an
AI tool in its own name doesn't count: the certificate is a statement only a
person can make.

### Code style

`gofmt` and `goimports` (CI checks both), lowercase package names matching the
module id without dashes, `context.Context` first on anything that does I/O,
errors wrapped with `%w` and naming the module, JSON field names from the
schema and never ad hoc. Follow the existing style of the code you're
changing, and don't reformat code unrelated to your change: it hides the real
change in the diff and makes review slower.

### Dependencies

Add new dependencies only when they're genuinely needed. For each one:

- Explain in the pull request why it's needed and why the standard library or
  the three approved modules aren't enough.
- Verify that it's the real, actively maintained package from the expected
  publisher by checking its registry page and source repository, not just its
  name. AI tools sometimes suggest packages that don't exist, and malicious
  packages are sometimes published under those names.
- Confirm that its license is compatible with the GNU Affero General Public
  License v3 (Apache-2.0, MIT, BSD and ISC are; GPL-2.0-only is not), and add it
  to [THIRD-PARTY-NOTICES.md](THIRD-PARTY-NOTICES.md) if it is vendored or
  shipped.
- Commit the lockfile changes along with it (`go.sum`; if you add a JavaScript
  dependency to a wrapper, commit its `package-lock.json` too).

### Breaking changes

The schemas, the CLI flags and the job file are contracts. If your change
breaks any of them, or changes an existing report's meaning:

- Discuss it before implementing it, and read the versioning rule in
  `specs/SPEC-bundle-schema.md`.
- Bump the schema major version rather than editing a published `1.json`.
- Say so explicitly in the commit body and in the pull request, explaining who
  is affected and how to migrate.

## Review times

Omnisnap is maintained in limited time, so reviews and replies may take a while,
sometimes several weeks. A slow response doesn't mean your contribution was
ignored or is unwelcome.

- If 30 days pass without a response, one polite follow-up on the pull request
  or discussion is welcome.
- Reviews may go through several rounds of requested changes.
- Pull requests waiting on changes from the contributor may be closed after 30
  days without activity. You're welcome to reopen them when you're ready to
  continue.
- Pull requests waiting on the maintainer are never closed for inactivity.

## AI-assisted contributions

Contributions made with the help of AI tools (code assistants, chat models,
agents, and similar) are welcome, provided the conditions below are met. The
project ships agent skills for exactly this work; using them doesn't lower the
bar.

### Due diligence

In addition to the basic requirements, by submitting an AI-assisted
contribution, you confirm that:

- **You understand the problem.** You know what you're trying to achieve and
  why, and you could describe it without the tool's help.
- **You understand the solution.** You have read all of the generated content
  and can explain, at least conceptually, how it works and why it solves the
  problem.
- **You have verified it.** You have run the tests, looked at a report where
  the change affects capture or triage, considered edge cases, and confirmed
  that every API, dependency and reference it uses actually exists.
- **You have checked licensing.** To the best of your knowledge, the
  contribution does not reproduce code under a license incompatible with the
  GNU Affero General Public License v3.
- **You take responsibility.** You are the author of record, whatever name the
  commit metadata shows, and are accountable for the contribution as if you had
  written every line yourself. "The AI wrote it" is not an acceptable answer
  during review.

A simple test: if the maintainer asks "why does this work?" or "why this
approach?", you should be able to answer in your own words.

### Disclosure

AI use must be disclosed in the pull request description, including:

- The tool or tools used, by name (for example "Claude Code"), without model
  names or versions.
- What they were used for (for example: implementation, tests, refactoring,
  documentation).
- How you reviewed and verified the output, with the commands and their
  results.

Minor assistance, such as single-line autocompletion or spelling and grammar
fixes, does not need to be disclosed.

Commits made with an AI tool must also credit it with a `Co-Authored-By:`
trailer that names the tool, **not a model or its version**:

```
Co-Authored-By: Claude <noreply@anthropic.com>
```

Some tools add a trailer with a model name automatically; change it to the form
above before you push. A trailer or a "Generated with" footer shows that a tool
was involved, but it doesn't replace the disclosure in the pull request
description, which also covers what the tool was used for and how you verified
its output.

### Maintainer discretion

The maintainer may decline any AI-assisted contribution at their sole
discretion, without any obligation to justify the decision. This is not a
judgment of you as a contributor: reviewing generated code takes significant
time, and the maintainer needs to be able to protect it.

Contributions found to contain undisclosed AI-generated content may be closed
without review.

## License

Omnisnap is released under the
[GNU Affero General Public License v3](LICENSE.txt), or (at your option) any
later version. By contributing, you agree that your contributions will be
licensed under the same terms. The vendored agent skills keep their own licenses, listed in
[THIRD-PARTY-NOTICES.md](THIRD-PARTY-NOTICES.md).

Running Omnisnap, auditing a site with it, or using the GitHub Action does not
change the license of your project or of the pages it captures: the AGPL covers
Omnisnap and modified versions of it, not the sites it renders or the reports
it writes. The network clause (§13) matters if you offer a modified Omnisnap to
other people as a service.

## Questions

If something isn't covered here, ask in
[Discussions](https://github.com/GerMP21/Omnisnap/discussions).
