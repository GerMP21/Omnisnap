//go:build browsers && linux

package capture

import (
	"context"

	"github.com/GerMP21/Omnisnap/internal/discovery"
	"github.com/GerMP21/Omnisnap/internal/process"
)

// startWebKitFamily starts the WebKit engine discovery found: on Linux,
// WebKitGTK or WPE (StartWebKit picks the port from the paths).
func startWebKitFamily(ctx context.Context, b discovery.Browser) (string, map[string]any, func(), error) {
	p, err := process.StartWebKit(ctx, process.WebKitOptions{Driver: b.Driver, Browser: b.Executable})
	if err != nil {
		return "", nil, nil, err
	}
	return p.URL, p.Capabilities, func() { _ = p.Close(context.Background()) }, nil
}
