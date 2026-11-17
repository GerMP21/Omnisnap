package process

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// blinkArgs are the spike's Blink flags (SPEC-process.md, "Launch
// configuration"): headless, no scrollbars, DPR 1, and no disk cache (a crawl
// would otherwise write hundreds of MB per session, which WSL's virtual disk
// never gives back).
func blinkArgs(profile string) []string {
	return []string{
		"--headless=new",
		"--hide-scrollbars",
		"--no-first-run",
		"--no-default-browser-check",
		"--force-device-scale-factor=1",
		"--disk-cache-size=1",
		"--media-cache-size=1",
		"--remote-debugging-port=0",
		"--user-data-dir=" + profile,
	}
}

// Blink is a running Blink browser.
type Blink struct {
	*Handle
	WSURL string // DevTools websocket of the browser, for cdp.Dial
}

// StartBlink launches exe headless in a fresh profile and waits until its
// DevTools endpoint is up. root is where temp dirs go ("" = the OS temp dir).
func StartBlink(ctx context.Context, exe, root string) (*Blink, error) {
	h, err := spawn(ctx, root, exe, nil, func(d tempDir) []string { return blinkArgs(d.profile()) })
	if err != nil {
		return nil, err
	}
	ws, err := h.waitDevTools(ctx)
	if err != nil {
		_ = h.Close(context.WithoutCancel(ctx))
		return nil, err
	}
	return &Blink{Handle: h, WSURL: ws}, nil
}

// waitDevTools waits for DevToolsActivePort, which the browser writes into
// its profile once the endpoint listens (port 0 lets it pick a free one).
func (h *Handle) waitDevTools(ctx context.Context) (string, error) {
	file := filepath.Join(h.tmp.profile(), "DevToolsActivePort")
	tick := time.NewTicker(25 * time.Millisecond)
	defer tick.Stop()
	for {
		if b, err := os.ReadFile(file); err == nil {
			if ws, err := parseDevToolsActivePort(string(b)); err == nil {
				return ws, nil
			}
		}
		select {
		case <-h.exit:
			return "", fmt.Errorf("process: browser exited before DevTools was ready: %s", lastLines(h.output(), 5))
		case <-ctx.Done():
			return "", fmt.Errorf("process: DevTools not ready: %w", ctx.Err())
		case <-tick.C:
		}
	}
}

// parseDevToolsActivePort reads "<port>\n<browser path>\n".
func parseDevToolsActivePort(s string) (string, error) {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) < 2 {
		return "", fmt.Errorf("process: DevToolsActivePort incomplete")
	}
	port, err := strconv.Atoi(strings.TrimSpace(lines[0]))
	if err != nil || port <= 0 || port > 65535 {
		return "", fmt.Errorf("process: DevToolsActivePort port %q", lines[0])
	}
	p := strings.TrimSpace(lines[1])
	if !strings.HasPrefix(p, "/devtools/browser/") {
		return "", fmt.Errorf("process: DevToolsActivePort path %q", p)
	}
	return fmt.Sprintf("ws://127.0.0.1:%d%s", port, p), nil
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, " | ")
}
