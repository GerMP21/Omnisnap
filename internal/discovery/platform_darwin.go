//go:build darwin

package discovery

import (
	"os/exec"
	"strings"
)

// platformSources: app bundles, Spotlight and Safari.
func platformSources() []source { return darwinSources() }

func platformDefaults(o *Options) {
	if o.MDFind == nil {
		o.MDFind = mdfind
	}
	if o.SafariEnabled == nil {
		o.SafariEnabled = safariEnabled
	}
}

// mdfind runs the Spotlight query of SPEC-discovery.md.
func mdfind(query string) []string {
	out, err := exec.Command("mdfind", query).Output()
	if err != nil {
		return nil
	}
	return strings.Split(strings.TrimSpace(string(out)), "\n")
}

// safariEnabled asks safaridriver for its diagnostics: exit 0 means Remote
// Automation is on. The definitive answer is StartSafari's error, which names
// the fix; this note only spares the user a launch attempt.
func safariEnabled() bool {
	return exec.Command("/usr/bin/safaridriver", "--diagnose").Run() == nil
}
