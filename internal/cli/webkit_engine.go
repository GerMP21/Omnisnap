//go:build linux || darwin

package cli

import (
	"context"

	"github.com/GerMP21/Omnisnap/internal/drivers"
	"github.com/GerMP21/Omnisnap/internal/drivers/classic"
)

// WebKit is driven only on Linux (WebKitGTK, WPE) and macOS (Safari); the
// other OSes get webkit_other.go's error and never build this engine.

// webkitEngine drives WebKit over Classic; stop closes the driver (and, on
// Linux, its private display).
type webkitEngine struct {
	classic *classic.Client
	stop    func(context.Context) error
}

func (e *webkitEngine) NewSession(ctx context.Context) (drivers.Session, error) {
	return e.classic.NewSession(ctx)
}

func (e *webkitEngine) Close(ctx context.Context) error { return e.stop(ctx) }
