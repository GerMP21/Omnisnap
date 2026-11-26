//go:build !linux

package process

// ApplyOOMScore does nothing outside Linux: Windows and macOS have no
// out-of-memory score, and Omnisnap changes no other process priority or
// memory setting there (change cap-live-browsers).
func (h *Handle) ApplyOOMScore() error { return nil }

// ApplyOOMScore does nothing outside Linux (see the method).
func ApplyOOMScore() error { return nil }
