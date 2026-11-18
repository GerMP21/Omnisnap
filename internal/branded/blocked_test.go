package branded

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/GerMP21/Omnisnap/internal/schema"
)

func TestBlockedHosts(t *testing.T) {
	failed := []schema.FailedRequest{
		{URL: "https://ads.example.net/tag.js", Err: "net::ERR_BLOCKED_BY_CLIENT"},
		{URL: "https://sub.ads.example.net/x", Err: "net::ERR_BLOCKED_BY_CLIENT"},
		{URL: "https://ads.example.net/tag.js", Err: "net::ERR_BLOCKED_BY_CLIENT"},
		{URL: "https://example.com/api", Err: "net::ERR_EMPTY_RESPONSE"},
	}
	got := BlockedHosts(failed)
	if len(got) != 2 || got[0] != "ads.example.net" || got[1] != "sub.ads.example.net" {
		t.Errorf("BlockedHosts = %v", got)
	}
	if len(BlockedHosts(nil)) != 0 {
		t.Error("no failures produced hosts")
	}
}

func TestHostOf(t *testing.T) {
	for in, want := range map[string]string{
		"https://cdn.example.com/a.png": "cdn.example.com",
		"//cdn.example.com/a.png":       "cdn.example.com",
		"cdn.example.com":               "cdn.example.com",
		"www.example.com":               "example.com",
		"/local/path.png":               "",
		"image.png":                     "",
	} {
		if got := HostOf(in); got != want {
			t.Errorf("HostOf(%q) = %q; want %q", in, got, want)
		}
	}
}

// Task 66: a cause's resources are linked to the host the browser blocked,
// through subdomains.
func TestMatches(t *testing.T) {
	blocked := []string{"ads.example.net", "tracker.io"}
	if host, ok := Matches("sub.ads.example.net", blocked); !ok || host != "ads.example.net" {
		t.Errorf("subdomain: %q, %v", host, ok)
	}
	if host, ok := Matches("cdn.tracker.io", blocked); !ok || host != "tracker.io" {
		t.Errorf("subdomain: %q, %v", host, ok)
	}
	if _, ok := Matches("example.net", blocked); ok {
		t.Error("a different host matched")
	}
	if _, ok := Matches("notads.example.net", blocked); ok {
		t.Error("a suffix without a dot matched")
	}
}

func TestTemplateFreshness(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	cache := t.TempDir()
	dir := TemplateDir(cache, "brave", "157.0.0")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	old := now.Add(-24 * time.Hour)
	if err := os.Chtimes(dir, old, old); err != nil {
		t.Fatal(err)
	}
	got, err := Find(cache, "brave", "157.0.8103.78", now)
	if err != nil || got == nil || got.Dir != dir {
		t.Fatalf("Find = %+v, %v; want the fresh template", got, err)
	}
	// A major version change invalidates it.
	if got, _ := Find(cache, "brave", "158.0.0", now); got != nil {
		t.Errorf("template fresh across a major version: %+v", got)
	}
	// Older than 7 days invalidates it.
	if err := os.Chtimes(dir, now.Add(-8*24*time.Hour), now.Add(-8*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if got, _ := Find(cache, "brave", "157.0.8103.78", now); got != nil {
		t.Errorf("8-day-old template still fresh: %+v", got)
	}
	if stale, _ := Stale(cache, "brave", "157.0.8103.78", now); len(stale) != 1 {
		t.Errorf("Stale = %v; want the old template", stale)
	}
	// No cache: no error, no template.
	if got, err := Find(filepath.Join(cache, "missing"), "brave", "157", now); got != nil || err != nil {
		t.Errorf("Find(missing cache) = %v, %v", got, err)
	}
}
