//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/GerMP21/Omnisnap/internal/pipeline"
	"github.com/GerMP21/Omnisnap/internal/schema"
	"github.com/GerMP21/Omnisnap/internal/schema/schematest"
	"github.com/GerMP21/Omnisnap/internal/triage/evidence"
)

// Task 47: cause parity with the spike on every golden page pair — kinds,
// selectors, sides, tiers, consequences and knowledge-base entries. The
// captures are the goldens, so this isolates the port from the machine.

type parityFinding struct {
	Kinds        []string `json:"kinds"`
	Sel          string   `json:"sel"`
	Side         string   `json:"side"`
	Tier         string   `json:"tier"`
	Consequences []string `json:"consequences"`
	KB           *struct {
		ID string `json:"id"`
	} `json:"kb"`
	Evidence struct {
		Mechanism *struct {
			Verdict string `json:"verdict"`
		} `json:"mechanism"`
		Consensus *struct {
			Verdict string `json:"verdict"`
		} `json:"consensus"`
		Perception *struct {
			Score *float64 `json:"score"`
		} `json:"perception"`
		Calibration *struct {
			Explained bool `json:"explained"`
		} `json:"calibration"`
	} `json:"evidence"`
}

// goldenProbes decodes a golden probes file; nil when absent.
func goldenProbes(t *testing.T, slug, engine, run string) map[string]*schema.Capture {
	t.Helper()
	f, err := os.Open(filepath.Join(gateGoldenDir, slug, engine+"-"+run+".probes.json.gz"))
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	m, err := evidence.ReadProbes(f)
	if err != nil {
		t.Fatalf("%s %s-%s probes: %v", slug, engine, run, err)
	}
	return m
}

func loadParityFindings(t *testing.T, slug, engine string) []parityFinding {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(gateGoldenDir, slug, "findings-"+engine+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Findings []parityFinding `json:"findings"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	return doc.Findings
}

func TestGoldenCauseParity(t *testing.T) {
	entries, err := os.ReadDir(gateGoldenDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		slug := e.Name()
		for _, eng := range []string{"firefox", "webkit"} {
			if _, err := os.Stat(filepath.Join(gateGoldenDir, slug, eng+"-a.json")); err != nil {
				continue
			}
			third := "webkit"
			if eng == "webkit" {
				third = "firefox"
			}
			pair := pipeline.Run(pipeline.Inputs{
				SlugBase: slug + "@" + eng, BaselineEngine: "chrome", Engine: eng,
				A:       loadGolden(t, slug, "chrome-a.json"),
				A2:      loadGolden(t, slug, "chrome-b.json"),
				B:       loadGolden(t, slug, eng+"-a.json"),
				B2:      loadGolden(t, slug, eng+"-b.json"),
				ProbesA: goldenProbes(t, slug, "chrome", "a"),
				ProbesB: goldenProbes(t, slug, eng, "a"),
				Thirds: []pipeline.ThirdCapture{{Engine: third,
					A: loadGolden(t, slug, third+"-a.json"),
					B: loadGolden(t, slug, third+"-b.json")}},
			})
			want := loadParityFindings(t, slug, eng)
			got := pair.Causes
			used := make([]bool, len(got))
			var problems []string
			for _, w := range want {
				idx := -1
				for i, c := range got {
					if used[i] {
						continue
					}
					if c.Selector == w.Sel && sameKinds(c.Kinds, w.Kinds) && string(c.Tier) == w.Tier {
						idx = i
						break
					}
				}
				if idx < 0 {
					// Fall back to selector+kinds (tier may differ when a
					// consequence is not implemented yet).
					for i, c := range got {
						if used[i] {
							continue
						}
						if c.Selector == w.Sel && sameKinds(c.Kinds, w.Kinds) {
							idx = i
							problems = append(problems, slug+"/"+eng+": "+w.Sel+" tier "+string(c.Tier)+", golden "+w.Tier)
							break
						}
					}
				}
				if idx < 0 {
					problems = append(problems, slug+"/"+eng+": missing "+strings.Join(w.Kinds, "+")+" at "+w.Sel)
					continue
				}
				used[idx] = true
				c := got[idx]
				if !sameSet(c.Consequences, w.Consequences) {
					problems = append(problems, slug+"/"+eng+": "+w.Sel+" consequences "+strings.Join(c.Consequences, ",")+", golden "+strings.Join(w.Consequences, ","))
				}
				gotKB, wantKB := "", ""
				if c.KB != nil {
					gotKB = *c.KB
				}
				if w.KB != nil {
					wantKB = w.KB.ID
				}
				if gotKB != wantKB {
					problems = append(problems, slug+"/"+eng+": "+w.Sel+" kb "+gotKB+", golden "+wantKB)
				}
				if w.Evidence.Calibration != nil {
					gotCal := false
					if c.Evidence.Calibration != nil {
						gotCal = c.Evidence.Calibration.Explained
					}
					if gotCal != w.Evidence.Calibration.Explained {
						problems = append(problems, slug+"/"+eng+": "+w.Sel+" calibration explained "+fmt.Sprint(gotCal)+", golden "+fmt.Sprint(w.Evidence.Calibration.Explained))
					}
				}
				if w.Evidence.Consensus != nil {
					gotC, wantC := "", w.Evidence.Consensus.Verdict
					// The spike's findings use its internal names; the
					// summary schema names the sides.
					switch wantC {
					case "B-odd":
						wantC = "engine-odd"
					case "A-odd":
						wantC = "baseline-odd"
					case "unknown":
						wantC = ""
					}
					if c.Evidence.Consensus != nil {
						gotC = string(*c.Evidence.Consensus)
					}
					if gotC != wantC {
						problems = append(problems, slug+"/"+eng+": "+w.Sel+" consensus "+gotC+", golden "+wantC)
					}
				}
				if w.Evidence.Perception != nil && w.Evidence.Perception.Score != nil {
					gotP := -1.0
					if c.Evidence.Perception != nil {
						gotP = *c.Evidence.Perception
					}
					if gotP != *w.Evidence.Perception.Score {
						problems = append(problems, fmt.Sprintf("%s/%s: %s perception %v, golden %v", slug, eng, w.Sel, gotP, *w.Evidence.Perception.Score))
					}
				}
				if w.Evidence.Mechanism != nil && w.Evidence.Mechanism.Verdict != "" {
					gotMech := ""
					if c.Evidence.Mechanism != nil {
						gotMech = *c.Evidence.Mechanism
					}
					if gotMech != w.Evidence.Mechanism.Verdict {
						problems = append(problems, slug+"/"+eng+": "+w.Sel+" mechanism "+gotMech+", golden "+w.Evidence.Mechanism.Verdict)
					}
				}
			}
			for i := range got {
				if used[i] {
					continue
				}
				problems = append(problems, slug+"/"+eng+": extra "+strings.Join(got[i].Kinds, "+")+" at "+got[i].Selector)
			}
			for _, p := range problems {
				t.Errorf("%s", p)
			}
		}
	}
}

func sameKinds(a, b []string) bool {
	as := append([]string{}, a...)
	bs := append([]string{}, b...)
	sort.Strings(as)
	sort.Strings(bs)
	return strings.Join(as, ",") == strings.Join(bs, ",")
}

func sameSet(a, b []string) bool {
	as := append([]string{}, a...)
	bs := append([]string{}, b...)
	sort.Strings(as)
	sort.Strings(bs)
	return strings.Join(as, ",") == strings.Join(bs, ",")
}

// The pipeline's causes, intent, dynamic and blocked data make a schema-valid
// summary result (Task 58): the viewer, merge and the Action can consume it.
func TestPipelineSummaryIsSchemaValid(t *testing.T) {
	vp := schema.Viewport{Width: 1280, Height: 800, DeviceScaleFactor: 1}
	job := schema.Job{Pages: []schema.PageRef{{URL: "https://example.com/", Slug: "01-card-width"}}, Viewport: &vp}
	s := schema.NewSummary("dev", job)
	s.Hosts = []schema.Host{{Machine: schema.Machine{OS: "linux", Arch: "amd64", ID: "runner"}}}
	entries, err := os.ReadDir(gateGoldenDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		slug := e.Name()
		if _, err := os.Stat(filepath.Join(gateGoldenDir, slug, "firefox-a.json")); err != nil {
			continue
		}
		pair := pipeline.Diff(slug+"@firefox", "chrome", "firefox",
			loadGolden(t, slug, "chrome-a.json"), loadGolden(t, slug, "chrome-b.json"),
			loadGolden(t, slug, "firefox-a.json"), loadGolden(t, slug, "firefox-b.json"),
			pipeline.ThirdCapture{Engine: "webkit", A: loadGolden(t, slug, "webkit-a.json"), B: loadGolden(t, slug, "webkit-b.json")})
		s.Results = append(s.Results, schema.Result{
			Host:     schema.Machine{OS: "linux", Arch: "amd64", ID: "runner"},
			Page:     schema.ResultPage{Slug: slug, URL: "https://example.com/" + slug},
			Baseline: schema.ResultBaseline{Engine: "chrome", Version: "157"},
			Engine:   schema.ResultEngine{Engine: "firefox", Family: schema.FamilyGecko, Version: "160", Profile: schema.ProfileFresh},
			Status:   schema.StatusOK, Warnings: pair.Warnings, Stats: pair.Stats,
			Shots: &schema.Shots{
				Baseline: schema.ShotRef{Src: slug + "/chrome-a.png", W: 1280, H: 800},
				Engine:   schema.ShotRef{Src: slug + "/firefox-a.png", W: 1280, H: 800},
			},
			Causes: pair.Causes, Intent: pair.Intent, Blocked: pair.Blocked, Dynamic: pair.Dynamic,
			Findings: slug + "/findings.json",
		})
		s.Pages = append(s.Pages, schema.PageTotal{Slug: slug, URL: "https://example.com/" + slug,
			Report: pair.Stats.Report, Info: pair.Stats.Info})
	}
	if len(s.Results) == 0 {
		t.Skip("no golden captures")
	}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	schematest.Validate(t, "summary", b)
}
