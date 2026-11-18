//go:build windows

package install

import (
	"fmt"
	"os/exec"
)

// unpackArchive extracts the MSI with an administrative install: msiexec /a
// copies the files without installing anything or touching the registry
// (SPEC-install.md: "without running an installer").
func unpackArchive(archive, dest string) error {
	out, err := exec.Command("msiexec", "/a", archive, "/qn", "TARGETDIR="+dest).CombinedOutput()
	if err != nil {
		return fmt.Errorf("install: unpack: msiexec /a: %v: %s", err, out)
	}
	return nil
}
