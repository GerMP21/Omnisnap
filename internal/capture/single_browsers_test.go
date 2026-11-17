//go:build browsers && unix

package capture

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/GerMP21/Omnisnap/internal/discovery"
	"github.com/GerMP21/Omnisnap/internal/drivers/cdp"
	"github.com/GerMP21/Omnisnap/internal/groundtruth"
	"github.com/GerMP21/Omnisnap/internal/process"
	"github.com/GerMP21/Omnisnap/internal/schema"
)

// One planted page in a real Blink browser, through process, cdp and capture.
func TestSingleBlinkCapture(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	found, err := discovery.Find(ctx, discovery.Options{})
	if err != nil {
		t.Fatal(err)
	}
	var browser *discovery.Browser
	for i := range found {
		if found[i].Family == schema.FamilyBlink {
			browser = &found[i]
			break
		}
	}
	if browser == nil {
		t.Skip("no Blink browser installed")
	}

	srv, err := groundtruth.Start("../../testdata/planted")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = srv.Close() }()

	proc, err := process.StartBlink(ctx, browser.Executable, "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = proc.Close(context.Background()) }()
	b, err := cdp.Dial(ctx, proc.WSURL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = b.Close() }()
	s, err := b.NewSession(ctx)
	if err != nil {
		t.Fatal(err)
	}

	out := t.TempDir()
	res, err := Page(ctx, s, Options{Out: out, Slug: "01-card-width", URL: srv.URL("01-card-width"),
		Engine: browser.ID, Version: browser.Version, Run: "a"})
	if err != nil {
		t.Fatal(err)
	}
	c := readCapture(t, filepath.Join(out, "01-card-width", browser.ID+"-a.json"))
	if c.Meta.VW != 1280 || c.Meta.VH != 800 {
		t.Errorf("viewport %dx%d; want 1280x800", c.Meta.VW, c.Meta.VH)
	}
	if c.Meta.Post.Nodes != c.Meta.Nodes || c.Meta.Post.DocH != c.Meta.DocH {
		t.Errorf("page changed during capture: %+v vs %d nodes, docH %d", c.Meta.Post, c.Meta.Nodes, c.Meta.DocH)
	}
	target := findID(c.Root, "target")
	if target == nil || math.Abs(target.B[2]-329.3) > 0.1 {
		t.Errorf("#target = %+v; want the golden width 329.3 (a third of the grid, fonts aside)", target)
	}
	if _, err := os.Stat(filepath.Join(out, "01-card-width", browser.ID+"-a.png")); err != nil {
		t.Error(err)
	}
	t.Logf("%s %s: %d nodes in %d ms, docH %d", browser.ID, browser.Version, res.Nodes, res.MS, c.Meta.DocH)
}

func findID(n schema.Node, id string) *schema.Node {
	if n.ID == id {
		return &n
	}
	for _, k := range n.K {
		if f := findID(k, id); f != nil {
			return f
		}
	}
	return nil
}
