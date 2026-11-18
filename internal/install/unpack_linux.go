//go:build linux

package install

import (
	"fmt"
	"os"
	"os/exec"
)

// unpackArchive extracts a .tar.xz into dest. Go's standard library has no xz
// decoder and the project takes no dependency for one: xz(1) does the
// decompression (present on every Linux and macOS image; the Windows build
// uses the MSI instead).
func unpackArchive(archive, dest string) error {
	xz, err := exec.LookPath("xz")
	if err != nil {
		return fmt.Errorf("install: unpack: xz is required to extract %s: %w", archive, err)
	}
	tar, err := exec.LookPath("tar")
	if err != nil {
		return fmt.Errorf("install: unpack: tar is required: %w", err)
	}
	dx := exec.Command(xz, "-dc", archive)
	tx := exec.Command(tar, "-xf", "-", "-C", dest)
	pipe, err := dx.StdoutPipe()
	if err != nil {
		return fmt.Errorf("install: unpack: %w", err)
	}
	tx.Stdin = pipe
	tx.Stderr = os.Stderr
	if err := dx.Start(); err != nil {
		return fmt.Errorf("install: unpack: %w", err)
	}
	if err := tx.Start(); err != nil {
		_ = dx.Process.Kill()
		return fmt.Errorf("install: unpack: %w", err)
	}
	if err := tx.Wait(); err != nil {
		return fmt.Errorf("install: unpack: tar: %w", err)
	}
	if err := dx.Wait(); err != nil {
		return fmt.Errorf("install: unpack: xz: %w", err)
	}
	return nil
}
