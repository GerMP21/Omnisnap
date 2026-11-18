//go:build browsers && linux

package drivers_test

import (
	"context"
	"os"
	"os/exec"
	"testing"

	"github.com/GerMP21/Omnisnap/internal/discovery"
	"github.com/GerMP21/Omnisnap/internal/drivers"
	"github.com/GerMP21/Omnisnap/internal/drivers/classic"
	"github.com/GerMP21/Omnisnap/internal/process"
	"github.com/GerMP21/Omnisnap/internal/schema"
)

// webkitHarness finds WebKitGTK (WebKitWebDriver + MiniBrowser). nil when it
// is not installed: conformance then covers the other families.
func webkitHarness(t *testing.T) *harness {
	t.Helper()
	// The lane needs a private display (weston) to size its window; without one
	// it runs on the user's desktop and the viewport case cannot pass. Skip
	// unless weston is installed (set OMNISNAP_WEBKIT_CONFORMANCE=0 to force).
	if _, err := exec.LookPath("weston"); err != nil || os.Getenv("OMNISNAP_WEBKIT_CONFORMANCE") == "0" {
		return nil
	}
	driver := os.Getenv("OMNISNAP_WEBKIT_DRIVER")
	browser := os.Getenv("OMNISNAP_WEBKIT_BROWSER")
	// Discovery owns the install paths (Arch ships MiniBrowser under
	// /usr/lib/webkitgtk-6.0/, not /usr/libexec/); the env vars win for an
	// unusual setup.
	if driver == "" || browser == "" {
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
	if driver == "" {
		driver = findExecutable("", "WebKitWebDriver")
	}
	if driver == "" || browser == "" {
		return nil
	}
	opts := process.WebKitOptions{Driver: driver, Browser: browser}
	var wk *process.WebKit
	restart := func(t *testing.T) {
		t.Helper()
		if wk != nil {
			_ = wk.Close(context.Background())
		}
		p, err := process.StartWebKit(context.Background(), opts)
		if err != nil {
			t.Fatalf("start WebKit: %v", err)
		}
		wk = p
	}
	restart(t)
	return &harness{
		name: "webkit (classic)",
		newSess: func(t *testing.T, ctx context.Context) drivers.Session {
			c := classic.New(wk.URL)
			c.Capabilities = wk.Capabilities
			if err := c.Status(ctx); err != nil {
				// A hung script leaves the driver executing it, so the session
				// cannot be deleted and the next case finds "A session already
				// exists". A caller restarts the browser; do the same.
				t.Logf("webkit driver not free (%v); restarting it", err)
				restart(t)
				c = classic.New(wk.URL)
				c.Capabilities = wk.Capabilities
				if err := c.Status(ctx); err != nil {
					t.Fatalf("webkit driver after restart: %v", err)
				}
			}
			s, err := c.NewSession(ctx)
			if err != nil {
				// The driver's reply says only "Session terminated without a
				// reply"; its stderr names the cause (a sandbox refusal, a
				// missing library).
				t.Fatalf("%v (driver output: %s)", err, wk.RecentOutput(8))
			}
			return s
		},
		close: func() {
			if wk != nil {
				_ = wk.Close(context.Background())
			}
		},
	}
}
