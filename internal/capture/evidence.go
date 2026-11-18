package capture

import (
	"context"
	"fmt"
	"io"
	"math"

	"github.com/GerMP21/Omnisnap/assets/inject"
	"github.com/GerMP21/Omnisnap/internal/drivers"
	"github.com/GerMP21/Omnisnap/internal/fsutil"
	"github.com/GerMP21/Omnisnap/internal/schema"
)

// Evidence measures what the pixels and the probes say about the page as it is
// right now, and writes <base>.png and <base>.probes.json.gz:
//
//   - the full-page screenshot, capped at maxShotW/maxShotH;
//   - a re-measure, to detect a page that moved while it was captured;
//   - the causal probes (skipped with NoProbes), each a re-extract under one
//     neutralization.
//
// It fills the capture's Shot, Post, Probes and (when the window could not be
// sized) ShotNote. Page calls it for a clean capture; the mutation lab calls it
// while a bug is injected, so a mutant carries the same evidence as the clean
// run it will be compared against (SPEC-ground-truth.md §2).
func Evidence(ctx context.Context, s drivers.Session, o Options, c *schema.Capture, base string) error {
	w := min(int(math.Ceil(math.Max(float64(c.Meta.VW), float64(c.Meta.DocW)))), maxShotW)
	h := min(c.Meta.DocH, maxShotH)
	stepCtx, cancel := context.WithTimeout(ctx, o.scriptTimeout())
	png, err := s.Screenshot(stepCtx, drivers.Rect{Width: float64(w), Height: float64(h)})
	cancel()
	if err != nil {
		return fmt.Errorf("screenshot: %w", err)
	}
	var post schema.Capture
	if err := evaluateJSON(ctx, s, o, inject.Extract, &post); err != nil {
		return fmt.Errorf("re-measure: %w", err)
	}
	probes := schema.ProbesMeta{IDs: []string{}}
	if !o.NoProbes {
		probeCtx, cancelProbes := context.WithTimeout(ctx, o.scriptTimeout())
		probes, err = runProbes(probeCtx, s, o, base)
		cancelProbes()
		if err != nil {
			return err
		}
	}
	m := &c.Meta
	// Assert the viewport the page really got (Task 24): a window that could
	// not be sized changes every vh unit and vw line break, so it is recorded
	// in the capture instead of being silently measured.
	if vp := o.viewport(); m.VW != vp.Width || m.VH != vp.Height {
		m.ShotNote = fmt.Sprintf("viewport could only be sized to %dx%d", m.VW, m.VH)
	}
	m.Post = schema.Post{DocH: post.Meta.DocH, Nodes: post.Meta.Nodes}
	m.Probes = probes
	m.Shot = schema.Shot{W: w, H: h}
	return fsutil.WriteAtomic(base+".png", func(f io.Writer) error {
		_, err := f.Write(png)
		return err
	})
}
