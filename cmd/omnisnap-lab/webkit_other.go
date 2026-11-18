//go:build !linux && !darwin

package main

import (
	"context"
	"fmt"

	"github.com/GerMP21/Omnisnap/internal/discovery"
)

// launchLabWebKit: WebKit is driven only on Linux and macOS.
func launchLabWebKit(context.Context, discovery.Browser) (labEngine, error) {
	return labEngine{}, fmt.Errorf("WebKit cannot be driven on this OS")
}
