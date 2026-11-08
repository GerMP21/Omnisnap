# Spec: branded

Module id: `branded` · Package: `internal/branded` · Depends on: `capture`, `triage` · Milestone: M1 · Conventions: [CONVENTIONS.md](CONVENTIONS.md)

## Objective

Run branded Blink browsers (Brave, Edge, Opera, Vivaldi) **as shipped**, because their built-in features (Brave Shields, Edge tracking prevention) are exactly what makes them differ from Chrome, and explain each difference by its cause: the requests the browser blocked. `--neutral` turns that off on request (PRD 1.1.0).

Spike evidence: the Blink family is pixel-identical to Chrome on every test page; branded browsers differ only where their features act. Brave blocks nothing from a fresh profile and blocks from a **warmed** one (0 vs 3 on the test page). Edge downloads its tracking-prevention lists as a component and blocked nothing under headless automation even after a forced update.

## Behaviour

### Warmed template profiles

- `omnisnap warm <browser>` (and automatically on first use, with a progress message): launch the browser with a fresh template profile, visit a few neutral pages, wait until the blocking components report ready (Brave: adblock component and filter-list versions; Edge: Trust Protection Lists component), close.
- Templates live in the user cache (`<cache>/omnisnap/templates/<browser>-<version>/`); every capture uses a **copy** (`SPEC-process.md`).
- A template is rebuilt when the browser's major version changes or it is older than 7 days.
- Filter-list and component versions are recorded in `host.json` and `summary.json`.

### Blocked-request attribution

- `drivers` reports failed requests; requests failing with `ERR_BLOCKED_BY_CLIENT` (or the browser's equivalent) are blocked requests.
- Per result: blocked hosts for baseline and engine. When the engine blocked requests the baseline loaded, the result gets a warning naming them, and every cause in an element whose resources (src/href, or children's) come from a blocked host is linked to that block (`cause.blockedBy`).
- A branded browser's causes without blocked requests and with Blink-identical geometry are not expected: they are reported normally.

### `--neutral`

Fresh profile, Brave with Shields disabled through its launch switches or prefs where possible, Edge with tracking prevention set to off by prefs. The result records `profile: neutral`. Not the default.

## Commands

```
go test ./internal/branded/...                          # unit: attribution, template freshness logic
go test -tags browsers ./internal/branded/...           # Brave warm vs fresh blocking on a local test page with tracker hosts
go run ./cmd/omnisnap warm brave
```

## Project structure

`internal/branded/{warm.go, templates.go, blocked.go, neutral.go}`; `testdata/branded/` local test page that requests known-blocked hosts (served from a local server with host mapping).

## Code style

Per conventions. Attribution is a pure function over a result and its failed-request lists.

## Testing strategy

- **Unit:** attribution links a cause to a blocked host through `src`, `href` and descendants; warning text.
- **Integration (Linux, Windows):** Brave fresh vs warmed on the local tracker page: blocked count 0 vs ≥ 1; Edge measured and reported (the spike's open question).
- **Regression:** Chromium, Brave, Edge on the planted suite: 0 causes (they are Blink).

## Boundaries

- **Always:** run as shipped by default; record list versions.
- **Ask first:** adding a browser-specific handler (one adapter per family is the rule; a branded handler only when a real bug demands it).
- **Never:** use the user's real profile or its lists; neutralize by default.

## Success criteria

1. Brave from a warmed template blocks requests on the tracker test page; from a fresh profile it does not (reproduces the spike).
2. A blocked-resource difference on a test page (an ad slot that collapses) is reported with `blockedBy` naming the host.
3. Chromium, Brave and Edge produce 0 causes on the planted suite.
4. Templates refresh on a browser major-version change; a warm completes in < 60 s.

## Open questions

- Edge tracking prevention blocked nothing under headless automation (spike). Is it disabled under automation, or headless? Test headed on a Windows runner before claiming Edge support.
- Should `warm` run automatically in CI (first run per runner) or require the cache to be restored from a previous run?
