//go:build e2e && linux

package e2e

import (
	"context"
	"testing"

	"github.com/GerMP21/Omnisnap/internal/drivers"
	"github.com/GerMP21/Omnisnap/internal/drivers/classic"
	"github.com/GerMP21/Omnisnap/internal/process"
)

// webkitGoldenEngine is the WebKitGTK engine for the golden-geometry gate:
// the WebKit goldens come from MiniBrowser, which runs only on Linux.
func webkitGoldenEngine(driver, browser string) (engine, bool) {
	if driver == "" || browser == "" {
		return engine{}, false
	}
	return engine{golden: "webkit", id: "webkit", start: func(t *testing.T) (drivers.Session, func()) {
		p, err := process.StartWebKit(context.Background(), process.WebKitOptions{Driver: driver, Browser: browser})
		if err != nil {
			t.Fatal(err)
		}
		c := classic.New(p.URL)
		c.Capabilities = p.Capabilities
		if err := c.Status(context.Background()); err != nil {
			t.Fatal(err)
		}
		s, err := c.NewSession(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		return s, func() { _ = p.Close(context.Background()) }
	}}, true
}
