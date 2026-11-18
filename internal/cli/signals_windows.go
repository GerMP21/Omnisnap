//go:build windows

package cli

import "os"

// stopSignals: Windows has Ctrl-C and TerminateProcess; the latter runs no
// handler and is covered by the Job Object and the sweeper.
func stopSignals() []os.Signal { return []os.Signal{os.Interrupt} }
