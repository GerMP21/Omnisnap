package capture

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/GerMP21/Omnisnap/assets/inject"
	"github.com/GerMP21/Omnisnap/internal/drivers"
	"github.com/GerMP21/Omnisnap/internal/fsutil"
	"github.com/GerMP21/Omnisnap/internal/schema"
)

// probeIDs are the causal probes of SPEC-capture.md (probes.js, unchanged):
// whole-pixel line heights, one font, neutral form controls, and all three.
var probeIDs = []string{"lh", "font", "controls", "all"}

// runProbes installs probes.js, re-extracts under each probe, and writes
// <base>.probes.json.gz. The page is left reverted: the capture's own files
// were already written from the un-probed state.
func runProbes(ctx context.Context, s drivers.Session, o Options, base string) (schema.ProbesMeta, error) {
	start := time.Now()
	out := make(map[string]any, len(probeIDs))
	if _, err := s.Evaluate(ctx, inject.Probes); err != nil {
		return schema.ProbesMeta{}, fmt.Errorf("probes: install: %w", err)
	}
	for _, id := range probeIDs {
		applied, err := s.Evaluate(ctx, fmt.Sprintf("window.__omnisnapProbe.apply(%q)", id))
		if err != nil {
			return schema.ProbesMeta{}, fmt.Errorf("probes: apply %s: %w", id, err)
		}
		raw, err := s.Evaluate(ctx, inject.Extract)
		if err != nil {
			return schema.ProbesMeta{}, fmt.Errorf("probes: extract under %s: %w", id, err)
		}
		var capture map[string]any
		if err := json.Unmarshal([]byte(raw), &capture); err != nil {
			return schema.ProbesMeta{}, fmt.Errorf("probes: %s capture: %w", id, err)
		}
		if meta, ok := capture["meta"].(map[string]any); ok {
			meta["probe"] = json.RawMessage(applied)
		}
		out[id] = capture
		if _, err := s.Evaluate(ctx, "window.__omnisnapProbe.revert()"); err != nil {
			return schema.ProbesMeta{}, fmt.Errorf("probes: revert %s: %w", id, err)
		}
	}
	b, err := json.Marshal(out)
	if err != nil {
		return schema.ProbesMeta{}, fmt.Errorf("probes: encode: %w", err)
	}
	// gzip with a zero mtime: the same page gives byte-identical files.
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	zw.ModTime = time.Time{}
	if _, err := zw.Write(b); err != nil {
		return schema.ProbesMeta{}, fmt.Errorf("probes: gzip: %w", err)
	}
	if err := zw.Close(); err != nil {
		return schema.ProbesMeta{}, fmt.Errorf("probes: gzip: %w", err)
	}
	if err := fsutil.WriteAtomic(base+".probes.json.gz", func(w io.Writer) error {
		_, err := w.Write(buf.Bytes())
		return err
	}); err != nil {
		return schema.ProbesMeta{}, err
	}
	return schema.ProbesMeta{IDs: probeIDs, MS: time.Since(start).Milliseconds()}, nil
}
