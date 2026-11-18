// Package merge combines bundles captured on several machines into one bundle
// and one report (SPEC-merge.md). Each host keeps its own baseline: a
// Chrome-on-Linux vs Safari-on-macOS comparison would mostly report OS font
// differences, so results are concatenated, never cross-host diffed.
package merge

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/GerMP21/Omnisnap/internal/schema"
)

// Bundle is one input: its directory and parsed summary.
type Bundle struct {
	Dir     string
	Summary schema.Summary
}

// Read loads <dir>/summary.json.
func Read(dir string) (Bundle, error) {
	b, err := os.ReadFile(filepath.Join(dir, "summary.json"))
	if err != nil {
		return Bundle{}, fmt.Errorf("merge: %s: %w (is it an omnisnap bundle?)", dir, err)
	}
	var s schema.Summary
	if err := json.Unmarshal(b, &s); err != nil {
		return Bundle{}, fmt.Errorf("merge: %s/summary.json: %w", dir, err)
	}
	return Bundle{Dir: dir, Summary: s}, nil
}

// Options configure a merge.
type Options struct {
	Out         string
	RenameHosts bool // a second bundle from the same host gets -2, -3...
}

// Copy is one file to bring into the merged bundle.
type Copy struct {
	Src, Dst string
}

// Merged is the result: the summary and the files to copy.
type Merged struct {
	Summary schema.Summary
	Copies  []Copy
}

// Merge is a pure function over the inputs plus a copy plan.
func Merge(bundles []Bundle, o Options) (Merged, error) {
	if len(bundles) == 0 {
		return Merged{}, fmt.Errorf("merge: no bundles")
	}
	if o.Out == "" {
		return Merged{}, fmt.Errorf("merge: an output directory is required (-o)")
	}
	// Same job on every machine (engines may differ).
	for i := 1; i < len(bundles); i++ {
		if field, ok := sameJob(bundles[0].Summary.Job, bundles[i].Summary.Job); !ok {
			return Merged{}, fmt.Errorf("merge: %s and %s ran different jobs: %s differs", bundles[0].Dir, bundles[i].Dir, field)
		}
		if major(bundles[i].Summary.Schema) != major(bundles[0].Summary.Schema) {
			return Merged{}, fmt.Errorf("merge: %s writes schema %s, %s writes %s: major versions differ",
				bundles[i].Dir, bundles[i].Summary.Schema, bundles[0].Dir, bundles[0].Summary.Schema)
		}
	}
	// Host ids: each input must come from a distinct machine unless renamed.
	hostIDs := make([]string, len(bundles))
	used := map[string]int{}
	for i, b := range bundles {
		id := hostID(b.Summary)
		if n := used[id]; n > 0 {
			if !o.RenameHosts {
				return Merged{}, fmt.Errorf("merge: %s and %s are both from host %q (use --rename-hosts)", bundles[hostIDsIndex(hostIDs, id)].Dir, b.Dir, id)
			}
			used[id] = n + 1
			id = fmt.Sprintf("%s-%d", id, n+1)
		} else {
			used[id] = 1
		}
		hostIDs[i] = id
	}

	out := schema.Summary{
		Schema:    bundles[0].Summary.Schema,
		Tool:      bundles[0].Summary.Tool,
		Job:       bundles[0].Summary.Job,
		Generated: newestGenerated(bundles),
	}
	var copies []Copy
	for i, b := range bundles {
		hostID := hostIDs[i]
		s := b.Summary
		// Hosts: this bundle's machine entry, with the (possibly renamed) id.
		hosts := s.Hosts
		if len(hosts) == 0 {
			hosts = []schema.Host{{Machine: schema.Machine{ID: hostID}}}
		}
		for _, h := range hosts {
			h.Machine.ID = hostID
			out.Hosts = append(out.Hosts, h)
		}
		for _, sk := range s.Skipped {
			sk.Host = hostID
			out.Skipped = append(out.Skipped, sk)
		}
		for _, r := range s.Results {
			r.Host.ID = hostID
			if r.Shots != nil {
				r.Shots.Baseline.Src = path.Join(hostID, r.Shots.Baseline.Src)
				r.Shots.Engine.Src = path.Join(hostID, r.Shots.Engine.Src)
			}
			if r.Findings != "" {
				r.Findings = path.Join(hostID, r.Findings)
			}
			out.Results = append(out.Results, r)
		}
		// Copy the page directories the results reference, whole: captures,
		// screenshots, probes and findings stay together.
		slugs := map[string]bool{}
		for _, r := range s.Results {
			slugs[r.Page.Slug] = true
		}
		for slug := range slugs {
			dir := filepath.Join(b.Dir, slug)
			err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if d.IsDir() {
					return nil
				}
				rel, err := filepath.Rel(b.Dir, p)
				if err != nil {
					return err
				}
				copies = append(copies, Copy{Src: p, Dst: filepath.Join(o.Out, hostID, rel)})
				return nil
			})
			if err != nil {
				return Merged{}, fmt.Errorf("merge: %s: %w", b.Dir, err)
			}
		}
	}
	// Deterministic order: page, host, engine; hosts by id; pages aggregated.
	sort.SliceStable(out.Results, func(i, j int) bool {
		a, b := out.Results[i], out.Results[j]
		if a.Page.Slug != b.Page.Slug {
			return a.Page.Slug < b.Page.Slug
		}
		if a.Page.URL != b.Page.URL {
			return a.Page.URL < b.Page.URL
		}
		if a.Host.ID != b.Host.ID {
			return a.Host.ID < b.Host.ID
		}
		return a.Engine.Engine < b.Engine.Engine
	})
	sort.SliceStable(out.Hosts, func(i, j int) bool { return out.Hosts[i].Machine.ID < out.Hosts[j].Machine.ID })
	sort.SliceStable(out.Skipped, func(i, j int) bool {
		if out.Skipped[i].Host != out.Skipped[j].Host {
			return out.Skipped[i].Host < out.Skipped[j].Host
		}
		return out.Skipped[i].Engine < out.Skipped[j].Engine
	})
	out.Pages = aggregatePages(out.Results)
	sort.SliceStable(copies, func(i, j int) bool { return copies[i].Dst < copies[j].Dst })
	return Merged{Summary: out, Copies: copies}, nil
}

// Apply writes summary.json and the files (hard links when possible).
func Apply(m Merged, out string, writeSummary func(dir string, s schema.Summary) error) error {
	for _, c := range m.Copies {
		if err := os.MkdirAll(filepath.Dir(c.Dst), 0o755); err != nil {
			return fmt.Errorf("merge: %w", err)
		}
		if err := os.Link(c.Src, c.Dst); err == nil {
			continue
		}
		if err := copyFile(c.Src, c.Dst); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return fmt.Errorf("merge: %w", err)
	}
	return writeSummary(out, m.Summary)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("merge: %w", err)
	}
	defer func() { _ = in.Close() }()
	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("merge: %w", err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return fmt.Errorf("merge: %w", err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("merge: %w", err)
	}
	return nil
}

// sameJob compares the job's fields except the engines (per host) and the
// output directory (per bundle).
func sameJob(a, b schema.Job) (string, bool) {
	ca, cb := canonicalJob(a), canonicalJob(b)
	if ca != cb {
		return "job", false
	}
	return "", true
}

func canonicalJob(j schema.Job) string {
	j.Engines = nil
	j.Output = ""
	if j.Viewport != nil {
		vp := *j.Viewport
		j.Viewport = &vp
	}
	if j.Runs != nil {
		r := *j.Runs
		j.Runs = &r
	}
	if j.Probes != nil {
		v := *j.Probes
		j.Probes = &v
	}
	if j.FontParity != nil {
		v := *j.FontParity
		j.FontParity = &v
	}
	if j.Neutral != nil {
		v := *j.Neutral
		j.Neutral = &v
	}
	if j.Timeouts != nil {
		t := *j.Timeouts
		j.Timeouts = &t
	}
	b, _ := json.Marshal(j)
	return string(b)
}

// major extracts the schema major version: .../summary/1.json -> "1".
func major(id string) string {
	name := path.Base(id)
	name = strings.TrimSuffix(name, ".json")
	if i := strings.LastIndexByte(name, '/'); i >= 0 {
		name = name[i+1:]
	}
	return name
}

// hostID is the bundle's machine id, or os/arch when it has none.
func hostID(s schema.Summary) string {
	for _, h := range s.Hosts {
		if h.Machine.ID != "" {
			return h.Machine.ID
		}
		return h.Machine.OS + "-" + h.Machine.Arch
	}
	for _, r := range s.Results {
		if r.Host.ID != "" {
			return r.Host.ID
		}
		return r.Host.OS + "-" + r.Host.Arch
	}
	return "host"
}

func hostIDsIndex(ids []string, want string) int {
	for i := len(ids) - 1; i >= 0; i-- {
		if ids[i] == want {
			return i
		}
	}
	return 0
}

// newestGenerated keeps the output deterministic: the newest input timestamp.
func newestGenerated(bundles []Bundle) time.Time {
	var best time.Time
	for _, b := range bundles {
		if b.Summary.Generated.After(best) {
			best = b.Summary.Generated
		}
	}
	return best
}

// aggregatePages sums Report/Info per page across hosts.
func aggregatePages(results []schema.Result) []schema.PageTotal {
	bySlug := map[string]*schema.PageTotal{}
	var order []string
	for _, r := range results {
		key := r.Page.Slug + "\x00" + r.Page.URL
		p := bySlug[key]
		if p == nil {
			p = &schema.PageTotal{Slug: r.Page.Slug, URL: r.Page.URL}
			bySlug[key] = p
			order = append(order, key)
		}
		p.Report += r.Stats.Report
		p.Info += r.Stats.Info
	}
	sort.SliceStable(order, func(i, j int) bool { return order[i] < order[j] })
	var out []schema.PageTotal
	for _, k := range order {
		out = append(out, *bySlug[k])
	}
	return out
}
