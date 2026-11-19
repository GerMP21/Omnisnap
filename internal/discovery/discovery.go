// Package discovery finds the installed browsers without launching any
// (SPEC-discovery.md): enough for process to launch each one and for the
// report to name it.
package discovery

import (
	"context"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/GerMP21/Omnisnap/internal/schema"
)

// Protocol is how an engine family is driven.
type Protocol string

// Protocols.
const (
	ProtocolCDP     Protocol = "cdp"     // Blink
	ProtocolBiDi    Protocol = "bidi"    // Gecko
	ProtocolClassic Protocol = "classic" // WebKit (safaridriver, WebKitWebDriver)
)

// Source says where a browser was found.
type Source string

// Sources, best first: when several find the same browser, the first wins.
const (
	SourceRegistry  Source = "registry"
	SourceAppBundle Source = "app-bundle"
	SourceDesktop   Source = "desktop-file"
	SourcePath      Source = "path"
	SourceSnap      Source = "snap"
	SourceFlatpak   Source = "flatpak"
	SourceCache     Source = "omnisnap-cache"
)

// Browser is one installed browser.
type Browser struct {
	ID         string        `json:"id"`   // "chrome", "chromium", "firefox-esr", "webkitgtk", ...
	Name       string        `json:"name"` // "Google Chrome"
	Family     schema.Family `json:"family"`
	Protocol   Protocol      `json:"protocol"`
	Executable string        `json:"executable"`       // absolute path, as installed (symlinks kept)
	Driver     string        `json:"driver,omitempty"` // WebDriver binary, for Classic
	Version    string        `json:"version"`          // "" when it cannot be read without launching
	Source     Source        `json:"source"`
	Notes      []string      `json:"notes,omitempty"` // restrictions the caller must respect
}

// Options configure Find; the zero value scans this machine.
type Options struct {
	FS     fs.FS               // the root file system; default os.DirFS("/")
	Getenv func(string) string // default os.Getenv

	// Windows: Registry returns `reg query <key> /s` output (recorded dumps
	// in tests); FileVersion reads a file's VERSIONINFO. Both default to the
	// real lookups on Windows and stay nil elsewhere.
	Registry    func(key string) (string, bool)
	FileVersion func(abs string) string

	// macOS: MDFind returns bundle paths for a Spotlight query; SafariEnabled
	// says whether safaridriver's one-time enable step has run. It has no
	// default: macOS cannot answer without starting the driver, and discovery
	// never launches anything (StartSafari reports a driver not enabled).
	MDFind        func(query string) []string
	SafariEnabled func() bool
}

// source is one place browsers are found. Sources return what they can and
// never fail the whole discovery.
type source interface {
	scan(ctx context.Context, e *env) []Browser
}

// Find returns every browser found on this machine, sorted by id. It never
// starts a browser: a version that cannot be read from files is "".
func Find(ctx context.Context, opts Options) ([]Browser, error) {
	return find0(ctx, opts, platformSources())
}

func find0(ctx context.Context, opts Options, sources []source) ([]Browser, error) {
	if opts.FS == nil {
		opts.FS = os.DirFS("/")
	}
	if opts.Getenv == nil {
		opts.Getenv = os.Getenv
	}
	platformDefaults(&opts)
	e := &env{fs: opts.FS, getenv: opts.Getenv, registry: opts.Registry, fileVersion: opts.FileVersion,
		mdfind: opts.MDFind, safariEnabled: opts.SafariEnabled}
	seen := map[string]bool{}
	var out []Browser
	for _, s := range sources {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		for _, b := range s.scan(ctx, e) {
			if seen[b.ID] {
				continue
			}
			seen[b.ID] = true
			if b.Version == "" {
				b.Version = e.version(b)
			}
			if b.Version == "" {
				b.Notes = append(b.Notes, "version unknown: not readable from files (application.ini, snap or flatpak metadata, dpkg or pacman database; rpm is not read)")
			}
			out = append(out, b)
		}
	}
	// OMNISNAP_WEBKIT picks the WebKit implementation when both are installed:
	// "wpe" (headless, exact viewport) or "gtk" (the GTK port). Unset lists
	// both; callers that keep one engine per family then prefer WebKitGTK.
	if mode := e.getenv("OMNISNAP_WEBKIT"); mode != "" {
		kept := out[:0]
		for _, b := range out {
			if b.Family == schema.FamilyWebKit {
				if mode == "wpe" && b.ID != "wpe" {
					continue
				}
				if mode == "gtk" && b.ID == "wpe" {
					continue
				}
			}
			kept = append(kept, b)
		}
		out = kept
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// env is the machine as discovery sees it.
type env struct {
	fs            fs.FS
	getenv        func(string) string
	registry      func(key string) (string, bool)
	fileVersion   func(abs string) string
	mdfind        func(query string) []string
	safariEnabled func() bool
	dpkg          map[string]string // package -> version, read once
}

// rel turns an absolute path into an fs.FS path.
func rel(abs string) string {
	p := strings.TrimPrefix(path.Clean("/"+abs), "/")
	if p == "" {
		return "."
	}
	return p
}

// resolve follows symlinks; absolute targets are taken from the file
// system's root. It returns the final absolute path and its info.
func (e *env) resolve(abs string) (string, fs.FileInfo, bool) {
	p := path.Clean("/" + abs)
	for range 40 {
		info, err := fs.Lstat(e.fs, rel(p))
		if err != nil {
			return "", nil, false
		}
		if info.Mode()&fs.ModeSymlink == 0 {
			return p, info, true
		}
		target, err := fs.ReadLink(e.fs, rel(p))
		if err != nil {
			return "", nil, false
		}
		if path.IsAbs(target) {
			p = path.Clean(target)
		} else {
			p = path.Join(path.Dir(p), target)
		}
	}
	return "", nil, false // a symlink loop
}

// executable reports whether abs (after symlinks) is an executable file.
func (e *env) executable(abs string) bool {
	_, info, ok := e.resolve(abs)
	return ok && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0
}

func (e *env) read(abs string) (string, bool) {
	b, err := fs.ReadFile(e.fs, rel(abs))
	return string(b), err == nil
}

// lookPath finds name in $PATH.
func (e *env) lookPath(name string) (string, bool) {
	for _, dir := range strings.Split(e.getenv("PATH"), ":") {
		if dir == "" || !path.IsAbs(dir) {
			continue
		}
		if p := path.Join(dir, name); e.executable(p) {
			return p, true
		}
	}
	return "", false
}

func (e *env) home() string { return e.getenv("HOME") }
