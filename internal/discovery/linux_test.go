package discovery

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/GerMP21/Omnisnap/internal/schema"
)

// tree builds a fake root file system. Keys are absolute paths; values are
// file contents. A value starting with "->" makes a symlink; "exe:" an
// executable file.
func tree(files map[string]string) fstest.MapFS {
	m := fstest.MapFS{}
	for p, v := range files {
		p = strings.TrimPrefix(p, "/")
		switch {
		case strings.HasPrefix(v, "->"):
			m[p] = &fstest.MapFile{Data: []byte(strings.TrimSpace(v[2:])), Mode: fs.ModeSymlink | 0o777}
		case strings.HasPrefix(v, "exe:"):
			m[p] = &fstest.MapFile{Data: []byte(v[4:]), Mode: 0o755}
		default:
			m[p] = &fstest.MapFile{Data: []byte(v), Mode: 0o644}
		}
	}
	return m
}

func find(t *testing.T, fsys fs.FS, env map[string]string) map[string]Browser {
	t.Helper()
	got, err := find0(context.Background(), Options{FS: fsys, Getenv: func(k string) string { return env[k] }}, linuxSources())
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Browser{}
	for _, b := range got {
		if _, dup := byID[b.ID]; dup {
			t.Errorf("%s listed twice", b.ID)
		}
		byID[b.ID] = b
	}
	return byID
}

var linuxEnv = map[string]string{"PATH": "/usr/local/bin:/usr/bin:/bin:/snap/bin", "HOME": "/home/u"}

func TestLinuxPathAndPacmanVersion(t *testing.T) {
	got := find(t, tree(map[string]string{
		"/usr/bin/chromium": "exe:",
		"/var/lib/pacman/local/chromium-157.0.8103.78-1.1/desc": "%NAME%\nchromium\n\n%VERSION%\n157.0.8103.78-1.1\n\n%BASE%\nchromium\n",
	}), linuxEnv)
	b, ok := got["chromium"]
	if !ok {
		t.Fatalf("chromium not found: %v", got)
	}
	want := Browser{ID: "chromium", Name: "Chromium", Family: schema.FamilyBlink, Protocol: ProtocolCDP,
		Executable: "/usr/bin/chromium", Version: "157.0.8103.78", Source: SourcePath}
	if !sameBrowser(b, want) {
		t.Errorf("got %+v\nwant %+v", b, want)
	}
}

// /usr/bin/firefox is often a shell script or a symlink; the version is in
// the install directory's application.ini, not in any package database.
func TestLinuxFirefoxVersionFromApplicationIni(t *testing.T) {
	got := find(t, tree(map[string]string{
		"/usr/bin/firefox":                       "exe:#!/bin/sh\nexec /usr/lib/firefox/firefox \"$@\"\n",
		"/usr/lib/firefox/firefox":               "exe:",
		"/usr/lib/firefox/application.ini":       "[App]\nVendor=Mozilla\nName=Firefox\nVersion=160.0\nBuildID=1\n",
		"/usr/bin/firefox-esr":                   "-> /usr/lib64/firefox-esr/firefox-esr",
		"/usr/lib64/firefox-esr/firefox-esr":     "exe:",
		"/usr/lib64/firefox-esr/application.ini": "[App]\nName=Firefox\nVersion=153.6.0\n",
	}), linuxEnv)
	if b := got["firefox"]; b.Version != "160.0" || b.Family != schema.FamilyGecko || b.Protocol != ProtocolBiDi {
		t.Errorf("firefox = %+v; want gecko over BiDi, version 160.0", b)
	}
	if b := got["firefox-esr"]; b.Version != "153.6.0" || b.Executable != "/usr/bin/firefox-esr" {
		t.Errorf("firefox-esr = %+v; want 153.6.0 through the symlink", b)
	}
}

// A desktop file is the only trace of a browser outside PATH; its Exec line
// has env prefixes, quotes and field codes.
func TestLinuxDesktopFileAndDpkgVersion(t *testing.T) {
	got := find(t, tree(map[string]string{
		"/usr/share/applications/google-chrome.desktop": "[Desktop Entry]\nName=Google Chrome\n" +
			"Exec=env CHROME_X=1 \"/opt/google/chrome/google-chrome\" --flag %U\n" +
			"[Desktop Action new-window]\nExec=/opt/other/thing --new-window\n",
		"/opt/google/chrome/google-chrome": "exe:",
		"/var/lib/dpkg/status": "Package: libfoo\nVersion: 1.0\n\n" +
			"Package: google-chrome-stable\nStatus: install ok installed\nVersion: 1:155.0.8059.39-1\n\n",
	}), linuxEnv)
	b := got["chrome"]
	if b.Executable != "/opt/google/chrome/google-chrome" || b.Source != SourceDesktop || b.Version != "155.0.8059.39" {
		t.Errorf("chrome = %+v; want the desktop file's Exec, dpkg version without epoch and revision", b)
	}
}

// The same browser seen by several sources is one entry, from the best source.
func TestLinuxDedupe(t *testing.T) {
	got := find(t, tree(map[string]string{
		"/usr/share/applications/chromium.desktop": "[Desktop Entry]\nExec=chromium %U\n",
		"/usr/bin/chromium":                        "exe:",
	}), linuxEnv)
	if b := got["chromium"]; b.Executable != "/usr/bin/chromium" || b.Source != SourceDesktop {
		t.Errorf("chromium = %+v; want one entry from the desktop file, Exec resolved through PATH", b)
	}
}

// Snap: profiles must go where the snap can read them (private /tmp); the
// version comes from snap.yaml, because `--version` of a snap is slow.
func TestLinuxSnap(t *testing.T) {
	got := find(t, tree(map[string]string{
		"/snap/bin/firefox":                    "-> /usr/bin/snap",
		"/usr/bin/snap":                        "exe:",
		"/snap/firefox/current/meta/snap.yaml": "name: firefox\nversion: '160.0.1-1'\nsummary: Mozilla Firefox\n",
		"/var/lib/snapd/desktop/applications/firefox_firefox.desktop": "[Desktop Entry]\nExec=env BAMF_DESKTOP_FILE_HINT=/var/lib/snapd/desktop/applications/firefox_firefox.desktop /snap/bin/firefox %u\n",
	}), linuxEnv)
	b := got["firefox"]
	if b.Source != SourceSnap || b.Version != "160.0.1-1" || b.Executable != "/snap/bin/firefox" || !hasNote(b, "private /tmp") {
		t.Errorf("firefox = %+v; want the snap, its snap.yaml version and the private /tmp note", b)
	}
}

// Flatpak: system and user installs; the version comes from the app's metainfo.
func TestLinuxFlatpak(t *testing.T) {
	got := find(t, tree(map[string]string{
		"/var/lib/flatpak/exports/bin/org.chromium.Chromium":                                                                  "exe:",
		"/var/lib/flatpak/app/org.chromium.Chromium/current/active/files/share/metainfo/org.chromium.Chromium.metainfo.xml":   `<?xml version="1.0"?><component><releases><release version="157.0.8103.78" date="2026-11-01"/><release version="156.0"/></releases></component>`,
		"/home/u/.local/share/flatpak/exports/bin/com.brave.Browser":                                                          "exe:",
		"/home/u/.local/share/flatpak/app/com.brave.Browser/current/active/files/share/appdata/com.brave.Browser.appdata.xml": `<component><releases><release version="1.90.1" /></releases></component>`,
		"/var/lib/flatpak/exports/share/applications/org.chromium.Chromium.desktop":                                           "[Desktop Entry]\nExec=/usr/bin/flatpak run --branch=stable --arch=x86_64 --command=/app/bin/chromium --file-forwarding org.chromium.Chromium @@u %U @@\n",
	}), linuxEnv)
	if b := got["chromium"]; b.Source != SourceFlatpak || b.Version != "157.0.8103.78" || b.Executable != "/var/lib/flatpak/exports/bin/org.chromium.Chromium" || !hasNote(b, "flatpak") {
		t.Errorf("chromium = %+v; want the system flatpak with its metainfo version", b)
	}
	if b := got["brave"]; b.Source != SourceFlatpak || b.Version != "1.90.1" {
		t.Errorf("brave = %+v; want the user flatpak with its appdata version", b)
	}
}

func TestLinuxWebKitGTK(t *testing.T) {
	for name, files := range map[string]map[string]string{
		"fedora/alma 6.0": {
			"/usr/libexec/webkitgtk-6.0/MiniBrowser":     "exe:",
			"/usr/libexec/webkitgtk-6.0/WebKitWebDriver": "exe:",
		},
		"debian 4.1": {
			"/usr/lib/x86_64-linux-gnu/webkit2gtk-4.1/MiniBrowser": "exe:",
			"/usr/bin/WebKitWebDriver":                             "exe:",
			"/var/lib/dpkg/status":                                 "Package: libwebkit2gtk-4.1-0\nVersion: 2.50.4-1~deb13u1\n\n",
		},
	} {
		t.Run(name, func(t *testing.T) {
			b, ok := find(t, tree(files), linuxEnv)["webkitgtk"]
			if !ok || b.Family != schema.FamilyWebKit || b.Protocol != ProtocolClassic || b.Driver == "" || !hasNote(b, "display") {
				t.Errorf("webkitgtk = %+v (found %v); want WebKit over Classic with a driver and the display note", b, ok)
			}
		})
	}
	b := find(t, tree(map[string]string{"/usr/libexec/webkitgtk-6.0/MiniBrowser": "exe:"}), linuxEnv)["webkitgtk"]
	if !hasNote(b, "WebKitWebDriver") {
		t.Errorf("without a driver: %+v; want it listed with a note, not dropped", b)
	}
}

// An unreadable version is "" with a note, never a launch.
func TestLinuxUnknownVersion(t *testing.T) {
	b := find(t, tree(map[string]string{"/usr/bin/microsoft-edge-stable": "exe:"}), linuxEnv)["edge"]
	if b.ID != "edge" || b.Version != "" || !hasNote(b, "version") {
		t.Errorf("edge = %+v; want an empty version with a note", b)
	}
}

func TestNotExecutableIsIgnored(t *testing.T) {
	if got := find(t, tree(map[string]string{"/usr/bin/chromium": "not executable"}), linuxEnv); len(got) != 0 {
		t.Errorf("got %v; a file without exec bits is not a browser", got)
	}
}

// Discovery must never launch a browser (SPEC-discovery.md, "Never"). The OS
// lookup helpers are the exception: mdfind, reg query and
// `safaridriver --diagnose` answer questions about the machine.
func TestDiscoveryNeverLaunchesBrowsers(t *testing.T) {
	allowed := map[string]bool{
		"platform_darwin.go":    true, // mdfind, safaridriver --diagnose
		"winversion_windows.go": true, // reg query, PowerShell FileVersionInfo
	}
	files, _ := filepath.Glob("*.go")
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), `"os/exec"`) && !strings.HasSuffix(f, "_test.go") && !allowed[f] {
			t.Errorf("%s imports os/exec", f)
		}
	}
}

func BenchmarkLinuxFixture(b *testing.B) {
	fsys := tree(map[string]string{
		"/usr/bin/chromium": "exe:",
		"/usr/bin/firefox":  "-> /usr/lib/firefox/firefox", "/usr/lib/firefox/firefox": "exe:",
		"/usr/lib/firefox/application.ini":            "[App]\nVersion=160.0\n",
		"/usr/share/applications/chromium.desktop":    "[Desktop Entry]\nExec=chromium %U\n",
		"/usr/libexec/webkitgtk-6.0/MiniBrowser":      "exe:",
		"/usr/libexec/webkitgtk-6.0/WebKitWebDriver":  "exe:",
		"/var/lib/pacman/local/chromium-157.0-1/desc": "%VERSION%\n157.0-1\n",
	})
	opts := Options{FS: fsys, Getenv: func(k string) string { return linuxEnv[k] }}
	for b.Loop() {
		if _, err := find0(context.Background(), opts, linuxSources()); err != nil {
			b.Fatal(err)
		}
	}
}

func hasNote(b Browser, sub string) bool {
	for _, n := range b.Notes {
		if strings.Contains(n, sub) {
			return true
		}
	}
	return false
}

func sameBrowser(a, b Browser) bool {
	return a.ID == b.ID && a.Name == b.Name && a.Family == b.Family && a.Protocol == b.Protocol &&
		a.Executable == b.Executable && a.Driver == b.Driver && a.Version == b.Version && a.Source == b.Source
}
