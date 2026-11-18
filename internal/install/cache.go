package install

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Cache layout (SPEC-install.md):
//
//	<cache>/browsers/firefox/<version>/firefox/firefox        Linux
//	<cache>/browsers/firefox/<version>/Firefox.app/...        macOS
//	<cache>/browsers/firefox/<version>/firefox/firefox.exe    Windows
//	<cache>/downloads/firefox-<version>.<ext>
//
// <cache> is LOCALAPPDATA\omnisnap on Windows, ~/Library/Caches/omnisnap on
// macOS, $XDG_CACHE_HOME/omnisnap (or ~/.cache/omnisnap) elsewhere.

// CacheDir is Omnisnap's cache root on this machine.
func CacheDir(getenv func(string) string) (string, error) {
	if getenv == nil {
		getenv = os.Getenv
	}
	var root string
	switch runtime.GOOS {
	case "windows":
		root = getenv("LOCALAPPDATA")
		if root == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", fmt.Errorf("install: cache: %w", err)
			}
			root = filepath.Join(home, "AppData", "Local")
		}
		return filepath.Join(root, "omnisnap", "browsers"), nil
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("install: cache: %w", err)
		}
		return filepath.Join(home, "Library", "Caches", "omnisnap", "browsers"), nil
	default:
		root = getenv("XDG_CACHE_HOME")
		if root == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", fmt.Errorf("install: cache: %w", err)
			}
			root = filepath.Join(home, ".cache")
		}
		return filepath.Join(root, "omnisnap", "browsers"), nil
	}
}

// downloadsDir holds archives; it is cleaned after each install.
func downloadsDir(cache string) string { return filepath.Join(cache, "downloads") }

// versionDir is where one installed version lives.
func versionDir(cache, version string) string { return filepath.Join(cache, "firefox", version) }

// executablePath is the browser binary inside a version's directory.
func executablePath(dir string) string {
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(dir, "Firefox.app", "Contents", "MacOS", "firefox")
	case "windows":
		return filepath.Join(dir, "firefox", "firefox.exe")
	default:
		return filepath.Join(dir, "firefox", "firefox")
	}
}

// Source is the marker written next to an install: what was downloaded and when.
type Source struct {
	Version     string    `json:"version"`
	Channel     Channel   `json:"channel"`
	URL         string    `json:"url"`
	InstalledAt time.Time `json:"installedAt"`
	Verified    string    `json:"verified"` // "sha512+openpgp"
}

const sourceFile = "omnisnap-source.json"

// Installed lists the cached Firefox versions, newest first.
func Installed(cache string) []Source {
	entries, err := os.ReadDir(filepath.Join(cache, "firefox"))
	if err != nil {
		return nil
	}
	var out []Source
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(cache, "firefox", e.Name(), sourceFile))
		if err != nil {
			continue
		}
		var s Source
		if json.Unmarshal(b, &s) != nil || s.Version == "" {
			continue
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return compareVersions(out[i].Version, out[j].Version) > 0 })
	return out
}

// InstalledExecutable returns the newest cached build's binary.
func InstalledExecutable(cache string) (Source, string, bool) {
	list := Installed(cache)
	if len(list) == 0 {
		return Source{}, "", false
	}
	exe := executablePath(versionDir(cache, list[0].Version))
	if _, err := os.Stat(exe); err != nil {
		return Source{}, "", false
	}
	return list[0], exe, true
}

// Uninstall removes every cached Firefox build. It leaves nothing behind
// (SPEC-install.md, criterion 3).
func Uninstall(cache string) error {
	if err := os.RemoveAll(filepath.Join(cache, "firefox")); err != nil {
		return fmt.Errorf("install: uninstall: %w", err)
	}
	if err := os.RemoveAll(downloadsDir(cache)); err != nil {
		return fmt.Errorf("install: uninstall: %w", err)
	}
	return nil
}

// compareVersions orders "160.0.1" after "153.7.0esr"; non-numeric suffixes
// (esr) are ignored. Enough to pick the newest cache entry.
func compareVersions(a, b string) int {
	as, bs := versionNumbers(a), versionNumbers(b)
	for i := 0; i < len(as) || i < len(bs); i++ {
		var x, y int
		if i < len(as) {
			x = as[i]
		}
		if i < len(bs) {
			y = bs[i]
		}
		if x != y {
			if x > y {
				return 1
			}
			return -1
		}
	}
	return 0
}

func versionNumbers(v string) []int {
	var out []int
	for _, part := range strings.FieldsFunc(v, func(r rune) bool { return r < '0' || r > '9' }) {
		n, err := strconv.Atoi(part)
		if err != nil {
			continue
		}
		out = append(out, n)
	}
	return out
}
