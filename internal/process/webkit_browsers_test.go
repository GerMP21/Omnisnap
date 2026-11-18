//go:build linux && browsers

package process

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/GerMP21/Omnisnap/internal/discovery"
	"github.com/GerMP21/Omnisnap/internal/schema"
)

// webkitBinaries finds WebKitWebDriver and MiniBrowser on this machine.
func webkitBinaries(t *testing.T) (driver, browser string) {
	t.Helper()
	driver = os.Getenv("OMNISNAP_WEBKIT_DRIVER")
	if driver == "" {
		for _, name := range []string{"WebKitWebDriver"} {
			if p, err := exec.LookPath(name); err == nil {
				driver = p
			}
		}
	}
	browser = os.Getenv("OMNISNAP_WEBKIT_BROWSER")
	if browser == "" {
		// Discovery owns the install paths (Arch: /usr/lib/webkitgtk-6.0/).
		if found, err := discovery.Find(context.Background(), discovery.Options{}); err == nil {
			for _, b := range found {
				if b.Family != schema.FamilyWebKit {
					continue
				}
				if driver == "" {
					driver = b.Driver
				}
				if browser == "" {
					browser = b.Executable
				}
				break
			}
		}
	}
	if driver == "" || browser == "" {
		t.Skip("WebKitGTK not installed: set OMNISNAP_WEBKIT_DRIVER and OMNISNAP_WEBKIT_BROWSER")
	}
	return driver, browser
}

// TestWebKitLaunch starts the real driver under a private Weston display (when
// installed) and checks it answers, then leaves nothing behind.
func TestWebKitLaunch(t *testing.T) {
	driver, browser := webkitBinaries(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	w, err := StartWebKit(ctx, WebKitOptions{Driver: driver, Browser: browser})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(w.URL, "http://127.0.0.1:") {
		t.Errorf("URL = %q", w.URL)
	}
	if w.Visible() && os.Getenv("WEBKIT_HEADLESS") != "0" {
		t.Logf("weston not installed: windows are visible (SPEC-process.md)")
	}
	dir := w.Dir()
	if err := w.Close(ctx); err != nil {
		t.Errorf("close: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("temp dir %s survived Close", dir)
	}
}
