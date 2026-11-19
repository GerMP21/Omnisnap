//go:build linux

package process

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// WebKitGTK has no headless mode of its own. It gets a private, invisible
// Wayland display (Weston's headless backend) instead of the user's desktop;
// the bonus is that we choose the screen size, so the window can always reach
// 1280x800 (SPEC-process.md; spike display_linux in capture.mjs).
//
// WEBKIT_HEADLESS=0 shows the windows instead, for debugging.
const headlessEnv = "WEBKIT_HEADLESS"

// sandboxDisabled reports the CI-only escape hatch shared with Blink
// (OMNISNAP_NO_SANDBOX=1). Ubuntu 24.04+ denies unprivileged user namespaces,
// and WebKit's bubblewrap sandbox needs one: the browser dies at startup with
// "bwrap: Creating new namespace failed", and the driver reports the session
// terminated without a reply.
func sandboxDisabled() bool { return os.Getenv("OMNISNAP_NO_SANDBOX") == "1" }

// Display is a private Weston headless display.
type Display struct {
	h       *Handle
	socket  string // WAYLAND_DISPLAY
	runtime string // XDG_RUNTIME_DIR
}

// WaylandDisplay is the socket name to put in WAYLAND_DISPLAY.
func (d *Display) WaylandDisplay() string { return d.socket }

// XDGRuntimeDir is the directory holding the socket.
func (d *Display) XDGRuntimeDir() string { return d.runtime }

// Close stops Weston and deletes its temp dir; idempotent.
func (d *Display) Close(ctx context.Context) error { return d.h.Close(ctx) }

// StartDisplay starts a private Weston headless display. A missing weston, or
// WEBKIT_HEADLESS=0, is not an error: nil means "use the visible desktop".
func StartDisplay(ctx context.Context) (*Display, error) {
	if os.Getenv(headlessEnv) == "0" {
		return nil, nil
	}
	weston, err := exec.LookPath("weston")
	if err != nil {
		return nil, nil // no weston: windows will be visible
	}
	runtime := os.Getenv("XDG_RUNTIME_DIR")
	if runtime == "" {
		runtime = fmt.Sprintf("/run/user/%d", os.Getuid())
	}
	socket := fmt.Sprintf("omnisnap-%d-%d", os.Getpid(), time.Now().UnixMilli())
	// A temp dir for Weston's own state; it is deleted with the process. The
	// socket goes in runtime, where Wayland expects it.
	h, err := spawnEnv(ctx, "", weston, nil, func(tempDir) ([]string, []string) {
		return []string{
			"--backend=headless", "--socket=" + socket,
			"--width=1920", "--height=1200", "--idle-time=0",
		}, []string{"XDG_RUNTIME_DIR=" + runtime}
	})
	if err != nil {
		return nil, err
	}
	sock := filepath.Join(runtime, socket)
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		if _, err := os.Stat(sock); err == nil {
			return &Display{h: h, socket: socket, runtime: runtime}, nil
		}
		select {
		case <-h.exit:
			_ = h.Close(context.WithoutCancel(ctx))
			return nil, fmt.Errorf("process: weston exited: %s", lastLines(h.output(), 5))
		case <-ctx.Done():
			_ = h.Close(context.WithoutCancel(ctx))
			return nil, fmt.Errorf("process: weston headless did not start: %w", ctx.Err())
		case <-tick.C:
		}
	}
}

// webkitFontArgs point WebKitGTK's generic families at Blink's fonts (the
// spike's PARITY_FONTS), so the same machine measures the same fonts in every
// engine.
func webkitFontArgs(noFontParity bool) []string {
	if noFontParity {
		return []string{"--automation"}
	}
	return []string{
		"--automation",
		"--default-font-family=Liberation Serif",
		"--serif-font-family=Liberation Serif",
		"--sans-serif-font-family=Liberation Sans",
		"--monospace-font-family=Noto Sans Mono",
	}
}

// WebKit is a running WebKitWebDriver (and, when available, its private
// display). It speaks WebDriver Classic: build a classic.Client for its URL.
type WebKit struct {
	*Handle
	URL          string         // http://127.0.0.1:<port>
	Capabilities map[string]any // session capabilities for classic.Client

	display *Display
	visible bool // no private display: windows appear on the desktop
}

// Visible reports whether the MiniBrowser windows are shown (no weston).
func (w *WebKit) Visible() bool { return w.visible }

// WebKitOptions configures StartWebKit.
type WebKitOptions struct {
	Driver  string   // WebKitWebDriver
	Browser string   // MiniBrowser
	Args    []string // extra MiniBrowser args after the font args
	Root    string   // "" = OS temp dir
	// NoFontParity disables the font-parity args (WEBKIT_FONT_PARITY=0).
	NoFontParity bool
}

// StartWebKit starts a private headless display (when weston is installed) and
// WebKitWebDriver, and waits until the driver answers GET /status.
func StartWebKit(ctx context.Context, o WebKitOptions) (*WebKit, error) {
	// The WPE port is a WebKit engine with a different driver, capabilities and
	// launch flags; callers pass the paths discovery found, so the choice is
	// made here.
	if strings.Contains(o.Browser, "wpe-webkit") || strings.HasSuffix(o.Driver, "WPEWebDriver") {
		return StartWPE(ctx, WPEOptions{Driver: o.Driver, Browser: o.Browser, Root: o.Root,
			Args: o.Args, NoFontParity: o.NoFontParity})
	}
	display, err := StartDisplay(ctx)
	if err != nil {
		return nil, err
	}
	port, err := freePort()
	if err != nil {
		if display != nil {
			_ = display.Close(context.WithoutCancel(ctx))
		}
		return nil, err
	}
	args := append(webkitFontArgs(o.NoFontParity), o.Args...)
	browserOptions := map[string]any{"binary": o.Browser, "args": args}
	env := []string{}
	if sandboxDisabled() {
		env = append(env, "WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS=1")
	}
	if display != nil {
		env = append(env, []string{
			"WAYLAND_DISPLAY=" + display.WaylandDisplay(),
			"XDG_RUNTIME_DIR=" + display.XDGRuntimeDir(),
			"GDK_BACKEND=wayland",
			"DISPLAY=", // never fall back to the user's X/WSLg desktop
			// Headless Weston has no GPU: GTK4's default (Vulkan/GL) renderer
			// hangs or dumps core before the first frame. Cairo is the
			// software fallback, and WebKit's DMA-BUF renderer must be off too.
			"GSK_RENDERER=cairo",
			"WEBKIT_DISABLE_DMABUF_RENDERER=1",
			// The private display has no desktop session behind it, so nothing
			// can answer a portal: on CI runners the session bus activates
			// xdg-desktop-portal, which waits for a graphical session that never
			// starts, and every GDK portal query (Settings, the host app-id
			// Registry) blocks for D-Bus's 25 s default. A few of those per
			// MiniBrowser outlast POST /session. no-portals makes GDK skip them
			// before touching the bus (gdk_display_should_use_portal).
			// Source: https://docs.gtk.org/gtk4/running.html#gdk_debug
			gdkDebugNoPortals(os.Getenv("GDK_DEBUG")),
		}...)
	}
	h, err := spawnEnv(ctx, o.Root, o.Driver, []string{fmt.Sprintf("--port=%d", port)}, func(tempDir) ([]string, []string) {
		return nil, env
	})
	if err != nil {
		if display != nil {
			_ = display.Close(context.WithoutCancel(ctx))
		}
		return nil, err
	}
	w := &WebKit{
		Handle: h,
		URL:    fmt.Sprintf("http://127.0.0.1:%d", port),
		Capabilities: map[string]any{
			"alwaysMatch": map[string]any{
				"browserName":              "MiniBrowser",
				"webkitgtk:browserOptions": browserOptions,
			},
		},
		display: display,
		visible: display == nil,
	}
	if err := waitDriver(ctx, w.URL); err != nil {
		_ = w.Close(context.WithoutCancel(ctx))
		return nil, err
	}
	return w, nil
}

// gdkDebugNoPortals adds no-portals to the caller's GDK_DEBUG instead of
// replacing it, so a developer's own debug flags still reach MiniBrowser. GDK
// splits the list on any of ":;, \t" (gdk_parse_debug_var).
func gdkDebugNoPortals(inherited string) string {
	if inherited == "" {
		return "GDK_DEBUG=no-portals"
	}
	return "GDK_DEBUG=" + inherited + ",no-portals"
}

// RecentOutput is the driver's stderr tail. When a session dies before the
// browser answers (a sandbox refusal, a missing library), this carries the
// reason and the driver's reply does not.
func (w *WebKit) RecentOutput(n int) string { return lastLines(w.output(), n) }

// Close stops the driver, every browser it spawned (same process group) and
// the private display.
func (w *WebKit) Close(ctx context.Context) error {
	err := w.Handle.Close(ctx)
	if w.display != nil {
		if derr := w.display.Close(ctx); derr != nil {
			err = derr
		}
		w.display = nil
	}
	return err
}
