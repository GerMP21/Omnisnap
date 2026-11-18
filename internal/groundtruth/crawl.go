package groundtruth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/GerMP21/Omnisnap/internal/fsutil"
)

// Crawl planning and resume (Task 73): limited, sharded, resumable, and
// guarded by free disk space. The capture itself is injected, so the policy is
// testable without a browser or the network.

// CrawlOptions describe one lab crawl.
type CrawlOptions struct {
	Out    string
	Limit  int // 0: every site of this shard
	Shards int // 1: no sharding
	Shard  int // 0-based
	Guard  DiskGuard
}

// CrawlPlan selects the shard and the limit, in rank order.
func CrawlPlan(sites []Site, o CrawlOptions) []Site {
	shards := o.Shards
	if shards < 1 {
		shards = 1
	}
	var sel []Site
	for i, s := range SortedSites(append([]Site{}, sites...)) {
		if i%shards != o.Shard {
			continue
		}
		sel = append(sel, s)
		if o.Limit > 0 && len(sel) >= o.Limit {
			break
		}
	}
	return sel
}

// marker is <out>/<slug>/crawled.json: written after a site's captures.
type marker struct {
	URL   string `json:"url"`
	Title string `json:"title,omitempty"`
}

// Done reports whether the site already has a completed crawl.
func Done(out, slug string) bool {
	_, err := os.Stat(filepath.Join(out, slug, "crawled.json"))
	return err == nil
}

// MarkDone records a completed site.
func MarkDone(out, slug, url, title string) error {
	dir := filepath.Join(out, slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("groundtruth: %w", err)
	}
	return fsutil.WriteAtomic(filepath.Join(dir, "crawled.json"), func(w io.Writer) error {
		return json.NewEncoder(w).Encode(marker{URL: url, Title: title})
	})
}

// CrawlResult is the lab crawl's tally.
type CrawlResult struct {
	Captured, Skipped, Failed int
}

// CrawlSite captures one site's pages. It returns an error for a failed
// capture (the site is then not marked done, so --resume retries it).
type CrawlSite func(ctx context.Context, s Site) error

// Crawl walks the plan: skipped sites already done, a disk check before each
// capture (a stop leaves everything resumable), sharding and limits handled by
// CrawlPlan. A capture failure is counted, not fatal.
func Crawl(ctx context.Context, sites []Site, o CrawlOptions, capture CrawlSite, logf func(format string, args ...any)) (CrawlResult, error) {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	if err := EnsureOut(o.Out); err != nil {
		return CrawlResult{}, err
	}
	var res CrawlResult
	for i, s := range sites {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		if Done(o.Out, s.Slug) {
			res.Skipped++
			continue
		}
		d := o.Guard.Check(o.Out)
		if d.Err != nil {
			return res, d.Err
		}
		if d.Stop {
			return res, fmt.Errorf("groundtruth: crawl stopped: %.1f GB free, below the %.0f GB floor (everything done is resumable)", d.FreeGB, maxF(o.Guard.StopGB, 5))
		}
		if d.Warn {
			logf("warning: %.1f GB free; the crawl stops below %.0f GB", d.FreeGB, maxF(o.Guard.StopGB, 5))
		}
		logf("[%d/%d] %s", i+1, len(sites), s.URL)
		if err := capture(ctx, s); err != nil {
			res.Failed++
			logf("  failed: %v", err)
			continue
		}
		if err := MarkDone(o.Out, s.Slug, s.URL, s.Title); err != nil {
			return res, err
		}
		res.Captured++
	}
	return res, nil
}

func maxF(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
