package discovery

import (
	"context"
	"io/fs"
	"path"
	"runtime"
	"strings"

	"github.com/GerMP21/Omnisnap/internal/schema"
)

// cacheSource: Firefox installed by `omnisnap install firefox`
// (SPEC-install.md). The cache layout is install's; discovery duplicates the
// two path rules rather than importing install (which depends on discovery).

type cacheSource struct{}

// cacheRoot is the browsers cache directory on this OS.
func (e *env) cacheRoot() string {
	switch runtime.GOOS {
	case "windows":
		if root := e.getenv("LOCALAPPDATA"); root != "" {
			return path.Join(strings.ReplaceAll(root, `\`, "/"), "omnisnap", "browsers")
		}
		if home := e.home(); home != "" {
			return path.Join(home, "AppData", "Local", "omnisnap", "browsers")
		}
	case "darwin":
		if home := e.home(); home != "" {
			return path.Join(home, "Library", "Caches", "omnisnap", "browsers")
		}
	default:
		if root := e.getenv("XDG_CACHE_HOME"); root != "" {
			return path.Join(root, "omnisnap", "browsers")
		}
		if home := e.home(); home != "" {
			return path.Join(home, ".cache", "omnisnap", "browsers")
		}
	}
	return ""
}

// cacheExecutable is the browser binary inside a cached version's directory.
func (e *env) cacheExecutable(dir string) string {
	switch runtime.GOOS {
	case "darwin":
		return path.Join(dir, "Firefox.app", "Contents", "MacOS", "firefox")
	case "windows":
		return path.Join(dir, "firefox", "firefox.exe")
	default:
		return path.Join(dir, "firefox", "firefox")
	}
}

func (cacheSource) scan(_ context.Context, e *env) []Browser {
	root := e.cacheRoot()
	if root == "" {
		return nil
	}
	entries, err := fs.ReadDir(e.fs, rel(path.Join(root, "firefox")))
	if err != nil {
		return nil
	}
	var out []Browser
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		version := entry.Name()
		dir := path.Join(root, "firefox", version)
		if _, ok := e.read(path.Join(dir, "omnisnap-source.json")); !ok {
			continue // not an install of ours
		}
		exe := e.cacheExecutable(dir)
		// Windows has no execute bit: Go reports firefox.exe as 0666.
		runs := e.executable
		if runtime.GOOS == "windows" {
			runs = e.executableWindows
		}
		if !runs(exe) {
			continue
		}
		// An ESR install keeps its own id (SPEC-discovery lists firefox-esr):
		// sharing "firefox" with the system build would hide the copy the user
		// asked for, because discovery keeps one browser per id.
		id := "firefox"
		if strings.HasSuffix(strings.ToLower(version), "esr") {
			id = "firefox-esr"
		}
		out = append(out, Browser{
			ID: id, Name: "Firefox (installed by omnisnap)", Family: schema.FamilyGecko,
			Protocol: ProtocolBiDi, Executable: exe, Version: version, Source: SourceCache,
			Notes: []string{"installed by omnisnap install: remove it with omnisnap uninstall firefox"},
		})
	}
	return out
}
