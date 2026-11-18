package triage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/GerMP21/Omnisnap/internal/layoutdiff"
	"github.com/GerMP21/Omnisnap/internal/schema"
)

const goldenDir = "../../testdata/golden/planted"

func loadCapture(t *testing.T, slug, file string) *schema.Capture {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(goldenDir, slug, file))
	if err != nil {
		t.Fatal(err)
	}
	var c schema.Capture
	if err := json.Unmarshal(b, &c); err != nil {
		t.Fatal(err)
	}
	return &c
}

type goldenFinding struct {
	ID           string   `json:"id"`
	Kinds        []string `json:"kinds"`
	Sel          string   `json:"sel"`
	Side         string   `json:"side"`
	Tier         string   `json:"tier"`
	Consequences []string `json:"consequences"`
	KB           *struct {
		ID string `json:"id"`
	} `json:"kb"`
	Severity int `json:"severity"`
}

func loadFindings(t *testing.T, slug, engine string) []goldenFinding {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(goldenDir, slug, "findings-"+engine+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Findings []goldenFinding `json:"findings"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	return doc.Findings
}

// TestGoldenTriageParity is Task 52/53/57's gate: our grouped causes match the
// spike's findings on tier, consequences, knowledge-base entry and selector.
func TestGoldenTriageParity(t *testing.T) {
	if testing.Short() {
		t.Skip("golden parity over the planted suite")
	}
	entries, err := os.ReadDir(goldenDir)
	if err != nil {
		t.Fatal(err)
	}
	pairs := []struct{ engine, golden, idBase string }{
		{"firefox", "firefox", ""}, // Firefox keeps the bare slug
		{"webkit", "webkit", "@webkit"},
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		slug := e.Name()
		for _, p := range pairs {
			if _, err := os.Stat(filepath.Join(goldenDir, slug, p.engine+"-a.json")); err != nil {
				continue
			}
			p := p
			t.Run(slug+"/"+p.golden, func(t *testing.T) {
				A, A2, B, B2, _, _ := layoutdiff.PairRuns(
					layoutdiff.Prep(loadCapture(t, slug, "chrome-a.json"), "chrome"),
					layoutdiff.Prep(loadCapture(t, slug, "chrome-b.json"), "chrome"),
					layoutdiff.Prep(loadCapture(t, slug, p.engine+"-a.json"), p.golden),
					layoutdiff.Prep(loadCapture(t, slug, p.engine+"-b.json"), p.golden))
				maskA, maskB := layoutdiff.DynamicMask(A, A2), layoutdiff.DynamicMask(B, B2)
				m := layoutdiff.MatchTrees(A, B)
				res := layoutdiff.Analyze(A, B, m, maskA, maskB)
				got := Build(res, A, B, slug+p.idBase, p.golden, Options{})
				want := loadFindings(t, slug, p.golden)

				byID := map[string]schema.Cause{}
				for _, c := range got {
					byID[c.ID] = c
				}
				checked := 0
				for _, w := range want {
					if onlyIntentKinds(w.Kinds) {
						continue // Task 46/48
					}
					checked++
					g, ok := byID[w.ID]
					if !ok {
						// Report ids can differ when the spike merged causes;
						// fall back to kinds+selector.
						for _, c := range got {
							if c.Selector == w.Sel && strings.Join(c.Kinds, "+") == strings.Join(w.Kinds, "+") {
								g, ok = c, true
								break
							}
						}
					}
					if !ok {
						t.Errorf("missing cause %q (%v at %s)", w.ID, w.Kinds, w.Sel)
						continue
					}
					if string(g.Tier) != w.Tier {
						t.Errorf("%s tier %q; golden %q", w.ID, g.Tier, w.Tier)
					}
					if !sameSet(g.Consequences, w.Consequences) {
						t.Errorf("%s consequences %v; golden %v", w.ID, g.Consequences, w.Consequences)
					}
					gkb, wkb := "", ""
					if g.KB != nil {
						gkb = *g.KB
					}
					if w.KB != nil {
						wkb = w.KB.ID
					}
					if gkb != wkb {
						t.Errorf("%s kb %q; golden %q", w.ID, gkb, wkb)
					}
				}
				if checked == 0 {
					t.Logf("no comparable findings")
				}
			})
		}
	}
}

func onlyIntentKinds(kinds []string) bool {
	for _, k := range kinds {
		if !strings.HasPrefix(k, "intent-") {
			return false
		}
	}
	return len(kinds) > 0
}

func sameSet(a, b []string) bool {
	as := append([]string{}, a...)
	bs := append([]string{}, b...)
	sort.Strings(as)
	sort.Strings(bs)
	return strings.Join(as, ",") == strings.Join(bs, ",")
}
