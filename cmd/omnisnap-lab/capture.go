package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/GerMP21/Omnisnap/internal/capture"
	"github.com/GerMP21/Omnisnap/internal/discovery"
	"github.com/GerMP21/Omnisnap/internal/drivers"
	"github.com/GerMP21/Omnisnap/internal/drivers/bidi"
	"github.com/GerMP21/Omnisnap/internal/drivers/cdp"
	"github.com/GerMP21/Omnisnap/internal/groundtruth"
	"github.com/GerMP21/Omnisnap/internal/process"
	"github.com/GerMP21/Omnisnap/internal/schema"
)

// labEngine is one running browser for the crawl.
type labEngine struct {
	id      string
	version string
	new     func(ctx context.Context) (drivers.Session, error)
	close   func()
}

// newCapturer launches one browser per installed family (the product's
// pipeline: discovery, process, drivers, capture). The caller closes it when
// the crawl ends.
func newCapturer(out, engines string) (groundtruth.CrawlSite, func(), error) {
	ctx := context.Background()
	found, err := discovery.Find(ctx, discovery.Options{})
	if err != nil {
		return nil, nil, err
	}
	wanted := map[string]bool{}
	for _, e := range strings.Split(engines, ",") {
		if e = strings.TrimSpace(e); e != "" {
			wanted[e] = true
		}
	}
	var running []labEngine
	closer := func() {
		for i := len(running) - 1; i >= 0; i-- {
			running[i].close()
		}
	}
	seen := map[schema.Family]bool{}
	for _, b := range found {
		if seen[b.Family] {
			continue
		}
		if len(wanted) > 0 && !wanted[b.ID] && !wanted[string(b.Family)] {
			continue
		}
		e, err := launchLabEngine(ctx, b)
		if err != nil {
			// A family that cannot start is a note: the crawl goes on with
			// the others.
			fmt.Printf("  %s: %v (skipped)\n", b.ID, err)
			continue
		}
		seen[b.Family] = true
		running = append(running, e)
	}
	if len(running) == 0 {
		return nil, nil, fmt.Errorf("no browser could be launched")
	}
	captureSite := func(ctx context.Context, s groundtruth.Site) error {
		for _, e := range running {
			sess, err := e.new(ctx)
			if err != nil {
				return fmt.Errorf("%s: %w", e.id, err)
			}
			_, capErr := capture.Page(ctx, sess, capture.Options{
				Out: out, Slug: s.Slug, URL: s.URL, Engine: e.id, Version: e.version, Run: "a",
			})
			_ = sess.Close(context.WithoutCancel(ctx))
			if capErr != nil {
				return fmt.Errorf("%s: %w", e.id, capErr)
			}
		}
		return nil
	}
	return captureSite, closer, nil
}

func launchLabEngine(ctx context.Context, b discovery.Browser) (labEngine, error) {
	switch b.Family {
	case schema.FamilyGecko:
		p, err := process.StartGecko(ctx, b.Executable, "")
		if err != nil {
			return labEngine{}, err
		}
		c, err := bidi.Dial(ctx, p.WSURL)
		if err != nil {
			_ = p.Close(context.WithoutCancel(ctx))
			return labEngine{}, err
		}
		return labEngine{id: b.ID, version: b.Version,
			new:   func(ctx context.Context) (drivers.Session, error) { return c.NewSession(ctx) },
			close: func() { _ = c.Close(context.Background()); _ = p.Close(context.Background()) }}, nil
	case schema.FamilyWebKit:
		return launchLabWebKit(ctx, b)
	default:
		p, err := process.StartBlink(ctx, b.Executable, "")
		if err != nil {
			return labEngine{}, err
		}
		c, err := cdp.Dial(ctx, p.WSURL)
		if err != nil {
			_ = p.Close(context.WithoutCancel(ctx))
			return labEngine{}, err
		}
		return labEngine{id: b.ID, version: b.Version,
			new:   func(ctx context.Context) (drivers.Session, error) { return c.NewSession(ctx) },
			close: func() { _ = c.Close(); _ = p.Close(context.Background()) }}, nil
	}
}
