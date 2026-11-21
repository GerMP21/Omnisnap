# Branded browsers

**Role.** Capture Blink browsers that ship their own blocking, Brave (Shields) and Edge (tracking prevention), **as users run them**, and explain their differences from Chrome by the requests they blocked. Module `branded`, package [`internal/branded`](https://github.com/GerMP21/Omnisnap/tree/main/internal/branded).

A fresh Brave profile has no filter lists yet and behaves like Chrome. To capture Brave as shipped, the profile must have downloaded its lists first: that is what a **warmed template** is.

## Inputs and outputs

- **In:** a discovered Blink browser and the Omnisnap cache; at analysis time, the failed requests each capture recorded.
- **Out:** a template profile in the cache; per result, the blocked hosts on each side; per cause, `blockedBy` (the blocked hosts its element's resources come from).

## How it works

### Warmed template profiles

`omnisnap warm <browser>` runs [`Warm`](https://github.com/GerMP21/Omnisnap/blob/main/internal/branded/warm.go) ([`internal/cli/warm.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/warm.go)):

1. Launch the browser headless with a fresh profile.
2. Visit the neutral [`WarmPages`](https://github.com/GerMP21/Omnisnap/blob/main/internal/branded/warm.go) (`example.com`, `wikipedia.org`), waiting for network idle and then a fixed settle (8 s per page by default) so the blocking components download their lists.
3. Copy the profile into `<cache>/templates/<browser>-<version>/` ([`TemplateDir`](https://github.com/GerMP21/Omnisnap/blob/main/internal/branded/templates.go)).

A template is **fresh** while the browser's major version is the same and it is younger than 7 days ([`Fresh`](https://github.com/GerMP21/Omnisnap/blob/main/internal/branded/templates.go), [`TemplateMaxAge`](https://github.com/GerMP21/Omnisnap/blob/main/internal/branded/templates.go)). When the CLI launches a Blink browser and a fresh template exists for it, the browser starts from a **copy** of the template ([`startBlink`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/env.go), [`StartBlinkTemplate`](https://github.com/GerMP21/Omnisnap/blob/main/internal/process/template.go)); otherwise from a fresh profile. The copy skips lock and socket files, and the template never accumulates state.

### Blocked-request attribution

Capture records every failed request; those failing with `ERR_BLOCKED_BY_CLIENT` are blocked by the browser itself ([Capture pipeline](Capture-Pipeline)). In the analysis ([`internal/pipeline/diff.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/pipeline/diff.go)):

1. [`BlockedHosts`](https://github.com/GerMP21/Omnisnap/blob/main/internal/branded/blocked.go) lists the distinct hosts the engine blocked.
2. For every cause, the resources (`src`/`href` keys) of its element and its descendants are matched against them with [`Matches`](https://github.com/GerMP21/Omnisnap/blob/main/internal/branded/blocked.go); a subdomain matches its parent's blocked host.
3. A match goes into `cause.blockedBy`, and the result lists the blocked hosts per side.

A branded browser's cause with no blocked request behind it is not expected: it is reported normally.

## Failure handling

A warm page that fails to load is logged and the warm goes on with the next page. A missing or stale template is not an error: the browser starts fresh.

## Tests

- [`blocked_test.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/branded/blocked_test.go): `TestBlockedHosts`, `TestHostOf`, `TestMatches`, `TestTemplateFreshness`.
- [`warm_browsers_test.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/branded/warm_browsers_test.go) (`browsers` tag): `TestWarmTemplateBlocksTrackers` (a warmed Brave blocks the trackers on [`testdata/branded/trackers.html`](https://github.com/GerMP21/Omnisnap/blob/main/testdata/branded/trackers.html) where a fresh one does not) and `TestEdgeTrackingPrevention`.

## Where the code differs from the spec

- **`--neutral` only labels the result.** The spec says `--neutral` launches fresh profiles with Brave Shields and Edge tracking prevention turned off by switches or prefs. In the code, the flag sets `job.neutral`, and the only effect is `profile: neutral` in the results and `host.json` ([`bundle.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/bundle.go)). The launch is unchanged: a branded browser with a fresh template still starts from it, with its blocking on.
- **No automatic warm.** The spec warms a template "automatically on first use, with a progress message". The code warms only on `omnisnap warm`; without a template the browser starts fresh, like Chrome.
- **Fixed settle, not component readiness.** The spec waits "until the blocking components report ready" (Brave's adblock component and list versions, Edge's Trust Protection Lists). `Warm` waits for network idle and a fixed settle per page.
- **Filter-list and component versions are not recorded** in `host.json` or `summary.json`, as the spec asks. The template's version line is printed by `omnisnap warm` only.
- **Edge.** The spec's own open question: Edge's tracking prevention blocked nothing under headless automation or headed on Linux in the spike, so Edge is not claimed for `blockedBy` attribution.

## Spec

[`specs/SPEC-branded.md`](https://github.com/GerMP21/Omnisnap/blob/main/specs/SPEC-branded.md).

## Source files

- [`internal/cli/warm.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/warm.go): `omnisnap warm`
- [`internal/branded/blocked.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/branded/blocked.go): the hosts a branded browser blocked
- [`internal/branded/templates.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/branded/templates.go): warmed templates and whether they are stale
- [`internal/branded/warm.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/branded/warm.go): `Warm`: build a template profile
- [`internal/process/template.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/process/template.go): launching from a template profile
