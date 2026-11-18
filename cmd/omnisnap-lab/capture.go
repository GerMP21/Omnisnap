package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

// engineSlot is one engine's browser for the crawl; dead marks a browser that
// crashed, so the next page relaunches it instead of failing forever.
type engineSlot struct {
	b    discovery.Browser
	e    labEngine
	dead bool
}

// newCapturer launches one browser per installed family (the product's
// pipeline: discovery, process, drivers, capture). The caller closes it when
// the crawl ends. A browser that crashes is relaunched and the page retried
// once: over hundreds of sites a single crash must not end the run.
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
	var slots []*engineSlot
	closer := func() {
		for i := len(slots) - 1; i >= 0; i-- {
			if slots[i].e.close != nil {
				slots[i].e.close()
			}
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
		slots = append(slots, &engineSlot{b: b, e: e})
	}
	if len(slots) == 0 {
		return nil, nil, fmt.Errorf("no browser could be launched")
	}
	// Two runs per engine: run b is the dynamic mask, and the mutant lab needs
	// both before it can label a page.
	captureWith := func(sess drivers.Session, e labEngine, s groundtruth.Site) error {
		for _, run := range []string{"a", "b"} {
			if _, err := capture.Page(ctx, sess, capture.Options{
				Out: out, Slug: s.Slug, URL: s.URL, Engine: e.id, Version: e.version, Run: run,
			}); err != nil {
				return err
			}
		}
		return nil
	}
	relaunch := func(slot *engineSlot) error {
		if slot.e.close != nil {
			slot.e.close()
		}
		e, err := launchLabEngine(ctx, slot.b)
		if err != nil {
			slot.dead = true
			return err
		}
		slot.e, slot.dead = e, false
		return nil
	}
	captureSite := func(ctx context.Context, s groundtruth.Site) error {
		for _, slot := range slots {
			if slot.dead {
				if err := relaunch(slot); err != nil {
					return fmt.Errorf("%s: %w", slot.b.ID, err)
				}
			}
			sess, err := slot.e.new(ctx)
			var capErr error
			if err == nil {
				capErr = captureWith(sess, slot.e, s)
				_ = sess.Close(context.WithoutCancel(ctx))
			}
			if err == nil && capErr == nil {
				continue
			}
			// The browser died between pages (a crash closes the connection):
			// relaunch it and try this page once more.
			if err := relaunch(slot); err != nil {
				return fmt.Errorf("%s: %w", slot.b.ID, err)
			}
			sess, err = slot.e.new(ctx)
			if err != nil {
				slot.dead = true
				return fmt.Errorf("%s: %w", slot.b.ID, err)
			}
			capErr = captureWith(sess, slot.e, s)
			_ = sess.Close(context.WithoutCancel(ctx))
			if capErr != nil {
				slot.dead = true
				return fmt.Errorf("%s: %w", slot.b.ID, capErr)
			}
		}
		// page.json is what diff, mutate and dataset key on; the CLI's bundle
		// writer makes it, so the lab must too.
		meta := map[string]string{"slug": s.Slug, "url": s.URL, "group": "crawl"}
		b, _ := json.Marshal(meta)
		if err := os.WriteFile(filepath.Join(out, s.Slug, "page.json"), append(b, '\n'), 0o644); err != nil {
			return fmt.Errorf("page.json: %w", err)
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
