# Spec Delta

## Purpose

Defines how `omnisnap install firefox` turns Mozilla's official download into a browser in Omnisnap's cache on every OS: what is downloaded, what is verified, how it is unpacked without running a vendor installer, and how it fails.

## ADDED Requirements

### Requirement: Windows downloads Mozilla's setup executable
On Windows, `omnisnap install firefox` SHALL download Mozilla's `Firefox Setup <version>.exe` for the host architecture from Mozilla's release archive, under the path Mozilla's `SHA512SUMS` lists it by (`win64/`, `win64-aarch64/` or `win32/`, then `en-US/`).

#### Scenario: The download names the setup executable
- **WHEN** a user runs `omnisnap install firefox --channel esr` on 64-bit x86 Windows
- **THEN** the download notice names a URL ending in `/win64/en-US/Firefox%20Setup%20<version>.exe` on `ftp.mozilla.org`
- **AND** no `.msi` is downloaded

### Requirement: Verification covers exactly the file that is unpacked
The install SHALL check the downloaded file's SHA-512 against its line in `SHA512SUMS`, after verifying the signature on `SHA512SUMS` against Mozilla's pinned release key. Only that verified file SHALL be unpacked, and nothing SHALL reach the cache if either check fails.

#### Scenario: A tampered setup executable is refused
- **GIVEN** a downloaded `Firefox Setup <version>.exe` whose bytes differ from its `SHA512SUMS` line
- **WHEN** the install verifies it
- **THEN** the install fails with a checksum error
- **AND** the cache holds no Firefox directory for that version

#### Scenario: A tampered embedded archive is refused
- **GIVEN** a setup executable whose embedded archive was altered after `SHA512SUMS` was signed
- **WHEN** the install verifies it
- **THEN** the install fails before unpacking, because the SHA-512 covers the whole executable, embedded archive included

### Requirement: Windows unpacks without running an installer
On Windows, the install SHALL extract the browser from the 7-Zip archive embedded in the verified setup executable without executing the executable, its stub or any program it contains, and SHALL write nothing outside Omnisnap's cache and its temporary files.

#### Scenario: Nothing outside the cache changes
- **WHEN** `omnisnap install firefox` completes on Windows
- **THEN** `firefox.exe` is under `%LOCALAPPDATA%\omnisnap\browsers\firefox\<version>\firefox\`
- **AND** no `HKCU\Software\Mozilla` or `HKLM\Software\Mozilla` key was created, and no Start menu entry or "Apps & features" entry exists for it

#### Scenario: No vendor process runs
- **WHEN** the install unpacks the setup executable
- **THEN** no process is started from the downloaded executable or from any file extracted from it

### Requirement: Windows install needs no administrator rights
On Windows, `omnisnap install firefox` SHALL succeed for a standard user without elevation, and SHALL NOT trigger a User Account Control prompt.

#### Scenario: A standard user installs
- **GIVEN** a Windows account without administrator rights
- **WHEN** it runs `omnisnap install firefox`
- **THEN** the install completes without a UAC prompt and `omnisnap browsers` lists the cached Firefox

### Requirement: Windows extractor order and success check
On Windows, the install SHALL extract the embedded archive with the operating system's own `tar.exe` first, then with an installed 7-Zip (`7z.exe` on `PATH` or in 7-Zip's default install folder). An attempt SHALL count as successful only if the extracted tree holds `core/firefox.exe`; otherwise the next extractor is tried.

#### Scenario: The system tar extracts it
- **GIVEN** a Windows whose `tar.exe` can read LZMA and BCJ2 7z archives
- **WHEN** the install unpacks the verified setup executable
- **THEN** the browser is extracted with `tar.exe` and 7-Zip is not needed

#### Scenario: The system tar cannot, 7-Zip can
- **GIVEN** a Windows whose `tar.exe` fails on the archive, or exits 0 without producing `core/firefox.exe`, and 7-Zip installed
- **WHEN** the install unpacks the verified setup executable
- **THEN** the browser is extracted with 7-Zip and the install completes

#### Scenario: A tar on PATH that is not the system's is not used
- **GIVEN** a GNU tar earlier on `PATH` (for example Git Bash's)
- **WHEN** the install picks its first extractor
- **THEN** it uses the `tar.exe` in the Windows system folder, not the one on `PATH`

### Requirement: Windows extraction failure is explained
When no extractor produces `core/firefox.exe`, the install SHALL fail before anything reaches the cache, with an error that names each extractor tried and its failure, and tells the user to install 7-Zip and retry.

#### Scenario: Neither extractor works
- **GIVEN** a Windows whose `tar.exe` cannot read the archive and no 7-Zip installed
- **WHEN** the user runs `omnisnap install firefox`
- **THEN** the command exits non-zero with an error naming `tar.exe` and its failure, saying 7-Zip was not found, and telling the user to install 7-Zip from https://www.7-zip.org and retry
- **AND** the cache holds no Firefox directory for that version, and the verified download stays in the cache's `downloads/` folder so the retry does not download it again

### Requirement: Disk space covers the unpacked browser
Before downloading, the install SHALL refuse to start when the free space in the cache is less than the space the download, any intermediate copy and the unpacked browser need together, estimated from the archive's size.

#### Scenario: Too little space for the unpacked tree
- **GIVEN** a Windows cache volume with free space of three times the setup executable's size
- **WHEN** the user runs `omnisnap install firefox`
- **THEN** the install refuses before downloading, with an error that states the free space and the space needed

#### Scenario: Enough space
- **GIVEN** free space above the stated need
- **WHEN** the user runs `omnisnap install firefox`
- **THEN** the disk check passes and the download starts
