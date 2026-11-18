//go:build darwin

package process

import (
	"context"
	"fmt"
	"strings"
)

// Safari opens a visible window by design (safaridriver has no headless mode);
// on CI runners that is fine, on a developer Mac it is a window they can see
// (SPEC-process.md, open questions).
//
// safaridriver must be enabled once per machine: `safaridriver --enable`
// (needs admin). Discovery reports "not enabled" as a note, and StartSafari
// returns that instruction instead of a bare timeout (Task 23).

// Safari is a running safaridriver.
type Safari struct {
	*Handle
	URL          string         // http://127.0.0.1:<port>
	Capabilities map[string]any // session capabilities for classic.Client
}

// SafariOptions configures StartSafari.
type SafariOptions struct {
	Driver string // safaridriver, e.g. /usr/bin/safaridriver
	Root   string // "" = OS temp dir
}

// StartSafari starts safaridriver and waits until it answers GET /status.
func StartSafari(ctx context.Context, o SafariOptions) (*Safari, error) {
	port, err := freePort()
	if err != nil {
		return nil, err
	}
	h, err := spawn(ctx, o.Root, o.Driver, []string{fmt.Sprintf("--port=%d", port)}, nil)
	if err != nil {
		return nil, err
	}
	s := &Safari{
		Handle:       h,
		URL:          fmt.Sprintf("http://127.0.0.1:%d", port),
		Capabilities: map[string]any{"alwaysMatch": map[string]any{"browserName": "safari"}},
	}
	if err := waitDriver(ctx, s.URL); err != nil {
		out := h.output()
		_ = s.Close(context.WithoutCancel(ctx))
		if notEnabled(out) {
			return nil, fmt.Errorf("process: safaridriver is not enabled: run `safaridriver --enable` once (needs admin), then retry")
		}
		return nil, err
	}
	return s, nil
}

// notEnabled recognises the driver's refusal to start without the one-time
// enable step.
func notEnabled(output string) bool {
	o := strings.ToLower(output)
	return strings.Contains(o, "not enabled") || strings.Contains(o, "enable safaridriver") ||
		strings.Contains(o, "could not start a session")
}
