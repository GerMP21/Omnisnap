//go:build !linux && !darwin

package cli

import (
	"context"
	"fmt"

	"github.com/GerMP21/Omnisnap/internal/discovery"
)

// launchWebKit: WebKit is driven only on Linux (WebKitGTK) and macOS (Safari).
func launchWebKit(_ context.Context, b discovery.Browser) (engine, error) {
	return nil, fmt.Errorf("%s: WebKit cannot be driven on this OS", b.ID)
}
