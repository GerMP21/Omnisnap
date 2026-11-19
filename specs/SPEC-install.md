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
- **Unpack:** Linux `.tar.xz`, macOS `.dmg` (mounted read-only, app copied), Windows (the installer's archive, extracted without running an installer or touching the registry).
- **Never automatic:** no other command downloads; when Firefox is missing, `cli` prints a hint naming this command.
- Progress and size shown; resumable download; disk space checked before starting (needs ≥ 2× the archive size free).

## Commands

```
go test ./internal/install/...                         # unit: URL building, checksum + signature verification with fixtures
go test -tags network ./internal/install/...           # downloads the real release (CI weekly, not per PR)
go run ./cmd/omnisnap install firefox --channel esr
```

## Project structure

`internal/install/{install.go, mozilla.go, verify.go, unpack_{linux,darwin,windows}.go, cache.go}`; `testdata/install/` (fake archives, a test-signed SHA512SUMS).

## Code style

Per conventions. Verification happens before anything is unpacked into the cache; a failed check deletes the download.

## Testing strategy

- **Unit:** release URL per OS/arch/channel; checksum mismatch rejected; bad signature rejected; truncated download resumed; cache layout.
- **Network (`network` tag, weekly CI on Linux and Windows):** install ESR, discover it, launch it through `process`, capture a planted page, uninstall. macOS is not in that matrix (hosted macOS runners cost about ten times an Ubuntu job); its unpack path is **not tested** yet: the unit fixture is a Linux tar.xz, so the install tests skip on macOS (as on Windows, which lacks an MSI fixture) until a DMG fixture exists (gap recorded 2026-11-19).

## Boundaries

- **Always:** verify checksum and signature; tell the user what is downloaded, from where, and its size.
- **Ask first:** supporting another browser (Chromium builds, WebKit builds); changing the source.
- **Never:** download without the explicit command; run vendor installers; touch the system's Firefox or registry.

## Success criteria

1. On the Windows and Linux CI runners, `omnisnap install firefox --channel esr` installs a verified Firefox that `omnisnap browsers` lists and that captures the planted suite. The macOS path (dmg unpack) is meant to be covered by unit tests; it is not yet: they need a DMG fixture and skip on macOS until one exists (open gap).
2. A tampered archive or `SHA512SUMS` is rejected and nothing lands in the cache.
3. `omnisnap uninstall firefox` leaves no files behind.
4. No other command makes a network request to Mozilla (asserted with a network-blocking test harness).

## Open questions

- Release or ESR by default? ESR moves slower (fewer version differences across a team); proposed default: release, matching what users run.
- Should the Action use `install` on runners whose image lacks Firefox, or rely on the image?
