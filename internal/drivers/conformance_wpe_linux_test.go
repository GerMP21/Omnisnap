//go:build browsers && linux

package drivers_test

import (
	"context"
	"testing"

	"github.com/GerMP21/Omnisnap/internal/drivers"
	"github.com/GerMP21/Omnisnap/internal/drivers/classic"
	"github.com/GerMP21/Omnisnap/internal/process"
)

// wpeHarness drives the WPE port through WPEWebDriver. Unlike the GTK port it
// needs no compositor and MiniBrowser fixes the viewport with --size.
func wpeHarness(t *testing.T) *harness {
	t.Helper()
	driver, browser := process.FindWPE()
	if driver == "" || browser == "" {
		return nil
	}
	opts := process.WPEOptions{Driver: driver, Browser: browser}
	var wk *process.WebKit
	restart := func(t *testing.T) {
		t.Helper()
		if wk != nil {
			_ = wk.Close(context.Background())
		}
		p, err := process.StartWPE(context.Background(), opts)
		if err != nil {
			t.Fatalf("start WPE: %v", err)
		}
		wk = p
	}
	restart(t)
	return &harness{
		name: "wpe (classic)",
		newSess: func(t *testing.T, ctx context.Context) drivers.Session {
			c := classic.New(wk.URL)
			c.Capabilities = wk.Capabilities
			if err := c.Status(ctx); err != nil {
				// Same as the GTK lane: a hung script wedges the driver.
				t.Logf("wpe driver not free (%v); restarting it", err)
				restart(t)
				c = classic.New(wk.URL)
				c.Capabilities = wk.Capabilities
				if err := c.Status(ctx); err != nil {
					t.Fatalf("wpe driver after restart: %v", err)
				}
			}
			s, err := c.NewSession(ctx)
			if err != nil {
				t.Fatal(err)
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
