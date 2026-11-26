# Design

## Context

See proposal.md for why, and `specs/release-versioning/spec.md` for the required behavior. What shapes the approach (checked 2026-11-25):

- **The tooling already understands a stable tag.**
  - `release.yml` passes `--prerelease` only for a tag with a dash.
  - `tools/release`'s `packageVersions` and `tools/site/build.py`'s `versions` turn `v0.2.0` into `0.2.0` for npm and PyPI, and `v0.2.0-rc.1` into `0.2.0-rc.1` and `0.2.0rc1`.
  - The `packages` job publishes with `--tag latest`. (Changed in review, 2026-11-26: a candidate goes to `next` once a stable version exists on npm, so the first candidate after `v0.2.0` cannot replace it; rc.1 is still `latest`.)
  - `deploy.yml` and `e2e-report.yml` pick the newest release that has `SHA256SUMS`, prerelease or not.
  - `tools/release --stage` refuses a tag that the package files were not bumped for (`checkPackageVersions`).
- **The release notes are generated only.** `gh release create … --generate-notes`. gh can put extra notes ahead of the generated ones with `--notes` (`gh release create --help`: "Additional release notes can be prepended to automatically generated notes by using the `--notes` flag").
- **Where the tag is pinned.** A grep for `v0.1.0-rc.7` finds it in `README.md` (install, Action, inputs table, roadmap), the three user guides, `specs/SPEC-ci-action.md` and `SPEC-ci-action-fork-template.md`, `docs/internals/GitHub-Action.md`, `Release-Process.md` and `Website.md` (as an example), the version defaults in `action/action.yml` and `report.yml`, the fork template, and the package files.
- **The npm dist-tag.** A stale `next: 0.1.0-rc.6` sits on all six npm packages. The trusted publisher has no dist-tag permission, so the owner removes it with their own login.
- **The other four changes.** This change depends on B2, B4, B5 and B6 being merged. Its own code is one workflow edit. The rest is the release, done twice (candidate, then stable), and its wording.

## Goals / Non-Goals

**Goals:**
- Cut `v0.2.0` with the least new machinery: one workflow edit (release notes), then the existing Release-Process steps twice.
- Make the two version bumps hard to get half right. Every file that names the tag moves together, and the existing `--stage` check plus one grep prove it.

**Non-Goals:**
- Deciding what earns `v1`, or adding a moving `@v0.2` tag. See Open Questions.
- Backlog B7. Its "breaking" rows now target `0.3.0`.
- Changing how candidates are published.

## Decisions

### 1. The path: one candidate, then the stable, from the same code

```
 main (B2, B4, B5, B6 + this change's release.yml edit merged)
   |
   | bump to 0.2.0-rc.1 (Release-Process step 1)   <- PR, owner merges
   v
 tag v0.2.0-rc.1 --> release.yml
                       release ----> deploy
                         |  \
                         |   e2e-action, e2e-report (B5)
                         |                \
                         v                 v
                      packages (npm 0.2.0-rc.1, PyPI 0.2.0rc1)
   |
   | owner dispatches "Install (network)" on the tag (B4)
   | all green?  no --> fix on main, cut v0.2.0-rc.2, repeat
   | yes
   v
 bump to 0.2.0 + README/PyPI wording + release notes   <- PR, owner merges
   |
   v
 tag v0.2.0 --> release.yml (no --prerelease: "Latest")
```

The second PR changes only version strings, wording and the notes file. That is the spec's "a stable release was first a candidate". A failing candidate is never fixed by moving its tag: the fix goes to `main` and becomes `-rc.2`.

*Alternative:* tag `v0.2.0` straight from `main`. Rejected by the owner: B4 and B5 can only be proven on the runners, on a real tag.

### 2. No `v0.1.0` stable

The `v0.1.0-rc.*` line ends at rc.7. Nothing is published as `0.1.0`. A consumer on `0.1.0-rc.7` moves to `0.2.0` directly.

For npm the jump is harmless: a caret range on a prerelease (`^0.1.0-rc.7`) never resolves to `0.2.0`, so nobody is moved without asking. For PyPI, `pip install omnisnap` picks the newest stable, which is `0.2.0`. That is the point. The `go.mod` retractions (rc.1, rc.3) stay as they are.

### 3. Release notes: an optional hand-written file, prepended

In `release.yml`'s "Publish the release" step, if `docs/release-notes/$TAG.md` exists, pass its content with `--notes "$(cat …)"` next to `--generate-notes`. gh documents that pairing as prepending. Only the create path reads the file. A re-run of an existing release (the `gh release upload --clobber` path) keeps the notes the release already has, so the owner's later edits on GitHub are never overwritten.

`docs/release-notes/v0.2.0.md` covers:
- what `0.2.0` promises (the 0.x rule)
- the selector change from B2 (change `fix-selector-sibling-index`): selectors now follow the DOM (`:nth-of-type` counts `display: none` siblings, as CSS does) and are escaped CSS (`md\:flex`), and DOM audit findings use the same selector as causes. So a selector recorded from a `0.1.0` candidate may differ for the same element, and bundles from a candidate and from `0.2.0` should not be merged together. Before/after from `testdata/planted/selector-hidden-sibling.html`: the third card, behind a hidden first one, was `div#projects-list > div.project-card:nth-of-type(2)` (which selects the card before it) and is now `…:nth-of-type(3)`
- the browser cap from B6: its flag or field and its default
- the Windows install fix from B4
- nothing to change for `report.yml` consumers but the ref

Candidates get no notes file unless there is something to say.

*Alternatives:* edit the notes by hand after the release (rejected: easy to forget, and the release is "Latest" with thin notes in the meantime); a `CHANGELOG.md` (rejected for now: one more file to keep in step for a project with one stable release; can come later without changing anything here).

### 4. The 0.x promise lives in two places

- `specs/CONVENTIONS.md` gets a "Versioning" section, which is normative and states the spec's requirements in the project's own words.
- The README status block carries the short user version:

  > Status: `v0.2.0`. Within `0.2.x`, patches fix bugs and never change the CLI, the schemas, the progress events, the Action's inputs or the npm and PyPI APIs; `0.3.0` may, and its notes say how to move.

The schema-major rule is already implicit in `SPEC-bundle-schema.md`'s URLs. CONVENTIONS states it outright.

### 5. The version bumps are checked mechanically

Each bump PR is checked with `go run ./tools/release --version <tag> --dry-run --stage`, the same check the release workflow runs. It is also checked with one grep for the old tag: `git grep -n 'v0\.1\.0-rc\.7' -- . ':!docs/status-2026-11-19.md'`. Every hit left must be history text, such as the decision log in `SPEC-ci-action.md`, `tools/release`'s comments about past tags, or `go.mod`'s retractions.

At `v0.2.0` the same check runs for `0.2.0-rc.1`, `0.2.0rc1` and `--pre`.

### 6. What gets reworded at the stable release, and what doesn't

| File | At `v0.2.0-rc.1` | At `v0.2.0` |
|---|---|---|
| version files, `report.yml`/`action.yml` defaults, fork template | `v0.2.0-rc.1` | `v0.2.0` |
| README status block | "candidate `v0.2.0-rc.1`" | the 0.x promise |
| README / PyPI README `--pre` | kept: pip needs it for a candidate | removed with its sentence |
| README roadmap ("M2 = v1", "`v1` waits for…") | unchanged | M2 "shipped as `0.2.0`"; `v1` criteria "not decided" |
| `SPEC-ci-action.md` decision log | adds the rc.1 entry | records "stable `0.2.0`, not `0.1.0`, not `v1`", with the reasons |
| `docs/status-2026-11-19.md` | unchanged (a dated report) | unchanged |

### 7. Housekeeping is tracked here, done by the owner where it needs them

- **npm dist-tag.** The owner runs it by hand with their own login, any time before `v0.2.0`:
  `for p in omnisnap @omnisnap/linux-x64 @omnisnap/linux-arm64 @omnisnap/darwin-x64 @omnisnap/darwin-arm64 @omnisnap/win32-x64; do npm dist-tag rm "$p" next; done`
- **`add-developer-wiki`.** It is archived after the owner confirms task 7.6: the wiki workflow has published from `main` on every merge since #12.
- **`add-omnisnap-website`.**
  - Tasks 1.1 and 6.2 are the owner's.
  - 5.4 is checked against `ci.yml`.
  - 1.2 and 6.1 are checked against what rc.7 already did.
  - The change is archived once all five are ticked.

## Risks / Trade-offs

- [The candidate is red for a reason outside the four changes, such as a runner image change] → Fix on `main`, cut `-rc.2`. The tag is never moved. The cost is one more tag.
- [A consumer reads "0.2.0" as "there was a 0.1.0 I missed"] → The release notes and the README say that `0.1.0` was only ever previewed as candidates.
- [The second PR sneaks in a code change] → The spec scenario (`git diff v0.2.0-rc.1 v0.2.0` touches no Go, JS or workflow logic) is a task check before tagging.
- [The generated notes for `v0.2.0` list only the bump commit, since the previous release is rc.1] → The hand-written notes carry the substance. Check how the generated part reads on rc.1, and pass `--notes-start-tag v0.1.0-rc.7` for `v0.2.0` if it is useless.
- [Users already pinned to `0.1.0-rc.7` on npm stay there] → Intended. The notes say how to move.

## Migration Plan

1. Merge B4, B5, B2 and B6, plus this change's `release.yml` edit.
2. Bump PR to `v0.2.0-rc.1`. The owner merges it and pushes the tag.
3. Watch the release run (release, deploy, both self-tests, packages). The owner dispatches "Install (network)" on the tag.
4. If anything is red, fix on `main` and go back to step 2 with `-rc.2`.
5. Stable PR (version, wording, notes). The owner merges it and pushes `v0.2.0`.
6. Check the spec's scenarios: plain `pip` and `npm` installs, "Latest", the dist-tags, `omnisnap.org`.

**Rollback:** there is no rollback of a tag. A broken `v0.2.0` is followed by `v0.2.1`. If the binary itself is unusable, `v0.2.0` is retracted in `go.mod` and yanked on PyPI and npm (npm `deprecate`, not `unpublish`).

## Open Questions

- What earns `v1`? Candidates: a run of `0.x` minors with no breaking change, consumers beyond the owner's own sites, B7 answered, viewports or iframes. When decided, it becomes a checkpoint in `tasks/todo.md`. It does not change this release.
- Should consumers be told to pin a moving `@v0.2`? It would be a new tag kind, added later without changing anything here.
