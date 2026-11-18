//go:build browsers && unix

package capture

import (
	"context"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/GerMP21/Omnisnap/internal/discovery"
	"github.com/GerMP21/Omnisnap/internal/drivers"
	"github.com/GerMP21/Omnisnap/internal/drivers/cdp"
	"github.com/GerMP21/Omnisnap/internal/groundtruth"
	"github.com/GerMP21/Omnisnap/internal/process"
	"github.com/GerMP21/Omnisnap/internal/schema"
)

// probeBudget is Task 27's cost gate: parity probes add at most this much to a
// capture on the planted suite (spike: well under it).
const probeBudget = 1500 * time.Millisecond

// Task 27: the causal probes add at most 1.5 s (median) to a capture on the
// planted suite.
func TestProbeCost(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	srv, err := groundtruth.Start("../../testdata/planted")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = srv.Close() }()

	found, err := discovery.Find(ctx, discovery.Options{})
	if err != nil {
		t.Fatal(err)
	}
	var sess drivers.Session
	var closeAll func()
	for _, b := range found {
		if b.Family != schema.FamilyBlink {
			continue
		}
		proc, err := process.StartBlink(ctx, b.Executable, "")
		if err != nil {
			t.Fatalf("start Blink: %v", err)
		}
		br, err := cdp.Dial(ctx, proc.WSURL)
		if err != nil {
			t.Fatalf("dial CDP: %v", err)
		}
		if sess, err = br.NewSession(ctx); err != nil {
			t.Fatal(err)
		}
		closeAll = func() { _ = br.Close(); _ = proc.Close(context.Background()) }
		break
	}
	if sess == nil {
		t.Skip("no Blink browser installed")
	}
	defer closeAll()

	out := t.TempDir()
	var samples []time.Duration
	for _, page := range []string{"01-card-width", "03-overflow", "07-font", "11-clip", "19-list-rhythm"} {
		if _, err := Page(ctx, sess, Options{Out: out, Slug: page, URL: srv.URL(page),
			Engine: "blink", Version: "test", Run: "a"}); err != nil {
			t.Fatalf("%s: %v", page, err)
		}
		c := readCapture(t, filepath.Join(out, page, "blink-a.json"))
		if len(c.Meta.Probes.IDs) != 4 {
			t.Errorf("%s: probes %v", page, c.Meta.Probes.IDs)
		}
		samples = append(samples, time.Duration(c.Meta.Probes.MS)*time.Millisecond)
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	median := samples[len(samples)/2]
	t.Logf("probe cost median %v over %d pages", median, len(samples))
	if median > probeBudget {
		t.Errorf("probe median %v; want <= %v", median, probeBudget)
	}
}
