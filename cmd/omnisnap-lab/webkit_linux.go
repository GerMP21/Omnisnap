//go:build linux

package main

import (
	"context"
	"fmt"

	"github.com/GerMP21/Omnisnap/internal/discovery"
	"github.com/GerMP21/Omnisnap/internal/drivers"
	"github.com/GerMP21/Omnisnap/internal/drivers/classic"
	"github.com/GerMP21/Omnisnap/internal/process"
)

// launchLabWebKit starts WebKitGTK under the private headless display.
func launchLabWebKit(ctx context.Context, b discovery.Browser) (labEngine, error) {
	if b.Driver == "" {
		return labEngine{}, fmt.Errorf("WebKitWebDriver not found")
	}
	p, err := process.StartWebKit(ctx, process.WebKitOptions{Driver: b.Driver, Browser: b.Executable})
	if err != nil {
		return labEngine{}, err
	}
	c := classic.New(p.URL)
	c.Capabilities = p.Capabilities
	if err := c.Status(ctx); err != nil {
		_ = p.Close(context.WithoutCancel(ctx))
		return labEngine{}, err
	}
	return labEngine{id: b.ID, version: b.Version,
		new:   func(ctx context.Context) (drivers.Session, error) { return c.NewSession(ctx) },
		close: func() { _ = p.Close(context.Background()) }}, nil
}
