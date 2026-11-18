//go:build !linux && !windows && !darwin

package discovery

// Other platforms have no browser to drive.
func platformSources() []source { return nil }

func platformDefaults(*Options) {}
