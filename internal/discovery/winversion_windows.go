//go:build windows

package discovery

import (
	"os"
	"os/exec"
	"strings"
	"syscall"
)

// regQuery runs `reg query <key> /s`: one text dump per key, which is also
// exactly what the recorded test fixtures are.
func regQuery(key string) (string, bool) {
	cmd := exec.Command("reg", "query", key, "/s")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.Output()
	if err != nil {
		return "", false // the key does not exist: normal for a missing browser
	}
	return string(out), true
}

// fileVersion reads a Windows executable's VERSIONINFO. The registry's
// DisplayVersion normally answers first (every Chromium-family browser has
// one); this covers portable builds, where the file's metadata is the only
// source. Go's standard library has no VERSIONINFO reader and Omnisnap takes
// no dependency for it, so PowerShell's FileVersionInfo does the one lookup
// (hidden, no profile, ~300 ms, only when needed).
func fileVersion(path string) string {
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command",
		`(Get-Item -LiteralPath $env:OMNISNAP_FILE).VersionInfo.FileVersion`)
	cmd.Env = append(os.Environ(), "OMNISNAP_FILE="+path)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
