//go:build e2e && !linux

package e2e

// webkitGoldenEngine: the WebKit goldens come from WebKitGTK's MiniBrowser,
// which runs only on Linux, so other OSes have no WebKit golden engine.
func webkitGoldenEngine(driver, browser string) (engine, bool) { return engine{}, false }
