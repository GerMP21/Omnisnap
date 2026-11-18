package merge

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GerMP21/Omnisnap/internal/schema"
)

// makeBundle writes a small bundle for one host.
func makeBundle(t *testing.T, dir, hostID, osName, engine string, report int, mutate func(*schema.Summary)) Bundle {
	t.Helper()
	vp := schema.Viewport{Width: 1280, Height: 800, DeviceScaleFactor: 1}
	j := schema.Job{Pages: []schema.PageRef{{URL: "https://example.com/", Slug: "home"}}, Viewport: &vp}
	s := schema.NewSummary("dev", j)
	s.Generated = time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	s.Hosts = []schema.Host{{Machine: schema.Machine{OS: osName, Arch: "amd64", ID: hostID}}}
	s.Results = []schema.Result{{
		Host:     schema.Machine{OS: osName, Arch: "amd64", ID: hostID},
		Page:     schema.ResultPage{Slug: "home", URL: "https://example.com/"},
		Baseline: schema.ResultBaseline{Engine: "chrome", Version: "157"},
		Engine:   schema.ResultEngine{Engine: engine, Family: schema.FamilyBlink, Version: "157", Profile: schema.ProfileFresh},
		Status:   schema.StatusOK,
		Warnings: []string{},
		Stats:    schema.Stats{MatchRate: 1, Report: report, Info: 0},
		Shots: &schema.Shots{
			Baseline: schema.ShotRef{Src: "home/chrome-a.png", W: 1280, H: 900},
			Engine:   schema.ShotRef{Src: "home/" + engine + "-a.png", W: 1280, H: 900},
		},
		Findings: "home/findings.json",
		Causes:   []schema.Cause{},
		Intent:   []schema.IntentViolation{},
		Blocked:  schema.Sides{Baseline: []string{}, Engine: []string{}},
		Dynamic:  schema.Sides{Baseline: []string{}, Engine: []string{}},
	}}
	s.Pages = []schema.PageTotal{{Slug: "home", URL: "https://example.com/", Report: report}}
	if mutate != nil {
		mutate(&s)
	}
	if err := os.MkdirAll(filepath.Join(dir, "home"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"chrome-a.png", engine + "-a.png", "findings.json"} {
		if err := os.WriteFile(filepath.Join(dir, "home", f), []byte(f), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	b, err := json.MarshalIndent(s, "", " ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "summary.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	return Bundle{Dir: dir, Summary: s}
}

func TestMergeThreeHosts(t *testing.T) {
	u := makeBundle(t, t.TempDir(), "ubuntu", "linux", "firefox", 2, nil)
	w := makeBundle(t, t.TempDir(), "windows", "windows", "edge", 1, nil)
	mac := makeBundle(t, t.TempDir(), "macos", "darwin", "safari", 3, nil)
	out := t.TempDir()
	m, err := Merge([]Bundle{u, w, mac}, Options{Out: out, RenameHosts: false})
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Summary.Hosts) != 3 || len(m.Summary.Results) != 3 {
		t.Fatalf("hosts %d, results %d; want 3 and 3", len(m.Summary.Hosts), len(m.Summary.Results))
	}
	// Sorted by page, host, engine: macos, ubuntu, windows.
	if got := []string{m.Summary.Results[0].Host.ID, m.Summary.Results[1].Host.ID, m.Summary.Results[2].Host.ID}; strings.Join(got, ",") != "macos,ubuntu,windows" {
		t.Errorf("host order %v", got)
	}
	r := m.Summary.Results[0]
	if r.Shots.Baseline.Src != "macos/home/chrome-a.png" || r.Findings != "macos/home/findings.json" {
		t.Errorf("paths not rewritten: %+v, %q", r.Shots, r.Findings)
	}
	// Page totals sum across hosts.
	if len(m.Summary.Pages) != 1 || m.Summary.Pages[0].Report != 6 {
		t.Errorf("pages %+v; want one page with 6 Report causes", m.Summary.Pages)
	}
	// Every page file is copied once, under its host.
	if len(m.Copies) != 9 {
		t.Errorf("%d copies; want 9", len(m.Copies))
	}
	for _, c := range m.Copies {
		if !strings.HasPrefix(c.Dst, out) {
			t.Errorf("copy %s outside the output", c.Dst)
		}
	}
}

func TestMergeOrderIndependent(t *testing.T) {
	u := makeBundle(t, t.TempDir(), "ubuntu", "linux", "firefox", 2, nil)
	w := makeBundle(t, t.TempDir(), "windows", "windows", "edge", 1, nil)
	mac := makeBundle(t, t.TempDir(), "macos", "darwin", "safari", 3, nil)
	a, err := Merge([]Bundle{u, w, mac}, Options{Out: "out"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := Merge([]Bundle{mac, u, w}, Options{Out: "out"})
	if err != nil {
		t.Fatal(err)
	}
	ja, _ := json.Marshal(a.Summary)
	jb, _ := json.Marshal(b.Summary)
	if string(ja) != string(jb) {
		t.Error("merge is order-dependent")
	}
}

func TestMergeConflicts(t *testing.T) {
	u := makeBundle(t, t.TempDir(), "ubuntu", "linux", "firefox", 1, nil)
	other := makeBundle(t, t.TempDir(), "windows", "windows", "edge", 1, nil)
	other.Summary.Job.Viewport.Width = 1024
	if _, err := Merge([]Bundle{u, other}, Options{Out: "out"}); err == nil || !strings.Contains(err.Error(), "job") {
		t.Errorf("different job: err = %v; want a job conflict", err)
	}
	dup := makeBundle(t, t.TempDir(), "ubuntu", "linux", "firefox", 1, nil)
	if _, err := Merge([]Bundle{u, dup}, Options{Out: "out"}); err == nil || !strings.Contains(err.Error(), "ubuntu") {
		t.Errorf("duplicate host: err = %v; want a host-id conflict", err)
	}
	m, err := Merge([]Bundle{u, dup}, Options{Out: "out", RenameHosts: true})
	if err != nil {
		t.Fatal(err)
	}
	ids := m.Summary.Hosts[0].Machine.ID + "," + m.Summary.Hosts[1].Machine.ID
	if ids != "ubuntu,ubuntu-2" {
		t.Errorf("renamed hosts %q; want ubuntu,ubuntu-2", ids)
	}
}

func TestApplyWritesFilesAndSummary(t *testing.T) {
	u := makeBundle(t, t.TempDir(), "ubuntu", "linux", "firefox", 1, nil)
	out := t.TempDir()
	m, err := Merge([]Bundle{u}, Options{Out: out})
	if err != nil {
		t.Fatal(err)
	}
	var wrote schema.Summary
	if err := Apply(m, out, func(dir string, s schema.Summary) error {
		wrote = s
		b, _ := json.Marshal(s)
		return os.WriteFile(filepath.Join(dir, "summary.json"), b, 0o644)
	}); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"ubuntu/home/chrome-a.png", "ubuntu/home/findings.json", "summary.json"} {
		if _, err := os.Stat(filepath.Join(out, p)); err != nil {
			t.Errorf("%s: %v", p, err)
		}
	}
	if len(wrote.Results) != 1 {
		t.Errorf("summary callback got %d results", len(wrote.Results))
	}
	// Inputs untouched.
	if _, err := os.Stat(filepath.Join(u.Dir, "summary.json")); err != nil {
		t.Error(err)
	}
}
