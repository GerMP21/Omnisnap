//go:build e2e

package e2e

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GerMP21/Omnisnap/internal/groundtruth"
	"github.com/GerMP21/Omnisnap/internal/pipeline"
	"github.com/GerMP21/Omnisnap/internal/schema"
)

// Task 59: the ground-truth gates on every PR. The captures are the golden
// ones (the captured port is checked separately by TestGoldenGeometry), so
// this gates the comparison: recall on the planted bugs, no Report on
// controls, knowledge-base pages explained, budgets respected.

const gateGoldenDir = "../../testdata/golden/planted"

func loadGolden(t *testing.T, slug, file string) *schema.Capture {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(gateGoldenDir, slug, file))
	if err != nil {
		t.Fatal(err)
	}
	var c schema.Capture
	if err := json.Unmarshal(b, &c); err != nil {
		t.Fatal(err)
	}
	return &c
}

// wilson is the 95% lower bound of a recall, as the spike reports it.
func wilson(k, n int) float64 {
	if n == 0 {
		return 1
	}
	p := float64(k) / float64(n)
	z := 1.96
	return (p + z*z/(2*float64(n)) - z*math.Sqrt((p*(1-p)+z*z/(4*float64(n)))/float64(n))) / (1 + z*z/float64(n))
}

func TestPlantedGates(t *testing.T) {
	if testing.Short() {
		t.Skip("ground-truth gates")
	}
	expectations, err := groundtruth.LoadPlanted("../../testdata/planted")
	if err != nil {
		t.Fatal(err)
	}
	bySlug := map[string]groundtruth.Page{}
	for _, p := range expectations {
		bySlug[p.Slug] = p
	}
	pairs := []string{"firefox", "webkit"}
	found, total, leaks, budgetBreaks := 0, 0, 0, 0
	intentPages, intentHit := 0, 0
	for slug, page := range bySlug {
		if _, err := os.Stat(filepath.Join(gateGoldenDir, slug, "chrome-a.json")); err != nil {
			continue
		}
		for _, eng := range pairs {
			third := "webkit"
			if eng == "webkit" {
				third = "firefox"
			}
			pair := pipeline.Diff(slug+"@"+eng, "chrome", eng,
				loadGolden(t, slug, "chrome-a.json"),
				loadGolden(t, slug, "chrome-b.json"),
				loadGolden(t, slug, eng+"-a.json"),
				loadGolden(t, slug, eng+"-b.json"),
				pipeline.ThirdCapture{Engine: third,
					A: loadGolden(t, slug, third+"-a.json"),
					B: loadGolden(t, slug, third+"-b.json")})
			reports := 0
			for _, c := range pair.Causes {
				if c.Tier == schema.TierReport {
					reports++
				}
			}
			switch {
			case page.Kind == groundtruth.KindNative:
				// Genuine form-control differences: scored only through
				// OnlyTags, not as a control (the spike's gate says the same).
			case page.Kind == groundtruth.KindPlanted && eng == "firefox":
				if page.MaxFindings != nil && len(pair.Causes) > *page.MaxFindings {
					budgetBreaks++
					t.Errorf("%s: %d causes; budget %d", slug, len(pair.Causes), *page.MaxFindings)
				}
				if page.Intent {
					intentPages++
					ok := false
					for _, v := range pair.Intent {
						if len(page.Expect) == 0 || hitOne(page.Expect, v.Selector) {
							ok = true
						}
					}
					if ok {
						intentHit++
					}
				}
				for _, group := range page.Expect {
					total++
					if hit(pair.Causes, group.AnyOf) {
						found++
					} else {
						t.Errorf("%s: no Report cause at %v (found %v)", slug, group.AnyOf, selectors(pair.Causes))
					}
				}
			default:
				// Controls, KB fixtures, and the WebKit pair of planted pages:
				// nothing may be a Report cause.
				leaks += reports
				if reports > 0 {
					t.Errorf("%s/%s: %d Report cause(s) on a control: %v", slug, eng, reports, reportSelectors(pair.Causes))
				}
			}
		}
	}
	recall := float64(found) / float64(max(total, 1))
	if recall < 0.9 {
		t.Errorf("planted recall %.2f (%d/%d, Wilson lower bound %.2f); want >= 0.9", recall, found, total, wilson(found, total))
	}
	if leaks != 0 || budgetBreaks != 0 {
		t.Errorf("%d control leaks, %d budget breaks; want 0", leaks, budgetBreaks)
	}
	if intentPages > 0 && float64(intentHit)/float64(intentPages) < 5.0/6.0 {
		t.Errorf("intent violations found on %d/%d planted intent pages; want >= 5/6", intentHit, intentPages)
	}
	t.Logf("intent gates: %d/%d planted intent pages", intentHit, intentPages)
	t.Logf("planted gates: recall %d/%d (%.0f%%, Wilson lower bound %.2f), %d control leaks, %d budget breaks",
		found, total, recall*100, wilson(found, total), leaks, budgetBreaks)
}

// hitOne reports a selector at an element with one of the ids.
func hitOne(groups []groundtruth.ExpectGroup, sel string) bool {
	for _, g := range groups {
		for _, id := range g.AnyOf {
			if strings.Contains(sel, "#"+id) {
				return true
			}
		}
	}
	return false
}

// hit reports a Report cause at an element with one of the ids.
func hit(causes []schema.Cause, ids []string) bool {
	for _, c := range causes {
		if c.Tier != schema.TierReport {
			continue
		}
		for _, id := range ids {
			if strings.Contains(c.Selector, "#"+id) {
				return true
			}
			for _, inst := range c.Instances {
				if strings.Contains(inst, "#"+id) {
					return true
				}
			}
		}
	}
	return false
}

func selectors(causes []schema.Cause) []string {
	var out []string
	for _, c := range causes {
		out = append(out, string(c.Tier)+":"+strings.Join(c.Kinds, "+")+"@"+c.Selector)
	}
	return out
}

func reportSelectors(causes []schema.Cause) []string {
	var out []string
	for _, c := range causes {
		if c.Tier == schema.TierReport {
			out = append(out, strings.Join(c.Kinds, "+")+"@"+c.Selector)
		}
	}
	return out
}
