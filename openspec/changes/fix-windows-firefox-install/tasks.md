# Tasks

## 1. Before building

- [x] 1.1 Record the evidence the design rests on: list `Firefox Setup <v>.msi` with `7z l` (no `!File` table, one `Binary.WrappedExe` stream) and the setup `.exe` with `7z l -slt` (7z at offset 0x20200, `LZMA:25 BCJ2`, `core/firefox.exe`), for the current ESR and the current release. Verify: both outputs match design.md's Context diagram, and any difference (offset, method, folder name) is corrected in design.md before task 2.1.
- [x] 1.2 Write the normative spec changes in `specs/SPEC-install.md` from `specs/firefox-install/spec.md`:
  - the Windows unpack bullet (the setup `.exe`'s embedded 7z, system `tar.exe` then 7-Zip, nothing executed);
  - the disk rule (per-OS factor, design decision 5);
  - the testing strategy (stub plus 7z fixture instead of WiX; the network job also runs on PRs that touch `internal/install/`);
  - a decision-log entry dated the day of the change, citing run 45198060633.

  Verify: every requirement in the delta has a counterpart in SPEC-install.md, and no mention of `msiexec` or WiX is left there.

## 2. Find and carve the embedded archive (OS-neutral)

- [x] 2.1 Add `internal/install/sfx.go` with `find7z` and `carve7z` (design decision 2): signature scan in 1 MiB chunks with overlap; accept a hit only if the major version is 0, the start-header CRC matches, `NextHeaderSize > 0` and the archive fits in the file; copy exactly `32 + NextHeaderOffset + NextHeaderSize` bytes. Standard library only. Verify: `go vet ./internal/install/` is clean and `go.mod` is unchanged.
- [ ] 2.2 Add `internal/install/sfx_test.go`, which builds synthetic inputs in code with no tools. Cases:
  - a stub with a decoy signature (wrong CRC) followed by a valid header: the decoy is skipped and the offset is the real one;
  - the carved length is exact and the trailing bytes are dropped;
  - a corrupt CRC, a truncated file (next header past EOF), and a file with no signature are each rejected with an error naming the file.

  Verify: `go test ./internal/install/ -run 7z` passes on Linux, Windows and macOS in CI.
- [x] 2.3 Add `extract7z(archive, dest, tools, want)` to `sfx.go` (design decision 3): tries each extractor in order, treats "exit 0 but `want` missing" as a failure, clears `dest` except the archive between attempts, and returns one error naming every attempt plus the "install 7-Zip from https://www.7-zip.org and retry" hint. Verify: a unit test with fake extractors (a Go test binary re-exec, or small scripts per OS) covers: the first succeeds; the first exits 0 without the file and the second succeeds; both fail (the error names both and carries the hint); and no extractors at all.
- [x] 2.4 Add a tool-backed carve-and-extract test that builds a real LZMA + BCJ2 archive with `7z`/`7zz`, prepends a stub with a decoy, carves it and extracts with `bsdtar` or `7z`, skipping when the tools are missing (the Windows lane is the authoritative run). Verify: it passes locally on a Linux machine with `7z` and `bsdtar`, and reports a skip, not a failure, without them.

## 3. Windows unpack and download

- [x] 3.1 Change `archivePath` and `archiveName` in `internal/install/mozilla.go` to `<plat>/en-US/Firefox Setup <v>.exe` and `firefox-<v>-setup.exe`, and update the "Layout verified" comment with the date of task 1.1. Verify: `TestArchiveURL` (install_test.go) expects `win64/en-US/Firefox%20Setup%20160.0.exe` and `win32/...exe`, and `TestChecksumForNamesWithSpaces` uses the `.exe` line; both pass.
- [x] 3.2 Rewrite `internal/install/unpack_windows.go`: carve into `<tmp>/firefox.7z`, build the extractor list (`%SystemRoot%\System32\tar.exe` by full path; then `7z` from `PATH` or `%ProgramFiles%\7-Zip\7z.exe`), call `extract7z` with `want = core/firefox.exe`, and delete `firefox.7z` afterwards. Remove `msiexec`, and update the comments in `install.go` and `download.go` that name the MSI. Verify: `GOOS=windows go vet ./internal/install/` is clean, and `grep -rn msiexec internal/` finds nothing.
- 3.3 Dropped (owner, 2026-11-25): a failed extraction deletes the download, as today (design decision 4). Comments in `install.go` and `download.go` that name the MSI are updated in task 3.2.
- [ ] 3.4 Replace the WiX fixture in `internal/install/fixture_windows_test.go` with the stub plus 7z fixture (design decision 6): `7z a -t7z -m0=LZMA -mf=BCJ2 -ms=on` on `core/firefox.exe` and `setup.exe`, a stub with a decoy signature, a 4 KiB tail standing in for the Authenticode signature, named `Firefox Setup 999.0.exe`, signed with `signedFixture`. Find 7-Zip on `PATH` or in `%ProgramFiles%\7-Zip`; `needFixtureTool` when it is missing. Update `fixture_test.go`'s comment. Verify: CI's `windows-latest` unit job runs `TestInstallVerifiesAndUnpacks` with the new fixture and passes.
- [ ] 3.5 Add a Windows-only test that runs each extractor alone on the fixture (tar only, 7-Zip only) and logs which ones work. A tool that is present but fails is logged, not fatal; at least one must work. Verify: the `windows-latest` CI log shows the result for `tar.exe` and for 7-Zip, and the result is recorded in `docs/internals/Install.md` (task 3.6).
- [x] 3.6 Update `docs/internals/Install.md` for this group:
  - the Windows unpack step: carve, then `tar.exe`, then 7-Zip;
  - nothing executed and no registry writes;
  - `sfx.go` and `unpack_windows.go` in `## Source files`;
  - the new tests in `## Tests`;
  - "Where the code differs from the spec": "Windows needs a 7z-capable `tar.exe` or 7-Zip".

  Verify: `go test ./test/internals/` passes.

## 4. Disk-space check

- [ ] 4.1 Replace the 2x factor in `install.go` with the per-OS constants from design decision 5 (Windows 6x, Linux and macOS 5x), with the table as the comment, and make the refusal state the free space and the space needed. Verify: `TestInstallRefusesLowDisk` gets cases just below and just above the bound on the host OS, and passes on all three CI runners.
- [ ] 4.2 Log the unpacked size next to the download size in `TestInstallESRFromMozilla` (network tag). Verify: the network run's log (task 6.1) shows the ratio for Linux and Windows, both below the factor, and the numbers are recorded in design.md's decision 5 (raise the factor if either is not below it).
- [x] 4.3 Update the disk step in `docs/internals/Install.md` ("twice that free" becomes the per-OS factor). Verify: `go test ./test/internals/` passes.

## 5. Network job and remaining docs

- [ ] 5.1 Add `pull_request` with `paths: [internal/install/**, .github/workflows/install.yml]` to `.github/workflows/install.yml`, keep `schedule` and `workflow_dispatch`, and update its header comment. Verify: actionlint reports no new findings, and the PR for this change shows the "Install (network)" checks.
- [x] 5.2 Update `docs/internals/Testing-and-CI.md`: `install.yml` runs weekly, by hand, and on PRs touching `internal/install/`. Also check `Architecture.md` and `Repository-Layout.md` (`grep -rl internal/install docs/internals/`) and update any line this change makes wrong, such as an "MSI" mention or the install fixtures description. Verify: `go test ./test/internals/` passes, and `grep -rn -i 'msi\b\|msiexec\|wix' docs/internals/` finds nothing.
- 5.3 Dropped (owner, 2026-11-25): `docs/status-2026-11-19.md` is a dated report and stays as it is; the answer to its `msiexec /a` question lives in `SPEC-install.md`'s decision log (task 1.2).

## 6. On the runners

- [ ] 6.1 Dispatch "Install (network)" by hand on this change's branch (`gh workflow run install.yml --ref <branch>`), or let the new PR trigger run it. Verify: `install ESR (windows-latest)` and `install ESR (ubuntu-latest)` are both green: install, discover through the cache source, launch through `process`, uninstall leaving nothing. Record the run ID.
- [ ] 6.2 On a Windows machine or VM without admin rights, run `omnisnap install firefox` (release channel) with the branch's binary. Verify:
  - no UAC prompt;
  - `omnisnap browsers` lists the cached Firefox;
  - `reg query HKCU\Software\Mozilla` and `reg query HKLM\Software\Mozilla` show no key created by the install;
  - "Apps & features" has no new Firefox entry;
  - `omnisnap uninstall firefox` leaves the cache empty.

  If no such machine is available, the owner does it before `v0.2.0` (change `release-v0-2-0`).
- [ ] 6.3 Close the bookkeeping:
  - tick the "Weekly network job" box in `tasks/todo.md` Task 37 with the run ID from 6.1;
  - update `README.md`'s M0 milestone row ("first runs on 2026-11-23" becomes done, with the run);
  - set `tasks/backlog.md` B4's status to done with a pointer to this change.

  Verify: `grep -n 'Weekly network job' tasks/todo.md` shows `[x]` and the run ID, and `go test ./test/internals/` passes.

## Workflow follow-up

- Archive the change after the PR is merged and the network job is green for the `v0.2.0-rc.1` tag.
