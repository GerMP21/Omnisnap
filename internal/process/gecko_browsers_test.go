//go:build browsers

package process

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// firefoxExecutable finds Firefox: $OMNISNAP_FIREFOX, else the first on PATH.
func firefoxExecutable(t *testing.T) string {
	t.Helper()
	if p := os.Getenv("OMNISNAP_FIREFOX"); p != "" {
		return p
	}
	for _, name := range []string{"firefox", "firefox-esr"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	t.Skip("no Firefox found: set OMNISNAP_FIREFOX")
	return ""
}

// TestGeckoLaunch starts the real Firefox with the spike's profile and checks
// the profile carried the prefs and was removed on Close.
func TestGeckoLaunch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	g, err := StartGecko(ctx, firefoxExecutable(t), "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(g.WSURL, "ws://") {
		t.Errorf("WSURL = %q", g.WSURL)
	}
	profile := g.tmp.profile()
	b, err := os.ReadFile(filepath.Join(profile, "user.js"))
	if err != nil {
		t.Fatalf("user.js: %v", err)
	}
	if string(b) != string(prefsFile(true)) {
		t.Errorf("profile user.js differs from prefsFile(true):\n%s", b)
	}
	dir := g.Dir()
	if err := g.Close(ctx); err != nil {
		t.Errorf("close: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("temp dir %s survived Close", dir)
	}
}
