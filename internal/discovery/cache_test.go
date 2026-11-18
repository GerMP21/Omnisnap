package discovery

import (
	"context"
	"path"
	"runtime"
	"testing"
	"testing/fstest"
)

// cacheExe is the browser path the cache source expects inside a version
// directory. It mirrors cacheExecutable so the fixture cannot drift from it
// (the Windows name is firefox.exe, not firefox).
func cacheExe(root, version string) string {
	return (&env{}).cacheExecutable(path.Join(root, "firefox", version))
}

// cacheFixture is the cache root this OS reads, as a fake file tree path.
func cacheFixture() (root string, env map[string]string) {
	switch runtime.GOOS {
	case "windows":
		return "/local/omnisnap/browsers", map[string]string{"LOCALAPPDATA": "/local"}
	case "darwin":
		return "/home/dev/Library/Caches/omnisnap/browsers", map[string]string{"HOME": "/home/dev"}
	default:
		return "/home/dev/.cache/omnisnap/browsers", map[string]string{"HOME": "/home/dev"}
	}
}

// A build installed by `omnisnap install firefox` is discovered from the
// cache, with its version and source (SPEC-install.md, Task 37).
func TestCacheSource(t *testing.T) {
	root, env := cacheFixture()
	fsys := tree(map[string]string{
		cacheExe(root, "999.0"):                               "exe:",
		path.Join(root, "firefox/999.0/omnisnap-source.json"): `{"version":"999.0"}`,
		path.Join(root, "firefox/998.0/firefox/firefox"):      "exe:",
		path.Join(root, "firefox/998.0/not-our-marker.json"):  `{}`,
	})
	got, err := find0(context.Background(), Options{FS: fsys, Getenv: func(k string) string { return env[k] }}, linuxSources())
	if err != nil {
		t.Fatal(err)
	}
	var cached []Browser
	for _, b := range got {
		if b.Source == SourceCache {
			cached = append(cached, b)
		}
	}
	if len(cached) != 1 {
		t.Fatalf("cached browsers %+v; want one (998.0 has no marker)", cached)
	}
	b := cached[0]
	if b.ID != "firefox" || b.Version != "999.0" || b.Family != "gecko" {
		t.Errorf("cached = %+v", b)
	}
	if !hasNote(b, "omnisnap install") {
		t.Errorf("notes %v; want how it got there", b.Notes)
	}
}

// XDG_CACHE_HOME wins over ~/.cache (Linux layout only).
func TestCacheSourceXDG(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("XDG is a Linux/XDG-base-dir convention")
	}
	fsys := tree(map[string]string{
		cacheExe("/cache/omnisnap/browsers", "999.0"):                 "exe:",
		"/cache/omnisnap/browsers/firefox/999.0/omnisnap-source.json": `{"version":"999.0"}`,
	})
	env := map[string]string{"HOME": "/home/dev", "XDG_CACHE_HOME": "/cache"}
	got, err := find0(context.Background(), Options{FS: fsys, Getenv: func(k string) string { return env[k] }}, linuxSources())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, b := range got {
		found = found || (b.Source == SourceCache && b.Version == "999.0")
	}
	if !found {
		t.Error("XDG_CACHE_HOME install not found")
	}
}

var _ = fstest.MapFS{}

// An ESR install is its own browser, not a duplicate of the system Firefox.
func TestCacheSourceESRHasItsOwnID(t *testing.T) {
	root, env := cacheFixture()
	fsys := tree(map[string]string{
		cacheExe(root, "999.0esr"):                               "exe:",
		path.Join(root, "firefox/999.0esr/omnisnap-source.json"): `{"version":"999.0esr"}`,
	})
	got, err := find0(context.Background(), Options{FS: fsys, Getenv: func(k string) string { return env[k] }}, linuxSources())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "firefox-esr" || got[0].Version != "999.0esr" {
		t.Fatalf("got %+v; want one firefox-esr 999.0esr", got)
	}
}
