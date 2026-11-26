# Proposal

## Why

`omnisnap install firefox` does not work on Windows, one of the three systems it documents. The weekly "Install (network)" job's first scheduled run (run 45198060633, 2026-11-23) failed on `windows-latest` with `install: the unpacked archive has no firefox.exe`, after the download and the signature check had both passed. Mozilla's Windows MSI holds no Firefox files: its only payload is `Binary.WrappedExe`, the regular `Firefox Setup <v>.exe`, which the MSI runs as a custom action. An administrative install (`msiexec /a`) copies the MSI's File table, which is empty, so it leaves nothing usable (checked 2026-11-25 on `Firefox Setup 153.8.0esr.msi`). This is backlog B4, and it blocks the stable `v0.2.0` (change `release-v0-2-0`).

## What Changes

- **Windows downloads the `.exe`, not the `.msi`.** The archive becomes `win64/en-US/Firefox Setup <v>.exe` (and `win32/`, `win64-aarch64/`), the file `SHA512SUMS` lists under that path. The verification is unchanged: the signed sums and the SHA-512 cover exactly the file that is unpacked.
- **Unpack without running anything.** The `.exe` is a 7-Zip self-extracting archive: a small stub with a 7z archive appended (LZMA + BCJ2, the browser under `core/`). Omnisnap finds the embedded archive itself, with the standard library only, and extracts it with Windows' own `tar.exe` (bsdtar). If that fails, it falls back to 7-Zip (`7z.exe`) when one is installed. Neither the stub nor `setup.exe` is ever executed, and nothing is written outside the cache: no registry key, Start menu entry or uninstaller entry, and no administrator rights are needed.
- **A clear error when no extractor works.** When neither tool can extract the archive, the install fails before anything reaches the cache. The error names both attempts and tells the user to install 7-Zip. An extractor that exits 0 but leaves no `core/firefox.exe` counts as a failure: that is the class of bug this change fixes.
- **A disk check that fits the unpacked size.** The check asks for enough free space for the download, the carved archive and the unpacked tree (about 4x the download on Windows). Today's 2x passes a disk that cannot hold a Firefox that unpacks to 360 MB from a 90 MB download.
- **A Windows fixture in Mozilla's format.** The unit test builds a stub with an appended LZMA + BCJ2 7z archive, the same shape as Mozilla's download, instead of a WiX MSI. With this fixture the unit test would have caught the bug. An OS-neutral test covers finding the embedded archive.
- **The network job also runs on pull requests that touch `internal/install/`.** It is the only test of Mozilla's real download, and the bug went unseen for a week because it ran only on Mondays.

## Capabilities

### New Capabilities

- `firefox-install`: the contract of `omnisnap install firefox` that this change touches. How Mozilla's build is verified and unpacked on each OS without running an installer, the Windows extractor order and its failure, the disk-space check, and that no administrator rights or system changes are needed.

### Modified Capabilities

None. There are no OpenSpec specs yet. The normative `specs/SPEC-install.md` is updated in this change to match: the Windows unpack wording, the disk rule and the testing strategy.

## Impact

- **Code:**
  - `internal/install/mozilla.go`: the Windows archive path and local name.
  - `internal/install/unpack_windows.go`: the extractor chain replaces `msiexec /a`.
  - A new OS-neutral file for finding and copying out the embedded 7z archive.
  - `internal/install/install.go`: the disk-space factor per OS, and comments that name the MSI.
- **Tests:** `internal/install/fixture_windows_test.go` (stub + 7z instead of WiX), a new carve test on every OS, and the Windows-only extractor tests.
- **Dependencies:** none added. Windows' `tar.exe` ships with the OS, and 7-Zip is an optional fallback the user installs.
- **Workflows:** `.github/workflows/install.yml` gains a `pull_request` trigger with a paths filter.
- **Docs:**
  - `specs/SPEC-install.md`.
  - `docs/internals/Install.md`, `Testing-and-CI.md`, and `Architecture.md` and `Repository-Layout.md` where they name the files.
  - `README.md`'s milestone row, `tasks/todo.md` Task 37, and `tasks/backlog.md` B4's status.
- **Users:** Windows 10 builds whose `tar.exe` predates LZMA support need 7-Zip installed. The error tells them so.
