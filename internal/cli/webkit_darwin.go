//go:build darwin

package cli

import (
	"context"
	"fmt"

	"github.com/GerMP21/Omnisnap/internal/discovery"
	"github.com/GerMP21/Omnisnap/internal/drivers/classic"
	"github.com/GerMP21/Omnisnap/internal/process"
)

// launchWebKit starts Safari through safaridriver (the only WebKit on macOS).
func launchWebKit(ctx context.Context, b discovery.Browser) (engine, error) {
	if b.Driver == "" {
		return nil, fmt.Errorf("%s: safaridriver not found: cannot be driven", b.ID)
	}
	p, err := process.StartSafari(ctx, process.SafariOptions{Driver: b.Driver})
	if err != nil {
		return nil, err
	}
	c := classic.New(p.URL)
	c.Capabilities = p.Capabilities
	if err := c.Status(ctx); err != nil {
		_ = p.Close(context.WithoutCancel(ctx))
		return nil, err
	}
	return &webkitEngine{classic: c, stop: p.Close}, nil
}
