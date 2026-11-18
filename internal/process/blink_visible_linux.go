//go:build linux

package process

import (
	"context"
	"errors"
)

// visibleSetup gives a headed browser a private Weston display, so the
// branded checks never open a window on the user's desktop. Chromium must be
// told to use Wayland explicitly: DISPLAY is empty.
func visibleSetup(ctx context.Context) (env, args []string, closeFn func(context.Context) error, err error) {
	d, err := StartDisplay(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	if d == nil {
		return nil, nil, nil, errors.New("process: a visible browser needs weston; install it or set WEBKIT_HEADLESS=0 to use the desktop")
	}
	return []string{
			"WAYLAND_DISPLAY=" + d.WaylandDisplay(),
			"XDG_RUNTIME_DIR=" + d.XDGRuntimeDir(),
			"GDK_BACKEND=wayland",
			"DISPLAY=",
		}, []string{"--ozone-platform=wayland"},
		func(c context.Context) error { return d.Close(c) }, nil
}
