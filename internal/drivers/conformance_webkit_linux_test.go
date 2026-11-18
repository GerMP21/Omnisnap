//go:build browsers && linux

package drivers_test

import (
	"context"
	"os"
	"testing"

	"github.com/GerMP21/Omnisnap/internal/drivers"
	"github.com/GerMP21/Omnisnap/internal/drivers/classic"
	"github.com/GerMP21/Omnisnap/internal/process"
)

// webkitHarness finds WebKitGTK (WebKitWebDriver + MiniBrowser). nil when it
// is not installed: conformance then covers the other families.
func webkitHarness(t *testing.T) *harness {
	t.Helper()
	driver := os.Getenv("OMNISNAP_WEBKIT_DRIVER")
	if driver == "" {
		driver = findExecutable("", "WebKitWebDriver")
	}
	browser := os.Getenv("OMNISNAP_WEBKIT_BROWSER")
	if browser == "" {
		for _, p := range []string{
			"/usr/libexec/webkitgtk-6.0/MiniBrowser",
			"/usr/libexec/webkit2gtk-4.1/MiniBrowser",
			"/usr/lib/x86_64-linux-gnu/webkit2gtk-4.1/MiniBrowser",
			"/usr/lib64/webkit2gtk-4.1/MiniBrowser",
		} {
			if _, err := os.Stat(p); err == nil {
				browser = p
				break
			}
		}
	}
	if driver == "" || browser == "" {
		return nil
	}
	wk, err := process.StartWebKit(context.Background(), process.WebKitOptions{Driver: driver, Browser: browser})
	if err != nil {
		t.Fatalf("start WebKit: %v", err)
	}
	return &harness{
		name: "webkit (classic)",
		newSess: func(t *testing.T, ctx context.Context) drivers.Session {
			c := classic.New(wk.URL)
			c.Capabilities = wk.Capabilities
			if err := c.Status(ctx); err != nil {
				t.Fatal(err)
			}
			s, err := c.NewSession(ctx)
			if err != nil {
				t.Fatal(err)
			}
			return s
		},
		close: func() { _ = wk.Close(context.Background()) },
	}
}
