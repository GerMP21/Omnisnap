# Install

**Role.** `omnisnap install firefox`: download the official Mozilla Firefox build for this OS and architecture, verify it, unpack it into the Omnisnap cache, and make it discoverable. A machine with only Blink browsers cannot compare engine families; this gives it Gecko. It is opt-in: no other command downloads anything. Module `install`, package [`internal/install`](https://github.com/GerMP21/Omnisnap/tree/main/internal/install).

## Commands

```
omnisnap install firefox [--channel release|esr] [--version <v>]
omnisnap uninstall firefox [--channel release|esr]
```

When a job has only one engine family, the CLI prints a hint naming `omnisnap install firefox` ([`run.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/run.go)).

## How it works

[`Firefox`](https://github.com/GerMP21/Omnisnap/blob/main/internal/install/install.go), in order. Nothing is written into the cache before both checks pass.

1. **Version.** Without `--version`, ask Mozilla's version feed (`product-details.mozilla.org`) for the channel's latest ([`ResolveVersion`](https://github.com/GerMP21/Omnisnap/blob/main/internal/install/mozilla.go)).
2. **Disk space.** The archive's size from a `HEAD` request; refuse to start with less than [`diskFactor`](https://github.com/GerMP21/Omnisnap/blob/main/internal/install/install.go) times that free: 6x on Windows (the download, the carved 7z archive and the unpacked `core/`, about 4x), 5x on Linux and macOS (the download and the unpacked tree). The refusal states the free space and the space needed. Measured on 153.8.0esr for Linux: 82 MB download, 310 MB unpacked (3.78x); `TestInstallESRFromMozilla` logs the ratio on each run.
3. **Download** from `ftp.mozilla.org/pub/firefox/releases/` into the cache's `downloads/` folder, **resuming** a partial file with a `Range` request ([`download`](https://github.com/GerMP21/Omnisnap/blob/main/internal/install/download.go)). Progress and size are printed.
4. **Verify.** Fetch the release's `SHA512SUMS` and `SHA512SUMS.asc`; verify the detached OpenPGP signature against Mozilla's release key, which is pinned in the binary ([`mozilla-key.asc`](https://github.com/GerMP21/Omnisnap/blob/main/internal/install/mozilla-key.asc)), in pure Go: RFC 4880 v4 RSA signatures, PKCS#1 v1.5, no gpg ([`VerifyDetached`](https://github.com/GerMP21/Omnisnap/blob/main/internal/install/verify.go)). Then check the archive's SHA-512 against its line in the signed sums (listed under its release path, since macOS and Windows archives are renamed on download).
5. **Unpack** into a temp dir inside the cache, then move it into place:
   - Linux: `.tar.xz`, decompressed with `xz(1)` (Go's standard library has no xz decoder, and the project takes no dependency for one);
   - macOS: the `.dmg` mounted read-only, `Firefox.app` copied out, detached;
   - Windows: Omnisnap downloads `Firefox Setup <v>.exe`. That is a 7-Zip self-extracting archive: a PE stub, then a 7z archive (LZMA + BCJ2, the browser under `core/`, the NSIS `setup.exe` beside it), then the Authenticode signature. Mozilla's Windows Installer package only wraps this same `.exe` and runs it, so an administrative install of it copies nothing (backlog B4). The install never runs anything from the download, writes nothing outside the cache, and needs no administrator rights:
     - [`find7z`](https://github.com/GerMP21/Omnisnap/blob/main/internal/install/sfx.go) scans the verified `.exe` for the 7z signature. A hit counts only with a valid start header (major version 0, a matching start-header CRC, an archive that ends inside the file), so a stray copy of the signature bytes is skipped.
     - [`carve7z`](https://github.com/GerMP21/Omnisnap/blob/main/internal/install/sfx.go) copies exactly the archive to `<tmp>/firefox.7z`.
     - [`extract7z`](https://github.com/GerMP21/Omnisnap/blob/main/internal/install/sfx.go) tries `%SystemRoot%\System32\tar.exe` (bsdtar) by its full path, never a `tar` on `PATH`, since Git Bash's GNU tar cannot read 7z. It then tries 7-Zip (`7z` on `PATH`, or `%ProgramFiles%\7-Zip\7z.exe`). An attempt counts only if it leaves `core/firefox.exe`, and the temp dir is emptied between attempts. When neither works, the error names each attempt and says to install 7-Zip from https://www.7-zip.org.
     - `finalize` moves `core/` to `<version>/firefox/`. `setup.exe` is deleted with the temp dir and never run.
6. **Mark** the install with `omnisnap-source.json` (what was downloaded, and when).

The browsers cache ([`CacheDir`](https://github.com/GerMP21/Omnisnap/blob/main/internal/install/cache.go)) is `%LOCALAPPDATA%\omnisnap\browsers` on Windows, `~/Library/Caches/omnisnap/browsers` on macOS, and `$XDG_CACHE_HOME/omnisnap/browsers` (or `~/.cache/omnisnap/browsers`) elsewhere; downloads go in its `downloads/` folder. Warmed templates live next to it, under `omnisnap/templates/`. [Discovery](Discovery) finds them as source `omnisnap-cache`, with ESR under its own id.

Key rotation by Mozilla means a new Omnisnap release with the new key. `TestMozillaKeyIsTheReleaseKey` pins the fingerprint.

## Failure handling

A tampered archive, tampered sums, a bad signature, a full disk or a failed unpack stop the install before anything reaches the cache. An interrupted download stays in `downloads/` so the next attempt resumes it; once the download is complete, a failed check or a failed unpack deletes it (on Windows, "install 7-Zip and retry" downloads the setup again).

## Tests

- [`install_test.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/install/install_test.go), against a local server with a fake Firefox from [`testdata/install`](https://github.com/GerMP21/Omnisnap/tree/main/testdata/install) and sums signed by a test key: `TestInstallVerifiesAndUnpacks`, `TestInstallRejectsTamperedArchive`, `TestInstallRejectsTamperedSums`, `TestDownloadResumes`, `TestInstallRefusesLowDisk` (just below, well below and exactly at this OS's disk factor). On Windows the fixture is a setup `.exe` in Mozilla's shape (stub with a decoy signature, LZMA + BCJ2 7z archive, trailing signature bytes), built with 7-Zip at test time ([`buildSetupExe`](https://github.com/GerMP21/Omnisnap/blob/main/internal/install/sfx_fixture_test.go)); on macOS a DMG built with `hdiutil`; on Linux the committed `.tar.xz`.
- [`sfx_test.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/install/sfx_test.go), on every OS with no tools: `TestFind7zSkipsADecoyAndCarvesExactly`, `TestFind7zAcrossTheChunkBoundary`, `TestCarve7zRejects` (corrupt CRC, truncated, no signature, only a decoy), and `TestExtract7zChain` with fake extractors (this test binary re-run): the first works; the first exits 0 without the browser and the second works; both fail; 7-Zip missing; no extractors.
- [`sfx_fixture_test.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/install/sfx_fixture_test.go): `TestCarveAndExtractARealArchive` builds a real archive with 7-Zip and extracts it with bsdtar and 7-Zip; it skips without 7-Zip.
- [`fixture_windows_test.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/install/fixture_windows_test.go): `TestWindowsExtractorsEachAlone` runs `tar.exe` and 7-Zip each alone on the fixture and logs which one works on the runner; at least one must.
- [`verify_test.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/install/verify_test.go): a real Mozilla signature verifies; tampered data, a corrupt signature and a garbage key are rejected.
- [`network_test.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/install/network_test.go) (`network` tag): `TestInstallESRFromMozilla` installs a real ESR from Mozilla. It installs Mozilla's real ESR, discovers it, launches it and uninstalls it, leaving nothing. The `install.yml` workflow runs it on Linux and Windows: weekly, by hand, and on pull requests that touch `internal/install/` ([Testing and CI](Testing-and-CI)). It also logs the unpacked size against the download. Discovery keeps the first browser it finds per id, so on a machine with a system Firefox ESR the cache copy is not the one listed and the test fails at discovery. The CI runners have no system ESR.

## Where the code differs from the spec

- **`uninstall --channel` is ignored.** The flag is accepted and validated, but [`Uninstall`](https://github.com/GerMP21/Omnisnap/blob/main/internal/install/cache.go) removes every cached Firefox build, whatever the channel.
- **Windows needs a 7z-capable `tar.exe` or 7-Zip.** Unpacking runs Windows' own `tar.exe`, then 7-Zip. A `tar.exe` built without LZMA (Windows 10's libarchive 3.5.2) fails, and the user has to install 7-Zip.
- **Linux needs `xz(1)`.** The spec's "no external tool" is about signature verification, which is pure Go; unpacking a `.tar.xz` on Linux runs `xz`.

## Spec

[`specs/SPEC-install.md`](https://github.com/GerMP21/Omnisnap/blob/main/specs/SPEC-install.md).

## Source files

- [`internal/cli/install.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/cli/install.go): `omnisnap install` and `uninstall`
- [`internal/install/install.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/install/install.go): `Firefox`: the whole install, end to end
- [`internal/install/mozilla.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/install/mozilla.go): channels, targets, version resolution, archive URLs
- [`internal/install/download.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/install/download.go): the download, its progress and the checksum
- [`internal/install/verify.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/install/verify.go): the OpenPGP check of SHA512SUMS against the pinned key
- [`internal/install/embed.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/install/embed.go): Mozilla's release signing key, embedded
- [`internal/install/cache.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/install/cache.go): the cache layout and what is installed
- [`internal/install/unpack_linux.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/install/unpack_linux.go): unpacking on Linux (needs `xz`)
- [`internal/install/unpack_darwin.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/install/unpack_darwin.go): unpacking on macOS
- [`internal/install/sfx.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/install/sfx.go): finding and copying out the 7z archive in Windows' setup `.exe`, and the extractor chain
- [`internal/install/unpack_windows.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/install/unpack_windows.go): unpacking on Windows (carve, then `tar.exe`, then 7-Zip)
- [`internal/install/disk_unix.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/install/disk_unix.go): free disk space, on Unix
- [`internal/install/disk_windows.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/install/disk_windows.go): free disk space, on Windows
