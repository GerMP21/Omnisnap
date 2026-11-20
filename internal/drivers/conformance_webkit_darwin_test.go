//go:build browsers && darwin

package drivers_test

import (
	"context"
	"os"
	"testing"
	"time"

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
	opts := process.SafariOptions{Driver: driver}
	saf, err := process.StartSafari(context.Background(), opts)
	if err != nil {
		t.Skipf("Safari not usable here: %v", err)
	}
	restart := func(t *testing.T) {
		t.Helper()
		_ = saf.Close(context.Background())
		p, err := process.StartSafari(context.Background(), opts)
		if err != nil {
			t.Fatalf("restart safaridriver: %v", err)
		}
		saf = p
	}
	open := func(ctx context.Context) (drivers.Session, error) {
		c := classic.New(saf.URL)
		c.Capabilities = saf.Capabilities
		if err := c.Status(ctx); err != nil {
			return nil, err
		}
		return c.NewSession(ctx)
	}
	// Safari's first session launches the browser cold. On a hosted macOS
	// runner that has taken more than a subtest's 30 s: in CI run 44687496792
	// the first POST /session timed out, and the retry below then ran on the
	// same, already expired context and failed at once. Pay the cold start
	// here, on its own budget, so every subtest starts against a warm Safari.
	warm, cancel := context.WithTimeout(context.Background(), safariWarmUp)
	s, err := open(warm)
	cancel()
	if err != nil {
		t.Logf("safari warm-up: %v; restarting safaridriver", err)
		restart(t)
	} else {
		closeSession(t, s)
	}
	return &harness{
		name: "safari (classic)",
		newSess: func(t *testing.T, ctx context.Context) drivers.Session {
			s, err := open(ctx)
			if err != nil {
				// Safari attaches one session at a time, and a session wedged
				// by a hung script (typed errors: the driver runs it until the
				// 60 s script timeout) cannot be deleted, so the next session
				// is refused ("already paired"). closeSession leaves the
				// restart to the harness; do what the WebKitGTK harness does.
				// Source: https://developer.apple.com/documentation/webkit/about-webdriver-for-safari
				t.Logf("safari not free (%v); restarting safaridriver", err)
				restart(t)
				if s, err = open(ctx); err != nil {
					t.Fatalf("safari after restart: %v", err)
				}
			}
			return s
		},
		close: func() { _ = saf.Close(context.Background()) },
	}
}

// safariWarmUp bounds the first session, which pays for launching Safari.
const safariWarmUp = 90 * time.Second
