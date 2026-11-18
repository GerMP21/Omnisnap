//go:build network

package install

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GerMP21/Omnisnap/internal/discovery"
	"github.com/GerMP21/Omnisnap/internal/drivers/bidi"
	"github.com/GerMP21/Omnisnap/internal/process"
)

// Task 37 / SPEC-install.md: the weekly network job installs Mozilla's real
// ESR, discovers it, launches it, and uninstalls it leaving nothing.
func TestInstallESRFromMozilla(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	parent := t.TempDir()
	cache := filepath.Join(parent, "omnisnap", "browsers")
	res, err := Firefox(ctx, Options{Channel: ESR, Cache: cache})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("installed Firefox %s from %s", res.Version, res.URL)
	if _, err := os.Stat(res.Executable); err != nil {
		t.Fatal(err)
	}
	// Discovery finds it through the cache source.
	browsers, err := discovery.Find(ctx, discovery.Options{Getenv: func(k string) string {
		if k == "XDG_CACHE_HOME" {
			return parent
		}
		return os.Getenv(k)
	}})
	if err != nil {
		t.Fatal(err)
	}
	var found *discovery.Browser
	for i := range browsers {
		if browsers[i].Source == discovery.SourceCache {
			found = &browsers[i]
		}
	}
	if found == nil || found.Version != res.Version {
		t.Fatalf("cache install not discovered: %+v", browsers)
	}
	// It runs: launch, evaluate, close.
	g, err := process.StartGecko(ctx, found.Executable, "")
	if err != nil {
		t.Fatal(err)
	}
	br, err := bidi.Dial(ctx, g.WSURL)
	if err != nil {
		t.Fatal(err)
	}
	s, err := br.NewSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := s.Evaluate(ctx, "1 + 1"); err != nil || got != "2" {
		t.Errorf("evaluate = %q, %v", got, err)
	}
	_ = s.Close(ctx)
	_ = br.Close(ctx)
	if err := g.Close(ctx); err != nil {
		t.Error(err)
	}
	if !strings.Contains(found.Executable, "firefox") {
		t.Errorf("executable %q", found.Executable)
	}
	if err := Uninstall(cache); err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(cache); len(entries) != 0 {
		t.Errorf("%d entries left after uninstall", len(entries))
	}
}
