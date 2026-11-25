//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GerMP21/Omnisnap/internal/capture"
	"github.com/GerMP21/Omnisnap/internal/drivers"
	"github.com/GerMP21/Omnisnap/internal/pipeline"
	"github.com/GerMP21/Omnisnap/internal/schema"
)

// Change fix-selector-sibling-index: every cause's selector, pasted into the
// baseline page, selects the element whose box the cause reports. Runs in the
// planted-gates CI job (-run Planted) on the runner's Chrome and Firefox.

// selectorFixture is the page built for this change: display: none same-tag
// siblings before the planted differences, which carry data-expect.
const selectorFixture = "selector-hidden-sibling"

// boxOf is evaluated in the baseline page: the border box of the selector's
// first match in document px (as extract.js measures it) and its data-expect,
// or null when nothing matches.
const boxOf = `(sel) => {
  const el = document.querySelector(sel);
  if (!el) return null;
  const r = el.getBoundingClientRect();
  return { b: [r.x + scrollX, r.y + scrollY, r.width, r.height], expect: el.getAttribute('data-expect') || '' };
}`

func TestPlantedSelectors(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e capture suite")
	}
	var chrome, other *engine
	for _, e := range engines(t) {
		switch e.golden {
		case "chrome":
			chrome = &e
		case "firefox":
			if other == nil {
				other = &e
			}
		}
	}
	if chrome == nil || other == nil {
		t.Skip("needs Chrome and Firefox")
	}
	base := plantedBase(t)
	out := t.TempDir()
	slugs := append([]string{selectorFixture}, plantedSlugs(t)...)
	for _, slug := range slugs {
		t.Run(slug, func(t *testing.T) {
			url := base + "/" + slug
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			// The baseline session stays open after its capture: the
			// selectors are checked in the page the boxes were measured in.
			s, closeAll := chrome.start(t)
			defer closeAll()
			a := captureIn(ctx, t, s, *chrome, out, slug, url)
			b := captureFresh(ctx, t, *other, out, slug, url)
			pair := pipeline.Run(pipeline.Inputs{SlugBase: slug, BaselineEngine: "chrome", Engine: other.id, A: a, B: b})
			expected := map[string]bool{}
			for _, c := range pair.Causes {
				if c.Boxes.Baseline == nil {
					continue // only in the engine: not in this page
				}
				got := evalBox(ctx, t, s, c.Selector)
				if got == nil {
					t.Errorf("%s (%v): querySelector(%q) matches nothing", c.ID, c.Kinds, c.Selector)
					continue
				}
				for i := range 4 {
					if math.Abs(got.B[i]-c.Boxes.Baseline[i]) > 0.1+1e-6 {
						t.Errorf("%s (%v): querySelector(%q) has box %v; the cause reports %v", c.ID, c.Kinds, c.Selector, got.B, *c.Boxes.Baseline)
						break
					}
				}
				if got.Expect != "" {
					expected[got.Expect] = true
				}
			}
			if slug == selectorFixture {
				for _, want := range []string{"wide-card", "tall-step"} {
					if !expected[want] {
						t.Errorf("no cause selects the data-expect=%q element: %v", want, selectors(pair.Causes))
					}
				}
			}
			t.Logf("%d causes: %v", len(pair.Causes), selectors(pair.Causes))
		})
	}
}

// captureIn captures run a in an open session and leaves the page loaded.
func captureIn(ctx context.Context, t *testing.T, s drivers.Session, e engine, out, slug, url string) *schema.Capture {
	t.Helper()
	if _, err := capture.Page(ctx, s, capture.Options{Out: out, Slug: slug, URL: url, Engine: e.id, Run: "a", NoProbes: true}); err != nil {
		t.Fatalf("capture %s in %s: %v", slug, e.id, err)
	}
	return readCapture(t, filepath.Join(out, slug, e.id+"-a.json"))
}

// captureFresh captures run a in a browser of its own, closed afterwards.
func captureFresh(ctx context.Context, t *testing.T, e engine, out, slug, url string) *schema.Capture {
	t.Helper()
	s, closeAll := e.start(t)
	defer closeAll()
	c := captureIn(ctx, t, s, e, out, slug, url)
	_ = s.Close(context.WithoutCancel(ctx))
	return c
}

func readCapture(t *testing.T, path string) *schema.Capture {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var c schema.Capture
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	return &c
}

type matched struct {
	B      [4]float64 `json:"b"`
	Expect string     `json:"expect"`
}

func evalBox(ctx context.Context, t *testing.T, s drivers.Session, sel string) *matched {
	t.Helper()
	arg, _ := json.Marshal(sel)
	raw, err := s.Evaluate(ctx, "("+boxOf+")("+string(arg)+")")
	if err != nil {
		t.Fatalf("querySelector(%q): %v", sel, err)
	}
	if strings.TrimSpace(raw) == "null" {
		return nil
	}
	var m matched
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("querySelector(%q) result %s: %v", sel, raw, err)
	}
	return &m
}
