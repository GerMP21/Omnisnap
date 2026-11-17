//go:build !linux

package process

import "errors"

// waitExitedNoReap is Linux-only (waitid with WNOWAIT). Elsewhere the exit
// watcher reaps at once: on Windows the process handle prevents pid reuse;
// on macOS a recycled group id remains a (very unlikely) risk.
func waitExitedNoReap(int) error { return errors.ErrUnsupported }
