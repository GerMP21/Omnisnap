package discovery

import (
	"context"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/GerMP21/Omnisnap/internal/schema"
)

// Windows sources (SPEC-discovery.md). They read the registry and file
// metadata only, never launch a browser, and are tested on every OS against
// recorded registry dumps (testdata/discovery/windows).

const (
	appPathsKey   = `HKEY_LOCAL_MACHINE\SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths\`
	appPaths32Key = `HKEY_LOCAL_MACHINE\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\App Paths\`
	appPathsHKCU  = `HKEY_CURRENT_USER\SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths\`
	uninstallKey  = `HKEY_LOCAL_MACHINE\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`
	uninstall32   = `HKEY_LOCAL_MACHINE\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall`
	uninstallHKCU = `HKEY_CURRENT_USER\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`
)

// winBrowser is one browser as Windows installs it.
type winBrowser struct {
	id      string
	name    string
	family  schema.Family
	exe     string   // the App Paths key: chrome.exe
	display []string // DisplayName prefixes in the uninstall keys
}

// windowsCatalog: the Windows executable names and install names. IEXPLORE is
// absent on purpose: Internet Explorer is not an Omnisnap engine.
var windowsCatalog = []winBrowser{
	{id: "chrome", name: "Google Chrome", family: schema.FamilyBlink, exe: "chrome.exe", display: []string{"Google Chrome"}},
	{id: "chromium", name: "Chromium", family: schema.FamilyBlink, exe: "chromium.exe", display: []string{"Chromium"}},
	{id: "edge", name: "Microsoft Edge", family: schema.FamilyBlink, exe: "msedge.exe", display: []string{"Microsoft Edge"}},
	{id: "brave", name: "Brave", family: schema.FamilyBlink, exe: "brave.exe", display: []string{"Brave"}},
	// Opera GX installs the same opera.exe: the uninstall entry names the variant.
	{id: "opera", name: "Opera", family: schema.FamilyBlink, exe: "opera.exe", display: []string{"Opera GX", "Opera"}},
	{id: "vivaldi", name: "Vivaldi", family: schema.FamilyBlink, exe: "vivaldi.exe", display: []string{"Vivaldi"}},
	{id: "firefox", name: "Firefox", family: schema.FamilyGecko, exe: "firefox.exe", display: []string{"Mozilla Firefox"}},
	{id: "firefox-esr", name: "Firefox ESR", family: schema.FamilyGecko, exe: "firefox.exe", display: []string{"Mozilla Firefox (ESR)"}},
}

// windowsSources are the registry sources, plus $PATH as a last resort.
func windowsSources() []source {
	return []source{registrySource{}, windowsPathSource{}, cacheSource{}}
}

// registrySource: App Paths (the executable) and Uninstall (the version and,
// for Opera GX, the name).
type registrySource struct{}

func (registrySource) scan(_ context.Context, e *env) []Browser {
	versions, names := e.installedVersions()
	var out []Browser
	taken := map[string]bool{} // one browser per executable name (firefox and ESR share it)
	for _, w := range windowsCatalog {
		exe, ok := e.appPath(w.exe)
		if !ok || taken[w.exe] {
			continue
		}
		// An ESR install and a normal Firefox use the same executable name;
		// the uninstall entry tells them apart.
		id, name := w.id, w.name
		if displayName, ok := names[w.exe]; ok {
			for _, prefix := range w.display {
				if strings.HasPrefix(strings.ToLower(displayName), strings.ToLower(prefix)) {
					name = displayName
					if prefix == "Mozilla Firefox (ESR)" {
						id = "firefox-esr"
					}
					break
				}
			}
			if id == "firefox" && strings.HasPrefix(strings.ToLower(displayName), "mozilla firefox (esr)") {
				id = "firefox-esr"
			}
		}
		b := Browser{ID: id, Name: name, Family: w.family, Protocol: protocolOf(w.family),
			Executable: exe, Source: SourceRegistry}
		b.Version = versions[w.exe]
		if b.Version == "" {
			b.Version = e.fileVersionOf(exe)
		}
		taken[w.exe] = true
		out = append(out, b)
	}
	return out
}

// windowsPathSource finds executables installed outside App Paths (portable
// builds), by name on $PATH.
type windowsPathSource struct{}

func (windowsPathSource) scan(_ context.Context, e *env) []Browser {
	var out []Browser
	for _, w := range windowsCatalog {
		if p, ok := e.lookPathWindows(w.exe); ok {
			out = append(out, Browser{ID: w.id, Name: w.name, Family: w.family,
				Protocol: protocolOf(w.family), Executable: p, Source: SourcePath,
				Version: e.fileVersionOf(p)})
		}
	}
	return out
}

// regValue is one value of one key in reg.exe output.
type regValue struct {
	name  string
	value string
}

var (
	regKeyLine   = regexp.MustCompile(`^(HKEY_[^\r\n]+)`)
	regValueLine = regexp.MustCompile(`^\s+(.+?)\s{2,}REG_\w+\s{2,}(.*)$`)
)

// parseReg parses `reg query <key> /s` output into path -> values. Both
// "(Default)" and named values appear; value names are case-insensitive in
// use but kept as recorded.
func parseReg(out string) map[string][]regValue {
	keys := map[string][]regValue{}
	current := ""
	for _, line := range strings.Split(out, "\n") {
		if m := regKeyLine.FindStringSubmatch(line); m != nil {
			current = strings.TrimSpace(m[1])
			keys[current] = nil
			continue
		}
		if current == "" {
			continue
		}
		if m := regValueLine.FindStringSubmatch(line); m != nil {
			keys[current] = append(keys[current], regValue{name: m[1], value: strings.TrimSpace(m[2])})
		}
	}
	return keys
}

// regValueOf finds a value by name (case-insensitive).
func regValueOf(values []regValue, name string) (string, bool) {
	for _, v := range values {
		if strings.EqualFold(v.name, name) {
			return v.value, true
		}
	}
	return "", false
}

// appPath returns the default value of an App Paths key, in the 64- and
// 32-bit hives (a 32-bit browser on 64-bit Windows lives in WOW6432Node).
func (e *env) appPath(exe string) (string, bool) {
	if e.registry == nil {
		return "", false
	}
	for _, root := range []string{appPathsKey, appPaths32Key, appPathsHKCU} {
		out, ok := e.registry(root + exe)
		if !ok {
			continue
		}
		for key, values := range parseReg(out) {
			if !strings.EqualFold(path.Base(strings.ReplaceAll(key, `\`, "/")), exe) {
				continue
			}
			for _, v := range values {
				// The default value has no name in the registry; reg.exe
				// prints it as "(Default)".
				if strings.EqualFold(v.name, "(Default)") || strings.EqualFold(v.name, "default") {
					if v.value != "" {
						return v.value, true
					}
				}
			}
		}
	}
	return "", false
}

// installedVersions reads every Uninstall key once: executable name ->
// DisplayVersion, and executable name -> DisplayName (for Opera GX, ESR).
// Chromium's uninstall key is a hash, so enumeration is the only way.
func (e *env) installedVersions() (versions, names map[string]string) {
	versions, names = map[string]string{}, map[string]string{}
	if e.registry == nil {
		return versions, names
	}
	for _, root := range []string{uninstallKey, uninstall32, uninstallHKCU} {
		out, ok := e.registry(root)
		if !ok {
			continue
		}
		entries := parseReg(out)
		paths := make([]string, 0, len(entries))
		for key := range entries {
			paths = append(paths, key)
		}
		sort.Strings(paths)
		for _, key := range paths {
			values := entries[key]
			display, _ := regValueOf(values, "DisplayName")
			if display == "" {
				continue
			}
			exe := displayExe(display, values)
			if exe == "" {
				continue
			}
			if v, ok := regValueOf(values, "DisplayVersion"); ok && v != "" {
				versions[exe] = v
			}
			if _, ok := names[exe]; !ok {
				names[exe] = display
			}
		}
	}
	return versions, names
}

// displayExe maps an uninstall DisplayName to the executable it installs.
func displayExe(display string, values []regValue) string {
	lower := strings.ToLower(display)
	for _, w := range windowsCatalog {
		for _, prefix := range w.display {
			if strings.HasPrefix(lower, strings.ToLower(prefix)) {
				// ESR before Firefox: both share firefox.exe.
				if strings.Contains(lower, "(esr)") {
					return "firefox.exe"
				}
				return w.exe
			}
		}
	}
	// DisplayIcon often names the executable: "C:\...\opera.exe",0
	if icon, ok := regValueOf(values, "DisplayIcon"); ok {
		p := strings.Trim(strings.SplitN(icon, ",", 2)[0], `"`)
		if base := path.Base(strings.ReplaceAll(p, `\`, "/")); strings.HasSuffix(strings.ToLower(base), ".exe") {
			return base
		}
	}
	return ""
}

// lookPathWindows splits on ';'.
func (e *env) lookPathWindows(name string) (string, bool) {
	for _, dir := range strings.Split(e.getenv("PATH"), ";") {
		if dir == "" || !isAbsWindows(dir) {
			continue
		}
		p := dir + `\` + name
		if e.executableWindows(p) {
			return p, true
		}
	}
	return "", false
}

// isAbsWindows accepts C:\dir, \\server\share and /dir: path.IsAbs only
// knows the last one.
func isAbsWindows(p string) bool {
	p = strings.ReplaceAll(p, `\`, "/")
	if len(p) >= 2 && p[1] == ':' {
		return true
	}
	return strings.HasPrefix(p, "//") || strings.HasPrefix(p, "/")
}

// executableWindows accepts any regular file: Windows has no execute bit.
func (e *env) executableWindows(abs string) bool {
	_, info, ok := e.resolve(strings.ReplaceAll(abs, `\`, "/"))
	return ok && info.Mode().IsRegular()
}

// fileVersionOf reads a Windows file's version, when the platform can.
func (e *env) fileVersionOf(abs string) string {
	if e.fileVersion == nil {
		return ""
	}
	return e.fileVersion(abs)
}
