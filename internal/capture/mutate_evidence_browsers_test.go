//go:build browsers && unix

package capture

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/GerMP21/Omnisnap/internal/discovery"
	"github.com/GerMP21/Omnisnap/internal/drivers"
	"github.com/GerMP21/Omnisnap/internal/drivers/cdp"
	"github.com/GerMP21/Omnisnap/internal/groundtruth"
	"github.com/GerMP21/Omnisnap/internal/process"
	"github.com/GerMP21/Omnisnap/internal/schema"
)

// A mutant carries what a clean capture carries: the screenshot and the
// probes are taken while the bug is applied, so its causes have the same
// evidence as the run they are compared against (SPEC-ground-truth.md §2).
func TestMutantCaptureTakesEvidence(t *testing.T) {
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
	var closed func()
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
		closed = func() { _ = br.Close(); _ = proc.Close(context.Background()) }
		break
	}
	if sess == nil {
		t.Skip("no Blink browser installed")
	}
	defer closed()

	out := t.TempDir()
	engine := "blink"
	results, err := groundtruth.CaptureMutants(ctx, sess, groundtruth.MutateOptions{
		Out: out, BaseSlug: "03-overflow", URL: srv.URL("03-overflow"), Engine: engine,
		PerEngine: 1,
		Evidence: func(ctx context.Context, s drivers.Session, c *schema.Capture, base string) error {
			return Evidence(ctx, s, Options{Out: out, Slug: "03-overflow", Engine: engine, Run: "a"}, c, base)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Err != nil {
		t.Fatalf("results = %+v", results)
	}
	dir := filepath.Join(out, results[0].Mutant.Slug)
	for _, name := range []string{engine + "-a.png", engine + "-a.probes.json.gz"} {
		if fi, err := os.Stat(filepath.Join(dir, name)); err != nil || fi.Size() == 0 {
			t.Errorf("%s: %v", name, err)
		}
	}
	m := results[0].Capture.Meta
	if len(m.Probes.IDs) != 4 || m.Shot.W == 0 || m.Shot.H == 0 || m.Post.Nodes == 0 {
		t.Errorf("meta = probes %v, shot %+v, post %+v", m.Probes, m.Shot, m.Post)
	}
	if m.Mutation == nil {
		t.Error("the capture has no mutation marker")
	}
}
