package discovery

import (
	"context"
	"path"
	"strings"

	"github.com/GerMP21/Omnisnap/internal/schema"
)

// macOS sources (SPEC-discovery.md, Task 32): app bundles by their
// well-known paths, then Spotlight (mdfind) by bundle id for installs
// elsewhere, and Safari with its driver's path. Files only: no browser or
// driver is launched (whether safaridriver is enabled is StartSafari's to
// report: there is no flag that only asks).

// macBrowser is one browser as macOS installs it.
type macBrowser struct {
	id       string
	name     string
	bundle   string // "Google Chrome.app"
	exe      string // Contents/MacOS/Google Chrome
	bundleID string
	family   schema.Family
}

var macCatalog = []macBrowser{
	{id: "chrome", name: "Google Chrome", bundle: "Google Chrome.app", exe: "Google Chrome", bundleID: "com.google.Chrome", family: schema.FamilyBlink},
	{id: "chromium", name: "Chromium", bundle: "Chromium.app", exe: "Chromium", bundleID: "org.chromium.Chromium", family: schema.FamilyBlink},
	{id: "edge", name: "Microsoft Edge", bundle: "Microsoft Edge.app", exe: "Microsoft Edge", bundleID: "com.microsoft.edgemac", family: schema.FamilyBlink},
	{id: "brave", name: "Brave", bundle: "Brave Browser.app", exe: "Brave Browser", bundleID: "com.brave.Browser", family: schema.FamilyBlink},
	{id: "opera", name: "Opera", bundle: "Opera.app", exe: "Opera", bundleID: "com.operasoftware.Opera", family: schema.FamilyBlink},
	{id: "vivaldi", name: "Vivaldi", bundle: "Vivaldi.app", exe: "Vivaldi", bundleID: "com.vivaldi.Vivaldi", family: schema.FamilyBlink},
	{id: "firefox", name: "Firefox", bundle: "Firefox.app", exe: "firefox", bundleID: "org.mozilla.firefox", family: schema.FamilyGecko},
}

// darwinSources are the bundle, Spotlight and Safari sources.
func darwinSources() []source {
	return []source{bundleSource{}, mdfindSource{}, safariSource{}, cacheSource{}}
}

// macDrivers are the places safaridriver is installed.
var safariDriverPaths = []string{"/usr/bin/safaridriver"}

// bundleApp reads a bundle's Info.plist: executable and version.
func (e *env) bundleApp(dir, exeName string) (executable, version, bundleID string, ok bool) {
	plist := path.Join(dir, "Contents", "Info.plist")
	raw, read := e.read(plist)
	if !read {
		return "", "", "", false
	}
	info, err := parsePlist([]byte(raw))
	if err != nil {
		return "", "", "", false
	}
	if v, ok := info["CFBundleExecutable"].(string); ok && v != "" {
		exeName = v
	}
	if v, ok := info["CFBundleShortVersionString"].(string); ok {
		version = v
	} else if v, ok := info["CFBundleVersion"].(string); ok {
		version = v
	}
	if v, ok := info["CFBundleIdentifier"].(string); ok {
		bundleID = v
	}
	return path.Join(dir, "Contents", "MacOS", exeName), version, bundleID, true
}

// bundleSource: the standard install locations.
type bundleSource struct{}

func (bundleSource) scan(_ context.Context, e *env) []Browser {
	var roots []string
	if e.home() != "" {
		roots = append(roots, path.Join(e.home(), "Applications"))
	}
	roots = append(roots, "/Applications")
	var out []Browser
	for _, w := range macCatalog {
		for _, root := range roots {
			dir := path.Join(root, w.bundle)
			if _, info, ok := e.resolve(dir); !ok || !info.IsDir() {
				continue
			}
			exe, version, bundleID, ok := e.bundleApp(dir, w.exe)
			if !ok {
				continue
			}
			if bundleID != "" && bundleID != w.bundleID {
				continue // a different app in a same-named bundle
			}
			if !e.executable(exe) {
				continue
			}
			out = append(out, Browser{ID: w.id, Name: w.name, Family: w.family,
				Protocol: protocolOf(w.family), Executable: exe, Version: version, Source: SourceAppBundle})
			break
		}
	}
	return out
}

// mdfindSource: Spotlight finds bundles installed outside /Applications.
type mdfindSource struct{}

func (mdfindSource) scan(_ context.Context, e *env) []Browser {
	if e.mdfind == nil {
		return nil
	}
	var out []Browser
	for _, w := range macCatalog {
		for _, dir := range e.mdfind("kMDItemCFBundleIdentifier == '" + w.bundleID + "'") {
			dir = strings.TrimSuffix(strings.TrimSpace(dir), "/")
			exe, version, bundleID, ok := e.bundleApp(dir, w.exe)
			if !ok || (bundleID != "" && bundleID != w.bundleID) {
				continue
			}
			out = append(out, Browser{ID: w.id, Name: w.name, Family: w.family,
				Protocol: protocolOf(w.family), Executable: exe, Version: version, Source: SourceAppBundle})
			break
		}
	}
	return out
}

// safariSource: Safari opens a visible window by design (safaridriver has no
// headless mode); the driver must be enabled once with sudo.
type safariSource struct{}

func (safariSource) scan(_ context.Context, e *env) []Browser {
	dir := "/Applications/Safari.app"
	exe, version, _, ok := e.bundleApp(dir, "Safari")
	if !ok {
		return nil
	}
	b := Browser{ID: "safari", Name: "Safari", Family: schema.FamilyWebKit,
		Protocol: ProtocolClassic, Executable: exe, Version: version, Source: SourceAppBundle}
	for _, d := range safariDriverPaths {
		if e.executable(d) {
			b.Driver = d
			break
		}
	}
	switch {
	case b.Driver == "":
		b.Notes = append(b.Notes, "safaridriver not found: Safari cannot be driven")
	case e.safariEnabled != nil && !e.safariEnabled():
		b.Notes = append(b.Notes, "safaridriver is not enabled: run `sudo safaridriver --enable` once")
	}
	return []Browser{b}
}
