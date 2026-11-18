//go:build e2e

package e2e

import (
	"encoding/json"
	"image"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GerMP21/Omnisnap/internal/appearance"
	"github.com/GerMP21/Omnisnap/internal/groundtruth"
	"github.com/GerMP21/Omnisnap/internal/layoutdiff"
	"github.com/GerMP21/Omnisnap/internal/pipeline"
)

// Task 50/51: appearance-only causes on the golden pairs. The spike did not
// run the appearance module, so the goldens only gate the controls: 0 causes
// where the two engines draw the same content.
func TestAppearanceControls(t *testing.T) {
	expectations, err := groundtruth.LoadPlanted("../../testdata/planted")
	if err != nil {
		t.Fatal(err)
	}
	kind := map[string]groundtruth.Kind{}
	for _, p := range expectations {
		kind[p.Slug] = p.Kind
	}
	entries, err := os.ReadDir(gateGoldenDir)
	if err != nil {
		t.Fatal(err)
	}
	seen, causesOnControls := 0, 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		slug := e.Name()
		if kind[slug] != groundtruth.KindControl {
			continue
		}
		imgA, okA := loadPNG(filepath.Join(gateGoldenDir, slug, "chrome-a.png"))
		imgB, okB := loadPNG(filepath.Join(gateGoldenDir, slug, "firefox-a.png"))
		if !okA || !okB {
			continue
		}
		seen++
		A := layoutdiff.Prep(loadGolden(t, slug, "chrome-a.json"), "chrome")
		B := layoutdiff.Prep(loadGolden(t, slug, "firefox-a.json"), "firefox")
		m := layoutdiff.MatchTrees(A, B)
		res := layoutdiff.Analyze(A, B, m, map[int]bool{}, map[int]bool{})
		gs := appearance.Detect(A, B, m, res.D, map[int]bool{}, map[int]bool{}, imgA, imgB)
		causesOnControls += len(gs)
		for _, g := range gs {
			t.Errorf("%s: %s drawn differently (SSIM %.3f); a control must be silent", slug, layoutdiff.Selector(g.Node), g.SSIM)
		}
	}
	if seen == 0 {
		t.Skip("no control captures with screenshots")
	}
	t.Logf("appearance controls: %d pages, %d causes", seen, causesOnControls)
}

// The planted appearance bugs (Task 51) are reported at the right element.
// The pages live in testdata/planted/appearance-*.html with their own answer
// file; captures are made by the test below when a browser is available.
func TestPlantedAppearancePages(t *testing.T) {
	b, err := os.ReadFile("../../testdata/planted/appearance-expectations.json")
	if err != nil {
		t.Skip("no appearance pages yet")
	}
	var doc struct {
		Pages []struct {
			Slug string `json:"slug"`
			ID   string `json:"id"`
		} `json:"pages"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Pages) == 0 {
		t.Skip("no appearance pages yet")
	}
	t.Logf("%d planted appearance pages; captured by the appearance e2e job", len(doc.Pages))
}

func loadPNG(path string) (image.Image, bool) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer func() { _ = f.Close() }()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, false
	}
	return img, true
}

// Task 51: the planted appearance pages fire in Firefox (Firefox-only CSS via
// @supports/-moz-appearance) and the controls stay silent.
func TestPlantedAppearanceBugs(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e capture suite")
	}
	b, err := os.ReadFile("../../testdata/planted/appearance-expectations.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Pages []struct {
			Slug  string  `json:"slug"`
			ID    string  `json:"id"`
			Check *string `json:"check"`
		} `json:"pages"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	base := plantedBase(t)
	var chrome, firefox *engine
	es := engines(t)
	for i := range es {
		switch es[i].golden {
		case "chrome":
			chrome = &es[i]
		case "firefox":
			firefox = &es[i]
		}
	}
	if chrome == nil || firefox == nil {
		t.Skip("appearance pages need Chromium and Firefox")
	}
	out := t.TempDir()
	for _, page := range doc.Pages {
		page := page
		t.Run(page.Slug, func(t *testing.T) {
			ca := capturePage(t, *chrome, out, page.Slug, base+"/"+page.Slug, "a")
			cb := capturePage(t, *firefox, out, page.Slug, base+"/"+page.Slug, "a")
			imgA, okA := loadPNG(filepath.Join(out, page.Slug, "chrome-a.png"))
			imgB, okB := loadPNG(filepath.Join(out, page.Slug, "firefox-a.png"))
			if !okA || !okB {
				t.Fatal("screenshots missing")
			}
			_ = okA
			_ = okB
			pair := pipeline.Run(pipeline.Inputs{
				SlugBase: page.Slug + "@firefox", BaselineEngine: "chrome", Engine: "firefox",
				A: ca, B: cb, ImagesA: imgA, ImagesB: imgB,
			})
			var got []string
			for _, c := range pair.Causes {
				if strings.Contains(c.Selector, "#"+page.ID) {
					got = append(got, string(c.Tier)+":"+strings.Join(c.Kinds, "+"))
				}
			}
			if page.Check != nil {
				if len(got) != 1 || got[0] != "report:appearance" {
					t.Errorf("%s causes at #%s = %v; want one report appearance cause", page.Slug, page.ID, got)
				}
			} else if len(got) != 0 {
				t.Errorf("%s control fired %v", page.Slug, got)
			}
		})
	}
}
