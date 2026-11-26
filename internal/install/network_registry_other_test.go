//go:build network && !windows

package install

import "testing"

// registrySnapshot: only Windows has a registry an installer could write to.
func registrySnapshot(*testing.T) string { return "" }
