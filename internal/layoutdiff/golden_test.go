package layoutdiff

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/GerMP21/Omnisnap/internal/schema"
)

// Golden parity (Task 47): the Go port must produce the spike's causes on
// every golden page pair. The goldens are the spike's own findings
// (testdata/golden/planted/<slug>/findings-<engine>.json, exported from the
// validated JavaScript).

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

// goldenFinding is the part of the spike's findings this stage produces.
type goldenFinding struct {
	Kinds []string `json:"kinds"`
	Sel   string   `json:"sel"`
	Side  string   `json:"side"`
	D     struct {
		DW, DH, DX, DY float64
		DLN            int `json:"dln"`
		Ovf            string
	} `json:"d"`
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

func onlyIntent(kinds []string) bool {
	for _, k := range kinds {
		if !strings.HasPrefix(k, "intent-") {
			return false
		}
	}
	return len(kinds) > 0
}

func causeKey(side, sel string, kinds []string) string {
	k := append([]string{}, kinds...)
	sort.Strings(k)
	return side + "|" + sel + "|" + strings.Join(k, ",")
}

// TestGoldenAlignParity compares the pre-triage causes with the spike's
// findings on every planted page and pair. Findings that only triage adds
// (consequences, tier, evidence) are ignored here; the selectors, sides and
// kinds must match.
func TestGoldenAlignParity(t *testing.T) {
	if testing.Short() {
		t.Skip("golden parity over the planted suite")
	}
	slugs, err := os.ReadDir(goldenDir)
	if err != nil {
		t.Fatal(err)
	}
	pairs := []struct{ engine, golden string }{{"firefox", "firefox"}, {"webkit", "webkit"}}
	for _, entry := range slugs {
		if !entry.IsDir() {
			continue
		}
		slug := entry.Name()
		for _, p := range pairs {
			if _, err := os.Stat(filepath.Join(goldenDir, slug, p.engine+"-a.json")); err != nil {
				continue
			}
			p := p
			t.Run(slug+"/"+p.golden, func(t *testing.T) {
				// The spike's run pairing and dynamic masks (Task 41): compare
				// the runs whose page states match best; the other run of each
				// engine masks dynamic content.
				A, A2, B, B2, _, _ := PairRuns(
					Prep(loadCapture(t, slug, "chrome-a.json"), "chrome"),
					Prep(loadCapture(t, slug, "chrome-b.json"), "chrome"),
					Prep(loadCapture(t, slug, p.engine+"-a.json"), p.golden),
					Prep(loadCapture(t, slug, p.engine+"-b.json"), p.golden))
				maskA := DynamicMask(A, A2)
				maskB := DynamicMask(B, B2)
				m := MatchTrees(A, B)
				res := Analyze(A, B, m, maskA, maskB)
				got := map[string]Cause{}
				for _, c := range res.Causes {
					sel := Selector(c.Node)
					if c.Side == "B" {
						sel = Selector(c.Node)
					}
					got[causeKey(c.Side, sel, c.Kinds)] = c
				}
				want := map[string]goldenFinding{}
				for _, f := range loadFindings(t, slug, p.golden) {
					// intent-* kinds come from the intent detector (Task 46)
					// and its join (Task 48); this stage does not make them.
					if onlyIntent(f.Kinds) {
						continue
					}
					want[causeKey(f.Side, f.Sel, f.Kinds)] = f
				}
				for k := range want {
					if _, ok := got[k]; !ok {
						t.Errorf("missing cause %q", k)
					}
				}
				for k, c := range got {
					if strings.Contains(k, "|intent-") {
						continue
					}
					f, ok := want[k]
					if !ok {
						// Triage (Tasks 52-57) drops or merges some
						// pre-triage causes; the align stage legitimately has
						// them (confirmed against the spike's own analyzer).
						t.Logf("extra pre-triage cause %q", k)
						continue
					}
					var d Delta
					if c.D != nil {
						d = *c.D
					}
					if fmt.Sprint(d.DW, d.DH, d.DX, d.DY, d.DLN, d.Ovf) != fmt.Sprint(f.D.DW, f.D.DH, f.D.DX, f.D.DY, f.D.DLN, f.D.Ovf) {
						t.Errorf("%q delta %v,%v,%v,%v,%v,%q; golden %v,%v,%v,%v,%v,%q", k,
							d.DW, d.DH, d.DX, d.DY, d.DLN, d.Ovf, f.D.DW, f.D.DH, f.D.DX, f.D.DY, f.D.DLN, f.D.Ovf)
					}
				}
			})
		}
	}
}
