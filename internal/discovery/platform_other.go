//go:build !linux

package discovery

// Windows and macOS sources come with Tasks 30 and 31.
func platformSources() []source { return nil }
