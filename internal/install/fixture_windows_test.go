//go:build windows

package install

import (
	"testing"
)

// hostRelease builds "Firefox Setup 999.0.exe" in the shape of Mozilla's
// download (buildSetupExe: a stub with a decoy signature, a solid LZMA + BCJ2
// 7z archive with core/firefox.exe and setup.exe, trailing bytes for the
// Authenticode signature) and signs it at test time. Until 2026-11-25 this was
// an MSI with a real file table, a shape Mozilla's MSI does not have, which
// is how the old administrative-install unpack passed here and failed on the
// real download (run 45198060633). 7-Zip ships on the Windows runner
// images; without it the test fails on CI and skips elsewhere.
func hostRelease(t *testing.T) hostFixture {
	t.Helper()
	sevenZip := sevenZipPath()
	if sevenZip == "" {
		needFixtureTool(t, "7-Zip (7z.exe)")
	}
	return signedFixture(t, buildSetupExe(t, sevenZip))
}

// Each extractor alone, on the same fixture: which ones work on this machine.
// A tool that is present but fails is logged, not fatal (the chain falls back);
// at least one must work, or `omnisnap install firefox` cannot work here.
func TestWindowsExtractorsEachAlone(t *testing.T) {
	sevenZip := sevenZipPath()
	if sevenZip == "" {
		needFixtureTool(t, "7-Zip (7z.exe)")
	}
	src := writeTemp(t, "Firefox Setup 999.0.exe", buildSetupExe(t, sevenZip))
	worked := 0
	for _, x := range windowsExtractors() {
		if x.path == "" {
			t.Logf("%s: not found", x.name)
			continue
		}
		dest := t.TempDir()
		archive := dest + `\firefox.7z`
		if err := carve7z(src, archive); err != nil {
			t.Fatal(err)
		}
		if err := extract7z(archive, dest, []extractor{x}, "core/firefox.exe"); err != nil {
			t.Logf("%s (%s): does NOT extract the archive: %v", x.name, x.path, err)
			continue
		}
		t.Logf("%s (%s): extracts the archive", x.name, x.path)
		worked++
	}
	if worked == 0 {
		t.Fatal("no extractor on this machine can unpack Mozilla's setup archive")
	}
}
