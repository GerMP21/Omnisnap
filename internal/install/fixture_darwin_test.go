//go:build darwin

package install

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// hostRelease builds "Firefox 999.0.dmg" the way Mozilla ships it: a disk
// image whose root holds Firefox.app (unpack_darwin.go mounts it and copies
// Firefox.app out). hdiutil is part of macOS.
// UNVERIFIED: Apple's hdiutil man page could not be fetched (its archive URL
// is gone); the flags are hdiutil(1)'s `create` verb as installed: -srcfolder
// copies the folder's contents to the volume root, -format UDZO is the
// compressed read-only format Mozilla's dmg uses. The macOS CI job runs this.
func hostRelease(t *testing.T) hostFixture {
	t.Helper()
	hdiutil, err := exec.LookPath("hdiutil")
	if err != nil {
		needFixtureTool(t, "hdiutil")
	}
	src := t.TempDir()
	macos := filepath.Join(src, "Firefox.app", "Contents", "MacOS")
	if err := os.MkdirAll(macos, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(macos, "firefox"), []byte("#!/bin/sh\necho 'Mozilla Firefox 999.0'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	dmg := filepath.Join(t.TempDir(), "Firefox 999.0.dmg")
	if out, err := exec.Command(hdiutil, "create", "-volname", "Firefox", "-srcfolder", src, "-format", "UDZO", "-ov", dmg).CombinedOutput(); err != nil {
		t.Fatalf("hdiutil create: %v: %s", err, out)
	}
	archive, err := os.ReadFile(dmg)
	if err != nil {
		t.Fatal(err)
	}
	return signedFixture(t, archive)
}
