package discovery

import (
	"context"
	"path"
	"strings"
)

// Linux sources. They read files only, so they are tested on every OS against
// fake file trees; platform_linux.go makes them the sources on Linux.
func linuxSources() []source {
	return []source{desktopSource{}, pathSource{}, snapSource{}, flatpakSource{}, webkitSource{}, wpeSource{}, cacheSource{}}
}

const (
	snapBin            = "/snap/bin"
	snapPrivateTmpNote = "snap: private /tmp: profiles must be created where the snap can read them"
	flatpakNote        = "flatpak: sandboxed file system: profiles must be created where the flatpak can read them"
)

// classify names the packaging of an executable found by path or desktop file.
func (e *env) classify(b Browser, fallback Source) Browser {
	b.Source = fallback
	if path.Dir(b.Executable) == snapBin {
		b.Source = SourceSnap
		b.Notes = append(b.Notes, snapPrivateTmpNote)
	}
	return b
}

// pathSource: the browsers' executable names on $PATH.
type pathSource struct{}

func (pathSource) scan(_ context.Context, e *env) []Browser {
	var out []Browser
	for _, k := range catalog {
		for _, exe := range k.exes {
			if p, ok := e.lookPath(exe); ok {
				out = append(out, e.classify(k.browser(p, ""), SourcePath))
				break
			}
		}
	}
	return out
}

// desktopSource: .desktop files, the only trace of a browser installed
// outside $PATH (e.g. /opt/google/chrome). Flatpak exports are left to
// flatpakSource, which knows how to read their versions.
type desktopSource struct{}

func (desktopSource) dirs(e *env) []string {
	dataHome := e.getenv("XDG_DATA_HOME")
	if dataHome == "" && e.home() != "" {
		dataHome = path.Join(e.home(), ".local/share")
	}
	dataDirs := e.getenv("XDG_DATA_DIRS")
	if dataDirs == "" {
		dataDirs = "/usr/local/share:/usr/share"
	}
	var dirs []string
	for _, d := range append([]string{dataHome}, strings.Split(dataDirs, ":")...) {
		if d != "" && !strings.Contains(d, "/flatpak/") {
			dirs = append(dirs, path.Join(d, "applications"))
		}
	}
	return append(dirs, "/var/lib/snapd/desktop/applications")
}

func (s desktopSource) scan(_ context.Context, e *env) []Browser {
	var out []Browser
	for _, k := range catalog {
	found:
		for _, dir := range s.dirs(e) {
			for _, id := range k.desktop {
				text, ok := e.read(path.Join(dir, id+".desktop"))
				if !ok {
					continue
				}
				if exe, ok := e.desktopExec(text); ok {
					out = append(out, e.classify(k.browser(exe, ""), SourceDesktop))
					break found
				}
			}
		}
	}
	return out
}

// desktopExec returns the program of the [Desktop Entry] Exec line, resolved
// to an executable: env prefixes, quotes and field codes are skipped.
func (e *env) desktopExec(text string) (string, bool) {
	section := ""
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			section = line
			continue
		}
		if section != "[Desktop Entry]" || !strings.HasPrefix(line, "Exec=") {
			continue
		}
		args := splitExec(strings.TrimPrefix(line, "Exec="))
		for len(args) > 0 && (args[0] == "env" || strings.Contains(args[0], "=")) {
			args = args[1:]
		}
		if len(args) == 0 || path.Base(args[0]) == "flatpak" {
			return "", false
		}
		if path.IsAbs(args[0]) {
			return args[0], e.executable(args[0])
		}
		return e.lookPath(args[0])
	}
	return "", false
}

// splitExec splits an Exec value into arguments, honouring double quotes
// (the desktop entry spec's only quoting).
func splitExec(s string) []string {
	var args []string
	var cur strings.Builder
	inQuote, has := false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"':
			inQuote, has = !inQuote, true
		case c == '\\' && inQuote && i+1 < len(s):
			i++
			cur.WriteByte(s[i])
		case (c == ' ' || c == '\t') && !inQuote:
			if has {
				args = append(args, cur.String())
				cur.Reset()
				has = false
			}
		default:
			cur.WriteByte(c)
			has = true
		}
	}
	if has {
		args = append(args, cur.String())
	}
	return args
}

// snapSource: snaps outside $PATH.
type snapSource struct{}

func (snapSource) scan(_ context.Context, e *env) []Browser {
	var out []Browser
	for _, k := range catalog {
		if k.snap == "" {
			continue
		}
		if p := path.Join(snapBin, k.snap); e.executable(p) {
			out = append(out, e.classify(k.browser(p, ""), SourceSnap))
		}
	}
	return out
}

// flatpakSource: system and user Flatpak installs, by their exported launchers.
type flatpakSource struct{}

func (flatpakSource) roots(e *env) []string {
	roots := []string{"/var/lib/flatpak"}
	if e.home() != "" {
		roots = append(roots, path.Join(e.home(), ".local/share/flatpak"))
	}
	return roots
}

func (s flatpakSource) scan(_ context.Context, e *env) []Browser {
	var out []Browser
	for _, k := range catalog {
		if k.flatpak == "" {
			continue
		}
		for _, root := range s.roots(e) {
			p := path.Join(root, "exports/bin", k.flatpak)
			if !e.executable(p) {
				continue
			}
			b := k.browser(p, SourceFlatpak)
			b.Version = e.flatpakVersion(root, k.flatpak)
			b.Notes = append(b.Notes, flatpakNote)
			out = append(out, b)
			break
		}
	}
	return out
}

// webkitSource: WebKitGTK's MiniBrowser and WebKitWebDriver, at the paths the
// distributions install them (6.0 first, then 4.1).
type webkitSource struct{}

var (
	miniBrowserPaths = []string{
		"/usr/libexec/webkitgtk-6.0/MiniBrowser",
		"/usr/lib/x86_64-linux-gnu/webkitgtk-6.0/MiniBrowser",
		"/usr/lib/aarch64-linux-gnu/webkitgtk-6.0/MiniBrowser",
		"/usr/lib/webkitgtk-6.0/MiniBrowser",
		"/usr/libexec/webkit2gtk-4.1/MiniBrowser",
		"/usr/lib/x86_64-linux-gnu/webkit2gtk-4.1/MiniBrowser",
		"/usr/lib/aarch64-linux-gnu/webkit2gtk-4.1/MiniBrowser",
		"/usr/lib/webkit2gtk-4.1/MiniBrowser",
	}
	webDriverPaths = []string{"/usr/bin/WebKitWebDriver", "/usr/local/bin/WebKitWebDriver"}
)

func (webkitSource) scan(_ context.Context, e *env) []Browser {
	for _, mb := range miniBrowserPaths {
		if !e.executable(mb) {
			continue
		}
		b := webkitgtk.browser(mb, SourcePath)
		for _, d := range append([]string{path.Join(path.Dir(mb), "WebKitWebDriver")}, webDriverPaths...) {
			if e.executable(d) {
				b.Driver = d
				break
			}
		}
		b.Notes = append(b.Notes, "needs a display: process provides a headless one")
		if b.Driver == "" {
			b.Notes = append(b.Notes, "WebKitWebDriver not found: cannot be driven")
		}
		return []Browser{b}
	}
	return nil
}

// wpeSource: the WPE port (wpewebkit). It is headless by itself and fixes the
// viewport with MiniBrowser's --size, so it is the better WebKit lane on
// machines without a compositor. OMNISNAP_WEBKIT=wpe selects it over the GTK
// port (see find0); without the variable both are listed and callers that keep
// one engine per family prefer WebKitGTK.
type wpeSource struct{}

var (
	wpeMiniBrowserPaths = []string{
		"/usr/lib/wpe-webkit-2.0/MiniBrowser",
		"/usr/lib64/wpe-webkit-2.0/MiniBrowser",
		"/usr/lib/x86_64-linux-gnu/wpe-webkit-2.0/MiniBrowser",
		"/usr/lib/aarch64-linux-gnu/wpe-webkit-2.0/MiniBrowser",
	}
	wpeDriverPaths = []string{"/usr/bin/WPEWebDriver", "/usr/local/bin/WPEWebDriver"}
)

func (wpeSource) scan(_ context.Context, e *env) []Browser {
	driver := e.getenv("OMNISNAP_WPE_DRIVER")
	if driver == "" {
		for _, d := range wpeDriverPaths {
			if e.executable(d) {
				driver = d
				break
			}
		}
	}
	if driver == "" {
		return nil
	}
	browser := e.getenv("OMNISNAP_WPE_BROWSER")
	if browser == "" {
		for _, mb := range wpeMiniBrowserPaths {
			if e.executable(mb) {
				browser = mb
				break
			}
		}
	}
	if browser == "" {
		return nil
	}
	b := wpe.browser(browser, SourcePath)
	b.Driver = driver
	b.Notes = append(b.Notes, "headless: no display needed", "the viewport is fixed at launch (--size)")
	return []Browser{b}
}
