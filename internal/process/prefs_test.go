package process

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The prefs are part of what is measured (SPEC-process.md): a change needs
// approval, so the exact file is pinned here.
func TestGeckoPrefsGolden(t *testing.T) {
	want := `// Written by omnisnap: a fresh profile per launch (SPEC-process.md).
user_pref("ui.useOverlayScrollbars", 1);
user_pref("widget.gtk.overlay-scrollbars.enabled", true);
user_pref("layout.css.devPixelsPerPx", "1.0");
user_pref("browser.cache.disk.enable", false);
user_pref("browser.cache.disk.capacity", 0);
user_pref("font.name.serif.x-western", "Liberation Serif");
user_pref("font.name.sans-serif.x-western", "Liberation Sans");
user_pref("font.name.monospace.x-western", "Noto Sans Mono");
user_pref("font.name.serif.x-unicode", "Liberation Serif");
user_pref("font.name.sans-serif.x-unicode", "Liberation Sans");
user_pref("font.name.monospace.x-unicode", "Noto Sans Mono");
`
	if got := string(prefsFile(true)); got != want {
		t.Errorf("prefsFile(true):\n%s\nwant:\n%s", got, want)
	}
	// FONT_PARITY=0 measures the machine as configured: no font prefs.
	noFonts := string(prefsFile(false))
	if strings.Contains(noFonts, "font.name.") {
		t.Errorf("prefsFile(false) carries font prefs:\n%s", noFonts)
	}
	if !strings.Contains(noFonts, `user_pref("layout.css.devPixelsPerPx", "1.0");`) {
		t.Errorf("prefsFile(false) lost the layout prefs:\n%s", noFonts)
	}
}

func TestGeckoArgs(t *testing.T) {
	got := strings.Join(geckoArgs("/tmp/p"), " ")
	want := "--headless --no-remote --remote-debugging-port=0 --profile /tmp/p"
	if got != want {
		t.Errorf("geckoArgs = %q; want %q", got, want)
	}
}

func TestGeckoRootForSnap(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// os.UserHomeDir reads USERPROFILE on Windows, HOME elsewhere.
	t.Setenv("USERPROFILE", home)
	if got, want := geckoRoot("/snap/bin/firefox", ""), filepath.Join(home, "snap", "firefox", "common"); got != want {
		t.Errorf("geckoRoot(snap) = %q; want %q", got, want)
	}
	if _, err := os.Stat(filepath.Join(home, "snap", "firefox", "common")); err != nil {
		t.Errorf("snap profile directory not created: %v", err)
	}
	// A normal Firefox keeps the OS temp dir ("" means os.MkdirTemp's default).
	if got := geckoRoot("/usr/bin/firefox", ""); got != "" {
		t.Errorf("geckoRoot(/usr/bin/firefox) = %q; want the OS temp dir", got)
	}
	// An explicit root always wins.
	if got := geckoRoot("/snap/bin/firefox", "/tmp/root"); got != "/tmp/root" {
		t.Errorf("geckoRoot with root = %q; want /tmp/root", got)
	}
}
