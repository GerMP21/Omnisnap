# Spec: process

Module id: `process` · Package: `internal/process` · Depends on: `discovery` · Milestone: M0 · Conventions: [CONVENTIONS.md](CONVENTIONS.md)

## Objective

Start browsers in a known, isolated state and **always** clean them up: every child process, every temporary profile, every headless display, even when `omnisnap` is killed. Users never see browser windows, never find their real profile touched, and never lose disk space or memory to leftovers.

Why it matters (spike evidence): killed runs left orphaned Firefox profiles in `/tmp` (2.5 GB, up to 600 MB each from HTTP cache); two parallel capture processes ran an 8 GB WSL VM out of memory; WebKitGTK popped up windows until a private headless display was used.

## Tech stack

`os/exec`; Windows Job Objects via `golang.org/x/sys/windows` (`JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`); Unix process groups (`Setpgid`, kill by negative pgid); Weston (`--backend=headless`) for WebKitGTK, Xvfb as fallback.

## Behaviour

### Launch configuration per family

| Family | Flags / prefs (from the spike, `lib/engines.mjs`) |
|---|---|
| Blink | `--headless=new`, `--hide-scrollbars`, `--no-first-run`, `--no-default-browser-check`, `--force-device-scale-factor=1`, `--disk-cache-size=1`, `--media-cache-size=1`, `--remote-debugging-port=0`, `--user-data-dir=<temp>` |
| Gecko | `--headless`, `--remote-debugging-port=0`, profile in `<temp>` with prefs: `ui.useOverlayScrollbars=1`, `widget.gtk.overlay-scrollbars.enabled=true`, `layout.css.devPixelsPerPx=1.0`, `browser.cache.disk.enable=false`, `browser.cache.disk.capacity=0`, font-parity prefs (`font.name.<generic>.<lang>`) |
| WebKitGTK | `WebKitWebDriver --port=<free>`; MiniBrowser `--automation` with font-parity args (`--default-font-family`, `--serif-font-family`, `--sans-serif-font-family`, `--monospace-font-family`); `GDK_BACKEND=wayland`, `WAYLAND_DISPLAY=<private weston socket>`, `DISPLAY` unset. Headless Weston has no GPU: `GSK_RENDERER=cairo` and `WEBKIT_DISABLE_DMABUF_RENDERER=1`, or the browser dumps core before the first frame. It has no desktop session either: `GDK_DEBUG=no-portals` (appended to any inherited `GDK_DEBUG`), or GDK waits 25 s per portal query on a session bus whose xdg-desktop-portal never answers (CI runners) and `POST /session` times out |
| WPE (optional) | `WPEWebDriver --port=<free>`; MiniBrowser `--automation --headless --size=<viewport>` and `--config-file` with a `[websettings]` block carrying the same four font families (WPE has no `--*-font-family` flags). No compositor needed. `OMNISNAP_WEBKIT=wpe` selects it over WebKitGTK; discovery lists both and the id is `wpe` |
| Safari | `safaridriver --port=<free>` (macOS); visible window by design (no headless mode) |

For the branded checks only, `process.StartBlinkVisible` drops `--headless=new` and, on Linux, runs the browser on the same private Weston display (`--ozone-platform=wayland`): Edge's tracking prevention is off in headless mode (`SPEC-branded.md`). The product always launches Blink headless.

Font parity (generic families resolve to the same fonts as Blink: Liberation Serif/Sans, Noto Sans Mono by default) is applied here because it is a launch option; `capture` verifies it.

### Profiles

- A fresh temporary profile per launch, under `<os-temp>/omnisnap-<pid>-<n>/`, never the user's profile (Chrome 136+ also refuses remote debugging on the default data dir).
- Snap Firefox: profile under `~/snap/firefox/common/omnisnap-<pid>-<n>/` (its `/tmp` is private).
- Branded browsers: a **copy** of the warmed template profile (`SPEC-branded.md`) per launch.
- A `owner.lock` file with the owning pid in each temp dir.

### Cleanup (three layers)

1. **Normal exit:** close sessions, kill the process tree, delete the temp dir.
2. **Abnormal exit:** Windows Job Object kills every child when `omnisnap` dies; on Unix each browser runs in its own process group and a signal handler (SIGINT, SIGTERM, SIGHUP) kills the groups and deletes the dirs.
3. **Next start (sweeper):** delete `omnisnap-*` temp dirs whose `owner.lock` pid is not alive; kill leftover processes recorded there. This also covers SIGKILL.

### Resources

- Headless display: one private Weston per `omnisnap` process (`--socket=omnisnap-<pid>-<ts>`), started lazily, stopped with the process.
- Concurrency: at most one browser per engine per `omnisnap` process (lanes in `capture` run engines in parallel, not instances); a configurable cap on concurrently live browsers (default 3).
- No disk caches (above): a crawl would otherwise write hundreds of MB per session, and on WSL the virtual disk on C: never gives it back.

## Commands

```
go test ./internal/process/...                     # unit: config building, sweeper logic on fake dirs
go test -tags browsers ./internal/process/...      # launch, kill -9 the parent, assert no orphans
```

## Project structure

`internal/process/{launch.go, profile.go, sweep.go, display_linux.go, job_windows.go, pgroup_unix.go}`; `testdata/process/`.

## Code style

Per conventions. Every `Start` returns a handle whose `Close(ctx)` is idempotent and always called via `defer`; the sweeper is a pure function over a directory listing plus a "pid alive" func, so it's unit-testable.

## Testing strategy

- Unit: launch arguments and prefs per family (golden lists); sweeper decisions on fake temp trees (alive / dead / foreign dirs).
- Integration (`browsers` tag, all three CI OSes): launch each engine 20 times; kill the parent with SIGKILL / TerminateProcess mid-capture; next start leaves **zero** browser processes and **zero** `omnisnap-*` dirs.
- WebKitGTK: no window appears (Weston headless); `WEBKIT_HEADLESS=0` debug switch shows windows. Without weston the lane runs visible on the desktop and the viewport cannot be set.
- WPE: no window ever appears; the viewport is fixed by `--size` at launch.
- Every browser gets `TMPDIR` (and `TMP`/`TEMP`) inside its own temp dir, so Chrome's singleton socket and Firefox's extension staging die with it; a path long enough to break Chrome's 108-byte singleton limit keeps the child's default instead.

## Boundaries

- **Always:** one temp root per run; delete it; record what was launched in `host.json`.
- **Ask first:** changing any launch flag or pref (they change what is measured); raising the live-browser cap.
- **Never:** touch the user's real profile or default data dirs; leave a process running after `omnisnap` exits; write outside the temp root and the job output.

## Success criteria

1. After 100 runs that end normally, 0 leftover processes and 0 leftover temp dirs on all three OSes.
2. After 20 runs killed with SIGKILL / TerminateProcess, the next start removes every leftover within 2 s; none survive.
3. A full planted-suite capture (31 pages × 3 engines × 2 runs) writes < 200 MB outside the job output directory at peak, and 0 bytes after exit.
4. Peak resident memory of all browsers together ≤ 3 GB for the planted suite with the default cap.
5. WebKitGTK and Safari integration tests pass headless (WebKitGTK) / unattended (Safari on macOS runners).

## Open questions

- Safari opens a visible window: acceptable on CI runners; on a developer Mac, warn before starting it?
- Should the sweeper also clean the spike-era leftovers (`/tmp/puppeteer_dev_*`)? Proposed: no; only `omnisnap-*`.
