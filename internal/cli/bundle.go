package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/GerMP21/Omnisnap/internal/fsutil"
	"github.com/GerMP21/Omnisnap/internal/schema"
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
	for _, b := range p.engines {
		host.Engines = append(host.Engines, schema.HostEngine{
			ID: b.ID, Family: b.Family, Version: b.Version, Executable: b.Executable, Profile: schema.ProfileFresh,
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
		for _, b := range compared {
			r := schema.Result{
				Host:     host.Machine,
				Page:     schema.ResultPage{Slug: page.Slug, URL: page.URL},
				Baseline: schema.ResultBaseline{Engine: p.baseline.ID, Version: p.baseline.Version},
				Engine:   schema.ResultEngine{Engine: b.ID, Family: b.Family, Version: b.Version, Profile: schema.ProfileFresh},
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
			}
			s.Results = append(s.Results, r)
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
