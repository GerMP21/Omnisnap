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
2. **Disk space.** The archive's size from a `HEAD` request; refuse to start with less than twice that free (the download plus the unpacked tree).
3. **Download** from `ftp.mozilla.org/pub/firefox/releases/` into the cache's `downloads/` folder, **resuming** a partial file with a `Range` request ([`download`](https://github.com/GerMP21/Omnisnap/blob/main/internal/install/download.go)). Progress and size are printed.
4. **Verify.** Fetch the release's `SHA512SUMS` and `SHA512SUMS.asc`; verify the detached OpenPGP signature against Mozilla's release key, which is pinned in the binary ([`mozilla-key.asc`](https://github.com/GerMP21/Omnisnap/blob/main/internal/install/mozilla-key.asc)), in pure Go: RFC 4880 v4 RSA signatures, PKCS#1 v1.5, no gpg ([`VerifyDetached`](https://github.com/GerMP21/Omnisnap/blob/main/internal/install/verify.go)). Then check the archive's SHA-512 against its line in the signed sums (listed under its release path, since macOS and Windows archives are renamed on download).
5. **Unpack** into a temp dir inside the cache, then move it into place:
   - Linux: `.tar.xz`, decompressed with `xz(1)` (Go's standard library has no xz decoder, and the project takes no dependency for one);
   - macOS: the `.dmg` mounted read-only, `Firefox.app` copied out, detached;
   - Windows: the MSI extracted with an administrative install (`msiexec /a`), which copies files without installing anything or touching the registry.
6. **Mark** the install with `omnisnap-source.json` (what was downloaded, and when).

The browsers cache ([`CacheDir`](https://github.com/GerMP21/Omnisnap/blob/main/internal/install/cache.go)) is `%LOCALAPPDATA%\omnisnap\browsers` on Windows, `~/Library/Caches/omnisnap/browsers` on macOS, and `$XDG_CACHE_HOME/omnisnap/browsers` (or `~/.cache/omnisnap/browsers`) elsewhere; downloads go in its `downloads/` folder. Warmed templates live next to it, under `omnisnap/templates/`. [Discovery](Discovery) finds them as source `omnisnap-cache`, with ESR under its own id.

Key rotation by Mozilla means a new Omnisnap release with the new key. `TestMozillaKeyIsTheReleaseKey` pins the fingerprint.

## Failure handling

A tampered archive, tampered sums, a bad signature or a full disk stop the install before anything reaches the cache; the partial download stays in `downloads/` so the next attempt resumes it.

## Tests

- [`install_test.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/install/install_test.go), against a local server with a fake Firefox from [`testdata/install`](https://github.com/GerMP21/Omnisnap/tree/main/testdata/install) and sums signed by a test key: `TestInstallVerifiesAndUnpacks`, `TestInstallRejectsTamperedArchive`, `TestInstallRejectsTamperedSums`, `TestDownloadResumes`, `TestInstallRefusesLowDisk`.
- [`verify_test.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/install/verify_test.go): a real Mozilla signature verifies; tampered data, a corrupt signature and a garbage key are rejected.
- [`network_test.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/install/network_test.go) (`network` tag): `TestInstallESRFromMozilla` installs a real ESR from Mozilla. It installs Mozilla's real ESR, discovers it, launches it and uninstalls it, leaving nothing. The weekly `install.yml` workflow runs it on Linux and Windows ([Testing and CI](Testing-and-CI)).

## Where the code differs from the spec

- **`uninstall --channel` is ignored.** The flag is accepted and validated, but [`Uninstall`](https://github.com/GerMP21/Omnisnap/blob/main/internal/install/cache.go) removes every cached Firefox build, whatever the channel.
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
- [`internal/install/unpack_windows.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/install/unpack_windows.go): unpacking on Windows
- [`internal/install/disk_unix.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/install/disk_unix.go): free disk space, on Unix
- [`internal/install/disk_windows.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/install/disk_windows.go): free disk space, on Windows
