package discovery

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/GerMP21/Omnisnap/internal/schema"
)

// windowsFixtures loads the recorded `reg query ... /s` dumps: the registry is
// text, so the Windows sources are tested on every OS (Task 30).
func windowsFixtures(t *testing.T, names ...string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, name := range names {
		b, err := os.ReadFile(filepath.Join("../../testdata/discovery/windows", name+".txt"))
		if err != nil {
			t.Fatal(err)
		}
		out[name] = string(b)
	}
	return out
}

// winReg returns a Registry lookup over the fixture dumps, keyed like reg.exe.
func winReg(t *testing.T, dumps map[string]string) func(string) (string, bool) {
	t.Helper()
	keys := map[string]string{
		appPathsKey + "chrome.exe":   "apppaths-chrome",
		appPathsKey + "chromium.exe": "apppaths-chromium",
		appPathsKey + "msedge.exe":   "apppaths-edge",
		appPathsKey + "brave.exe":    "apppaths-brave",
		appPathsKey + "vivaldi.exe":  "apppaths-vivaldi",
		appPathsKey + "firefox.exe":  "apppaths-firefox",
		appPathsKey + "IEXPLORE.EXE": "apppaths-iexplore",
		appPathsHKCU + "opera.exe":   "apppaths-opera",
		uninstallKey:                 "uninstall",
		uninstallHKCU:                "uninstall",
		uninstall32:                  "",
	}
	// Firefox's key is under HKLM; ESR reuses it.
	keys[appPathsKey+"firefox.exe"] = "apppaths-firefox"
	return func(key string) (string, bool) {
		name, ok := keys[key]
		if !ok || name == "" {
			return "", false
		}
		dump, ok := dumps[name]
		return dump, ok
	}
}

// winTree makes the executables the registry names, on a fake C: drive.
func winTree(paths ...string) fstest.MapFS {
	m := fstest.MapFS{}
	for _, p := range paths {
		m[strings.ReplaceAll(p, `\`, "/")] = &fstest.MapFile{Mode: 0o755}
	}
	return m
}

func findWindows(t *testing.T, fsys *fstest.MapFS, opts Options) map[string]Browser {
	t.Helper()
	if fsys == nil {
		fsys = &fstest.MapFS{}
	}
	opts.FS = fsys
	if opts.Getenv == nil {
		opts.Getenv = func(string) string { return "" }
	}
	got, err := find0(context.Background(), opts, windowsSources())
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Browser{}
	for _, b := range got {
		byID[b.ID] = b
	}
	return byID
}

func TestWindowsRegistryPitfalls(t *testing.T) {
	dumps := windowsFixtures(t, "apppaths-chrome", "apppaths-chromium", "apppaths-edge",
		"apppaths-brave", "apppaths-opera", "apppaths-vivaldi", "apppaths-firefox", "uninstall")
	fsys := winTree(
		`C:\Program Files\Google\Chrome\Application\chrome.exe`,
		`C:\Program Files\Chromium\Application\chromium.exe`,
		`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
		`C:\Program Files\BraveSoftware\Brave-Browser\Application\brave.exe`,
		`C:\Users\dev\AppData\Local\Programs\Opera GX\opera.exe`,
		`C:\Users\dev\AppData\Local\Vivaldi\Application\vivaldi.exe`,
		`C:\Program Files\Mozilla Firefox\firefox.exe`,
	)
	got := findWindows(t, &fsys, Options{Registry: winReg(t, dumps), FileVersion: func(string) string { return "0" }})

	want := map[string]string{
		"chrome":   "157.0.8103.78", // from the Uninstall key
		"chromium": "157.0.8103.0",  // its Uninstall key is a hash: enumeration finds it
		"edge":     "157.0.3405.86",
		"brave":    "1.83.112", // 32-bit hive (WOW6432Node)
		"opera":    "120.0.5543.93",
		"vivaldi":  "7.8.3925.71",
		"firefox":  "160.0",
	}
	for id, version := range want {
		b, ok := got[id]
		if !ok {
			t.Errorf("%s not found (found %v)", id, keys(got))
			continue
		}
		if b.Version != version {
			t.Errorf("%s version %q; want %q", id, b.Version, version)
		}
		if b.Source != SourceRegistry || !isAbsWindows(b.Executable) {
			t.Errorf("%s = %+v; want a registry source and an absolute executable", id, b)
		}
		if b.Family != schema.FamilyBlink && id != "firefox" {
			t.Errorf("%s family %q", id, b.Family)
		}
	}
	if got["firefox"].Protocol != ProtocolBiDi {
		t.Errorf("firefox protocol %q; want bidi", got["firefox"].Protocol)
	}
	// Opera GX installs opera.exe: the uninstall entry names the variant.
	if !strings.Contains(got["opera"].Name, "Opera GX") {
		t.Errorf("opera name %q; want the GX variant", got["opera"].Name)
	}
	// IEXPLORE is not an engine.
	if _, ok := got["iexplore"]; ok {
		t.Error("IEXPLORE discovered: Internet Explorer is not an engine")
	}
}

func keys(m map[string]Browser) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// Firefox ESR and Firefox share firefox.exe; the Uninstall entry decides.
func TestWindowsFirefoxESR(t *testing.T) {
	dumps := windowsFixtures(t, "apppaths-firefox", "uninstall-esr")
	fsys := winTree(`C:\Program Files\Mozilla Firefox\firefox.exe`)
	reg := winReg(t, dumps)
	got := findWindows(t, &fsys, Options{Registry: func(key string) (string, bool) {
		if key == uninstallKey || key == uninstall32 || key == uninstallHKCU {
			return dumps["uninstall-esr"], true
		}
		return reg(key)
	}})
	if _, ok := got["firefox"]; ok {
		t.Errorf("plain firefox discovered next to ESR: %+v", got["firefox"])
	}
	esr, ok := got["firefox-esr"]
	if !ok {
		t.Fatalf("firefox-esr not found: %v", keys(got))
	}
	if esr.Version != "140.9.0" || !strings.Contains(esr.Name, "ESR") {
		t.Errorf("esr = %+v; want the ESR version and name", esr)
	}
}

// A portable build has no Uninstall entry: the file's VERSIONINFO answers.
func TestWindowsFileVersionFallback(t *testing.T) {
	dumps := windowsFixtures(t, "apppaths-firefox", "uninstall")
	fsys := winTree(`C:\Program Files\Mozilla Firefox\firefox.exe`)
	got := findWindows(t, &fsys, Options{
		Registry:    winReg(t, dumps),
		FileVersion: func(abs string) string { return "161.0" },
	})
	if got["firefox"].Version != "160.0" {
		t.Errorf("firefox version %q; want the Uninstall DisplayVersion", got["firefox"].Version)
	}
	// Without the Uninstall entry (no uninstall dump), metadata answers.
	got = findWindows(t, &fsys, Options{
		Registry:    winReg(t, windowsFixtures(t, "apppaths-firefox")),
		FileVersion: func(abs string) string { return "161.0" },
	})
	if got["firefox"].Version != "161.0" {
		t.Errorf("portable firefox version %q; want the file's VERSIONINFO", got["firefox"].Version)
	}
}

// Portable browsers on PATH: no registry at all.
func TestWindowsPathSource(t *testing.T) {
	fsys := winTree(`C:\Portable\chrome.exe`, `C:\Portable\firefox.exe`)
	got := findWindows(t, &fsys, Options{
		Getenv:      func(k string) string { return map[string]string{"PATH": `C:\Windows;C:\Portable`}[k] },
		FileVersion: func(abs string) string { return "157.0" },
	})
	for _, id := range []string{"chrome", "firefox"} {
		if got[id].Source != SourcePath || got[id].Version != "157.0" {
			t.Errorf("%s = %+v; want a path source with the file version", id, got[id])
		}
	}
}
