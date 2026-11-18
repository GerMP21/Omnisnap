package capture

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/GerMP21/Omnisnap/internal/drivers"
	"github.com/GerMP21/Omnisnap/internal/schema"
)

// Concurrency of a job (SPEC-capture.md, "Runs and lanes"): every engine
// captures two interleaved runs (a, then b after every engine's a), and the
// protocol lanes run in parallel — CDP and BiDi together, WebDriver Classic
// alone (its scroll-and-stitch captures are slower, and Safari's driver
// serialises sessions anyway).

// Engine is one browser a job captures with: what to name its files and how to
// open a page in it.
type Engine struct {
	ID      string // browser id, e.g. "chromium"
	Family  schema.Family
	Version string
	// NewSession opens one page (tab) in the running browser.
	NewSession func(ctx context.Context) (drivers.Session, error)
}

// Plan is everything a page's captures share.
type Plan struct {
	Out      string
	Viewport schema.Viewport
	Timeouts schema.Timeouts
	Runs     int
	Engines  []Engine

	// Baseline is the engine whose run a decides whether a page is reachable
	// (the failure policy). Empty: every engine is captured without policy.
	Baseline string
	// FullCapture disables the failure policy (planted pages and the lab:
	// SPEC-capture.md, "live pages only").
	FullCapture bool
	// AllowSmallPages keeps the timeout/navigation rules but drops the
	// "under 50 elements means a bot wall" one (--allow-small-pages): a
	// planted or test page is small by design.
	AllowSmallPages bool

	// Resume skips a capture whose file already exists (--resume).
	Resume bool
	// RetryFailed also repeats captures that failed (--retry-failed).
	RetryFailed bool

	// NoProbes skips the causal probes ("probes": false).
	NoProbes bool
}

// Outcome is one capture's result, reported as soon as it is written.
type Outcome struct {
	Engine string
	Run    string
	Result Result
	Err    error
}

// laneOf groups engines by protocol lane: Classic is its own lane, everything
// else shares one. A Lane is a name (for logs) and its engines in job order.
type lane struct {
	name    string
	engines []Engine
}

// lanes splits engines into the protocol lanes of the job, keeping their
// order: the baseline's lane first.
func lanes(engines []Engine) []lane {
	var cdpLane, classicLane lane
	cdpLane.name = "cdp+bidi"
	classicLane.name = "classic"
	for _, e := range engines {
		if e.Family == schema.FamilyWebKit {
			classicLane.engines = append(classicLane.engines, e)
		} else {
			cdpLane.engines = append(cdpLane.engines, e)
		}
	}
	var out []lane
	if len(cdpLane.engines) > 0 {
		out = append(out, cdpLane)
	}
	if len(classicLane.engines) > 0 {
		out = append(out, classicLane)
	}
	return out
}

// RunPage captures one page in every engine of the plan: per lane, run a of
// every engine before run b, with the lanes in parallel. onOutcome is called
// once per capture, from several goroutines.
//
// The baseline's run a goes first (the failure policy): a timeout, a
// navigation error or a bot wall marks the page unreachable, and nothing else
// is captured.
func RunPage(ctx context.Context, p Plan, page schema.PageRef, onOutcome func(Outcome)) error {
	// A retried page must not keep both a stale failure and fresh captures.
	_ = os.Remove(filepath.Join(p.Out, page.Slug, "failed.json"))
	skip := map[string]map[string]bool{} // engine -> run -> already captured
	if p.Baseline != "" && !p.FullCapture {
		for _, e := range p.Engines {
			if e.ID != p.Baseline {
				continue
			}
			if p.done(page, e.ID, "a") {
				skip[e.ID] = map[string]bool{"a": true}
				if p.failedAttempt(page, e.ID, "a") {
					// It failed before: its later runs were skipped then and stay skipped.
					for i := 1; i < p.Runs; i++ {
						skip[e.ID][string(rune('a'+i))] = true
					}
				}
				break
			}
			res, err := captureOne(ctx, p, page, e, "a")
			onOutcome(Outcome{Engine: e.ID, Run: "a", Result: res, Err: err})
			if reason, bad := unreachable(res, err, p.AllowSmallPages); bad {
				return WriteFailure(p.Out, page, reason)
			}
			skip[e.ID] = map[string]bool{"a": true}
			break
		}
	}
	ls := lanes(p.Engines)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var errs []error
	for _, l := range ls {
		l := l
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := runLane(ctx, p, page, l, skip, func(o Outcome) {
				mu.Lock()
				defer mu.Unlock()
				onOutcome(o)
			}); err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if len(errs) > 0 {
		return errs[0]
	}
	return ctx.Err()
}

// runLane is one lane's sequential work: all run a, then all run b. An engine
// whose run a failed skips its later runs.
func runLane(ctx context.Context, p Plan, page schema.PageRef, l lane, skip map[string]map[string]bool, report func(Outcome)) error {
	failedA := map[string]bool{}
	for run := 0; run < p.Runs; run++ {
		letter := string(rune('a' + run))
		for _, e := range l.engines {
			if err := ctx.Err(); err != nil {
				return err
			}
			if skip[e.ID][letter] || (letter != "a" && failedA[e.ID]) {
				continue
			}
			if p.done(page, e.ID, letter) {
				if letter == "a" && p.failedAttempt(page, e.ID, letter) {
					failedA[e.ID] = true
				}
				continue
			}
			res, err := captureOne(ctx, p, page, e, letter)
			report(Outcome{Engine: e.ID, Run: letter, Result: res, Err: err})
			if letter == "a" && err != nil {
				failedA[e.ID] = true
			}
		}
	}
	return nil
}

// done says the bundle already holds this capture. An error file counts as an
// attempt (its run and later ones are skipped by the policy) unless
// --retry-failed asks for it again.
func (p Plan) done(page schema.PageRef, engine, run string) bool {
	if !p.Resume {
		return false
	}
	dir := filepath.Join(p.Out, page.Slug)
	if _, err := os.Stat(filepath.Join(dir, engine+"-"+run+".json")); err == nil {
		return true
	}
	if p.RetryFailed {
		return false
	}
	_, err := os.Stat(filepath.Join(dir, engine+"-"+run+".error.txt"))
	return err == nil
}

// failedAttempt says the bundle holds a failed attempt for this capture.
func (p Plan) failedAttempt(page schema.PageRef, engine, run string) bool {
	_, err := os.Stat(filepath.Join(p.Out, page.Slug, engine+"-"+run+".error.txt"))
	return err == nil
}

// captureOne opens a page in e, captures it and closes it.
func captureOne(ctx context.Context, p Plan, page schema.PageRef, e Engine, run string) (Result, error) {
	s, err := e.NewSession(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("open %s: %w", e.ID, err)
	}
	defer func() { _ = s.Close(context.WithoutCancel(ctx)) }()
	return Page(ctx, s, Options{
		Out: p.Out, Slug: page.Slug, URL: page.URL, Engine: e.ID, Version: e.Version, Run: run,
		Viewport: p.Viewport, Timeouts: p.Timeouts, NoProbes: p.NoProbes,
	})
}
