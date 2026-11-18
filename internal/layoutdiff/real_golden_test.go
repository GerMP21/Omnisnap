package layoutdiff

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GerMP21/Omnisnap/internal/schema"
)

// The real-site goldens are the spike's round-5 captures; they are git-ignored
// (71 MB) so the tests skip when the directory is absent. They are what Tasks
// 41 and 45 assert: run pairing, the dynamic mask and the warning text.
const realGoldenDir = "../../testdata/golden/real"

type realFindings struct {
	Warnings []string `json:"warnings"`
	Causes   []struct {
		Selector string `json:"selector"`
	} `json:"causes"`
}

func loadRealCapture(t *testing.T, path string) *schema.Capture {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	c := &schema.Capture{}
	if err := json.Unmarshal(b, c); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return c
}

// realPair runs the spike's pairing and masks for one golden site/pair.
func realPair(t *testing.T, site, engine string) (Warnings, Pair) {
	t.Helper()
	dir := filepath.Join(realGoldenDir, site)
	a := Prep(loadRealCapture(t, filepath.Join(dir, "chrome-a.json")), "chrome")
	b := Prep(loadRealCapture(t, filepath.Join(dir, engine+"-a.json")), engine)
	a2 := Prep(loadRealCapture(t, filepath.Join(dir, "chrome-b.json")), "chrome")
	b2 := Prep(loadRealCapture(t, filepath.Join(dir, engine+"-b.json")), engine)
	p := Pair{A: a, A2: a2, B: b, B2: b2}
	p.A, p.A2, p.B, p.B2, p.RunA, p.RunB = PairRuns(a, a2, b, b2)
	p.MaskA = DynamicMask(p.A, p.A2)
	p.MaskB = DynamicMask(p.B, p.B2)
	return WarningsOf(p), p
}

// Task 45: the warnings match the spike's, minus the pipeline-only note about
// design intent (the spike never wrote it).
func TestGoldenRealWarnings(t *testing.T) {
	if _, err := os.Stat(realGoldenDir); errors.Is(err, fs.ErrNotExist) {
		t.Skip("real-site goldens are not installed (git-ignored)")
	}
	entries, err := os.ReadDir(realGoldenDir)
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		for _, engine := range []string{"firefox", "webkit"} {
			golden := filepath.Join(realGoldenDir, e.Name(), "findings-"+engine+".json")
			b, err := os.ReadFile(golden)
			if err != nil {
				continue
			}
			var want realFindings
			if err := json.Unmarshal(b, &want); err != nil {
				t.Fatalf("%s: %v", golden, err)
			}
			got, _ := realPair(t, e.Name(), engine)
			wantList := make([]string, 0, len(want.Warnings))
			for _, w := range want.Warnings {
				if strings.HasPrefix(w, "design intent not checked") {
					continue
				}
				wantList = append(wantList, w)
			}
			if strings.Join(got.List, "\n") != strings.Join(wantList, "\n") {
				t.Errorf("%s/%s warnings:\n got: %v\nwant: %v", e.Name(), engine, got.List, wantList)
			}
			checked++
		}
	}
	if checked == 0 {
		t.Skip("no findings-<engine>.json in the real goldens")
	}
}

// Task 41: the spike's run pairing picks the matching states, and the mask
// covers the regions the page redraws between runs.
func TestGoldenRealPairingMasks(t *testing.T) {
	if _, err := os.Stat(realGoldenDir); errors.Is(err, fs.ErrNotExist) {
		t.Skip("real-site goldens are not installed (git-ignored)")
	}
	// python-org: webkit-a matches chrome-b's state; the mask names the three
	// regions that move between the two chrome runs.
	got, p := realPair(t, "python-org", "webkit")
	if p.RunA != "b" || p.RunB != "a" {
		t.Errorf("python-org/webkit pairing = (%s,%s); want (b,a)", p.RunA, p.RunB)
	}
	masked := strings.Join(append(append([]string{}, got.DynamicA...), got.DynamicB...), "|")
	for _, sel := range []string{"div#success-story-838 > blockquote > a", "tbody > tr > td > p > a", "div#dive-into-python > div.flex-viewport"} {
		if !strings.Contains(masked, sel) {
			t.Errorf("python-org mask %q misses %q", masked, sel)
		}
	}
	// stripe/firefox: the second run is the matching state.
	if _, p := realPair(t, "stripe", "firefox"); p.RunB != "b" {
		t.Errorf("stripe/firefox runB = %q; want b", p.RunB)
	}
}
