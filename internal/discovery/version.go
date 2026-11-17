package discovery

import (
	"io/fs"
	"path"
	"regexp"
	"strings"
)

// version reads a browser's version from files, cheapest first. It never
// launches anything: `--version` of a snap or flatpak alone takes seconds.
func (e *env) version(b Browser) string {
	k, ok := byID(b.ID)
	if !ok {
		return ""
	}
	if b.Source == SourceSnap {
		if v := e.snapVersion(path.Base(b.Executable)); v != "" {
			return v
		}
	}
	if b.Family == "gecko" {
		if v := e.applicationIni(b, k); v != "" {
			return v
		}
	}
	for _, pkg := range k.packages {
		if v := e.dpkgVersion(pkg); v != "" {
			return v
		}
		if v := e.pacmanVersion(pkg); v != "" {
			return v
		}
	}
	return ""
}

// applicationIni reads Firefox's version from the install directory: next to
// the real binary, or the usual install directories (/usr/bin/firefox is often
// a shell script).
func (e *env) applicationIni(b Browser, k known) string {
	var dirs []string
	if target, _, ok := e.resolve(b.Executable); ok {
		dirs = append(dirs, path.Dir(target))
	}
	for _, base := range []string{"/usr/lib", "/usr/lib64", "/opt"} {
		dirs = append(dirs, path.Join(base, k.id))
	}
	for _, d := range dirs {
		if text, ok := e.read(path.Join(d, "application.ini")); ok {
			if v := iniValue(text, "App", "Version"); v != "" {
				return v
			}
		}
	}
	return ""
}

func iniValue(text, section, key string) string {
	cur := ""
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			cur = strings.Trim(line, "[]")
			continue
		}
		if k, v, ok := strings.Cut(line, "="); ok && cur == section && strings.TrimSpace(k) == key {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// snapVersion reads /snap/<name>/current/meta/snap.yaml.
func (e *env) snapVersion(name string) string {
	text, ok := e.read(path.Join("/snap", name, "current/meta/snap.yaml"))
	if !ok {
		return ""
	}
	for _, line := range strings.Split(text, "\n") {
		if v, ok := strings.CutPrefix(line, "version:"); ok {
			return strings.Trim(strings.TrimSpace(v), `'"`)
		}
	}
	return ""
}

var releaseVersion = regexp.MustCompile(`<release\b[^>]*\bversion="([^"]+)"`)

// flatpakVersion reads the newest <release> of the app's metainfo (or the
// older appdata) inside the installed app.
func (e *env) flatpakVersion(root, appID string) string {
	share := path.Join(root, "app", appID, "current/active/files/share")
	for _, f := range []string{"metainfo/" + appID + ".metainfo.xml", "appdata/" + appID + ".appdata.xml", "metainfo/" + appID + ".appdata.xml"} {
		if text, ok := e.read(path.Join(share, f)); ok {
			if m := releaseVersion.FindStringSubmatch(text); m != nil {
				return m[1]
			}
		}
	}
	return ""
}

// dpkgVersion looks pkg up in /var/lib/dpkg/status (read once per Find).
func (e *env) dpkgVersion(pkg string) string {
	if e.dpkg == nil {
		e.dpkg = map[string]string{}
		text, _ := e.read("/var/lib/dpkg/status")
		for _, stanza := range strings.Split(text, "\n\n") {
			var name, version, status string
			for _, line := range strings.Split(stanza, "\n") {
				switch k, v, _ := strings.Cut(line, ": "); k {
				case "Package":
					name = v
				case "Version":
					version = v
				case "Status":
					status = v
				}
			}
			if name != "" && (status == "" || strings.HasSuffix(status, " installed")) {
				e.dpkg[name] = version
			}
		}
	}
	return upstreamVersion(e.dpkg[pkg])
}

// pacmanVersion reads /var/lib/pacman/local/<pkg>-<version>-<rel>/desc.
func (e *env) pacmanVersion(pkg string) string {
	entries, err := fs.ReadDir(e.fs, rel("/var/lib/pacman/local"))
	if err != nil {
		return ""
	}
	for _, d := range entries {
		rest, ok := strings.CutPrefix(d.Name(), pkg+"-")
		if !ok || rest == "" || rest[0] < '0' || rest[0] > '9' {
			continue // another package whose name starts the same
		}
		text, ok := e.read(path.Join("/var/lib/pacman/local", d.Name(), "desc"))
		if !ok {
			continue
		}
		if descValue(text, "NAME") == pkg || descValue(text, "NAME") == "" {
			return upstreamVersion(descValue(text, "VERSION"))
		}
	}
	return ""
}

func descValue(text, field string) string {
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		if strings.TrimSpace(l) == "%"+field+"%" && i+1 < len(lines) {
			return strings.TrimSpace(lines[i+1])
		}
	}
	return ""
}

// upstreamVersion drops a package epoch ("1:") and the packaging revision
// ("-1", "-1.1", "-1~deb13u1") from a dpkg or pacman version.
func upstreamVersion(v string) string {
	if _, after, ok := strings.Cut(v, ":"); ok {
		v = after
	}
	if i := strings.LastIndex(v, "-"); i > 0 {
		v = v[:i]
	}
	return v
}
