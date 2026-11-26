//go:build windows

package discovery

// platformSources: the registry (App Paths and Uninstall) and $PATH.
func platformSources() []source { return windowsSources() }

// platformDefaults wires the real Windows lookups when the caller did not
// inject recorded ones.
func platformDefaults(o *Options) {
	if o.FS == nil {
		o.FS = driveFS{} // os.DirFS("/") cannot open "C:/..."
	}
	if o.Registry == nil {
		o.Registry = regQuery
	}
	if o.FileVersion == nil {
		o.FileVersion = fileVersion
	}
}
