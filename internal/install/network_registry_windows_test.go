//go:build network && windows

package install

import (
	"os/exec"
	"strings"
	"testing"
)

// mozillaKeys are where a Firefox installer writes (fix-windows-firefox-install
// task 6.2): Mozilla's own keys, and the Uninstall keys that "Apps & features"
// lists. The install unpacks into the cache and must change none of them.
var mozillaKeys = []string{
	`HKCU\Software\Mozilla`,
	`HKLM\Software\Mozilla`,
	`HKLM\Software\WOW6432Node\Mozilla`,
	`HKCU\Software\Microsoft\Windows\CurrentVersion\Uninstall`,
	`HKLM\Software\Microsoft\Windows\CurrentVersion\Uninstall`,
	`HKLM\Software\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall`,
}

// registrySnapshot is `reg query /s` of each Mozilla key; a missing key is
// recorded as missing, so a key that appears changes the snapshot too.
func registrySnapshot(t *testing.T) string {
	t.Helper()
	var b strings.Builder
	for _, k := range mozillaKeys {
		out, err := exec.Command("reg", "query", k, "/s").CombinedOutput()
		b.WriteString("== " + k + "\n")
		if err != nil {
			b.WriteString("(missing)\n")
			continue
		}
		b.Write(out)
	}
	return b.String()
}
