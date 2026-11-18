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

func darwinFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("../../testdata/discovery/darwin", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// XML and binary Info.plist give the same fields (Task 32).
func TestParsePlistForms(t *testing.T) {
	for _, tc := range []struct{ file, bundleID, version string }{
		{"chrome-Info.plist", "com.google.Chrome", "157.0.8103.78"},
		{"firefox-Info.plist", "org.mozilla.firefox", "160.0"},
	} {
		info, err := parsePlist(darwinFixture(t, tc.file))
		if err != nil {
			t.Fatalf("%s: %v", tc.file, err)
		}
		if got, _ := info["CFBundleIdentifier"].(string); got != tc.bundleID {
			t.Errorf("%s: bundle id %q; want %q", tc.file, got, tc.bundleID)
		}
		if got, _ := info["CFBundleShortVersionString"].(string); got != tc.version {
			t.Errorf("%s: version %q; want %q", tc.file, got, tc.version)
		}
	}
	// The binary plist also carries an array: the decoder must not choke.
	info, err := parsePlist(darwinFixture(t, "firefox-Info.plist"))
	if err != nil {
		t.Fatal(err)
	}
	if arch, ok := info["LSArchitecturePriority"].([]plistValue); !ok || len(arch) != 2 || arch[0] != "arm64" {
		t.Errorf("LSArchitecturePriority = %v", info["LSArchitecturePriority"])
	}
}

// darwinTree places files on a fake root: pairs of path and content, where
// content "exe:" marks an executable and "dir" a directory.
func darwinTree(entries ...string) fstest.MapFS {
	m := fstest.MapFS{}
	for i := 0; i+1 < len(entries); i += 2 {
		f := &fstest.MapFile{Data: []byte(entries[i+1])}
		if entries[i+1] == "exe:" {
			f.Data, f.Mode = nil, 0o755
		}
		m[strings.TrimPrefix(entries[i], "/")] = f
	}
	return m
}

func findDarwin(t *testing.T, fsys fstest.MapFS, opts Options) map[string]Browser {
	t.Helper()
	opts.FS = fsys
	if opts.Getenv == nil {
		opts.Getenv = func(k string) string {
			if k == "HOME" {
				return "/Users/dev"
			}
			return ""
		}
	}
	got, err := find0(context.Background(), opts, darwinSources())
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Browser{}
	for _, b := range got {
		byID[b.ID] = b
	}
	return byID
}

func TestDarwinBundleSource(t *testing.T) {
	chrome := string(darwinFixture(t, "chrome-Info.plist"))
	fsys := darwinTree(
		"/Applications/Google Chrome.app/Contents/Info.plist", chrome,
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome", "exe:",
	)
	got := findDarwin(t, fsys, Options{Registry: nil})
	chromeB, ok := got["chrome"]
	if !ok {
		t.Fatalf("chrome not found: %v", keys(got))
	}
	if chromeB.Version != "157.0.8103.78" || chromeB.Source != SourceAppBundle || chromeB.Family != schema.FamilyBlink {
		t.Errorf("chrome = %+v", chromeB)
	}
	if !strings.HasSuffix(chromeB.Executable, "/Contents/MacOS/Google Chrome") {
		t.Errorf("executable %q; want the bundle's MacOS binary", chromeB.Executable)
	}
}

// Spotlight finds a bundle installed outside /Applications.
func TestDarwinMDFind(t *testing.T) {
	firefox := string(darwinFixture(t, "firefox-Info.plist"))
	fsys := darwinTree(
		"/Users/dev/Apps/Firefox.app/Contents/Info.plist", firefox,
		"/Users/dev/Apps/Firefox.app/Contents/MacOS/firefox", "exe:",
	)
	got := findDarwin(t, fsys, Options{MDFind: func(query string) []string {
		if strings.Contains(query, "org.mozilla.firefox") {
			return []string{"/Users/dev/Apps/Firefox.app/"}
		}
		return nil
	}})
	ff, ok := got["firefox"]
	if !ok {
		t.Fatalf("firefox not found via mdfind: %v", keys(got))
	}
	if ff.Version != "160.0" || ff.Protocol != ProtocolBiDi {
		t.Errorf("firefox = %+v", ff)
	}
	if !strings.HasSuffix(ff.Executable, "/Contents/MacOS/firefox") {
		t.Errorf("executable %q", ff.Executable)
	}
}

// Safari is discovered with safaridriver; an unenabled driver is a note, not
// a missing browser (StartSafari names the fix).
func hasDarwinNote(b Browser, substr string) bool {
	for _, n := range b.Notes {
		if strings.Contains(n, substr) {
			return true
		}
	}
	return false
}

func TestDarwinSafari(t *testing.T) {
	safari := string(darwinFixture(t, "safari-Info.plist"))
	fsys := darwinTree(
		"/Applications/Safari.app/Contents/Info.plist", safari,
		"/Applications/Safari.app/Contents/MacOS/Safari", "exe:",
		"/usr/bin/safaridriver", "exe:",
	)
	for _, tc := range []struct {
		enabled  bool
		wantNote bool
	}{{false, true}, {true, false}} {
		got := findDarwin(t, fsys, Options{SafariEnabled: func() bool { return tc.enabled }})
		s, ok := got["safari"]
		if !ok {
			t.Fatalf("safari not found: %v", keys(got))
		}
		if s.Driver != "/usr/bin/safaridriver" || s.Protocol != ProtocolClassic {
			t.Errorf("safari = %+v", s)
		}
		if has := hasDarwinNote(s, "not enabled"); has != tc.wantNote {
			t.Errorf("enabled=%v: driver note %v (notes %v)", tc.enabled, has, s.Notes)
		}
	}
	// Without safaridriver the browser cannot be driven at all.
	got := findDarwin(t, darwinTree(
		"/Applications/Safari.app/Contents/Info.plist", safari,
		"/Applications/Safari.app/Contents/MacOS/Safari", "exe:",
	), Options{})
	if s := got["safari"]; s.Driver != "" || !hasDarwinNote(s, "safaridriver not found") {
		t.Errorf("safari without a driver = %+v", s)
	}
}
