# Tasks

## 1. Before the candidate: release notes and the written promise

- [x] 1.1 In `.github/workflows/release.yml`, "Publish the release": when `docs/release-notes/$TAG.md` exists, the create path passes its content with `--notes` next to `--generate-notes`. The re-run path (`gh release upload --clobber`) does not touch the notes. Update the workflow's header comment. Verify: `actionlint .github/workflows/release.yml` reports nothing new, and the step's shell, run locally with `gh` replaced by `echo`, prints `--notes` only when the file exists.
  - Done 2026-11-26: `actionlint` clean. The step's shell, with `gh` stubbed to echo its arguments: `v0.2.0` with a notes file passes `--generate-notes --notes <file content as one argument>`; `v0.2.0-rc.1` with no file passes `--generate-notes --prerelease` and no `--notes`; a re-run calls only `gh release upload --clobber`.
  - Added in review, 2026-11-26 (owner's call: "next, after the stable"): the npm publish step tags a candidate `next` once the package has a stable version on npm, and `latest` before that, so rc.1 is still `latest` and no workflow logic changes between `v0.2.0-rc.1` and `v0.2.0` (the spec's "a stable release was first a candidate"). Checked with `npm` stubbed: rc.1 with only candidates on npm -> latest; `0.2.1-rc.1` after `0.2.0` -> next; `0.2.0` -> latest; a new package -> latest. The spec requirement, CONVENTIONS and Release-Process say the same.
  - Corrected in the release review, 2026-11-26 (ZANZIBAR-41): the stub run above had no `0.0.0`, but npm lists `omnisnap@0.0.0`, the deprecated name reservation, so the version-list check would have sent rc.1 to `next` on the main package and to `latest` on the five platform packages. The step now reads `npm view <pkg> dist-tags.latest`: a candidate is `latest` when that is empty (E404, a new package) or a candidate, `next` when it is a stable version; any other `npm view` failure stops the job. Checked with `npm` stubbed: latest=0.1.0-rc.7 -> latest, a new package -> latest, 0.2.0 -> latest, 0.2.1-rc.1 after 0.2.0 -> next; a 404 for real gives `latest`, an unreachable registry stops. The real `dist-tags.latest` is `0.1.0-rc.7` for all six packages today. `actionlint` clean.
- [x] 1.2 Add a "Versioning" section to `specs/CONVENTIONS.md` (design decision 4). It covers tag spellings per channel, a stable release as the default everywhere, candidates as prereleases, patches keeping the public surface (and listing what that surface is), minors carrying move notes, schema majors never being edited, tags never being moved, and stable releases cut from a candidate's code. Verify: each requirement in `openspec/changes/release-v0-2-0/specs/release-versioning/spec.md` has a counterpart sentence, and `go test ./test/internals/` passes.
  - Done 2026-11-26: `specs/CONVENTIONS.md` "Versioning" has one bullet per requirement (tags and spellings, stable as default, candidate as pre-release, patches keep the surface, minors say how to move, schema majors, tags never moved, stable from a candidate). `go test ./test/internals/` passes.
- [x] 1.3 Document the notes file in `docs/internals/Release-Process.md`: step 3 gets the `docs/release-notes/<tag>.md` rule, and step 1 says that a stable release is cut from a candidate's code (design decision 1). Verify: `go test ./test/internals/` passes, and `grep -rl 'release.yml' docs/internals/` lists no page that still describes notes as generated only.
  - Done 2026-11-26: Release-Process step 1 gives the stable spelling, points at CONVENTIONS "Versioning" and says a stable release is cut from a candidate's code (its PyPI candidate spelling is corrected to `X.Y.ZrcN`); step 3 has the notes-file rule and the re-run keeping the notes. `go test ./test/internals/` passes; of the pages naming `release.yml` (Release-Process, Testing-and-CI, Website), none describes the notes as generated only.
- [x] 1.4 In `tasks/backlog.md`: set B1's status to done (Task 90, 2026-11-25). Point B3 at this change, and rewrite its text from `v0.1.0` to `v0.2.0` with the owner's decision. B2, B4, B5 and B6 are pointed at their changes by those changes' own tasks. Retarget B7 to `0.3.0`. Verify: `grep -n 'v0\.1\.0' tasks/backlog.md` hits only history (the rc tags) and evidence.
  - Done 2026-11-26: B1 done (Task 90, 2026-11-25); B3 points at this change and is rewritten for `v0.2.0` through `v0.2.0-rc.1`, with B2, B4, B5 and B6 as the blocking list; B4's status names its change and `v0.2.0` (it said "Blocks `v0.1.0`"); B7 retargeted to `0.3.0` (its B2 row struck through as answered); B6's open question no longer says `0.1.x`. `grep -n 'v0\.1\.0' tasks/backlog.md` hits only the rc tags, their evidence, and B3's "not `v0.1.0`" decision.

## 2. Housekeeping (any time before `v0.2.0`)

- [x] 2.1 The owner removes the stale npm `next` dist-tag from the six packages with their own login (design decision 7 has the loop). Verify: for each package, `npm view <pkg> dist-tags` shows only `latest`.
  - Done by the owner; checked 2026-11-26: `npm view <pkg> dist-tags` is `{"latest":"0.1.0-rc.7"}` for `omnisnap` and the five `@omnisnap/*` platform packages, with no `next`.
- [x] 2.2 Confirm `add-developer-wiki` task 7.6 (the `wiki` workflow published from `main` after #12, with the run ID), tick it, and archive the change. Verify: `openspec list --json` no longer lists `add-developer-wiki`, and `openspec/changes/archive/` holds it.
- [x] 2.3 Close `add-omnisnap-website`:
  - check 5.4 against `ci.yml` (do the site tests run?) and tick it, or do it;
  - check 1.2 and 6.1 against the rc.7 deploy run, and tick them with the run ID;
  - the owner does 1.1 (Workers Builds disconnected) and 6.2 (the demo reviewed);
  - archive the change.

  Verify: `openspec list --json` no longer lists `add-omnisnap-website`.

## 3. The candidate `v0.2.0-rc.1`

- [x] 3.1 Confirm the four changes are merged on `main`: `fix-windows-firefox-install`, `run-self-tests-on-release`, `fix-selector-sibling-index` and `cap-live-browsers`. Verify: each change's `tasks.md` has every box ticked except the ones that wait for this tag, and `main`'s CI is green.
  - Done 2026-11-26: all four are on `main` (PR #21, merge e52dd42). `fix-windows-firefox-install`, `fix-selector-sibling-index` and `cap-live-browsers` are 19/19, 17/17 and 17/17; `run-self-tests-on-release` is 17/19, its open 5.3 and 5.4 wait for this tag. `main`'s CI (run 45926072781), Walkthrough (45926072786) and wiki (45926072824) are green on e52dd42.
- [x] 3.2 Bump every version file to `v0.2.0-rc.1` (Release-Process step 1):
  - the six npm `package.json` files and their pins
  - `packages/pypi/pyproject.toml` and `__init__.py`, and `packages/pypi/omnisnap/test/test_api.py`'s `test_version_exposed` (added in the release review, 2026-11-26: CI's `packages` job fails on a bump that misses it, and the rc.7 grep does not find the PyPI spelling)
  - the `version` defaults in `action/action.yml` and `report.yml`
  - the fork template
  - every pinned tag in `README.md`, `action/README.md` (added in the release review: its examples and inputs table pin rc.7 three times), the three guides, `SPEC-ci-action.md` and `SPEC-ci-action-fork-template.md`
  - `docs/internals/GitHub-Action.md`

  Add the rc.1 entry to the decision log in `SPEC-ci-action.md`. The README status block names the candidate, and `--pre` stays (a candidate needs it). Verify: `go run ./tools/release --version v0.2.0-rc.1 --dry-run --stage` passes, and the design decision 5 grep for `v0.1.0-rc.7` finds only history text.
  - Done 2026-11-26: 24 files bumped (PR #22, commit 0f48d91; all 19 checks green): the rc.7 bump's 23 plus `CONVENTIONS.md`, with the decision-log entry, the README status line (the candidate for `v0.2.0`; minors may change an interface until `v1`) and CONVENTIONS' npm sentence, which no longer names a version. `go run ./tools/release --version v0.2.0-rc.1 --dry-run --stage` passes (and refuses `v0.2.0-rc.2`, naming each file); the built binary prints `omnisnap v0.2.0-rc.1`. npm (9 pass) and PyPI (9 passed) wrapper tests, `tools/site`, `go test ./test/internals/ ./tools/release/` and `openspec validate --all` pass. The grep finds `v0.1.0-rc.7` only in history and in format examples (`tools/release` and `tools/site` tests, `Website.md`, the website spec's scenarios, the backlog's B7, the decision log). Workflow diff: version strings only.
- [x] 3.3 The owner merges the bump and pushes `v0.2.0-rc.1`. Verify: the release run is green with every job (`release`, `deploy`, `e2e-action`, `e2e-report`, `packages`); the GitHub release is marked pre-release; `npm view omnisnap version` is `0.2.0-rc.1`; `pip index versions --pre omnisnap` lists `0.2.0rc1`. Record the run ID here.
  - Done 2026-11-27: the tag is annotated, on `0931a83` (the merge of PR #22). Release run 46005759495 is green with every job: build and publish, the deploy, both self-tests and the npm and PyPI publish. The GitHub release is a pre-release with the five binaries and `SHA256SUMS`. `npm view <pkg> dist-tags` is `{"latest":"0.2.0-rc.1"}` for `omnisnap` and the five platform packages; `@omnisnap/linux-x64` showed it about 5 minutes after its publish (13:24:39 to 13:29:10 UTC), the others at once. `pip index versions --pre omnisnap` lists `0.2.0rc1`.
- [ ] 3.4 The owner dispatches "Install (network)" on the tag. Verify: green on `ubuntu-latest` and `windows-latest`. Record the run ID here and in the B4 change.
  - `v0.2.0-rc.1`, 2026-11-27: run 46008712513 passed on `ubuntu-latest` and failed on `windows-latest` twice, neither time from the install (backlog B16). Attempt 1: the runner image's own Firefox wrote its `HKCU\Software\Mozilla` Launcher keys between the registry snapshots. Attempt 2: the registry check passed, and `Uninstall`, run right after the browser closed, hit "Access is denied" on `firefox.exe`. Owner's call (ZANZIBAR-47): fix the test and cut `v0.2.0-rc.2` (task 3.6); this task is checked on rc.2's tag.
- [ ] 3.5 Check the candidate as a user would. `npx omnisnap@0.2.0-rc.1 version`, and `omnisnap version` from `pip install --pre omnisnap==0.2.0rc1`, both print `v0.2.0-rc.1` on Linux. `omnisnap.org` names `v0.2.0-rc.1`. Look at how the generated release notes read (design: Risks). Verify: the outputs are recorded here, and a decision on `--notes-start-tag` for the stable release is noted.
  - 2026-11-27, on Linux: `npx omnisnap@0.2.0-rc.1 version` and a plain `npm i omnisnap` print `omnisnap v0.2.0-rc.1`; `pip install --pre omnisnap==0.2.0rc1` gives `omnisnap v0.2.0-rc.1` and `__version__` `0.2.0rc1`; `omnisnap.org` names `v0.2.0-rc.1` and `0.2.0rc1`. The generated notes list PRs #18 to #22 against `v0.1.0-rc.7`; the owner adds hand-written notes in the GitHub release. Left: the `--notes-start-tag` decision for `v0.2.0`, whose generated notes would list only the bump PR against rc.1.
- [ ] 3.6 The candidate `v0.2.0-rc.2` (owner's call, 2026-11-27, ZANZIBAR-47): rc.1's code plus the B16 fixes in `internal/install` (the network test's registry check blames only HKLM, the Uninstall keys and lines naming the cache; `Uninstall` retries its removals for about a second) and the bump to rc.2. Verify: the pull request's CI is green, "Install (network)" on Windows included (the PR touches `internal/install/`); after the owner tags it, the release run is green with every job, npm `latest` is `0.2.0-rc.2` on all six packages and PyPI lists `0.2.0rc2`; "Install (network)" on the tag is green on both OSes (which closes 3.4); `npx omnisnap@0.2.0-rc.2 version` and `pip install --pre omnisnap==0.2.0rc2` print `v0.2.0-rc.2`. Record the run IDs here.

## 4. The stable `v0.2.0`

- [ ] 4.1 Write `docs/release-notes/v0.2.0.md` (design decision 3): the 0.x promise; the selector change, with one before/after example from a planted page; the browser cap (its flag or field and default, as merged), and the Action running four live browsers; selectors of elements inside an open shadow root naming the host (release review, 2026-11-26); the Windows install fix; and the move from `0.1.0-rc.*` (change the ref; nothing else, unless the merged changes say otherwise). Verify: every user-visible change listed in the four changes' proposals appears in the notes.
- [ ] 4.2 Bump to `v0.2.0` and reword (design decision 6):
  - the version files and every pinned tag
  - the README status block becomes the 0.x promise
  - `--pre` and its sentence go from `README.md` and `packages/pypi/README.md`
  - the roadmap's M2 row says "shipped as `0.2.0`", and the `v1` line says that its criteria are not decided yet
  - the `SPEC-ci-action.md` decision log records "stable `0.2.0`, not `0.1.0`, not `v1`" with the reasons

  Verify: `go run ./tools/release --version v0.2.0 --dry-run --stage` passes; `git grep -n -e '0\.2\.0-rc\.2' -e '0\.2\.0rc2' -e '--pre'` finds only history text; `git diff v0.2.0-rc.2 -- '*.go' '*.js' .github/workflows` shows only version-string lines.
- [ ] 4.3 The owner merges and pushes `v0.2.0`. Verify: the release run is green with every job, and the GitHub release is "Latest" and not a pre-release (`gh release view v0.2.0 --json isPrerelease,isLatest`). Record the run ID here.
- [ ] 4.4 Check the spec's scenarios:
  - plain `pip install omnisnap` and `npm i omnisnap` install `0.2.0` on Linux, Windows and macOS (the walkthrough or a by-hand run on each);
  - `omnisnap version` prints `v0.2.0`;
  - `npm view omnisnap dist-tags` is `{ latest: '0.2.0' }`;
  - `omnisnap.org` names `v0.2.0`;
  - "Install (network)" is green on the tag.

  Verify: every outcome is recorded here with its run ID or command output.
- [ ] 4.5 Update `README.md`'s "Next, from the backlog" list and the backlog statuses (B2-B6 done, with the tag; B7 for `0.3.0`). Verify: `go test ./test/internals/` passes.

## Workflow follow-up

- Archive this change after `v0.2.0` is out and task 4.4 is recorded, and the four changes it depends on are archived.
- When "what earns `v1`" is decided, add it to `tasks/todo.md` as a checkpoint with criteria (design: Open Questions).
