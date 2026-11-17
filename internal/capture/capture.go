// Package capture produces comparable captures (SPEC-capture.md): the same
// viewport, the page settled to the same state, geometry and screenshot taken
// at the same moment, written into the bundle.
package capture

import (
	"time"

	"github.com/GerMP21/Omnisnap/internal/schema"
)

// Capture discipline from the spike (capture.mjs). Changing any of these
// changes what is measured (SPEC-capture.md, "Ask first").
const (
	idleQuiet      = 500 * time.Millisecond // network idle: this long without a request in flight
	idleAfterLoad  = 8 * time.Second        // max wait for idle after load
	idleAfterSteps = 5 * time.Second        // max wait for idle after stabilize (lazy content)
	classicSettle  = 1500 * time.Millisecond
	maxShotW       = 2560 // screenshot cap (SPEC-bundle-schema.md, bundle layout)
	maxShotH       = 16000
)

// Every driver call of a capture has a deadline: navigation has its own,
// every other step the script timeout (a hung renderer must not stall a job).

// Options say what to capture and where.
type Options struct {
	Out      string // bundle directory
	Slug     string
	URL      string
	Engine   string // browser id, e.g. "chrome"
	Version  string
	Run      string          // "a", "b", ...
	Viewport schema.Viewport // zero: schema.DefaultViewport
	Timeouts schema.Timeouts // zero fields: the schema defaults

	settle time.Duration // tests shorten the Classic settle
}

// Result is what a capture reports for progress events.
type Result struct {
	Nodes   int
	MS      int64
	Blocked int
}

func (o Options) viewport() schema.Viewport {
	vp := o.Viewport
	if vp.Width == 0 || vp.Height == 0 {
		vp = schema.DefaultViewport
	}
	if vp.DeviceScaleFactor == 0 {
		vp.DeviceScaleFactor = 1
	}
	return vp
}

func (o Options) navigationTimeout() time.Duration {
	if o.Timeouts.NavigationMS > 0 {
		return time.Duration(o.Timeouts.NavigationMS) * time.Millisecond
	}
	return schema.DefaultNavigationMS * time.Millisecond
}

func (o Options) scriptTimeout() time.Duration {
	if o.Timeouts.ScriptMS > 0 {
		return time.Duration(o.Timeouts.ScriptMS) * time.Millisecond
	}
	return schema.DefaultScriptMS * time.Millisecond
}
