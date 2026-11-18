package process

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// geckoArgs are the spike's Gecko flags (SPEC-process.md): headless, no
// remote-control conflicts with a running Firefox, BiDi on a free port (0),
// and the fresh profile whose user.js carries prefsFile.
func geckoArgs(profile string) []string {
	return []string{
		"--headless",
		"--no-remote",
		"--remote-debugging-port=0",
		"--profile", profile,
	}
}

// Gecko is a running Firefox.
type Gecko struct {
	*Handle
	WSURL string // WebDriver BiDi endpoint, for bidi.Dial
}

// StartGecko launches Firefox headless in a fresh profile with the spike's
// prefs (font parity on) and waits until its BiDi endpoint is up. root is where
// temp dirs go ("" = the OS temp dir, or the snap profile directory).
func StartGecko(ctx context.Context, exe, root string) (*Gecko, error) {
	return StartGeckoOptions(ctx, GeckoOptions{Exe: exe, Root: root})
}

// GeckoOptions configures StartGecko. The zero value is the shipped default:
// font parity on.
type GeckoOptions struct {
	Exe  string
	Root string // "" = OS temp dir, or ~/snap/firefox/common for a snap
	// NoFontParity disables the font-parity prefs (FONT_PARITY=0 in the
	// spike: measure the machine as configured).
	NoFontParity bool
}

// StartGeckoOptions launches Firefox with explicit options.
func StartGeckoOptions(ctx context.Context, o GeckoOptions) (*Gecko, error) {
	root := geckoRoot(o.Exe, o.Root)
	h, err := spawn(ctx, root, o.Exe, nil, argsForGecko(o.NoFontParity))
	if err != nil {
		return nil, err
	}
	ws, err := h.waitBiDi(ctx)
	if err != nil {
		_ = h.Close(context.WithoutCancel(ctx))
		return nil, err
	}
	return &Gecko{Handle: h, WSURL: ws}, nil
}

// argsForGecko writes user.js into the fresh profile before Firefox starts.
func argsForGecko(noFontParity bool) func(tempDir) []string {
	return func(d tempDir) []string {
		// Best effort: a prefs failure surfaces as a missing pref, which the
		// capture's font check reports. A hard error here would be better, but
		// argsFor cannot fail; write and let StartGecko's readiness timeout
		// report it if Firefox refuses the profile.
		_ = os.WriteFile(filepath.Join(d.profile(), "user.js"), prefsFile(!noFontParity), 0o600)
		return geckoArgs(d.profile())
	}
}

// bidiEndpoint matches the line Firefox prints on stderr once BiDi listens:
// "WebDriver BiDi listening on ws://127.0.0.1:PORT".
var bidiEndpoint = regexp.MustCompile(`WebDriver BiDi listening on (ws://\S+)`)

// waitBiDi waits for the BiDi endpoint in the browser's output.
func (h *Handle) waitBiDi(ctx context.Context) (string, error) {
	tick := time.NewTicker(25 * time.Millisecond)
	defer tick.Stop()
	for {
		if m := bidiEndpoint.FindStringSubmatch(h.output()); m != nil {
			return m[1], nil
		}
		select {
		case <-h.exit:
			return "", fmt.Errorf("process: Firefox exited before BiDi was ready: %s", lastLines(h.output(), 5))
		case <-ctx.Done():
			return "", fmt.Errorf("process: BiDi not ready: %w", ctx.Err())
		case <-tick.C:
		}
	}
}

// snapFirefoxPrefix is where a snap Firefox may live; its /tmp is private to
// the snap, so a profile under the OS temp dir would be invisible to it.
const snapFirefoxPrefix = "/snap/"

// geckoRoot picks the directory for the profile. A snap Firefox gets
// ~/snap/firefox/common (SPEC-process.md, "Profiles"); everything else the
// caller's root or the OS temp dir.
func geckoRoot(exe, root string) string {
	if root != "" {
		return root
	}
	// /usr/bin/firefox is often a symlink to /snap/bin/firefox.
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	if !strings.HasPrefix(exe, snapFirefoxPrefix) && !strings.Contains(exe, "/snap/firefox/") {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "" // no home: the OS temp dir is the only option
	}
	dir := filepath.Join(home, "snap", "firefox", "common")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return ""
	}
	return dir
}
