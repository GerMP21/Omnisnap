package discovery

import (
	"context"
	"testing"
	"testing/fstest"
)

// A build installed by `omnisnap install firefox` is discovered from the
// cache, with its version and source (SPEC-install.md, Task 37).
func TestCacheSource(t *testing.T) {
	fsys := tree(map[string]string{
		"/home/dev/.cache/omnisnap/browsers/firefox/999.0/firefox/firefox":      "exe:",
		"/home/dev/.cache/omnisnap/browsers/firefox/999.0/omnisnap-source.json": `{"version":"999.0"}`,
		"/home/dev/.cache/omnisnap/browsers/firefox/998.0/firefox/firefox":      "exe:",
		"/home/dev/.cache/omnisnap/browsers/firefox/998.0/not-our-marker.json":  `{}`,
	})
	env := map[string]string{"HOME": "/home/dev"}
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

// XDG_CACHE_HOME wins over ~/.cache.
func TestCacheSourceXDG(t *testing.T) {
	fsys := tree(map[string]string{
		"/cache/omnisnap/browsers/firefox/999.0/firefox/firefox":      "exe:",
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
