//go:build windows

package install

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// wxs is the smallest package that carries firefox.exe in a cabinet embedded
// in the MSI, as Mozilla's "Firefox Setup <v>.msi" does; unpack_windows.go
// extracts it with an administrative install (msiexec /a), which copies the
// source image without installing (Microsoft Learn, "Administrative
// Installation": https://learn.microsoft.com/en-us/windows/win32/msi/administrative-installation).
// UNVERIFIED: the WiX v3 docs could not be fetched (their URL now redirects
// to a 404). The authoring follows the WiX v3 schema
// (http://schemas.microsoft.com/wix/2006/wi); the Windows CI job, whose image
// ships WiX Toolset 3.14 (actions/runner-images, Windows2025-VS2026-Readme.md),
// is what checks it.
//
// The MSI's ProductVersion is not the release version: an MSI version's major
// and minor parts must be below 256 (candle: "error CNDL0242 : Invalid product
// version '999.0.0.0'", CI run 44636485891). The installer only reads the
// release version (999.0) from the URL and SHA512SUMS.
const wxs = `<?xml version="1.0" encoding="UTF-8"?>
<Wix xmlns="http://schemas.microsoft.com/wix/2006/wi">
  <Product Id="*" Name="Mozilla Firefox 999.0" Language="1033" Version="1.0.0.0"
           Manufacturer="Omnisnap test fixture" UpgradeCode="6A3B1F0E-6C55-4E57-9D1A-0F2C8B7E5D41">
    <Package InstallerVersion="200" Compressed="yes" InstallScope="perMachine" />
    <MediaTemplate EmbedCab="yes" />
    <Directory Id="TARGETDIR" Name="SourceDir">
      <Directory Id="ProgramFilesFolder">
        <Directory Id="INSTALLDIR" Name="Mozilla Firefox">
          <Component Id="Firefox" Guid="2D7E4C19-8B0A-4F6E-A3C5-71E9D0B4F2A8">
            <File Id="firefox.exe" Name="firefox.exe" Source="firefox.exe" KeyPath="yes" />
          </Component>
        </Directory>
      </Directory>
    </Directory>
    <Feature Id="Main" Level="1">
      <ComponentRef Id="Firefox" />
    </Feature>
  </Product>
</Wix>
`

// hostRelease builds "Firefox Setup 999.0.msi" with WiX 3 (candle compiles,
// light links; -sval skips ICE validation, which needs nothing the fixture
// tests).
func hostRelease(t *testing.T) hostFixture {
	t.Helper()
	candle, light := wixTools()
	if candle == "" {
		needFixtureTool(t, "WiX Toolset v3 (candle.exe, light.exe)")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "firefox.wxs"), []byte(wxs), 0o644); err != nil {
		t.Fatal(err)
	}
	// Not a real PE: msiexec /a copies files, it never runs them.
	if err := os.WriteFile(filepath.Join(dir, "firefox.exe"), []byte("Mozilla Firefox 999.0 test fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	msi := filepath.Join(dir, "Firefox Setup 999.0.msi")
	for _, c := range [][]string{
		{candle, "-nologo", "-out", "firefox.wixobj", "firefox.wxs"},
		{light, "-nologo", "-sval", "-out", msi, "firefox.wixobj"},
	} {
		cmd := exec.Command(c[0], c[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v: %s", filepath.Base(c[0]), err, out)
		}
	}
	archive, err := os.ReadFile(msi)
	if err != nil {
		t.Fatal(err)
	}
	return signedFixture(t, archive)
}

// wixTools finds candle.exe and light.exe: the WIX variable the WiX 3
// installer sets, the default install folder, then PATH.
func wixTools() (candle, light string) {
	var dirs []string
	if w := os.Getenv("WIX"); w != "" {
		dirs = append(dirs, filepath.Join(w, "bin"))
	}
	if pf := os.Getenv("ProgramFiles(x86)"); pf != "" {
		if m, _ := filepath.Glob(filepath.Join(pf, "WiX Toolset v3*", "bin")); len(m) > 0 {
			dirs = append(dirs, m...)
		}
	}
	for _, d := range dirs {
		c, l := filepath.Join(d, "candle.exe"), filepath.Join(d, "light.exe")
		if fileExists(c) && fileExists(l) {
			return c, l
		}
	}
	c, err1 := exec.LookPath("candle.exe")
	l, err2 := exec.LookPath("light.exe")
	if err1 == nil && err2 == nil {
		return c, l
	}
	return "", ""
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
