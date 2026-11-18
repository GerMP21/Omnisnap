//go:build linux

package discovery

func platformSources() []source { return linuxSources() }

// platformDefaults: Linux discovery needs no OS-specific lookups.
func platformDefaults(*Options) {}
