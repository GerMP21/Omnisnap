package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

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
	noAudit  bool
}

// runJob captures every page in every engine, in groups of at most
// --max-browsers live browsers, and writes the bundle and the report. Findings never change the exit code: Omnisnap is report-only.
func runJob(ctx context.Context, j *schema.Job, o options, r *reporter, e env) int {
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
	p.noAudit = o.noAudit
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

	// At most maxBrowsers browsers are alive at once (change
	// cap-live-browsers; SPEC-process.md, "Resources"). Engines run in
	// consecutive groups, in job order with the baseline first: a group
	// launches until it has maxBrowsers live browsers (a failed launch does
	// not count), captures every page, and closes them all before the next
	// group launches. With maxBrowsers engines or fewer there is one group,
	// and the calls and events are those of a job without the cap.
	maxLive := o.maxBrowsers
	if maxLive < 1 {
		maxLive = defaultMaxBrowsers
	}
	running := map[string]*restartable{}
	closeGroup := func() {
		for id, eng := range running {
			_ = eng.Close(context.WithoutCancel(ctx))
			delete(running, id)
		}
	}
	defer closeGroup()
	if e.oom == nil {
		e.oom = func() error { return nil }
	}
	var oomOnce sync.Once
	afterSession := func() {
		if err := e.oom(); err != nil && o.verbose {
			oomOnce.Do(func() { fmt.Fprintf(r.err, "omnisnap: out-of-memory score not raised: %v\n", err) })
		}
	}

	var (
		usable      []discovery.Browser
		baseFonts   *capture.FontReport
		lost        = capture.NewLost()
		unreachable = map[string]bool{} // decided by the baseline in group 1
		started     = map[string]bool{} // page-start emitted
		touched     = map[string]bool{} // some group captured the page
		resumed     = map[string]bool{} // some group skipped it (--resume)
		single      bool                // the first group is the only one
		attempted   int
		ok          int
		mu          sync.Mutex // lanes report concurrently; progress output stays whole lines
	)
	hint := func() {
		// One family cannot show a cross-engine difference: name the way to
		// get a second one (SPEC-install.md).
		families := map[schema.Family]bool{}
		for _, b := range usable {
			families[b.Family] = true
		}
		if len(families) < 2 {
			r.event(nil, "  hint: only one engine family was captured; `omnisnap install firefox` adds WebKit's counterpart (Gecko)")
		}
	}
	queue := p.engines // baseline first (makePlan)
	for first := true; first || len(queue) > 0; first = false {
		if err := ctx.Err(); err != nil && !first {
			r.fatal(err) // interrupted between groups
			return ExitInternal
		}
		var group []discovery.Browser
		for len(queue) > 0 && len(group) < maxLive {
			b := queue[0]
			queue = queue[1:]
			eng, err := e.launch(ctx, b)
			if err != nil {
				r.event(schema.NewEngineEvent(r.now(), b.ID, schema.EngineFailed, b.Version, err.Error()), "  "+b.ID+": launch failed: "+err.Error())
				p.skipped = append(p.skipped, schema.Skipped{Engine: b.ID, Reason: "launch failed: " + err.Error()})
				continue
			}
			running[b.ID] = &restartable{b: b, launch: e.launch, cur: eng, afterSession: afterSession}
			group = append(group, b)
			r.event(schema.NewEngineEvent(r.now(), b.ID, schema.EngineLaunched, b.Version, ""), "")
		}
		if first {
			if _, ok := running[p.baseline.ID]; !ok {
				r.fatal(fmt.Errorf("%w: the baseline %s did not start", errNoBrowser, p.baseline.ID))
				return ExitNoBrowser
			}
			single = len(queue) == 0
		}
		if len(group) == 0 {
			if err := ctx.Err(); err != nil {
				// The launches failed because the run was interrupted, not
				// because the engines are broken: not a finished job.
				r.fatal(err)
				return ExitInternal
			}
			continue // every remaining engine failed to launch
		}
		usable = append(usable, group...)
		p.engines = usable
		if *j.FontParity {
			engs := make(map[string]engine, len(running))
			for id, eng := range running {
				engs[id] = eng
			}
			baseFonts = checkFonts(ctx, r, p.baseline.ID, baseFonts, group, engs)
		}
		if single {
			hint()
		}

		engines := make([]capture.Engine, len(group))
		ids := make([]string, len(group))
		for i, b := range group {
			ids[i] = b.ID
			engines[i] = capture.Engine{ID: b.ID, Family: b.Family, Version: b.Version,
				NewSession: running[b.ID].NewSession, Relaunch: running[b.ID].Relaunch}
		}
		job := capture.Plan{Out: p.out, Viewport: *j.Viewport, Timeouts: *j.Timeouts, Runs: *j.Runs, Engines: engines,
			Resume: o.resume, RetryFailed: o.retryFailed, NoProbes: !*j.Probes,
			AllowSmallPages: o.allowSmall, Lost: lost}
		if first {
			// The baseline's run a decides reachability, once (later groups
			// skip what it found unreachable).
			job.Baseline = p.baseline.ID
		}
		for _, page := range j.Pages {
			if err := ctx.Err(); err != nil {
				r.fatal(err)
				return ExitInternal
			}
			if unreachable[page.Slug] {
				continue // only later groups get here: the first group decided
			}
			if o.resume {
				if st := capture.ResumeState(p.out, page.Slug, ids, *j.Runs); st.Done && (!st.Unreachable || !o.retryFailed) {
					resumed[page.Slug] = true
					reason := "already done"
					if st.Unreachable {
						reason = "unreachable: " + st.Reason
						if first {
							unreachable[page.Slug] = true
						}
					}
					r.event(nil, "  skip "+page.Slug+": "+reason)
					continue
				}
			}
			if !started[page.Slug] {
				started[page.Slug] = true
				r.event(schema.NewPageStartEvent(r.now(), page.Slug, page.URL), page.Slug+"  "+page.URL)
				if err := capture.WritePageJSON(p.out, page.Slug, page.URL); err != nil {
					r.fatal(err)
					return ExitInternal
				}
			}
			touched[page.Slug] = true
			// Per lane: run a of every engine, then run b, so a live page
			// cannot drift between the group's engines; the CDP/BiDi and
			// Classic lanes overlap.
			err := capture.RunPage(ctx, job, page, func(o capture.Outcome) {
				mu.Lock()
				defer mu.Unlock()
				attempted++
				nodes := o.Result.Nodes
				ev := schema.NewCaptureEvent(r.now(), page.Slug, o.Engine, o.Run, o.Result.MS, &nodes, "")
				line := fmt.Sprintf("  %-10s run %s  ok  %d nodes, %d ms", o.Engine, o.Run, o.Result.Nodes, o.Result.MS)
				if o.Err != nil {
					ev = schema.NewCaptureEvent(r.now(), page.Slug, o.Engine, o.Run, o.Result.MS, nil, o.Err.Error())
					line = fmt.Sprintf("  %-10s run %s  FAILED  %v", o.Engine, o.Run, o.Err)
				} else {
					ok++
				}
				switch {
				case o.Lost != "":
					// One line for the loss (Task 89): the engine's later
					// captures are skipped quietly, their results name this
					// crash.
					r.event(ev, "")
					r.event(schema.NewEngineEvent(r.now(), o.Engine, schema.EngineFailed, engineVersion(p, o.Engine), o.Lost),
						fmt.Sprintf("  %-10s run %s  LOST  %s", o.Engine, o.Run, o.Lost))
				case o.Retried:
					r.event(ev, line+"  (in a fresh browser: the first attempt crashed)")
				default:
					r.event(ev, line)
				}
			})
			if err != nil {
				r.fatal(err)
				return ExitInternal
			}
			if first {
				if _, err := os.Stat(filepath.Join(p.out, page.Slug, "failed.json")); err == nil {
					unreachable[page.Slug] = true
				}
			}
			if single {
				r.event(schema.NewPageDoneEvent(r.now(), page.Slug, pageStatus(p, page.Slug)), "")
			}
		}
		// Every browser of this group is dead before the next one launches.
		closeGroup()
	}
	if !single {
		hint()
		// Each page is done once, after the last group's captures of it.
		for _, page := range j.Pages {
			if started[page.Slug] {
				r.event(schema.NewPageDoneEvent(r.now(), page.Slug, pageStatus(p, page.Slug)), "")
			}
		}
	}
	skipped := 0
	for slug := range resumed {
		if !touched[slug] {
			skipped++
		}
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
		fmt.Fprintf(r.out, "%d result(s), %d skipped engine(s)", len(s.Results), len(s.Skipped))
		if skipped > 0 {
			fmt.Fprintf(r.out, ", %d page(s) skipped (--resume)", skipped)
		}
		fmt.Fprintln(r.out)
	}
	if ok == 0 && attempted > 0 {
		fmt.Fprintf(r.err, "omnisnap: every capture failed (%d attempted)\n", attempted)
		return ExitAllFailed
	}
	if attempted == 0 && skipped == 0 {
		fmt.Fprintf(r.err, "omnisnap: nothing to capture\n")
		return ExitUsage
	}
	return ExitOK
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

func engineVersion(p *plan, id string) string {
	for _, b := range p.engines {
		if b.ID == id {
			return b.Version
		}
	}
	return ""
}

func pageStatus(p *plan, slug string) schema.PageStatus {
	for _, b := range p.engines {
		if _, err := os.Stat(filepath.Join(p.out, slug, b.ID+"-a.json")); err != nil {
			return schema.StatusFailed
		}
	}
	return schema.StatusOK
}
