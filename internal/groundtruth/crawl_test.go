package groundtruth

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func sites(n int) []Site {
	var out []Site
	for i := range n {
		out = append(out, Site{Slug: "t-s" + itoa(i) + ".com", URL: "https://s" + itoa(i) + ".com/", Rank: i + 1})
	}
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func TestCrawlPlanShardsAndLimit(t *testing.T) {
	plan := CrawlPlan(sites(10), CrawlOptions{Shards: 3, Shard: 1})
	if len(plan) != 3 || plan[0].Rank != 2 || plan[1].Rank != 5 {
		t.Errorf("shard 1: %+v; want ranks 2,5,8", plan)
	}
	if got := CrawlPlan(sites(10), CrawlOptions{Shards: 1, Limit: 4}); len(got) != 4 || got[3].Rank != 4 {
		t.Errorf("limit: %+v", got)
	}
}

// Kill and resume recaptures nothing done (Task 73).
func TestCrawlResumes(t *testing.T) {
	out := t.TempDir()
	all := sites(6)
	var captured []string
	capture := func(_ context.Context, s Site) error {
		captured = append(captured, s.Slug)
		return nil
	}
	// First run: killed after two sites (a context canceled by the capture).
	ctx, cancel := context.WithCancel(context.Background())
	killed := func(c context.Context, s Site) error {
		if len(captured) == 2 {
			cancel()
			return c.Err()
		}
		captured = append(captured, s.Slug)
		return nil
	}
	res, err := Crawl(ctx, all, CrawlOptions{Out: out}, killed, nil)
	if !errors.Is(err, context.Canceled) || res.Captured != 2 {
		t.Fatalf("killed crawl: %+v, %v", res, err)
	}
	// Resume: the two done sites are skipped.
	captured = nil
	res, err = Crawl(context.Background(), all, CrawlOptions{Out: out}, capture, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Captured != 4 || res.Skipped != 2 {
		t.Errorf("resume: %+v; want 4 captured, 2 skipped", res)
	}
	if len(captured) != 4 {
		t.Errorf("captured %v on resume; want only the missing sites", captured)
	}
}

// A failed capture is retried on the next run (it is not marked done).
func TestCrawlRetriesFailures(t *testing.T) {
	out := t.TempDir()
	var calls int
	fail := func(context.Context, Site) error {
		calls++
		return errors.New("capture failed")
	}
	res, err := Crawl(context.Background(), sites(2), CrawlOptions{Out: out}, fail, nil)
	if err != nil || res.Failed != 2 || res.Captured != 0 {
		t.Fatalf("failed crawl: %+v, %v", res, err)
	}
	calls = 0
	res, err = Crawl(context.Background(), sites(2), CrawlOptions{Out: out}, func(context.Context, Site) error {
		calls++
		return nil
	}, nil)
	if err != nil || res.Captured != 2 || res.Skipped != 0 {
		t.Errorf("retry: %+v, %v", res, err)
	}
}

// The disk guard stops the crawl before a capture and leaves it resumable.
func TestCrawlStopsOnLowDisk(t *testing.T) {
	out := t.TempDir()
	guard := DiskGuard{WarnGB: 20, StopGB: 5, Free: func(string) (uint64, error) {
		return 4 << 30, nil // 4 GB: below the floor
	}}
	captured := 0
	_, err := Crawl(context.Background(), sites(3), CrawlOptions{Out: out, Guard: guard},
		func(context.Context, Site) error { captured++; return nil }, nil)
	if err == nil || captured != 0 {
		t.Errorf("crawl ran below the floor: %d captures, err %v", captured, err)
	}
	// The warning zone still runs.
	guard.Free = func(string) (uint64, error) { return 10 << 30, nil }
	if _, err := Crawl(context.Background(), sites(2), CrawlOptions{Out: out, Guard: guard},
		func(context.Context, Site) error { captured++; return nil }, nil); err != nil || captured != 2 {
		t.Errorf("crawl in the warning zone: %d captures, %v", captured, err)
	}
}

func TestDoneMarker(t *testing.T) {
	out := t.TempDir()
	if Done(out, "t-x.com") {
		t.Error("undone site reported done")
	}
	if err := MarkDone(out, "t-x.com", "https://x.com/", "X"); err != nil {
		t.Fatal(err)
	}
	if !Done(out, "t-x.com") {
		t.Error("marker not read")
	}
	if _, err := os.Stat(filepath.Join(out, "t-x.com", "crawled.json")); err != nil {
		t.Error(err)
	}
}
