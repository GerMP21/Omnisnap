//go:build darwin || windows

package install

import (
	"os"
	"testing"
)

// needFixtureTool fails on CI and skips elsewhere: the runner images ship the
// tool, so a missing one in CI is a broken lane, not a reason to pass.
func needFixtureTool(t *testing.T, what string) {
	t.Helper()
	if os.Getenv("CI") != "" {
		t.Fatalf("%s is required to build this OS's install fixture", what)
	}
	t.Skipf("%s not found: cannot build this OS's install fixture", what)
}
