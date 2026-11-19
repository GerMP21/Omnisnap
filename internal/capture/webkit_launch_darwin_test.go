//go:build browsers && darwin

package capture

import (
	"context"

	"github.com/GerMP21/Omnisnap/internal/discovery"
	"github.com/GerMP21/Omnisnap/internal/process"
)

// startWebKitFamily starts the WebKit engine discovery found: on macOS,
// Safari through safaridriver.
func startWebKitFamily(ctx context.Context, b discovery.Browser) (string, map[string]any, func(), error) {
	p, err := process.StartSafari(ctx, process.SafariOptions{Driver: b.Driver})
	if err != nil {
		return "", nil, nil, err
	}
	return p.URL, p.Capabilities, func() { _ = p.Close(context.Background()) }, nil
}
