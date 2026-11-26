# Spec: discovery

Module id: `discovery` · Package: `internal/discovery` · Depends on: `bundle-schema` · Milestone: M0 · Conventions: [CONVENTIONS.md](CONVENTIONS.md)

## Objective

Find every browser installed on the machine, **without launching any**, and describe each one well enough for `process` to launch it and for the report to name it: id, family, engine, channel, executable, version, protocol, packaging (native, Flatpak, Snap, app bundle) and known restrictions.

Users: every `omnisnap` run (discovery runs first) and `omnisnap browsers`. Success: zero configuration on the three desktop OSes, under 300 ms on Windows and Linux.

## Tech stack

Standard library; `golang.org/x/sys/windows/registry` on Windows; `mdfind` / Info.plist parsing on macOS (plist parsing in-house, XML and binary formats, no dependency).

## Behaviour

### Output

```go
type Browser struct {
	ID         string   // "chrome", "chrome-beta", "chromium", "edge", "brave", "opera", "vivaldi", "firefox", "firefox-esr", "safari", "webkitgtk"
	Name       string   // "Google Chrome"
	Family     Family   // blink | gecko | webkit
	Protocol   Protocol // cdp | bidi | classic
	Executable string
	Driver     string   // WebDriver binary for classic (safaridriver, WebKitWebDriver)
	Version    string   // "" when unreadable without launching
	Source     string   // registry | desktop-file | path | flatpak | snap | app-bundle | omnisnap-cache
	Notes      []string // restrictions, e.g. "snap: private /tmp", "safaridriver not enabled"
}
```

### Sources per OS (from PRD 1.0.0 Feature 1 and the 1.1.0 corrections)

| OS | Sources | Known pitfalls (must be handled) |
|---|---|---|
| Windows | `HKLM`/`HKCU` `SOFTWARE\Clients\StartMenuInternet\*\shell\open\command`; App Paths; well-known install folders | Chromium key has a hashed suffix (`Chromium.<HASH>`): match the prefix · Opera GX registers `opera.exe`, not `launcher.exe` · filter `IEXPLORE.EXE` · `chrome.exe --version` prints nothing: read the version from file metadata (VERSIONINFO) or the versioned install folder |
| Linux | `.desktop` files (XDG data dirs), `PATH`, Flatpak exports (`/var/lib/flatpak/exports/bin`, `~/.local/share/flatpak/exports/bin`), Snap (`/snap/bin`), WebKitGTK (`/usr/libexec/webkitgtk-6.0/MiniBrowser` + `WebKitWebDriver`, and the 4.1 paths) | Snap Firefox has a private `/tmp`: note it so `process` puts profiles where the snap can read them · `--version` of a Flatpak/Snap is slow: read versions from package metadata (`/snap/<name>/current/meta/snap.yaml`, Flatpak `metadata`/appdata) · WebKitGTK needs a display (`process` provides one) |
| macOS | `mdfind "kMDItemCFBundleIdentifier == '<id>'"` for known bundle ids; `/Applications`, `~/Applications` | Version from `Info.plist` `CFBundleShortVersionString` · Safari: `safaridriver` exists but needs a one-time `safaridriver --enable` (admin). Discovery reports the driver's path and never runs it: no flag only asks (`--diagnose` starts the driver and never exits, which hung discovery on 2026-11-19), so a driver not enabled is reported by the launch (`StartSafari` names `safaridriver --enable`) |

Browsers installed by `omnisnap install` (source `omnisnap-cache`) come from the install cache (`SPEC-install.md`).

On Windows every absolute path starts with a drive letter, and `os.DirFS` refuses a name that holds a `:`. Discovery's default file system there is `driveFS`, which reads `C:/...` names from that drive; without it the install-cache and `PATH` sources found nothing on a real machine until `v0.2.0` (the fake trees hid it). Windows files have no execute bit, so on Windows any regular file at the expected path counts as the browser.

### Out of scope

Arc (maintenance mode), Tor Browser (blocks remote automation by design), mobile browsers.

## Commands

```
go test ./internal/discovery/...                           # fixture-based, all OSes
go test -tags browsers ./internal/discovery/...            # real machine: lists what it finds, checks timings
go run ./cmd/omnisnap browsers --json                      # manual check
```

## Project structure

`internal/discovery/{discovery.go, windows.go, linux.go, darwin.go, plist.go, version.go, cache.go, drivefs_windows.go}` with build tags per OS; `testdata/discovery/<os>/` fake file trees and registry dumps.

## Code style

Per conventions. Each source is a small type implementing `source.scan(ctx) ([]Browser, error)`; the OS file only lists which sources apply. Sources must not shell out to a browser.

## Testing strategy

- **Unit (all OSes):** fake filesystems (`fs.FS`) and recorded registry dumps reproduce each pitfall above; a test per pitfall. Fake trees cannot show how the real file system resolves paths, so Windows also has a test on a real temporary directory (`TestFindReadsDriveLetterPaths`).
- **Timing:** a benchmark on the fixture trees; the integration test asserts < 300 ms wall time on CI runners (Windows, Ubuntu), < 1 s on macOS (`mdfind` cost).
- **Integration:** on each CI runner, the expected browsers are found with non-empty versions (GitHub runners ship Chrome, Edge and Firefox).
- **Runners in use:** the CI matrices are `ubuntu-latest` and `windows-latest` (2026-11-18: hosted macOS runners cost about ten times an Ubuntu job and the account could not get them). macOS is reached through the manual `safari-smoke.yml` workflow and through the Action's `macos: every` input, where Safari adds the third family; the fixtures run on every OS regardless.

## Boundaries

- **Always:** return partial results with notes rather than failing the whole discovery.
- **Ask first:** adding a browser with its own handler (one adapter per engine family is the rule).
- **Never:** launch a browser, run `safaridriver --enable`, or read the user's profile directories beyond detecting that they exist.

## Success criteria

1. On the author's machines (Windows + Opera GX, Chromium; AlmaLinux 10 WSL2 with Chrome, Chromium, Brave, Edge, Firefox ESR, WebKitGTK) every installed browser is found with the right family, executable and version.
2. On GitHub's ubuntu and windows runners: Chrome, Edge and Firefox found with versions. On a macOS runner (manual `safari-smoke.yml`, or the Action with `macos: every`): Safari as well.
3. Wall time < 300 ms on Windows and Linux (median of 10 runs, warm file cache), < 1 s on macOS.
4. Each pitfall in the table has a passing fixture test.
5. `omnisnap browsers --json` output validates against the host schema's engine list.

## Open questions

- Should discovery report browsers it found but can't drive (e.g. Safari without `--enable`) as entries with a note, or omit them? Proposed: include with a note; `capture` skips them with a warning.
- Opera and Vivaldi: drive as Blink via CDP like Chrome (assumed); verify on Windows.
