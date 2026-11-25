# Design

## Context

See proposal.md for the motivation and specs/firefox-install/spec.md for the required behavior. What shapes the approach:

- **What Mozilla publishes for Windows** (checked 2026-11-25, `153.8.0esr`):
  - `win64/en-US/` holds only `Firefox Setup <v>.exe` and `Firefox Setup <v>.msi`. There is no `.zip` for release or ESR.
  - `SHA512SUMS` lists both under those paths, spaces unescaped.
- **The MSI is a wrapper.** `7z l` on the MSI shows the database tables (`!CustomAction`, `!AdminExecuteSequence`, …), no `!File` table, and one 90 MB stream, `Binary.WrappedExe`. That stream is byte for byte the setup `.exe`. `msiexec /a` copies only what the File table lists, which is nothing, so `findFile` finds no `firefox.exe`.
- **The setup `.exe` is a 7-Zip SFX:**

  ```
  Firefox Setup <v>.exe  (90,339,552 bytes)
  +---------------------------+  offset 0
  | 7zS.sfx stub (PE, x86)    |  ~128 KiB; its code holds the 7z signature
  |                           |  bytes too (it searches for them at run time)
  +---------------------------+  offset 0x20200 (131,584)
  | 7z archive                |  signature 37 7A BC AF 27 1C, version 0.4
  |   method: LZMA:25 + BCJ2  |  solid, one block
  |   core/  (84 entries)     |  core/firefox.exe and the rest of the browser
  |   setup.exe               |  the NSIS installer (never run, discarded)
  +---------------------------+  32 + NextHeaderOffset + NextHeaderSize
  | 3-byte tail               |
  +---------------------------+
  ```

  Unpacked, `core/` is 359 MB: about 4x the download.
- **Extractors on Windows.**
  - Windows has shipped `%SystemRoot%\System32\tar.exe` (bsdtar, libarchive) since Windows 10 1803. Whether it can decode LZMA depends on the build: Windows 10's libarchive 3.5.2 was built with zlib only. The build on `windows-latest` (Server 2025) is not checked yet.
  - The runner images also ship 7-Zip, and Git Bash puts a GNU tar on `PATH` in workflow `bash` steps. GNU tar cannot read 7z.
  - Locally, bsdtar 3.8.11 with liblzma lists and extracts Mozilla's archive (checked 2026-11-25).
- **libarchive's own SFX detection searches from offset 0x27000.** Mozilla's archive starts at 0x20200, before that window. Recent libarchive found it anyway on this machine, but an older build may not. Handing `tar.exe` the `.exe` directly is therefore not reliable.
- **Today's code path** (`internal/install/install.go`, `Firefox`):
  - disk check (2x the `HEAD` size)
  - download, resumable
  - verify the signed `SHA512SUMS`, then the SHA-512
  - `unpackArchive` into a temp dir in the cache
  - `finalize`: on Windows, `findFile(tmp, "firefox.exe")`, then move its folder to `<version>/firefox`
  - write `omnisnap-source.json`

  On any unpack failure, `cleanup()` deletes the download.
- **CONVENTIONS:** standard library first. Any other dependency is "ask first", and the owner chose not to add one.

## Goals / Non-Goals

**Goals:**
- The install works on Windows with only what Windows ships, falling back to a 7-Zip the user installs.
- The unit test's fixture has the same shape as Mozilla's download, so a mismatch shows up on every CI run, not once a week.
- Every attempt that fails says exactly what was tried.

**Non-Goals:**
- A pure-Go 7z or LZMA decoder. That is a dependency or a large amount of code, and the owner chose the system extractors.
- Running Mozilla's installer in any mode, `/ExtractDir` included (SPEC-install.md: "Never: run vendor installers").
- Changing the Linux or macOS unpack paths, beyond the disk factor.
- Installing 7-Zip for the user.

## Decisions

### 1. Download the `.exe`, not the `.msi`

The MSI adds nothing: its only payload is the same `.exe`. Getting the `.exe` out of it would need an MSI stream reader (CFB format) on top of everything below. Downloading the `.exe` saves that step and about 120 KB of download.

`archivePath` returns `<plat>/en-US/Firefox Setup <v>.exe`, and `archiveName` returns `firefox-<v>-setup.exe`. The local name keeps "setup" so nobody mistakes it for the browser.

*Alternative:* keep the MSI and extract `Binary.WrappedExe` from it. Rejected: more code, same result, and one more format to test.

### 2. Carve the embedded archive in Go, then extract the plain `.7z`

A new OS-neutral file (`internal/install/sfx.go`) with:

- `find7z(r io.ReaderAt, size int64) (off, n int64, err error)` scans for the signature `37 7A BC AF 27 1C`. Each hit is accepted only if all of these hold:
  - the major version byte is 0
  - the start-header CRC (CRC-32 of bytes 12..31, stored LE at 8..11) matches
  - `NextHeaderSize > 0`
  - `off + 32 + NextHeaderOffset + NextHeaderSize <= size`

  The first hit that passes wins. The CRC check is what skips the stub's own copy of the signature bytes.
- `carve7z(src, dst string) error` copies exactly `32 + NextHeaderOffset + NextHeaderSize` bytes from `off` into `dst`, which drops the tail.

`unpack_windows.go` carves into `<tmp>/firefox.7z`, extracts it into `<tmp>`, then deletes `firefox.7z`. `finalize` is unchanged: `findFile` finds `core/firefox.exe` and moves `core/` to `<version>/firefox/`. `setup.exe` stays in the temp dir and is removed with it, never run.

The stdlib's `hash/crc32` and `encoding/binary` are all it needs, and the scan reads in 1 MiB chunks with a 5-byte overlap.

*Alternative:* pass the `.exe` straight to the extractor. Rejected: libarchive's SFX window starts at 0x27000 (above), and a carved file behaves the same in both tools.

### 3. Extractor chain: system `tar.exe`, then 7-Zip, success means `core/firefox.exe`

```
 carved firefox.7z
       |
       v
 %SystemRoot%\System32\tar.exe -x -f firefox.7z -C <tmp>
       |  exit != 0, or no core/firefox.exe
       v
 7z.exe x -y -bd -o<tmp> firefox.7z      (exec.LookPath("7z"), then
       |  exit != 0, or no core/firefox.exe   %ProgramFiles%\7-Zip\7z.exe)
       v
 error: "install: unpack: could not extract Firefox Setup <v>.exe:
         tar.exe: <err/output>; 7-Zip: not found (or <err/output>).
         Install 7-Zip from https://www.7-zip.org and retry."
```

- The system tar is located by its full path under `SystemRoot`, never `exec.LookPath("tar")`. On a runner, and on any machine with Git for Windows, `PATH` may find GNU tar first, which cannot read 7z (spec: "A tar on PATH that is not the system's is not used").
- After each attempt, the check is `findFile(tmp, "firefox.exe")` under `core/`, not just the exit code. Before the next attempt, everything in `<tmp>` except `firefox.7z` is removed, so a partial tree from the first tool cannot leak into the second.
- A table-driven helper in `sfx.go` (`extract7z(archive, dest string, tools []extractor, want string) error`) runs the chain, so it can be tested on any OS with fake tools. `unpack_windows.go` only builds the Windows list.

*Alternative:* 7-Zip first. Rejected: most user machines have no 7-Zip, and every Windows 11 machine has `tar.exe`.

### 4. Keep the verified download when only the extraction failed

Today `cleanup()` deletes the download on any failure after it. For an extractor failure, the remedy is "install 7-Zip and retry", and deleting a verified 90 MB file makes the retry download it again.

On an `unpackArchive` error the download is kept. On the retry, `download` gets HTTP 416 for the complete file and returns, and the file is verified again before unpacking, so keeping it costs no safety. Checksum and signature failures still delete it, as today.

### 5. Disk factor per OS

The check stays before the download and stays an estimate from the `HEAD` size. The factor becomes:

| OS | Factor | Why |
|---|---|---|
| Windows | 6x | download (1) + carved `.7z` (1) + unpacked `core/` (~4) |
| Linux, macOS | 5x | download (1) + unpacked tree (~3.5-4); the factor is the same everywhere apart from the carved copy |

A named constant per OS with this table as its comment. The low-disk test (`TestInstallRefusesLowDisk`) already fakes `freeDisk`. It gets cases on both sides of the new bound. The network run logs the real unpacked size against the download, so the factor can be checked against Mozilla's actual builds (task 4.2).

*Alternative:* one factor for every OS. Rejected: it would either under-ask on Windows or refuse Linux installs that fit.

### 6. Windows fixture: a stub plus an appended LZMA + BCJ2 archive

`fixture_windows_test.go` drops WiX. `hostRelease` builds:

```
 stub   = "MZ" + padding + a decoy "7z\xBC\xAF\x27\x1C" with a wrong CRC + padding
 archive= 7z a -t7z -m0=LZMA -mf=BCJ2 -ms=on fx.7z core/firefox.exe setup.exe
 file   = stub || archive || 3 tail bytes  ->  "Firefox Setup 999.0.exe"
```

The decoy makes the CRC check load-bearing. The file is signed at test time like the other OSes' fixtures (`signedFixture`). 7-Zip comes from `PATH` or `%ProgramFiles%\7-Zip`. A missing 7-Zip fails on CI and skips elsewhere (`needFixtureTool`).

The test runs the real chain, so on `windows-latest` it proves whichever extractor works there. A second Windows test forces each extractor alone (tar only, 7-Zip only) and logs which one passed. That answers the open question about the runner's `tar.exe` on the first CI run.

The OS-neutral `sfx_test.go` builds a synthetic file with a valid 32-byte start header by hand, with no tools. It checks:

- the decoy is skipped
- the copy length is exact
- a truncated file and a corrupt CRC are rejected
- a file with no archive is rejected

When `7z` and `bsdtar` are present (Linux dev machines), one more test carves a real archive and extracts it. It skips without them, since the Windows lane is the authoritative run.

### 7. The network job also runs on pull requests that touch the installer

`install.yml` gains `pull_request` with `paths: [internal/install/**, .github/workflows/install.yml]`. The job downloads about 80-90 MB from Mozilla per OS, which is fine for the few PRs that touch the installer. It is the only test of Mozilla's real files, and B4 sat for a week behind the Monday schedule. `schedule` and `workflow_dispatch` stay.

## Risks / Trade-offs

- [The `tar.exe` on `windows-latest` cannot decode LZMA or BCJ2] → 7-Zip is on the image, so CI stays green. The forced-extractor test (decision 6) records which tool worked. If the system tar fails there, `docs/internals/Install.md` says that 7-Zip is effectively required on Server 2025, not just on Windows 10.
- [Windows 10 users have a `tar.exe` without LZMA] → the error tells them to install 7-Zip. Windows 10 is past end of support (2025-10-14).
- [Mozilla changes the setup's format (another SFX, a different compressor)] → `find7z` fails with "no 7z archive in <file>", the weekly and PR-triggered network job catches it, and the fixture mirrors the format, so the change has to be deliberate.
- [The stub's decoy signature happens to pass the CRC check] → the chance is 2^-32 per hit. Even then, the copy length check against the file size has to pass too, and the extractor would then fail with a clear error, not install something wrong.
- [A partial extraction by `tar.exe` followed by a 7-Zip pass mixes trees] → `<tmp>` is cleared between attempts (decision 3).
- [Keeping a verified download on extractor failure leaves 90 MB in the cache] → it is in `downloads/`, which `omnisnap uninstall firefox` already clears, and the retry needs it.

## Migration Plan

- There is no persisted state to migrate. Existing Windows caches cannot hold a Windows install, because none ever succeeded. A stray `firefox-<v>.msi` in `downloads/` from a failed run is harmless; `uninstall` clears `downloads/`.
- Rollout: the change lands in `v0.2.0-rc.1`. Its gate is "Install (network)" green on `windows-latest` and `ubuntu-latest`, dispatched by hand on the branch, and again for the tag (change `release-v0-2-0`).
- Rollback: revert the change. Windows goes back to failing as today, and Linux and macOS are unaffected apart from the disk factor.

## Open Questions

- Should a `v0.2.x` patch also check the release channel's MSI and EXE layout for `win64-aarch64`? Mozilla's aarch64 setup is expected to have the same SFX shape, but no runner covers it. If it differs, the network job cannot show it, and a user report would be the first sign. This doesn't change the approach.
