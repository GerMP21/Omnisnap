# Design

## Context

See proposal.md for the motivation and `specs/browser-concurrency/spec.md` for the required behaviour. What shapes the approach:

- **One browser per engine for the whole job.** `runJob` in `internal/cli/run.go` launches every engine up front (`running := map[string]*restartable{}`), checks font parity across all of them, then loops over pages. For each page it calls `capture.RunPage`, which captures the baseline's run a first (the failure policy) and then runs the protocol lanes in parallel (`internal/capture/lanes.go`: CDP+BiDi in one lane, WebDriver Classic in the other, each "run a of every engine, then run b"). A deferred loop closes every browser at the end.
- **The order is page-major.** Every page goes through every engine before the next page starts. SPEC-capture.md asks for this so a live page cannot drift between engines ("Interleaving").
- **Launches go through one place.** Every process `internal/process` starts (Blink and Gecko browsers, the WebKit and WPE drivers, which start MiniBrowser and the web processes, and the private Weston) goes through `spawnEnv` in `launch.go`. Each runs in its own process group (`setProcessGroup`), and `Handle.afterStart()` is a per-OS hook called right after `cmd.Start()`. It is empty on Unix and assigns the Job Object on Windows.
- **The crash policy already handles a killed browser.** A capture whose browser dies is retried once in a fresh browser (`Engine.Relaunch`), and a second crash takes the engine out of the job with a reason (`internal/capture/crash.go`, Task 89). An out-of-memory kill looks like any other crash.
- **The font check needs the baseline.** `checkFonts` (`internal/cli/fonts.go`) opens a session in every running engine, then compares each one's font report against the baseline's.
- **Linux `oom_score_adj`.** The value runs from -1000 to 1000 and is inherited across `fork`. Any process may raise its own or a same-user process's value. Lowering it below the floor (`oom_score_adj_min`) needs `CAP_SYS_RESOURCE`, and only privileged writes move that floor. At 1000 the kernel always prefers that process, and among several at 1000 it picks the largest.
  Source: https://www.kernel.org/doc/html/latest/filesystems/proc.html ("/proc/<pid>/oom_adj & /proc/<pid>/oom_score_adj")

## Goals / Non-Goals

**Goals:**
- A user's own applications are not the out-of-memory killer's first choice while Omnisnap runs.
- Peak memory grows with the cap, not with the number of installed browsers.
- Nothing changes for the jobs people run most (three engines or fewer), so the planted gates and the recorded progress streams carry over unchanged.

**Non-Goals:**
- A free-memory check before launch (backlog approach 3). Estimating a browser's footprint per page is unreliable, and the cap plus the score cover the incident. It stays an open question.
- cgroups or `systemd-run --scope` memory limits. They need a systemd user session, and the behaviour differs by distribution.
- Changing the default engine set, the cap's default, or any launch flag. Each of those is "ask first" in SPEC-process.md.
- Lowering memory use per browser.

## Decisions

### 1. The out-of-memory score: 1000, set after start and re-applied to the tree

`afterStart` gains a Linux implementation (a new `launch_linux.go`; `launch_unix.go` keeps darwin). It writes `1000` to `/proc/<pid>/oom_score_adj` for the process it just started. Children forked after that write inherit the value. To cover any child forked in the milliseconds before the write, a `Handle.ApplyOOMScore()` walks the handle's descendant tree and writes 1000 wherever a process reads less. It walks by `PPid` in `/proc/<pid>/stat`, not by process group, because a sandbox may move a process out of the group. It runs again each time a session is opened (`NewSession` in the CLI's engine wrapper), which is cheap: one `/proc` scan per page and engine.

- **Why 1000 and not, say, 500:** at 1000 every Omnisnap process outranks every user process whatever its size. Among Omnisnap's processes the kernel then picks the largest, which frees the most memory. A lower value would still lose to a large enough user browser, which is the very case in the incident.
- **Weston and the drivers too:** they are small, so the kernel picks a browser process first in practice. Treating every process Omnisnap launches the same keeps the rule simple, and a killed Weston is reported like a crashed WebKitGTK.
- **Failures are silent:** a write that fails (a process that exited, a hardened kernel) is ignored and logged under `--verbose`. The score is a safety net; it must not fail a capture.
- **Browsers that set their own scores:** Chrome adjusts its renderers' scores itself (zygote), and may lower one again after our write. An unprivileged write cannot lower a value below the floor, but Chrome's own values (well above 0) are still above a user's ordinary processes. The re-apply at each session restores 1000 for the next page.

*Alternatives:*
- `choom -n 1000 -- <exe>` as a wrapper sets the score before `exec`, with no gap. Rejected: it adds a util-linux dependency that minimal images may not have, and it is one more process in the tree that cleanup must know about.
- Raising `omnisnap`'s own score around `cmd.Start()` and restoring it afterwards. Rejected: Omnisnap itself is exposed during the window, and any other goroutine that forks then (a crash relaunch in the other lane) inherits 1000 too.

### 2. The cap groups engines; each group runs the whole page list

```
 today (page-major, all engines live):

   launch E1..E8 ---> page 1: E1..E8 ---> page 2: E1..E8 ---> ... ---> close all

 with the cap (N = 3, groups in job order, baseline first):

   group 1: launch E1 E2 E3 ---> page 1..P (E1 E2 E3) ---> close E1 E2 E3
   group 2: launch E4 E5 E6 ---> page 1..P (E4 E5 E6) ---> close E4 E5 E6
   group 3: launch E7 E8    ---> page 1..P (E7 E8)    ---> close E7 E8

   within a group, per page: baseline run a (group 1 only), then the lanes
   in parallel, each "run a of every engine, then run b" (unchanged)
```

`runJob` becomes a loop over groups:

1. Take engines from the job's list in order, baseline first, and launch them until N are live or the list runs out. A failed launch is recorded in `p.skipped` and the next engine is tried. The baseline failing to launch stays fatal, as today.
2. Run the font check for the group's engines. Keep the baseline's font report from group 1 and compare later groups' reports against that stored report, since the baseline browser is closed by then.
3. Loop over the pages with a `capture.Plan` whose `Engines` are the group's. Group 1 runs with the baseline and the failure policy. Later groups run with `Baseline: ""` and skip every page group 1 marked unreachable (the `failed.json` it wrote).
4. Close the group's browsers and wait for their process trees to die (`Handle.Close`) before step 1 launches the next group, so the cap holds at the moment of the next launch.

With N or more engines there is exactly one group, and the loop runs the same calls in the same order as today: that is how "jobs within the cap are unchanged" holds by construction, not by a special case.

*Alternatives:*
- **Rolling per page:** keep page-major order and cycle browsers within each page. This keeps full interleaving, but costs a launch per engine per page (twice with `runs: 2`, since run b follows every engine's run a). A Firefox or WebKitGTK launch takes seconds, so a 12-page job with eight engines would spend minutes launching. Rejected.
- **Rolling per engine:** start the next engine as soon as any one finishes all its pages, instead of whole groups. It overlaps a little better, but lanes finish at different times and the order would depend on timing, which makes progress streams and tests nondeterministic. Rejected for now. Groups are deterministic and the cost is the idle tail of a group's slower lane.
- **Keep the baseline live in every group:** this does not help drift unless the baseline captures each page again, which would change the bundle (more runs than `runs`). Rejected.

### 3. The cap is a CLI flag, not a job field

`--max-browsers N` goes in `internal/cli/options.go`, next to `--resume` and `--allow-small-pages`, which are also about how this run executes rather than what it captures. The job file describes the pages, engines, viewport and runs, and it is shared between machines and CI. How much memory a machine can spare is a property of the machine. It also keeps `schema/job.schema.json` and its URL major unchanged. N must be at least 1, and anything else is exit code 1 (usage) before discovery.

*Alternative:* a `maxBrowsers` job field, plus the flag as an override. Rejected: it changes a public schema for a value that belongs to the host, and a job file committed with `maxBrowsers: 8` would carry one developer's machine into everyone's runs.

### 4. Page status and progress across groups

`page start` is emitted when group 1 first touches a page (with `WritePageJSON`), and `page done` after the last group's captures of that page. To do that, `runJob` keeps the per-page status, and the page-done events of all pages are emitted after the last group, in page order. With one group, this is the same position as today: right after the page's last capture. `--resume` works per group: `capture.ResumeState` is asked with the group's engine ids, so a group whose captures for a page are all on disk skips it.

### 5. The Action does not set the flag

On `ubuntu-latest` the discovered set is Chrome, Chromium, Edge, Firefox and WebKitGTK. Chromium and Edge fail to launch (backlog B1, "Not in this item"), so with decision 2's rule three browsers are live and there is one group, as today. Windows and macOS runners discover three or fewer that start. So the Action keeps today's order without a new input.

## Risks / Trade-offs

- [Live pages can drift between groups: group 2 captures a page minutes after the baseline did] → It only applies to jobs with more engines than the cap, which today risk an out-of-memory kill instead. The README note says so and names `--max-browsers` for users who want the old order and have the memory. The run pairing within an engine (run a against run b) is untouched.
- [Wall time grows for big jobs: groups run one after another, and a group's faster lane waits for its slower one] → It's measured in task 1.1 and stated in the README note. The cap is a flag, so a CI with memory to spare can raise it.
- [Chrome lowers its renderers' scores again after our write] → The re-apply at every session restores 1000 for the next page, and a process at Chrome's own value still ranks above the user's ordinary processes. The task 2.4 check reads the scores in the middle of a capture.
- [A future launch path bypasses `spawnEnv` and misses the score] → A test asserts that every handle started through the package's start functions reads 1000 (task 2.4), and the developer wiki's Process-Management page states the rule.
- [Group order puts both WebKit engines (WebKitGTK and WPE) in one group, so the CDP+BiDi lane sits idle while they run] → Acceptable for now. The order stays the job's, and reordering for lane balance is an open question.

## Migration Plan

- Lands in `v0.2.0-rc.1` with the other `0.2.0` changes, and the release notes name the new flag and the grouped order.
- Rollback: `--max-browsers` with the engine count restores today's order on any machine. A code rollback is a revert of this change. No bundle, schema or cache format changes, so old and new bundles mix freely in `omnisnap merge`.

## Open Questions

- **Lane balance in groups:** should a group take one engine per lane where it can (for example, the first WebKit-family engine in group 1), to shorten wall time? It changes no requirement (the order within the job's groups is "job order" today). Decide from task 1.1's timings.
- **A memory check before launch (approach 3):** worth adding once the per-engine peaks from task 1.1 exist, as a warning only?
- **Exposing `max-browsers` in the Action and `report.yml`:** not needed for the hosted runners (decision 5). Revisit if a consumer with self-hosted runners asks.
