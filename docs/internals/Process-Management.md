# Process management

**Role.** Start each browser in a known, isolated state, and never leave one behind: not a process, not a profile, not a cache. Module `process`, package [`internal/process`](https://github.com/GerMP21/Omnisnap/tree/main/internal/process).

## Inputs and outputs

- **In:** a discovered browser (executable, family), a root for temp dirs, and per family a few options (font parity, viewport, a template profile).
- **Out:** a running browser the [drivers](Drivers) can talk to ([`Blink`](https://github.com/GerMP21/Omnisnap/blob/main/internal/process/blink.go), [`Gecko`](https://github.com/GerMP21/Omnisnap/blob/main/internal/process/gecko.go), [`WebKit`](https://github.com/GerMP21/Omnisnap/blob/main/internal/process/webkit.go) for WebKitGTK, WPE and Safari), each with a `Close` that kills the whole tree and deletes its temp dir.

## How it works

### One launch

Every browser starts through [`spawn`](https://github.com/GerMP21/Omnisnap/blob/main/internal/process/launch.go):

1. A fresh temp dir `<root>/omnisnap-<pid>-<n>/` with an `owner.lock` naming omnisnap's pid, and a `group.lock` naming the browser's pid, which is its process group id ([`profile.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/process/profile.go)). The profile lives inside it; never the user's profile (Chrome 136 and later also refuse remote debugging on the default data dir).
2. The browser leads its own process group on Unix ([`pgroup_unix.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/process/pgroup_unix.go)), so renderers, the zygote and utility processes can be signalled at once. On Windows it goes into a Job Object with `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE` ([`job_windows.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/process/job_windows.go)).
3. The last 16 KB of its output are kept for error messages.
4. On Linux it gets the highest out-of-memory score, `oom_score_adj` 1000, right after start; its children inherit it ([`launch_linux.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/process/launch_linux.go)). [`ApplyOOMScore`](https://github.com/GerMP21/Omnisnap/blob/main/internal/process/launch_linux.go) re-applies it to the tree of every live handle, found by parent pid in `/proc` (a sandbox may move a process out of the group), and the CLI calls it after every new session: renderers forked before the first write, and Chrome lowering its renderers' scores, get 1000 again. So under memory pressure the kernel kills one of Omnisnap's browsers, not the user's, and the crash policy handles it ([Capture pipeline](Capture-Pipeline)). Omnisnap's own process and processes it did not start are never written to; a failed write is ignored. Elsewhere it is a no-op ([`oom_other.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/process/oom_other.go)). Every launch goes through `spawnEnv`, so a new launch path gets the score too.

[`Handle.Close`](https://github.com/GerMP21/Omnisnap/blob/main/internal/process/launch.go) kills the tree (the group, or the job), waits for the exit, and only then removes the temp dir.

### Per family

| Family | Launch |
|---|---|
| Blink | `--headless=new --hide-scrollbars --no-first-run --no-default-browser-check --force-device-scale-factor=1 --disk-cache-size=1 --media-cache-size=1 --remote-debugging-port=0 --user-data-dir=<temp>` ([`blink.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/process/blink.go)). `OMNISNAP_NO_SANDBOX=1` adds `--no-sandbox --disable-dev-shm-usage` for restricted hosts (CI containers, Ubuntu 24.04+ AppArmor); the product never sets it itself. |
| Gecko | headless, `--remote-debugging-port=0`, a profile whose `user.js` sets overlay scrollbars, `devPixelsPerPx` 1.0, no disk cache, and the font-parity `font.name.<generic>.<lang>` prefs ([`prefs.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/process/prefs.go)). Snap Firefox gets its profile under `~/snap/firefox/common/`, because its `/tmp` is private. |
| WebKitGTK (Linux) | `WebKitWebDriver` on a free port; MiniBrowser with font-family arguments, on a **private Weston headless display** started for it ([`webkit.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/process/webkit.go)). No GPU and no desktop session: `GSK_RENDERER=cairo`, `WEBKIT_DISABLE_DMABUF_RENDERER=1`, `GDK_DEBUG=no-portals`. Without Weston, or with `WEBKIT_HEADLESS=0`, the windows are visible. |
| WPE (Linux) | `WPEWebDriver`; MiniBrowser `--headless --size=<viewport>` and a config file carrying the font families ([`wpe.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/process/wpe.go)). No compositor; the viewport is exact. |
| Safari (macOS) | `safaridriver` on a free port ([`safari_darwin.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/process/safari_darwin.go)); a visible window by design. |

Branded browsers start from a **copy** of a warmed template profile ([`template.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/process/template.go), [Branded browsers](Branded-Browsers)). Only the branded checks start Blink with a window (`StartBlinkVisible`); the product always runs it headless.

### Three cleanup layers

1. **Normal exit.** The CLI closes each group's engines before the next group launches, and every engine with `defer` ([`run.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/run.go)): sessions, the process tree, the temp dir.
2. **Abnormal exit.** SIGINT, SIGTERM and SIGHUP cancel the job's context ([`root.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/root.go), [`signals_unix.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/signals_unix.go)), and the same `defer` runs. On Windows, if omnisnap is killed outright, the kernel closes the Job Object handle and kills every browser in it.
3. **The next start.** Before any command other than `version` and `help`, the sweeper removes `omnisnap-*` temp dirs whose `owner.lock` names a process that is gone, and kills the process group in their `group.lock` ([`sweep.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/process/sweep.go), [`root.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/root.go)). This is what recovers from SIGKILL and power loss. Foreign directories, and the current process's own, are never touched.

No disk caches are kept: a crawl would otherwise write hundreds of MB per session, and on WSL the virtual disk never gives the space back.

## Failure handling

A browser that does not come up (no DevTools endpoint, a driver that does not answer `GET /status`) returns an error with its output tail; the CLI skips that engine with the reason. A crashed browser during a job is replaced once by the CLI's `restartable` wrapper ([CLI and orchestration](CLI-and-Orchestration), [Capture pipeline](Capture-Pipeline)).

## Tests

- [`TestCloseKillsTheProcessGroup`](https://github.com/GerMP21/Omnisnap/blob/main/internal/process/pgroup_unix_test.go), `TestExitedBrowserIsReapedOnlyByClose`.
- [`TestJobObjectKillsTheProcessTree`](https://github.com/GerMP21/Omnisnap/blob/main/internal/process/job_windows_test.go) and [`TestTerminatingOmnisnapKillsEveryBrowser`](https://github.com/GerMP21/Omnisnap/blob/main/internal/process/terminate_windows_test.go): it builds the binary, starts a job against a page that never finishes, calls `TerminateProcess` on omnisnap, and checks that every descendant exits. A descendant is found through its parent process id and counts only if it was created after that parent, because Windows reuses process ids and keeps a dead creator's id as the parent: without the check, a reused id once pulled `csrss.exe`, `services.exe` and every `svchost.exe` into the tree. The last check, that no browser is left running under the dead tree, flags only a process created before the tree had exited; one created later took a reused id (PR #23's first run flagged such an `msedge.exe`), and the test logs it with its start time.
- [`sweep_test.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/process/sweep_test.go): the sweep decisions, the leftover group killed, and `TestTwentyKilledRunsAreSwept`.
- [`TestBlinkArgsMatchTheSpec`](https://github.com/GerMP21/Omnisnap/blob/main/internal/process/blink_test.go), [`TestGeckoPrefsGolden`](https://github.com/GerMP21/Omnisnap/blob/main/internal/process/prefs_test.go), `TestWebKitFontArgs`, `TestGDKDebugNoPortals`.
- Launch and close cycles with real browsers (`browsers` build tag): `TestBlinkLaunchCloseCycles`, `TestGeckoLaunch`, `TestWebKitLaunch`, `TestWebKitCyclesLeaveNoProcesses`.

## Where the code differs from the spec

- **No cap on live browsers.** The spec asks for "a configurable cap on concurrently live browsers (default 3)". There is none in the code; the number is bounded only by one browser per engine per job.
- **Weston per WebKitGTK launch, not per process.** The spec has one private Weston per omnisnap process, started lazily. `StartWebKit` starts a display for each WebKitGTK launch and stops it on `Close`. In a job that is one display, since there is one browser per engine; a relaunched WebKitGTK gets a new one.

## Spec

[`specs/SPEC-process.md`](https://github.com/GerMP21/Omnisnap/blob/main/specs/SPEC-process.md).

## Source files

- [`internal/process/launch.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/process/launch.go): `Handle`: spawn, the profile, `Close`
- [`internal/process/blink.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/process/blink.go): `StartBlink`: Blink's flags
- [`internal/process/blink_visible_linux.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/process/blink_visible_linux.go): visible Blink windows on Linux
- [`internal/process/blink_visible_other.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/process/blink_visible_other.go): visible Blink windows elsewhere
- [`internal/process/gecko.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/process/gecko.go): `StartGecko`: Firefox's flags and the BiDi wait
- [`internal/process/prefs.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/process/prefs.go): Firefox's user.js preferences
- [`internal/process/webkit.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/process/webkit.go): WebKitGTK and its Weston display (Linux)
- [`internal/process/wpe.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/process/wpe.go): `StartWPE` (Linux)
- [`internal/process/safari_darwin.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/process/safari_darwin.go): `StartSafari` (macOS)
- [`internal/process/driver.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/process/driver.go): waiting for a WebDriver server
- [`internal/process/profile.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/process/profile.go): temporary profile directories
- [`internal/process/sweep.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/process/sweep.go): `Sweep`: leftovers from a killed run
- [`internal/process/launch_unix.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/process/launch_unix.go): killing the process tree, on macOS
- [`internal/process/launch_linux.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/process/launch_linux.go): killing the process tree, and the out-of-memory score (`ApplyOOMScore`), on Linux
- [`internal/process/oom_other.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/process/oom_other.go): `ApplyOOMScore` as a no-op outside Linux
- [`internal/process/launch_windows.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/process/launch_windows.go): killing the process tree, on Windows
- [`internal/process/pgroup_unix.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/process/pgroup_unix.go): process groups, on Unix
- [`internal/process/pgroup_windows.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/process/pgroup_windows.go): process groups, on Windows
- [`internal/process/job_windows.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/process/job_windows.go): Windows job objects
- [`internal/process/wait_linux.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/process/wait_linux.go): waiting for an exit without reaping, on Linux
- [`internal/process/wait_other.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/process/wait_other.go): the same, elsewhere
