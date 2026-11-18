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
func blinkArgs(profile string) []string { return blinkArgsMode(profile, true) }

// blinkArgsMode is blinkArgs with the window left visible for the branded
// checks: Edge's tracking prevention is off under --headless (SPEC-branded.md),
// so the only faithful way to measure it is a headed browser on a private
// display.
func blinkArgsMode(profile string, headless bool) []string {
	args := []string{}
	if headless {
		args = append(args, "--headless=new")
	}
	args = append(args,
		"--hide-scrollbars",
		"--no-first-run",
		"--no-default-browser-check",
		"--force-device-scale-factor=1",
		"--disk-cache-size=1",
		"--media-cache-size=1",
		"--remote-debugging-port=0",
		"--user-data-dir="+profile,
	)
	// Restricted hosts (CI containers, Ubuntu 24.04+ where AppArmor blocks
	// unprivileged user namespaces, small /dev/shm) crash the browser before
	// DevTools opens. OMNISNAP_NO_SANDBOX=1 is the documented escape hatch: no
	// sandbox, and shared memory in /tmp. It weakens isolation, so the product
	// never sets it itself; only CI does.
	if os.Getenv("OMNISNAP_NO_SANDBOX") == "1" {
		args = append(args, "--no-sandbox", "--disable-dev-shm-usage")
	}
	return args
}

// Blink is a running Blink browser.
type Blink struct {
	*Handle
	WSURL string // DevTools websocket of the browser, for cdp.Dial

	displayClose func(context.Context) error // the private display, when visible
}

// Close stops the browser and, when it had one, its private display.
func (b *Blink) Close(ctx context.Context) error {
	err := b.Handle.Close(ctx)
	if b.displayClose != nil {
		if derr := b.displayClose(ctx); derr != nil {
			err = derr
		}
		b.displayClose = nil
	}
	return err
}

// StartBlink launches exe headless in a fresh profile and waits until its
// DevTools endpoint is up. root is where temp dirs go ("" = the OS temp dir).
func StartBlink(ctx context.Context, exe, root string) (*Blink, error) {
	return startBlink(ctx, exe, root, true)
}

// StartBlinkVisible starts the browser with a window. On Linux that window
// lives on a private Weston display; without weston the call fails rather than
// opening a window on the user's desktop (TEST-only: the branded checks need
// the browser's shipped behaviour).
func StartBlinkVisible(ctx context.Context, exe, root string) (*Blink, error) {
	return startBlink(ctx, exe, root, false)
}

func startBlink(ctx context.Context, exe, root string, headless bool) (*Blink, error) {
	var env, extra []string
	var closeDisplay func(context.Context) error
	if !headless {
		var err error
		env, extra, closeDisplay, err = visibleSetup(ctx)
		if err != nil {
			return nil, err
		}
	}
	h, err := spawnEnv(ctx, root, exe, extra, func(d tempDir) ([]string, []string) {
		return blinkArgsMode(d.profile(), headless), env
	})
	if err != nil {
		if closeDisplay != nil {
			_ = closeDisplay(context.WithoutCancel(ctx))
		}
		return nil, err
	}
	ws, err := h.waitDevTools(ctx)
	if err != nil {
		_ = h.Close(context.WithoutCancel(ctx))
		if closeDisplay != nil {
			_ = closeDisplay(context.WithoutCancel(ctx))
		}
		return nil, err
	}
	return &Blink{Handle: h, WSURL: ws, displayClose: closeDisplay}, nil
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
