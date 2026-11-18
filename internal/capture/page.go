package capture

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/GerMP21/Omnisnap/assets/inject"
	"github.com/GerMP21/Omnisnap/internal/drivers"
	"github.com/GerMP21/Omnisnap/internal/fsutil"
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
	// A retry replaces the previous attempt: the bundle must never hold a
	// capture and an error for the same run (resume reads which files exist).
	for _, ext := range []string{".json", ".png", ".probes.json.gz", ".error.txt"} {
		if err := os.Remove(base + ext); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return Result{}, fmt.Errorf("capture: %w", err)
		}
	}
	res, err := page(ctx, s, o, base)
	if err != nil {
		err = fmt.Errorf("capture %s %s run %s: %w", o.Slug, o.Engine, o.Run, err)
		if werr := fsutil.WriteAtomic(base+".error.txt", func(f io.Writer) error {
			_, e := io.WriteString(f, err.Error()+"\n")
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
	stepCtx, cancel := context.WithTimeout(ctx, o.scriptTimeout())
	_, err := s.SetViewport(stepCtx, vp.Width, vp.Height, vp.DeviceScaleFactor)
	cancel()
	if err != nil {
		return Result{}, fmt.Errorf("viewport: %w", err)
	}
	// 2. Navigate, then let the network go quiet.
	navCtx, cancel := context.WithTimeout(ctx, o.navigationTimeout())
	err = s.Navigate(navCtx, o.URL)
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
	// 5-7. Screenshot, consistency re-measure and causal probes: the same
	// evidence a mutant carries (see Evidence), so the lab calls it too.
	if err := Evidence(ctx, s, o, &c, base); err != nil {
		return Result{}, err
	}

	m := &c.Meta
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

	// 8. Write the JSON last: its presence means the capture is complete.
	if err := fsutil.WriteAtomic(base+".json", func(f io.Writer) error { return json.NewEncoder(f).Encode(c) }); err != nil {
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
