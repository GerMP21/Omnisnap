package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/png" // the screenshots are PNG: register the decoder
	"io"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"

	"github.com/GerMP21/Omnisnap/internal/discovery"
	"github.com/GerMP21/Omnisnap/internal/domaudit"
	"github.com/GerMP21/Omnisnap/internal/fsutil"
	"github.com/GerMP21/Omnisnap/internal/pipeline"
	"github.com/GerMP21/Omnisnap/internal/schema"
	"github.com/GerMP21/Omnisnap/internal/triage/evidence"
	"github.com/GerMP21/Omnisnap/internal/viewer"
)

// writeBundle writes host.json, summary.json and report.html. The summary is
// built from the capture files in the bundle, not from memory, so a resumed
// run can rebuild it the same way.
//
// M0 (no layout diff yet): one result per page and engine compared with the
// baseline, without causes; with the baseline alone, one result per page
// whose engine is the baseline itself (a single-engine capture).
func writeBundle(p *plan) (schema.Summary, error) {
	host := schema.Host{Schema: schema.ID("host"), Machine: machine()}
	profile := schema.ProfileFresh
	if p.job.Neutral != nil && *p.job.Neutral {
		profile = schema.ProfileNeutral
	}
	for _, b := range p.engines {
		host.Engines = append(host.Engines, schema.HostEngine{
			ID: b.ID, Family: b.Family, Version: b.Version, Executable: b.Executable, Profile: profile,
		})
	}
	s := schema.NewSummary(Version, *p.job)
	s.Hosts = []schema.Host{{Machine: host.Machine, Engines: host.Engines}}
	s.Skipped = p.skipped

	compared := p.engines[1:]
	if len(compared) == 0 {
		compared = p.engines[:1]
	}
	for _, page := range p.job.Pages {
		total := schema.PageTotal{Slug: page.Slug, URL: page.URL}
		base, baseErr := readCapture(p.out, page.Slug, p.baseline.ID)
		var findings []pipeline.FindingsPair
		// Every engine's two runs, for the design-intent third opinion and
		// for masks.
		captures := map[string][2]*schema.Capture{}
		for _, eng := range append([]discovery.Browser{p.baseline}, compared...) {
			ca, _ := readCaptureRun(p.out, page.Slug, eng.ID, "a")
			cb, _ := readCaptureRun(p.out, page.Slug, eng.ID, "b")
			captures[eng.ID] = [2]*schema.Capture{ca, cb}
		}
		for _, b := range compared {
			r := schema.Result{
				Host:     host.Machine,
				Page:     schema.ResultPage{Slug: page.Slug, URL: page.URL},
				Baseline: schema.ResultBaseline{Engine: p.baseline.ID, Version: p.baseline.Version},
				Engine:   schema.ResultEngine{Engine: b.ID, Family: b.Family, Version: b.Version, Profile: profile},
				Status:   schema.StatusOK,
			}
			eng, engErr := readCapture(p.out, page.Slug, b.ID)
			switch {
			case baseErr != nil:
				r.Status = schema.StatusFailed
				r.Warnings = append(r.Warnings, "baseline capture missing: "+captureError(p.out, page.Slug, p.baseline.ID, baseErr))
			case engErr != nil:
				r.Status = schema.StatusFailed
				r.Warnings = append(r.Warnings, b.ID+" capture missing: "+captureError(p.out, page.Slug, b.ID, engErr))
			default:
				r.Shots = &schema.Shots{
					Baseline: schema.ShotRef{Src: path.Join(page.Slug, p.baseline.ID+"-a.png"), W: base.Meta.Shot.W, H: base.Meta.Shot.H},
					Engine:   schema.ShotRef{Src: path.Join(page.Slug, b.ID+"-a.png"), W: eng.Meta.Shot.W, H: eng.Meta.Shot.H},
				}
				r.Stats.MS = eng.Meta.MS
				r.Blocked = schema.Sides{Baseline: blockedHosts(base), Engine: blockedHosts(eng)}
				for _, c := range []*schema.Capture{base, eng} {
					if w := changedWarning(c); w != "" && !slices.Contains(r.Warnings, w) {
						r.Warnings = append(r.Warnings, w)
					}
				}
				if !p.noAudit {
					r.Audit = &schema.ResultAudit{
						Baseline: auditFindings(domaudit.Audit(base)),
						Engine:   auditFindings(domaudit.Audit(eng)),
					}
				}
				// The comparison itself (M1): causes, warnings, dynamic
				// regions, blocked hosts, match rate.
				baseB := captures[p.baseline.ID][1]
				engB := captures[b.ID][1]
				var thirds []pipeline.ThirdCapture
				for id, caps := range captures {
					if id == p.baseline.ID || id == b.ID || caps[0] == nil {
						continue
					}
					thirds = append(thirds, pipeline.ThirdCapture{Engine: id, A: caps[0], B: caps[1]})
				}
				sort.Slice(thirds, func(i, j int) bool { return thirds[i].Engine < thirds[j].Engine })
				pair := pipeline.Run(pipeline.Inputs{
					SlugBase: page.Slug + "@" + b.ID, BaselineEngine: p.baseline.ID, Engine: b.ID,
					A: base, A2: baseB, B: eng, B2: engB, Thirds: thirds,
					ProbesA: readProbes(p.out, page.Slug, p.baseline.ID, "a"),
					ProbesB: readProbes(p.out, page.Slug, b.ID, "a"),
					ImagesA: readImage(p.out, page.Slug, p.baseline.ID, "a"),
					ImagesB: readImage(p.out, page.Slug, b.ID, "a"),
				})
				r.Causes = pair.Causes
				r.Intent = pair.Intent
				r.Dynamic = pair.Dynamic
				r.Blocked = pair.Blocked
				r.Stats = pair.Stats
				for _, w := range pair.Warnings {
					if !slices.Contains(r.Warnings, w) {
						r.Warnings = append(r.Warnings, w)
					}
				}
				for _, note := range pair.Notes {
					if !slices.Contains(r.Warnings, note) {
						r.Warnings = append(r.Warnings, note)
					}
				}
				symptoms := make([]pipeline.Symptom, len(pair.Symptoms))
				for i, sym := range pair.Symptoms {
					symptoms[i] = pipeline.Symptom{Selector: sym.Selector, Kind: sym.Kind}
				}
				findings = append(findings, pipeline.FindingsPair{
					Engine: b.ID, Baseline: p.baseline.ID,
					Causes:   pipeline.CausesWithRefs(pair.Causes, pair.CauseRefs),
					Symptoms: symptoms, Warnings: pair.Warnings, Dynamic: pair.Dynamic,
					Blocked: pair.Blocked, Stats: pair.Stats, Features: pair.Features,
				})
			}
			s.Results = append(s.Results, r)
		}
		if len(findings) > 0 && baseErr == nil {
			if rel, err := pipeline.WriteFindings(p.out, page.Slug, page.URL, findings); err == nil {
				for i := range s.Results {
					if s.Results[i].Page.Slug == page.Slug {
						s.Results[i].Findings = rel
					}
				}
			} else {
				return s, err
			}
		}
		s.Pages = append(s.Pages, total)
	}

	if err := writeJSON(filepath.Join(p.out, "host.json"), host); err != nil {
		return s, err
	}
	if err := writeJSON(filepath.Join(p.out, "summary.json"), s); err != nil {
		return s, err
	}
	return s, viewer.Write(p.out, s)
}

func machine() schema.Machine { return schema.Machine{OS: runtime.GOOS, Arch: runtime.GOARCH} }

// auditFindings converts the audit's findings for the summary.
func auditFindings(fs []domaudit.Finding) []schema.AuditFinding {
	out := make([]schema.AuditFinding, len(fs))
	for i, f := range fs {
		out[i] = schema.AuditFinding{Check: f.Check, Selector: f.Selector, Box: f.Box, Severity: string(f.Severity)}
	}
	return out
}

// readImage decodes <engine>-<run>.png for the appearance check; nil when
// absent or unreadable.
func readImage(out, slug, engine, run string) image.Image {
	f, err := os.Open(filepath.Join(out, slug, engine+"-"+run+".png"))
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil
	}
	return img
}

// readProbes decodes <engine>-<run>.probes.json.gz; nil when absent or
// unreadable (the mechanism then stays unknown, never an error).
func readProbes(out, slug, engine, run string) map[string]*schema.Capture {
	f, err := os.Open(filepath.Join(out, slug, engine+"-"+run+".probes.json.gz"))
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	m, err := evidence.ReadProbes(f)
	if err != nil {
		return nil
	}
	return m
}

// readCaptureRun reads one run; a missing file is (nil, error).
func readCaptureRun(out, slug, engine, run string) (*schema.Capture, error) {
	b, err := os.ReadFile(filepath.Join(out, slug, engine+"-"+run+".json"))
	if err != nil {
		return nil, err
	}
	var c schema.Capture
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("%s %s-%s: %w", slug, engine, run, err)
	}
	return &c, nil
}

func readCapture(out, slug, engine string) (*schema.Capture, error) {
	b, err := os.ReadFile(filepath.Join(out, slug, engine+"-a.json"))
	if err != nil {
		return nil, err
	}
	var c schema.Capture
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("%s %s: %w", slug, engine, err)
	}
	return &c, nil
}

// captureError is the first line of the capture's error file, if any.
func captureError(out, slug, engine string, readErr error) string {
	b, err := os.ReadFile(filepath.Join(out, slug, engine+"-a.error.txt"))
	if err == nil {
		return strings.SplitN(strings.TrimSpace(string(b)), "\n", 2)[0]
	}
	if errors.Is(readErr, fs.ErrNotExist) {
		return "not captured"
	}
	return readErr.Error()
}

// blockedHosts lists the hosts of requests the browser itself blocked.
func blockedHosts(c *schema.Capture) []string {
	var hosts []string
	for _, f := range c.Meta.Failed {
		if !strings.Contains(strings.ToUpper(f.Err), "BLOCKED_BY_CLIENT") {
			continue
		}
		if u, err := url.Parse(f.URL); err == nil && u.Host != "" && !slices.Contains(hosts, u.Host) {
			hosts = append(hosts, u.Host)
		}
	}
	slices.Sort(hosts)
	return hosts
}

// changedWarning: the page moved between geometry and screenshot
// (SPEC-capture.md, step 6).
func changedWarning(c *schema.Capture) string {
	if c.Meta.Post.DocH != c.Meta.DocH || c.Meta.Post.Nodes != c.Meta.Nodes {
		return fmt.Sprintf("the page changed during capture (%d to %d px tall, %d to %d elements)",
			c.Meta.DocH, c.Meta.Post.DocH, c.Meta.Nodes, c.Meta.Post.Nodes)
	}
	return ""
}

func writeJSON(path string, v any) error {
	return fsutil.WriteAtomic(path, func(w io.Writer) error {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(v)
	})
}
