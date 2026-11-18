//go:build darwin

package install

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// unpackArchive mounts the .dmg read-only, copies Firefox.app out, and
// detaches. Never runs the vendor's installer.
func unpackArchive(archive, dest string) error {
	mnt, err := os.MkdirTemp("", "omnisnap-dmg-")
	if err != nil {
		return fmt.Errorf("install: unpack: %w", err)
	}
	defer func() { _ = os.RemoveAll(mnt) }()
	if out, err := exec.Command("hdiutil", "attach", "-nobrowse", "-readonly", "-mountpoint", mnt, archive).CombinedOutput(); err != nil {
		return fmt.Errorf("install: unpack: hdiutil attach: %v: %s", err, out)
	}
	defer func() { _ = exec.Command("hdiutil", "detach", mnt).Run() }()
	if out, err := exec.Command("cp", "-R", filepath.Join(mnt, "Firefox.app"), dest).CombinedOutput(); err != nil {
		return fmt.Errorf("install: unpack: copy: %v: %s", err, out)
	}
	return nil
}
