package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/GerMP21/Omnisnap/internal/capture"
	"github.com/GerMP21/Omnisnap/internal/discovery"
	"github.com/GerMP21/Omnisnap/internal/schema"
)

// errNoBrowser: no usable baseline (exit 2).
var errNoBrowser = errors.New("no usable browsers")

// plan is a resolved job: which browsers run, which were skipped.
type plan struct {
	job      *schema.Job
	out      string
	baseline discovery.Browser
	engines  []discovery.Browser // baseline first
	skipped  []schema.Skipped
}

// runJob captures every page in every engine and writes the bundle and the
// report. Findings never change the exit code: Omnisnap is report-only.
func runJob(ctx context.Context, j *schema.Job, r *reporter, e env) int {
	if err := resolve(j); err != nil {
		r.fatal(err)
		return ExitUsage
	}
	found, err := e.find(ctx)
	if err != nil {
		r.fatal(fmt.Errorf("discovery: %w", err))
		return ExitInternal
	}
	p, err := makePlan(j, found)
	if err != nil {
		r.fatal(err)
		return ExitNoBrowser
	}
	if err := os.MkdirAll(p.out, 0o755); err != nil {
		r.fatal(err)
		return ExitInternal
	}
	ids := make([]string, len(p.engines))
	for i, b := range p.engines {
		ids[i] = b.ID
		r.event(schema.NewEngineEvent(r.now(), b.ID, schema.EngineDiscovered, b.Version, ""), "")
	}
	r.event(schema.NewJobEvent(r.now(), len(j.Pages), ids, p.out),
		fmt.Sprintf("omnisnap %s: %d page(s), %s (baseline %s) -> %s", Version, len(j.Pages), strings.Join(ids, ", "), p.baseline.ID, p.out))
	for _, s := range p.skipped {
		r.event(nil, fmt.Sprintf("  skipped %s: %s", s.Engine, s.Reason))
	}

	// One browser per engine for the whole job, closed whatever happens.
	running := map[string]engine{}
	defer func() {
		for _, eng := range running {
			_ = eng.Close(context.WithoutCancel(ctx))
		}
	}()
	var usable []discovery.Browser
	for _, b := range p.engines {
		eng, err := e.launch(ctx, b)
		if err != nil {
			r.event(schema.NewEngineEvent(r.now(), b.ID, schema.EngineFailed, b.Version, err.Error()), "  "+b.ID+": launch failed: "+err.Error())
			p.skipped = append(p.skipped, schema.Skipped{Engine: b.ID, Reason: "launch failed: " + err.Error()})
			continue
		}
		running[b.ID] = eng
		usable = append(usable, b)
		r.event(schema.NewEngineEvent(r.now(), b.ID, schema.EngineLaunched, b.Version, ""), "")
	}
	if _, ok := running[p.baseline.ID]; !ok {
		r.fatal(fmt.Errorf("%w: the baseline %s did not start", errNoBrowser, p.baseline.ID))
		return ExitNoBrowser
	}
	p.engines = usable

	attempted, ok := 0, 0
	for _, page := range j.Pages {
		if err := ctx.Err(); err != nil {
			r.fatal(err)
			return ExitInternal
		}
		r.event(schema.NewPageStartEvent(r.now(), page.Slug, page.URL), page.Slug+"  "+page.URL)
		if err := capture.WritePageJSON(p.out, page.Slug, page.URL); err != nil {
			r.fatal(err)
			return ExitInternal
		}
		// Interleaved: run a of every engine, then run b, so a live page
		// cannot drift between engines.
		for run := 0; run < *j.Runs; run++ {
			letter := string(rune('a' + run))
			for _, b := range p.engines {
				attempted++
				res, err := captureOne(ctx, running[b.ID], p, b, page, letter)
				nodes := res.Nodes
				ev := schema.NewCaptureEvent(r.now(), page.Slug, b.ID, letter, res.MS, &nodes, "")
				line := fmt.Sprintf("  %-10s run %s  ok  %d nodes, %d ms", b.ID, letter, res.Nodes, res.MS)
				if err != nil {
					ev = schema.NewCaptureEvent(r.now(), page.Slug, b.ID, letter, res.MS, nil, err.Error())
					line = fmt.Sprintf("  %-10s run %s  FAILED  %v", b.ID, letter, err)
				} else {
					ok++
				}
				r.event(ev, line)
			}
		}
		r.event(schema.NewPageDoneEvent(r.now(), page.Slug, pageStatus(p, page.Slug)), "")
	}

	s, err := writeBundle(p)
	if err != nil {
		r.fatal(err)
		return ExitInternal
	}
	summary := filepath.Join(p.out, "summary.json")
	r.event(schema.NewDoneEvent(r.now(), summary), "")
	if !r.json {
		fmt.Fprintln(r.out, "report:", filepath.Join(p.out, "report.html"))
	}
	if !r.quiet && !r.json {
		fmt.Fprintf(r.out, "%d result(s), %d skipped engine(s)\n", len(s.Results), len(s.Skipped))
	}
	if ok == 0 {
		fmt.Fprintf(r.err, "omnisnap: every capture failed (%d attempted)\n", attempted)
		return ExitAllFailed
	}
	return ExitOK
}

func captureOne(ctx context.Context, eng engine, p *plan, b discovery.Browser, page schema.PageRef, run string) (capture.Result, error) {
	s, err := eng.NewSession(ctx)
	if err != nil {
		return capture.Result{}, err
	}
	defer func() { _ = s.Close(context.WithoutCancel(ctx)) }()
	return capture.Page(ctx, s, capture.Options{
		Out: p.out, Slug: page.Slug, URL: page.URL, Engine: b.ID, Version: b.Version, Run: run,
		Viewport: *p.job.Viewport, Timeouts: *p.job.Timeouts,
	})
}

// makePlan picks the baseline and the engines (SPEC-bundle-schema.md, job
// rules): the baseline must be a Blink browser that is installed (default
// chrome, else the first Blink found).
func makePlan(j *schema.Job, found []discovery.Browser) (*plan, error) {
	byID := map[string]discovery.Browser{}
	for _, b := range found {
		byID[b.ID] = b
	}
	p := &plan{job: j}
	out, err := filepath.Abs(j.Output)
	if err != nil {
		return nil, err
	}
	p.out = out

	switch {
	case j.Baseline != "":
		b, ok := byID[j.Baseline]
		if !ok {
			return nil, fmt.Errorf("%w: the baseline %s is not installed", errNoBrowser, j.Baseline)
		}
		if b.Family != schema.FamilyBlink {
			return nil, fmt.Errorf("%w: the baseline %s is not a Blink browser", errNoBrowser, j.Baseline)
		}
		p.baseline = b
	case byID[schema.DefaultBaseline].ID != "":
		p.baseline = byID[schema.DefaultBaseline]
	default:
		for _, b := range found {
			if b.Family == schema.FamilyBlink {
				p.baseline = b
				break
			}
		}
		if p.baseline.ID == "" {
			return nil, fmt.Errorf("%w: no Blink browser (Chrome, Chromium, Edge, Brave, ...) found for the baseline", errNoBrowser)
		}
	}

	wanted := j.Engines
	if len(wanted) == 0 {
		for _, b := range found {
			wanted = append(wanted, b.ID)
		}
	}
	p.engines = []discovery.Browser{p.baseline}
	for _, id := range wanted {
		b, ok := byID[id]
		switch {
		case id == p.baseline.ID:
		case !ok:
			p.skipped = append(p.skipped, schema.Skipped{Engine: id, Reason: "not installed"})
		case !canDrive(b):
			p.skipped = append(p.skipped, schema.Skipped{Engine: id, Reason: fmt.Sprintf("%s engines are not supported yet", b.Family)})
		default:
			p.engines = append(p.engines, b)
		}
	}
	return p, nil
}

func pageStatus(p *plan, slug string) schema.PageStatus {
	for _, b := range p.engines {
		if _, err := os.Stat(filepath.Join(p.out, slug, b.ID+"-a.json")); err != nil {
			return schema.StatusFailed
		}
	}
	return schema.StatusOK
}
