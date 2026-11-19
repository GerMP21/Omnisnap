//go:build darwin

package discovery

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// platformSources: app bundles, Spotlight and Safari.
func platformSources() []source { return darwinSources() }

// platformDefaults sets the macOS hooks. SafariEnabled stays nil: macOS has no
// way to ask safaridriver whether Remote Automation is on without starting it
// (`safaridriver --diagnose`, the probe this used, runs the driver with
// diagnostics on and never exits: discovery hung, so every command that
// discovers browsers hung on macOS). Discovery never launches anything
// (SPEC-discovery.md); a driver that is not enabled is reported by
// StartSafari, whose error names `safaridriver --enable`.
func platformDefaults(o *Options) {
	if o.MDFind == nil {
		o.MDFind = mdfind
	}
}

// mdfindTimeout bounds the Spotlight query: an index that is still building
// can stall it, and discovery must stay under a second (Task 32).
const mdfindTimeout = 2 * time.Second

// mdfind runs the Spotlight query of SPEC-discovery.md.
func mdfind(query string) []string {
	ctx, cancel := context.WithTimeout(context.Background(), mdfindTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "mdfind", query).Output()
	if err != nil {
		return nil
	}
	return strings.Split(strings.TrimSpace(string(out)), "\n")
}
