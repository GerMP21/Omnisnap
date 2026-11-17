package capture

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/GerMP21/Omnisnap/assets/inject"
	"github.com/GerMP21/Omnisnap/internal/drivers"
	"github.com/GerMP21/Omnisnap/internal/schema"
)

// Page captures one page in one engine for one run and writes
// <Out>/<Slug>/<Engine>-<Run>.{png,json}, or <Engine>-<Run>.error.txt when it
// fails. The PNG is written first: the JSON's presence means the capture is
// complete.
func Page(ctx context.Context, s drivers.Session, o Options) (Result, error) {
	dir := filepath.Join(o.Out, o.Slug)
	base := filepath.Join(dir, o.Engine+"-"+o.Run)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Result{}, fmt.Errorf("capture: %w", err)
	}
	res, err := page(ctx, s, o, base)
	if err != nil {
		err = fmt.Errorf("capture %s %s run %s: %w", o.Slug, o.Engine, o.Run, err)
		if werr := writeAtomic(base+".error.txt", func(f *os.File) error {
			_, e := f.WriteString(err.Error() + "\n")
			return e
		}); werr != nil {
			err = errors.Join(err, werr)
		}
	}
	return res, err
}

func page(ctx context.Context, s drivers.Session, o Options, base string) (Result, error) {
	start := time.Now()
	vp := o.viewport()

	// 1. Normalize the viewport (the root scrollbar is hidden by stabilize.js).
	if _, err := s.SetViewport(ctx, vp.Width, vp.Height, vp.DeviceScaleFactor); err != nil {
		return Result{}, err
	}
	// 2. Navigate, then let the network go quiet.
	navCtx, cancel := context.WithTimeout(ctx, o.navigationTimeout())
	err := s.Navigate(navCtx, o.URL)
	cancel()
	if err != nil {
		return Result{}, err
	}
	if err := settle(ctx, s, o, idleAfterLoad); err != nil {
		return Result{}, err
	}
	// 3. Stabilize: fonts, lazy content, animations, frozen timers.
	var stab schema.Stabilize
	if err := evaluateJSON(ctx, s, o, inject.Stabilize, &stab); err != nil {
		return Result{}, fmt.Errorf("stabilize: %w", err)
	}
	if err := settle(ctx, s, o, idleAfterSteps); err != nil {
		return Result{}, err
	}
	// 4. Extract the geometry.
	var c schema.Capture
	if err := evaluateJSON(ctx, s, o, inject.Extract, &c); err != nil {
		return Result{}, fmt.Errorf("extract: %w", err)
	}
	// 5. Screenshot the whole document, capped.
	w := min(int(math.Ceil(math.Max(float64(c.Meta.VW), float64(c.Meta.DocW)))), maxShotW)
	h := min(c.Meta.DocH, maxShotH)
	png, err := s.Screenshot(ctx, drivers.Rect{Width: float64(w), Height: float64(h)})
	if err != nil {
		return Result{}, fmt.Errorf("screenshot: %w", err)
	}
	// 6. Re-measure: a change means the page moved while being captured.
	var post schema.Capture
	if err := evaluateJSON(ctx, s, o, inject.Extract, &post); err != nil {
		return Result{}, fmt.Errorf("re-measure: %w", err)
	}

	m := &c.Meta
	m.Post = schema.Post{DocH: post.Meta.DocH, Nodes: post.Meta.Nodes}
	m.Probes = schema.ProbesMeta{IDs: []string{}} // probes come with Task 27
	m.Shot = schema.Shot{W: w, H: h}
	m.Stabilize = stab
	m.Failed = []schema.FailedRequest{}
	for _, f := range s.FailedRequests() {
		m.Failed = append(m.Failed, schema.FailedRequest{URL: f.URL, Err: f.Err})
		if strings.Contains(strings.ToUpper(f.Err), "BLOCKED_BY_CLIENT") {
			m.Blocked++ // blocked by the browser itself: explains branded browsers
		}
	}
	m.MS = time.Since(start).Milliseconds()
	m.Engine, m.Version = o.Engine, o.Version

	// 7. Write: PNG, then JSON.
	if err := writeAtomic(base+".png", func(f *os.File) error { _, e := f.Write(png); return e }); err != nil {
		return Result{}, err
	}
	if err := writeAtomic(base+".json", func(f *os.File) error { return json.NewEncoder(f).Encode(c) }); err != nil {
		_ = os.Remove(base + ".png")
		return Result{}, err
	}
	return Result{Nodes: m.Nodes, MS: m.MS, Blocked: m.Blocked}, nil
}

// settle waits for network idle where the driver can tell; otherwise a fixed
// wait (Classic has no network events). Not reaching idle is not an error:
// some pages poll forever.
func settle(ctx context.Context, s drivers.Session, o Options, limit time.Duration) error {
	if s.Capabilities().NetworkIdle {
		err := s.WaitNetworkIdle(ctx, idleQuiet, limit)
		if err != nil && !errors.Is(err, drivers.ErrTimeout) {
			return err
		}
		return ctx.Err()
	}
	d := o.settle
	if d == 0 {
		d = classicSettle
	}
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// evaluateJSON runs one of the injected scripts (they return JSON strings).
func evaluateJSON(ctx context.Context, s drivers.Session, o Options, script string, out any) error {
	ctx, cancel := context.WithTimeout(ctx, o.scriptTimeout())
	defer cancel()
	raw, err := s.Evaluate(ctx, script)
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(raw), out)
}
