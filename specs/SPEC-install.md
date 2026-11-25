# Spec: install

Module id: `install` · Package: `internal/install` · Depends on: `discovery` · Milestone: M0 · Conventions: [CONVENTIONS.md](CONVENTIONS.md)

## Objective

Let a developer who has no Firefox (common on Windows and macOS machines with Chrome only) get a second engine family with one explicit command, `omnisnap install firefox`, without Omnisnap ever downloading anything on its own (PRD 1.1.0: zero downloads by default; this is the only opt-in exception).

## Behaviour

- `omnisnap install firefox [--channel release|esr] [--version <v>]`: download the official Mozilla build for this OS and architecture from Mozilla's servers, verify it, unpack it into the Omnisnap cache, and make it discoverable (`discovery` source `omnisnap-cache`).
- `omnisnap uninstall firefox [--channel ...]`: remove it from the cache.
- `omnisnap browsers` shows installed-by-Omnisnap browsers with their source.
- **Cache location:** `%LOCALAPPDATA%\omnisnap\browsers\` (Windows), `~/Library/Caches/omnisnap/browsers/` (macOS), `$XDG_CACHE_HOME/omnisnap/browsers/` (Linux).
- **Verification:** the download's SHA-512 must match Mozilla's published `SHA512SUMS` for that release; the `SHA512SUMS` file's detached signature is verified against Mozilla's release signing key, pinned in the binary (key rotation = a release of Omnisnap). The signature is verified in pure Go — no gpg, no external tool, so it works the same on a machine that has never installed one.
- **Unpack:** Linux `.tar.xz`, macOS `.dmg` (mounted read-only, app copied). Windows downloads `Firefox Setup <v>.exe` (Mozilla's MSI only wraps that same `.exe`), a 7-Zip self-extracting archive. Omnisnap finds the embedded 7z archive itself (signature plus start-header CRC, standard library only), copies it out, and extracts it with the system's `%SystemRoot%\System32\tar.exe`, then with 7-Zip (`7z.exe` on `PATH` or in `%ProgramFiles%\7-Zip`). An attempt counts only if it leaves `core/firefox.exe`. A `tar` found on `PATH` (Git Bash's GNU tar) is never used. Nothing from the download is executed, nothing is written outside the cache, and no administrator rights are needed. When no extractor works, the error names each attempt and says to install 7-Zip from https://www.7-zip.org and retry.
- **Never automatic:** no other command downloads; when Firefox is missing, `cli` prints a hint naming this command.
- Progress and size shown; resumable download; disk space checked before starting, against an estimate from the archive's size that covers the download, any intermediate copy and the unpacked browser: 6× on Windows (download, carved archive, unpacked tree of about 4×), 5× on Linux and macOS. The refusal states the free space and the space needed.

## Commands

```
go test ./internal/install/...                         # unit: URL building, checksum + signature verification with fixtures
go test -tags network ./internal/install/...           # downloads the real release (CI weekly, by hand, and on PRs touching internal/install/)
go run ./cmd/omnisnap install firefox --channel esr
```

## Project structure

`internal/install/{install.go, mozilla.go, verify.go, sfx.go, unpack_{linux,darwin,windows}.go, cache.go}`; `testdata/install/` (fake archives, a test-signed SHA512SUMS).

## Code style

Per conventions. Verification happens before anything is unpacked into the cache; a failed check deletes the download.

## Testing strategy

- **Unit:** release URL per OS/arch/channel; checksum mismatch rejected; bad signature rejected; truncated download resumed; cache layout; on every OS, finding and copying out an embedded 7z archive (a decoy signature skipped, the exact length copied, corrupt and truncated input rejected) and the extractor chain with fake tools.
- **Network (`network` tag, CI on Linux and Windows: weekly, by hand, and on pull requests that touch `internal/install/`):** install ESR, discover it, launch it through `process`, capture a planted page, uninstall. macOS is not in that matrix (hosted macOS runners cost about ten times an Ubuntu job). The unit tests cover every OS's unpack path with a fixture in that OS's format: the committed tar.xz on Linux; on macOS a DMG the test builds with `hdiutil create` (Firefox.app at the volume root), on Windows a stub with a decoy signature, an appended LZMA + BCJ2 7z archive built with 7-Zip (`core/firefox.exe` and `setup.exe`, the shape of Mozilla's setup) and trailing bytes standing in for the Authenticode signature, plus a Windows-only test that runs `tar.exe` and 7-Zip each alone and logs which one works, each listed in a `SHA512SUMS` signed at test time with a throwaway key (`internal/install/{fixture,pgpsign}_test.go`). A missing tool fails the test on CI and skips it elsewhere. The low-disk refusal runs on all three with the committed archive (Task 37, 2026-11-19).

## Boundaries

- **Always:** verify checksum and signature; tell the user what is downloaded, from where, and its size.
- **Ask first:** supporting another browser (Chromium builds, WebKit builds); changing the source.
- **Never:** download without the explicit command; run vendor installers; touch the system's Firefox or registry.

## Success criteria

1. On the Windows and Linux CI runners, `omnisnap install firefox --channel esr` installs a verified Firefox that `omnisnap browsers` lists and that captures the planted suite. The macOS path (dmg unpack) is covered by unit tests with a DMG built on the runner (Testing strategy).
2. A tampered archive or `SHA512SUMS` is rejected and nothing lands in the cache.
3. `omnisnap uninstall firefox` leaves no files behind.
4. No other command makes a network request to Mozilla (asserted with a network-blocking test harness).

## Decisions

- Windows unpacks the setup `.exe`'s embedded 7z archive, not the MSI (2026-11-25). The first network run that reached the unpack step (run 45198060633, 2026-11-23) failed with `the unpacked archive has no firefox.exe`: Mozilla's MSI has no File table, only `Binary.WrappedExe` (the setup `.exe`) run as a custom action, so `msiexec /a` copied nothing, and the WiX fixture, a real file-table MSI, could not show it. The archive is extracted with the system `tar.exe` and then 7-Zip, so no dependency is added (a pure-Go 7z reader was the alternative); running the setup with `/ExtractDir` would have meant running the vendor's installer. A failed extraction deletes the download, like any other failure. Change `fix-windows-firefox-install`.

## Open questions

- Release or ESR by default? ESR moves slower (fewer version differences across a team); proposed default: release, matching what users run.
- Should the Action use `install` on runners whose image lacks Firefox, or rely on the image?
