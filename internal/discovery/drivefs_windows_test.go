//go:build windows

package discovery

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// With the default file system, discovery reads real Windows paths, which all
// start with a drive letter: the cache install under LOCALAPPDATA and a
// portable browser on PATH. os.DirFS("/") rejects every name with a ':' on
// Windows, so before driveFS neither source found anything on a real machine
// (PR #20, "install ESR (windows-latest)", run 45851219533).
func TestFindReadsDriveLetterPaths(t *testing.T) {
	dir := t.TempDir() // C:\...\Temp\...
	write := func(rel, data string) {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("local/omnisnap/browsers/firefox/999.0/firefox/firefox.exe", "MZ")
	write("local/omnisnap/browsers/firefox/999.0/omnisnap-source.json", `{"version":"999.0"}`)
	write("portable/chrome.exe", "MZ")
	env := map[string]string{
		"LOCALAPPDATA": filepath.Join(dir, "local"),
		"PATH":         filepath.Join(dir, "portable"),
	}
	got, err := find0(context.Background(), Options{
		Getenv:      func(k string) string { return env[k] },
		Registry:    func(string) (string, bool) { return "", false },
		FileVersion: func(string) string { return "" },
	}, windowsSources())
	if err != nil {
		t.Fatal(err)
	}
	bySource := map[Source]Browser{}
	for _, b := range got {
		bySource[b.Source] = b
	}
	if b, ok := bySource[SourceCache]; !ok || b.Version != "999.0" {
		t.Errorf("cache install not found: %+v", got)
	} else if _, err := os.Stat(b.Executable); err != nil {
		t.Errorf("cache executable %q: %v", b.Executable, err)
	}
	if b, ok := bySource[SourcePath]; !ok || b.ID != "chrome" {
		t.Errorf("portable chrome.exe on PATH not found: %+v", got)
	}
}

func TestDriveFSLocal(t *testing.T) {
	for name, want := range map[string]string{
		"C:/Program Files/Mozilla Firefox/firefox.exe": `C:\Program Files\Mozilla Firefox\firefox.exe`,
		"d:/x":                  `d:\x`,
		"C:":                    `C:\`,
		"Program Files/foo.exe": `\Program Files\foo.exe`,
		".":                     `\`,
		"C:/a:stream":           "", // an alternate data stream
		"C:/a\\b":               "",
		"C:/../x":               "",
		"/C:/x":                 "",
	} {
		got, err := driveFS{}.local("open", name)
		if want == "" {
			if err == nil {
				t.Errorf("%q: %q; want an error", name, got)
			}
			continue
		}
		if err != nil || got != want {
			t.Errorf("%q: %q, %v; want %q", name, got, err, want)
		}
	}
}
