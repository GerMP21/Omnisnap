//go:build windows

package install

import (
	"fmt"
	"os"
	"path/filepath"
)

// unpackArchive extracts Firefox from the verified "Firefox Setup <v>.exe":
// it copies out the embedded 7z archive (sfx.go) and extracts that with the
// system's tar.exe, then with 7-Zip. Nothing from the download is executed,
// nothing is written outside dest, and no administrator rights are needed
// (SPEC-install.md). The browser lands in dest/core/; finalize moves it.
func unpackArchive(archive, dest string) error {
	carved := filepath.Join(dest, "firefox.7z")
	if err := carve7z(archive, carved); err != nil {
		return err
	}
	defer func() { _ = os.Remove(carved) }()
	if err := extract7z(carved, dest, windowsExtractors(), "core/firefox.exe"); err != nil {
		return fmt.Errorf("install: unpack %s: %w", filepath.Base(archive), err)
	}
	return nil
}

// windowsExtractors is the order the archive is tried in: Windows' own
// tar.exe (bsdtar), then 7-Zip. tar.exe is taken from the system folder by
// its full path, never from PATH: on a runner or with Git for Windows, PATH
// finds GNU tar first, and GNU tar cannot read 7z.
func windowsExtractors() []extractor {
	return []extractor{
		{name: "tar.exe", path: systemTar(), args: tarArgs},
		{name: "7-Zip", path: sevenZipPath(), args: sevenZipArgs},
	}
}

// systemTar is %SystemRoot%\System32\tar.exe, shipped with Windows since
// Windows 10 1803; "" when it is missing.
func systemTar() string {
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	p := filepath.Join(root, "System32", "tar.exe")
	if _, err := os.Stat(p); err != nil {
		return ""
	}
	return p
}
