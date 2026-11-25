package install

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// buildSetupExe builds a fake "Firefox Setup 999.0.exe" in the shape of
// Mozilla's (design.md of fix-windows-firefox-install, checked on 161.0): a
// PE-looking stub, a solid LZMA + BCJ2 7z archive holding core/firefox.exe
// and setup.exe, then trailing bytes standing in for the Authenticode
// signature. The stub carries a decoy signature with a wrong CRC, so the
// start-header check is exercised too. sevenZip is the 7-Zip binary that
// builds the archive.
func buildSetupExe(t *testing.T, sevenZip string) []byte {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "core"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Not real programs: nothing in the archive is ever run.
	if err := os.WriteFile(filepath.Join(dir, "core", "firefox.exe"), []byte("Mozilla Firefox 999.0 test fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "setup.exe"), []byte("NSIS installer stand-in: never run\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(sevenZip, "a", "-t7z", "-m0=LZMA", "-mf=BCJ2", "-ms=on", "-bd", "fx.7z", "core", "setup.exe")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("7z a: %v: %s", err, out)
	}
	archive, err := os.ReadFile(filepath.Join(dir, "fx.7z"))
	if err != nil {
		t.Fatal(err)
	}
	stub := append([]byte("MZ"), bytes.Repeat([]byte{0}, 4094)...)
	stub = append(stub, decoy()...)
	stub = append(stub, bytes.Repeat([]byte{0}, 4096)...)
	signature := bytes.Repeat([]byte{0x30, 0x82}, 2048) // 4 KiB, DER-looking
	return bytes.Join([][]byte{stub, archive, signature}, nil)
}

// A real archive in Mozilla's shape is carved and extracted by the tools this
// machine has. The Windows lane runs the same path through the installer
// (fixture_windows_test.go); here it skips when the tools are missing.
func TestCarveAndExtractARealArchive(t *testing.T) {
	sevenZip := sevenZipPath()
	if sevenZip == "" {
		t.Skip("7-Zip not found: cannot build a real 7z archive")
	}
	src := writeTemp(t, "Firefox Setup 999.0.exe", buildSetupExe(t, sevenZip))
	tools := []extractor{{name: "7-Zip", path: sevenZip, args: sevenZipArgs}}
	if bsdtar, err := exec.LookPath("bsdtar"); err == nil {
		tools = append([]extractor{{name: "bsdtar", path: bsdtar, args: tarArgs}}, tools...)
	}
	for _, x := range tools {
		t.Run(x.name, func(t *testing.T) {
			dest := t.TempDir()
			archive := filepath.Join(dest, "firefox.7z")
			if err := carve7z(src, archive); err != nil {
				t.Fatal(err)
			}
			if err := extract7z(archive, dest, []extractor{x}, "core/firefox.exe"); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(filepath.Join(dest, "core", "firefox.exe"))
			if err != nil || !bytes.Contains(got, []byte("999.0")) {
				t.Fatalf("core/firefox.exe = %q, %v", got, err)
			}
		})
	}
}
