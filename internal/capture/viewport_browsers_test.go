//go:build browsers && linux

package capture

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/GerMP21/Omnisnap/internal/discovery"
	"github.com/GerMP21/Omnisnap/internal/drivers"
	"github.com/GerMP21/Omnisnap/internal/drivers/bidi"
	"github.com/GerMP21/Omnisnap/internal/drivers/cdp"
	"github.com/GerMP21/Omnisnap/internal/drivers/classic"
	"github.com/GerMP21/Omnisnap/internal/groundtruth"
	"github.com/GerMP21/Omnisnap/internal/process"
	"github.com/GerMP21/Omnisnap/internal/schema"
)

// Task 24: every engine reports the same viewport on a planted page — 1280x800
// with the same client width (no scrollbar, no gutter). Without this, vh units
// and every vw-dependent line break differ for reasons the diff would blame on
// the page.
func TestViewportAcrossEngines(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	found, err := discovery.Find(ctx, discovery.Options{})
	if err != nil {
		t.Fatal(err)
	}
	srv, err := groundtruth.Start("../../testdata/planted")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = srv.Close() }()

	// One browser per family, driven through that family's protocol.
	open := map[schema.Family]func() (drivers.Session, func(), error){}
	for _, b := range found {
		b := b
		switch b.Family {
		case schema.FamilyBlink:
			if _, ok := open[b.Family]; ok {
				continue
			}
			open[b.Family] = func() (drivers.Session, func(), error) {
				p, err := process.StartBlink(ctx, b.Executable, "")
				if err != nil {
					return nil, nil, err
				}
				c, err := cdp.Dial(ctx, p.WSURL)
				if err != nil {
					_ = p.Close(context.Background())
					return nil, nil, err
				}
				s, err := c.NewSession(ctx)
				if err != nil {
					_ = p.Close(context.Background())
					return nil, nil, err
				}
				return s, func() { _ = c.Close(); _ = p.Close(context.Background()) }, nil
			}
		case schema.FamilyGecko:
			open[b.Family] = func() (drivers.Session, func(), error) {
				p, err := process.StartGecko(ctx, b.Executable, "")
				if err != nil {
					return nil, nil, err
				}
				c, err := bidi.Dial(ctx, p.WSURL)
				if err != nil {
					_ = p.Close(context.Background())
					return nil, nil, err
				}
				s, err := c.NewSession(ctx)
				if err != nil {
					_ = p.Close(context.Background())
					return nil, nil, err
				}
				return s, func() {
					_ = c.Close(context.Background())
					_ = p.Close(context.Background())
				}, nil
			}
		case schema.FamilyWebKit:
			if b.Driver == "" {
				continue
			}
			// WebKitGTK can only be sized inside a private display (weston):
			// without one it runs on the desktop at MiniBrowser's default.
			if _, err := exec.LookPath("weston"); err != nil || os.Getenv("OMNISNAP_WEBKIT_VIEWPORT") == "0" {
				continue
			}
			open[b.Family] = func() (drivers.Session, func(), error) {
				p, err := process.StartWebKit(ctx, process.WebKitOptions{Driver: b.Driver, Browser: b.Executable})
				if err != nil {
					return nil, nil, err
				}
				c := classic.New(p.URL)
				c.Capabilities = p.Capabilities
				if err := c.Status(ctx); err != nil {
					_ = p.Close(context.Background())
					return nil, nil, err
				}
				s, err := c.NewSession(ctx)
				if err != nil {
					_ = p.Close(context.Background())
					return nil, nil, err
				}
				return s, func() { _ = p.Close(context.Background()) }, nil
			}
		}
	}
	if len(open) == 0 {
		t.Skip("no engines installed")
	}

	type got struct {
		engine     string
		vw, cw, vh int
	}
	var results []got
	for family, start := range open {
		s, closeAll, err := start()
		if err != nil {
			t.Errorf("%s: %v", family, err)
			continue
		}
		out := t.TempDir()
		if _, err := Page(ctx, s, Options{Out: out, Slug: "01-card-width", URL: srv.URL("01-card-width"),
			Engine: string(family), Version: "test", Run: "a"}); err != nil {
			t.Errorf("%s: %v", family, err)
			_ = s.Close(context.Background())
			closeAll()
			continue
		}
		c := readCapture(t, out+"/01-card-width/"+string(family)+"-a.json")
		results = append(results, got{engine: string(family), vw: c.Meta.VW, cw: c.Meta.CW, vh: c.Meta.VH})
		t.Logf("%s: viewport %dx%d, client %d", string(family), c.Meta.VW, c.Meta.VH, c.Meta.CW)
		if err := s.Close(context.Background()); err != nil && !errors.Is(err, drivers.ErrCrashed) {
			t.Errorf("%s: close: %v", family, err)
		}
		closeAll()
	}
	for _, r := range results {
		if r.vw != 1280 || r.vh != 800 {
			t.Errorf("%s: viewport %dx%d; want 1280x800", r.engine, r.vw, r.vh)
		}
	}
	// Equal client widths: no engine reserves a scrollbar or gutter.
	for _, r := range results[1:] {
		if r.cw != results[0].cw {
			t.Errorf("%s clientWidth %d; %s has %d — the root scrollbar or gutter differs", r.engine, r.cw, results[0].engine, results[0].cw)
		}
	}
	if len(results) < 2 {
		t.Logf("only %d engine family installed: viewport equality not compared", len(results))
	}
}
