//go:build browsers && darwin

package drivers_test

import (
	"context"
	"os"
	"testing"

	"github.com/GerMP21/Omnisnap/internal/drivers"
	"github.com/GerMP21/Omnisnap/internal/drivers/classic"
	"github.com/GerMP21/Omnisnap/internal/process"
)

// webkitHarness drives Safari through safaridriver (Task 23). The one-time
// `safaridriver --enable` is the CI job's step; without it StartSafari names
// the fix, and the suite skips instead of blaming the driver.
func webkitHarness(t *testing.T) *harness {
	t.Helper()
	driver := os.Getenv("OMNISNAP_SAFARIDRIVER")
	if driver == "" {
		driver = findExecutable("", "safaridriver")
	}
	if driver == "" {
		return nil
	}
	saf, err := process.StartSafari(context.Background(), process.SafariOptions{Driver: driver})
	if err != nil {
		t.Skipf("Safari not usable here: %v", err)
	}
	return &harness{
		name: "safari (classic)",
		newSess: func(t *testing.T, ctx context.Context) drivers.Session {
			c := classic.New(saf.URL)
			c.Capabilities = saf.Capabilities
			if err := c.Status(ctx); err != nil {
				t.Fatal(err)
			}
			s, err := c.NewSession(ctx)
			if err != nil {
				t.Fatal(err)
			}
			return s
		},
		close: func() { _ = saf.Close(context.Background()) },
	}
}
