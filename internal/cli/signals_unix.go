//go:build unix

package cli

import (
	"os"
	"syscall"
)

// stopSignals are the signals that end a run early. SIGHUP covers a closed
// terminal: without it the cleanup would not run (SPEC-process.md).
func stopSignals() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGHUP}
}
