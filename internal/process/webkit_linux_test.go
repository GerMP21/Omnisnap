//go:build linux

package process

import (
	"strings"
	"testing"
)

func TestWebKitFontArgs(t *testing.T) {
	got := strings.Join(webkitFontArgs(false), " ")
	want := "--automation --default-font-family=Liberation Serif --serif-font-family=Liberation Serif " +
		"--sans-serif-font-family=Liberation Sans --monospace-font-family=Noto Sans Mono"
	if got != want {
		t.Errorf("webkitFontArgs =\n  %s\nwant\n  %s", got, want)
	}
	if got := strings.Join(webkitFontArgs(true), " "); got != "--automation" {
		t.Errorf("webkitFontArgs(no parity) = %q", got)
	}
}

// The private display has no desktop session, so GDK must not wait on portals;
// a developer's own GDK_DEBUG flags are kept.
func TestGDKDebugNoPortals(t *testing.T) {
	for _, tc := range []struct{ inherited, want string }{
		{"", "GDK_DEBUG=no-portals"},
		{"misc", "GDK_DEBUG=misc,no-portals"},
		{"events:dnd", "GDK_DEBUG=events:dnd,no-portals"},
	} {
		if got := gdkDebugNoPortals(tc.inherited); got != tc.want {
			t.Errorf("gdkDebugNoPortals(%q) = %q, want %q", tc.inherited, got, tc.want)
		}
	}
}
