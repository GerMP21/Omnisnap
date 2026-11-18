package capture

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GerMP21/Omnisnap/internal/schema"
)

// Task 27: the probes run after the main capture, one re-extract each, and are
// written gzipped with the applied probe recorded in their meta.
func TestProbesAreWritten(t *testing.T) {
	f := newFake()
	o := opts(t)
	if _, err := Page(context.Background(), f, o); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(o.Out, "p", "chrome-a.probes.json.gz")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	zr, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	var probes map[string]map[string]any
	if err := json.NewDecoder(zr).Decode(&probes); err != nil {
		t.Fatal(err)
	}
	for _, id := range probeIDs {
		c, ok := probes[id]
		if !ok {
			t.Errorf("probe %q missing", id)
			continue
		}
		meta, _ := c["meta"].(map[string]any)
		if meta == nil || meta["probe"] == nil {
			t.Errorf("probe %q: no meta.probe", id)
		}
		if _, err := os.Stat(filepath.Join(o.Out, "p", "chrome-a.json")); err != nil {
			t.Error(err)
		}
	}
	// The page is left reverted: the calls end with revert.
	if last := f.calls[len(f.calls)-1]; last != "probe revert" {
		t.Errorf("last call %q; want the page reverted", last)
	}
	c := readCapture(t, filepath.Join(o.Out, "p", "chrome-a.json"))
	if strings.Join(c.Meta.Probes.IDs, ",") != strings.Join(probeIDs, ",") || c.Meta.Probes.MS < 0 {
		t.Errorf("meta.probes = %+v", c.Meta.Probes)
	}
}

func TestProbesCanBeSkipped(t *testing.T) {
	f := newFake()
	o := opts(t)
	o.NoProbes = true
	if _, err := Page(context.Background(), f, o); err != nil {
		t.Fatal(err)
	}
	for _, c := range f.calls {
		if strings.HasPrefix(c, "probe") || c == "probes" {
			t.Errorf("probe call %q with NoProbes", c)
		}
	}
	if _, err := os.Stat(filepath.Join(o.Out, "p", "chrome-a.probes.json.gz")); !os.IsNotExist(err) {
		t.Errorf("probes file written with NoProbes: %v", err)
	}
	c := readCapture(t, filepath.Join(o.Out, "p", "chrome-a.json"))
	if got := c.Meta.Probes; got.IDs == nil || len(got.IDs) != 0 {
		t.Errorf("meta.probes = %+v; want an empty id list (the schema requires it)", got)
	}
}

func TestProbesProduceDeterministicBytes(t *testing.T) {
	o := opts(t)
	if _, err := Page(context.Background(), newFake(), o); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(filepath.Join(o.Out, "p", "chrome-a.probes.json.gz"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Page(context.Background(), newFake(), o); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(filepath.Join(o.Out, "p", "chrome-a.probes.json.gz"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Error("probes file is not deterministic (gzip mtime?)")
	}
}

var _ = schema.ProbesMeta{}
