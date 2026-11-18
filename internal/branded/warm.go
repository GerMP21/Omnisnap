package branded

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/GerMP21/Omnisnap/internal/drivers/cdp"
	"github.com/GerMP21/Omnisnap/internal/process"
)

// WarmPages are neutral pages that trigger a branded browser's component and
// filter-list downloads. They are not research pages: only the downloads
// matter, and the template is a copy.
var WarmPages = []string{
	"https://example.com/",
	"https://www.wikipedia.org/",
}

// WarmOptions configure a template warm.
type WarmOptions struct {
	Browser string   // "brave", "edge", ...
	Exe     string   // the installed browser
	Version string   // its version, for the template name
	Cache   string   // the omnisnap cache root
	Pages   []string // default WarmPages
	Now     time.Time
	// Settle is how long each page gets for the blocking components to
	// download (the spike's warm waits; 8 s covers a slow runner).
	Settle time.Duration
	Logf   func(format string, args ...any)
}

// Warm builds a template profile (Task 65): launch the branded browser with a
// fresh profile, visit neutral pages so it downloads its blocking lists, and
// copy the profile into the cache. Every capture then uses a copy, so the
// template never accumulates state.
func Warm(ctx context.Context, o WarmOptions) (Template, error) {
	if o.Now.IsZero() {
		o.Now = time.Now()
	}
	if o.Settle == 0 {
		o.Settle = 8 * time.Second
	}
	if o.Logf == nil {
		o.Logf = func(string, ...any) {}
	}
	pages := o.Pages
	if len(pages) == 0 {
		pages = WarmPages
	}
	o.Logf("warming %s %s: %d page(s), about %s", o.Browser, o.Version, len(pages), time.Duration(len(pages))*o.Settle)
	p, err := process.StartBlink(ctx, o.Exe, "")
	if err != nil {
		return Template{}, err
	}
	defer func() { _ = p.Close(context.WithoutCancel(ctx)) }()
	c, err := cdp.Dial(ctx, p.WSURL)
	if err != nil {
		return Template{}, err
	}
	defer func() { _ = c.Close() }()
	s, err := c.NewSession(ctx)
	if err != nil {
		return Template{}, err
	}
	for _, pageURL := range pages {
		if err := s.Navigate(ctx, pageURL); err != nil {
			o.Logf("  %s: %v (kept warming)", pageURL, err)
			continue
		}
		if err := s.WaitNetworkIdle(ctx, 500*time.Millisecond, 20*time.Second); err != nil {
			o.Logf("  %s: network not idle: %v", pageURL, err)
		}
		select {
		case <-ctx.Done():
			return Template{}, ctx.Err()
		case <-time.After(o.Settle):
		}
	}
	_ = s.Close(ctx)
	dir := TemplateDir(o.Cache, o.Browser, o.Version)
	if err := os.RemoveAll(dir); err != nil {
		return Template{}, err
	}
	if err := copyProfile(p.ProfileDir(), dir); err != nil {
		return Template{}, err
	}
	// The template's age is when it was warmed.
	if err := os.Chtimes(dir, o.Now, o.Now); err != nil {
		return Template{}, err
	}
	o.Logf("template ready: %s", dir)
	return Template{Browser: o.Browser, Version: o.Version, Built: o.Now, Dir: dir}, nil
}

// copyProfile copies a profile, skipping singleton locks and sockets: a copied
// lock would make the next launch think the profile is in use.
func copyProfile(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if strings.HasPrefix(name, "Singleton") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		defer func() { _ = in.Close() }()
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
		if err != nil {
			return err
		}
		if _, err := out.ReadFrom(in); err != nil {
			_ = out.Close()
			return err
		}
		return out.Close()
	})
}

// VersionLine is the template's provenance line for host.json.
func (t Template) VersionLine() string {
	return fmt.Sprintf("%s %s warmed %s", t.Browser, t.Version, t.Built.UTC().Format(time.RFC3339))
}
