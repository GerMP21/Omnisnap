//go:build network

package install

import (
	"context"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
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
	regBefore := registrySnapshot(t)
	res, err := Firefox(ctx, Options{Channel: ESR, Cache: cache})
	if err != nil {
		t.Fatal(err)
	}
	// The install writes no registry key (Windows; checked before the launch
	// below, since Firefox itself writes keys when it starts).
	if regAfter := registrySnapshot(t); regAfter != regBefore {
		t.Errorf("the install changed the Mozilla or Uninstall registry keys:\nbefore:\n%s\nafter:\n%s", regBefore, regAfter)
	} else if regBefore != "" {
		t.Logf("registry: the Mozilla and Uninstall keys are unchanged by the install")
	}
	t.Logf("installed Firefox %s from %s", res.Version, res.URL)
	if _, err := os.Stat(res.Executable); err != nil {
		t.Fatal(err)
	}
	// The unpacked size against the download, to check diskFactor against
	// Mozilla's real builds.
	if dl := remoteSize(ctx, http.DefaultClient, res.URL); dl > 0 {
		unpacked := treeSize(t, res.Dir)
		t.Logf("download %s, unpacked %s: %.2fx (disk factor on %s: %dx)",
			HumanBytes(dl), HumanBytes(unpacked), float64(unpacked)/float64(dl), runtime.GOOS, diskFactor(runtime.GOOS))
	}
	// Discovery finds it through the cache source, pointed at the test's
	// cache: XDG_CACHE_HOME on Linux, LOCALAPPDATA on Windows.
	browsers, err := discovery.Find(ctx, discovery.Options{Getenv: func(k string) string {
		if k == "XDG_CACHE_HOME" || k == "LOCALAPPDATA" {
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

// treeSize is the total size of the files under dir.
func treeSize(t *testing.T, dir string) int64 {
	t.Helper()
	var n int64
	err := filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		n += info.Size()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}
