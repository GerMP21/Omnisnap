package process

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// StartBlinkTemplate launches a Blink browser from a copy of a warmed template
// profile (SPEC-branded.md, Task 65): every capture gets its own copy, so the
// template never accumulates cookies, cache or engagement scores.
func StartBlinkTemplate(ctx context.Context, exe, root, template string) (*Blink, error) {
	if _, err := os.Stat(template); err != nil {
		return nil, fmt.Errorf("process: template %s: %w", template, err)
	}
	h, err := spawn(ctx, root, exe, nil, func(d tempDir) []string {
		// A copy failure leaves an empty profile, which still works; the
		// browser's own error is more useful than a hard stop here.
		if err := CopyProfile(template, d.profile()); err != nil {
			_ = os.WriteFile(filepath.Join(d.dir, "template.error"), []byte(err.Error()), 0o600)
		}
		return blinkArgs(d.profile())
	})
	if err != nil {
		return nil, err
	}
	ws, err := h.waitDevTools(ctx)
	if err != nil {
		_ = h.Close(context.WithoutCancel(ctx))
		return nil, err
	}
	return &Blink{Handle: h, WSURL: ws}, nil
}

// CopyProfile copies a browser profile, skipping lock/socket files: a copied
// lock makes the next launch think the profile is in use.
func CopyProfile(src, dst string) error {
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
		// DevToolsActivePort names the port of the session that wrote it. A copy
		// would make the next launch dial the dead one (waitDevTools reads the
		// file before the browser rewrites it).
		if name == "DevToolsActivePort" {
			return nil
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o700)
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
		if _, err := io.Copy(out, in); err != nil {
			_ = out.Close()
			return err
		}
		return out.Close()
	})
}
