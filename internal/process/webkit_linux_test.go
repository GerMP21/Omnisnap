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
