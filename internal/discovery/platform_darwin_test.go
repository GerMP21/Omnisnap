//go:build darwin

package discovery

import "testing"

// The defaults must not ask safaridriver anything: there is no flag that only
// asks, and `safaridriver --diagnose` (the old probe) runs the driver and never
// exits, so every command that discovers browsers hung on macOS.
func TestDarwinDefaultsNeverRunSafaridriver(t *testing.T) {
	var o Options
	platformDefaults(&o)
	if o.SafariEnabled != nil {
		t.Fatal("platformDefaults set SafariEnabled: discovery would run safaridriver")
	}
	if o.MDFind == nil {
		t.Error("platformDefaults left MDFind unset")
	}
}
