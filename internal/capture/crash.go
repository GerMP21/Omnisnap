package capture

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/GerMP21/Omnisnap/internal/drivers"
	"github.com/GerMP21/Omnisnap/internal/fsutil"
	"github.com/GerMP21/Omnisnap/internal/schema"
)

// Crashes (SPEC-capture.md, failure policy; Task 89): a capture that fails
// because the browser or its page crashed is retried once in a fresh browser
// (Engine.Relaunch). When the retry crashes too, or the browser cannot be
// started again, the engine is lost: it captures nothing more in the job,
// each of its later captures is recorded as skipped with the crash named,
// and the other engines go on.

// Lost holds the engines a job has lost to crashes. One per job, shared by
// all its pages; safe for the lanes to use concurrently.
type Lost struct {
	mu      sync.Mutex
	reasons map[string]string
}

// NewLost returns an empty set.
func NewLost() *Lost { return &Lost{reasons: map[string]string{}} }

// Reason says whether engine is lost, and why.
func (l *Lost) Reason(engine string) (string, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	r, ok := l.reasons[engine]
	return r, ok
}

func (l *Lost) set(engine, reason string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.reasons[engine] = reason
}

// captureRetrying is captureOne with the crash policy.
func captureRetrying(ctx context.Context, p Plan, page schema.PageRef, e Engine, run string, lost *Lost) Outcome {
	res, err := captureOne(ctx, p, page, e, run)
	if !errors.Is(err, drivers.ErrCrashed) || ctx.Err() != nil {
		return Outcome{Engine: e.ID, Run: run, Result: res, Err: err}
	}
	where := fmt.Sprintf("%s crashed on %s run %s (%s)", e.ID, page.Slug, run, cause(err, page, e.ID, run))
	why := where + " and cannot be restarted"
	retried := false
	if e.Relaunch != nil {
		rerr := e.Relaunch(ctx)
		if rerr == nil {
			retried = true
			res, err = captureOne(ctx, p, page, e, run)
			if !errors.Is(err, drivers.ErrCrashed) {
				return Outcome{Engine: e.ID, Run: run, Result: res, Err: err, Retried: true}
			}
		}
		why = retryFailed(e.ID, page.Slug, run, where, rerr, err)
	}
	reason := why + "; it is out of the job, the other engines go on"
	lost.set(e.ID, reason)
	// The bundle names the loss, not only the driver's error (and records it
	// when opening the page failed before Page could).
	_ = writeSkipped(p, page, e.ID, run, reason)
	return Outcome{Engine: e.ID, Run: run, Result: res, Err: err, Retried: retried, Lost: reason}
}

// retryFailed says why the retry did not save the engine: the browser did
// not start again (rerr), or it crashed again (err).
func retryFailed(engine, slug, run, where string, rerr, err error) string {
	if rerr != nil {
		return fmt.Sprintf("%s and did not restart: %v", where, rerr)
	}
	return fmt.Sprintf("%s crashed twice on %s run %s: %s", engine, slug, run,
		cause(err, schema.PageRef{Slug: slug}, engine, run))
}

// cause is err without Page's "capture <slug> <engine> run <run>: " prefix,
// which the loss message already says.
func cause(err error, page schema.PageRef, engine, run string) string {
	return strings.TrimPrefix(err.Error(), fmt.Sprintf("capture %s %s run %s: ", page.Slug, engine, run))
}

// writeSkipped records a capture that was not taken, or was lost, as an
// error file: resume counts it as attempted, and --retry-failed takes it
// again.
func writeSkipped(p Plan, page schema.PageRef, engine, run, msg string) error {
	dir := filepath.Join(p.Out, page.Slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("capture: %w", err)
	}
	base := filepath.Join(dir, engine+"-"+run)
	for _, ext := range []string{".json", ".png", ".probes.json.gz"} {
		_ = os.Remove(base + ext)
	}
	return fsutil.WriteAtomic(base+".error.txt", func(w io.Writer) error {
		_, err := io.WriteString(w, msg+"\n")
		return err
	})
}
