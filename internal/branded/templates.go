package branded

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Warmed template profiles (Task 65): a branded browser needs a profile whose
// blocking lists are downloaded; a fresh one behaves like Chrome. Templates
// live in the cache, are copied per capture, and are rebuilt when the browser's
// major version changes or they are older than 7 days.

// TemplateMaxAge is how long a warmed template stays usable.
const TemplateMaxAge = 7 * 24 * time.Hour

// Template is one warmed profile on disk.
type Template struct {
	Browser string
	Version string // the browser version it was warmed with
	Built   time.Time
	Dir     string
}

// Fresh reports whether a template is still usable for version now: same major
// version and younger than TemplateMaxAge.
func (t Template) Fresh(now time.Time, version string) bool {
	if t.Dir == "" {
		return false
	}
	if major(t.Version) != major(version) {
		return false
	}
	return now.Sub(t.Built) < TemplateMaxAge
}

// major is the browser's major version ("157.0.8103.78" -> "157").
func major(v string) string {
	for i := range len(v) {
		if v[i] == '.' {
			return v[:i]
		}
	}
	return v
}

// TemplateDir is where a browser's template lives: <cache>/templates/<browser>-<version>/.
func TemplateDir(cache, browser, version string) string {
	return filepath.Join(cache, "templates", fmt.Sprintf("%s-%s", browser, version))
}

// Find returns the newest template for a browser that is fresh for version;
// nil when none is (the caller warms one).
func Find(cache, browser, version string, now time.Time) (*Template, error) {
	root := filepath.Join(cache, "templates")
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	prefix := browser + "-"
	var best *Template
	for _, e := range entries {
		if !e.IsDir() || len(e.Name()) <= len(prefix) || e.Name()[:len(prefix)] != prefix {
			continue
		}
		dir := filepath.Join(root, e.Name())
		info, err := e.Info()
		if err != nil {
			continue
		}
		t := &Template{Browser: browser, Version: e.Name()[len(prefix):], Built: info.ModTime(), Dir: dir}
		if !t.Fresh(now, version) {
			continue
		}
		if best == nil || t.Built.After(best.Built) {
			best = t
		}
	}
	return best, nil
}

// Stale lists every template of a browser that is not fresh for version, for
// the sweeper/warm to remove.
func Stale(cache, browser, version string, now time.Time) ([]string, error) {
	root := filepath.Join(cache, "templates")
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, nil
	}
	var out []string
	prefix := browser + "-"
	for _, e := range entries {
		if !e.IsDir() || len(e.Name()) <= len(prefix) || e.Name()[:len(prefix)] != prefix {
			continue
		}
		dir := filepath.Join(root, e.Name())
		info, err := e.Info()
		if err != nil {
			continue
		}
		t := Template{Browser: browser, Version: e.Name()[len(prefix):], Built: info.ModTime(), Dir: dir}
		if !t.Fresh(now, version) {
			out = append(out, dir)
		}
	}
	return out, nil
}
