//go:build browsers && !linux

package drivers_test

import "testing"

// wpeHarness: the WPE port only exists on Linux.
func wpeHarness(*testing.T) *harness { return nil }
