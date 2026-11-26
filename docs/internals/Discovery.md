# Discovery

**Role.** Find every browser installed on the machine, with its engine family, protocol and version, **without launching any**. It is the first thing a job does, and `omnisnap browsers` prints its result. Module `discovery`, package [`internal/discovery`](https://github.com/GerMP21/Omnisnap/tree/main/internal/discovery).

## Inputs and outputs

- **In:** the machine: its file system, environment, and on Windows the registry. Everything is reached through [`Options`](https://github.com/GerMP21/Omnisnap/blob/main/internal/discovery/discovery.go): a file system, `Getenv`, a registry query, a file-version reader, a Spotlight query and a Safari-enabled check. Tests replace each one with fixtures.
- **Out:** a sorted list of [`Browser`](https://github.com/GerMP21/Omnisnap/blob/main/internal/discovery/discovery.go): `id` (`chrome`, `firefox-esr`, `webkitgtk`, `wpe`, `safari`…), name, family, protocol (`cdp`, `bidi`, `classic`), executable, WebDriver binary for Classic, version (`""` when no file says), source, and notes the caller must respect.

## How it works

[`Find`](https://github.com/GerMP21/Omnisnap/blob/main/internal/discovery/discovery.go) runs the platform's **sources** in order, best first. Each source returns what it can and never fails the whole discovery. When two sources find the same id, the first wins.

| OS | Sources, in order |
|---|---|
| Linux | [`.desktop` files](https://github.com/GerMP21/Omnisnap/blob/main/internal/discovery/linux.go), [`PATH`](https://github.com/GerMP21/Omnisnap/blob/main/internal/discovery/linux.go), [Snap](https://github.com/GerMP21/Omnisnap/blob/main/internal/discovery/linux.go), [Flatpak](https://github.com/GerMP21/Omnisnap/blob/main/internal/discovery/linux.go), [WebKitGTK](https://github.com/GerMP21/Omnisnap/blob/main/internal/discovery/linux.go) (MiniBrowser and WebKitWebDriver at the known paths for 6.0 and 4.1), [WPE](https://github.com/GerMP21/Omnisnap/blob/main/internal/discovery/linux.go), the install cache |
| Windows | [the registry](https://github.com/GerMP21/Omnisnap/blob/main/internal/discovery/windows.go) (`App Paths` for the executable, `Uninstall` for the version and Opera GX's name; HKLM, WOW6432Node and HKCU), [`PATH`](https://github.com/GerMP21/Omnisnap/blob/main/internal/discovery/windows.go), the install cache |
| macOS | [app bundles](https://github.com/GerMP21/Omnisnap/blob/main/internal/discovery/darwin.go) in `~/Applications` and `/Applications` (`Info.plist`), [Spotlight](https://github.com/GerMP21/Omnisnap/blob/main/internal/discovery/darwin.go) (`mdfind` by bundle id), [Safari](https://github.com/GerMP21/Omnisnap/blob/main/internal/discovery/darwin.go), the install cache |

The **catalog** ([`catalog.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/discovery/catalog.go)) lists what discovery recognizes and every name each browser goes by: Chrome, Chrome Beta, Chromium, Brave, Edge, Opera, Vivaldi (Blink, CDP); Firefox and Firefox ESR (Gecko, BiDi); WebKitGTK and WPE (WebKit, Classic); Safari on macOS.

The **install cache** source ([`cache.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/discovery/cache.go)) finds the Firefox builds `omnisnap install firefox` put in the user cache (source `omnisnap-cache`). It duplicates install's two path rules instead of importing `install`, which itself depends on discovery. On Windows it accepts the `firefox.exe` it finds without an execute bit, as the registry and PATH sources do: Go reports every Windows file as `0666` or `0444`.

### Versions without launching

`--version` of a snap or flatpak takes seconds, and `chrome.exe --version` prints nothing on Windows, so versions come from files, cheapest first ([`version.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/discovery/version.go)):

1. a snap's `meta/snap.yaml`;
2. for Gecko, `application.ini` next to the executable;
3. the dpkg or pacman database, for the catalog's package names.

On Windows, the executable's VERSIONINFO resource; on macOS, `CFBundleShortVersionString` in `Info.plist`. When nothing says, the version is `""` and a note explains why (rpm is not read).

### Notes the caller must respect

- **Snap:** private `/tmp`, so profiles must be created where the snap can read them. [Process management](Process-Management) puts Snap Firefox's profile under `~/snap/firefox/common/`.
- **Flatpak:** sandboxed file system, same consequence.
- **WebKitGTK:** needs a display; process management provides a private headless one.
- **Safari:** `safaridriver` needs a one-time `safaridriver --enable` (admin). Discovery reports the driver's path and never runs it: the only flag that would ask, `--diagnose`, starts the driver and never exits, which hung discovery on 2026-11-19. A driver that is not enabled is reported at launch time.

### Choosing a WebKit

When both WebKitGTK and WPE are installed, `OMNISNAP_WEBKIT=wpe` keeps only WPE and `OMNISNAP_WEBKIT=gtk` only WebKitGTK; unset lists both ([`discovery.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/discovery/discovery.go)).

## Failure handling

A source that cannot read something returns fewer browsers, not an error. An executable that is not executable is ignored. `Find` fails only when its context is cancelled.

## Tests

- Per-OS tests on fixtures: [`linux_test.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/discovery/linux_test.go) (desktop files and dpkg, `PATH` and pacman, Snap, Flatpak, WebKitGTK, de-duplication, unknown versions), [`windows_test.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/discovery/windows_test.go) on recorded `reg query` dumps in [`testdata/discovery/windows`](https://github.com/GerMP21/Omnisnap/tree/main/testdata/discovery/windows) (the registry pitfalls: Chromium's hashed key, Opera GX, IEXPLORE), [`darwin_test.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/discovery/darwin_test.go) on `Info.plist` fixtures.
- `TestDiscoveryNeverLaunchesBrowsers` and `TestDarwinDefaultsNeverRunSafaridriver`: nothing is started.
- [`TestDiscoveryTiming`](https://github.com/GerMP21/Omnisnap/blob/main/internal/discovery/timing_browsers_test.go): median of 10 runs under 300 ms on Linux and Windows, under 1 s on macOS, on the CI runners.
- `TestExpectedBrowsers` checks, on each CI runner, that the browsers the runner image is known to have are found.

## Where the code differs from the spec

- **Windows sources.** The spec lists `StartMenuInternet\*\shell\open\command` keys and well-known install folders. The code reads `App Paths` and `Uninstall` (in HKLM, WOW6432Node and HKCU) and `PATH`, and no install folders.

## Spec

[`specs/SPEC-discovery.md`](https://github.com/GerMP21/Omnisnap/blob/main/specs/SPEC-discovery.md).

## Source files

- [`internal/discovery/discovery.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/discovery/discovery.go): `Find`, the types and the sources it runs
- [`internal/discovery/catalog.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/discovery/catalog.go): the known browsers and every name they go by
- [`internal/discovery/version.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/discovery/version.go): version reading per install kind
- [`internal/discovery/cache.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/discovery/cache.go): browsers installed by `omnisnap install`
- [`internal/discovery/linux.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/discovery/linux.go): Linux sources: PATH and desktop files
- [`internal/discovery/darwin.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/discovery/darwin.go): macOS sources: app bundles and Spotlight
- [`internal/discovery/windows.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/discovery/windows.go): Windows sources: the registry and PATH
- [`internal/discovery/winversion_windows.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/discovery/winversion_windows.go): `reg query` and file versions on Windows
- [`internal/discovery/plist.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/discovery/plist.go): the Info.plist reader, binary and XML
- [`internal/discovery/plist_xml.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/discovery/plist_xml.go): the XML plist decoder
- [`internal/discovery/platform_linux.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/discovery/platform_linux.go): the Linux source list
- [`internal/discovery/platform_darwin.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/discovery/platform_darwin.go): the macOS source list
- [`internal/discovery/platform_windows.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/discovery/platform_windows.go): the Windows source list
- [`internal/discovery/platform_other.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/discovery/platform_other.go): other systems: no sources
- [`internal/discovery/timing_browsers_test_os.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/discovery/timing_browsers_test_os.go): a helper for the `browsers`-tagged timing test
