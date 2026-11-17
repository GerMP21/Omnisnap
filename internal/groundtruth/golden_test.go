package groundtruth

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// Golden fixtures (testdata/golden/README.md): the spike's captures and
// findings that the Go port must reproduce. planted/ is committed; real/ holds
// third-party content and exists only where it was exported locally.
const goldenDir = "../../testdata/golden"

var (
	goldenEngines = []string{"chrome", "firefox", "webkit"}
	goldenPairs   = []string{"firefox", "webkit"}
	pngSignature  = []byte("\x89PNG\r\n\x1a\n")
)

type goldenMeta struct {
	Baseline     string   `json:"baseline"`
	FeatureNames []string `json:"featureNames"`
	Pages        []string `json:"pages"`
}

func TestGoldenPlantedCoversTheSuite(t *testing.T) {
	meta := readGoldenMeta(t, filepath.Join(goldenDir, "planted"))
	pages, err := LoadPlanted(plantedDir)
	if err != nil {
		t.Fatal(err)
	}
	var want []string
	for _, p := range pages {
		want = append(want, p.Slug)
	}
	slices.Sort(want)
	if !slices.Equal(meta.Pages, want) {
		t.Errorf("golden planted pages = %v, want the planted suite %v", meta.Pages, want)
	}
	checkGoldenGroup(t, "planted", meta, true)
}

func TestGoldenRealSites(t *testing.T) {
	dir := filepath.Join(goldenDir, "real")
	if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
		t.Skip("testdata/golden/real is not in git (third-party content): export it locally, see testdata/golden/README.md")
	}
	meta := readGoldenMeta(t, dir)
	if len(meta.Pages) != 10 {
		t.Errorf("%d real sites, want 10", len(meta.Pages))
	}
	checkGoldenGroup(t, "real", meta, false)
}

func checkGoldenGroup(t *testing.T, group string, meta goldenMeta, titleIsSlug bool) {
	t.Helper()
	if meta.Baseline != "chrome" {
		t.Errorf("%s: baseline %q, want chrome", group, meta.Baseline)
	}
	if len(meta.FeatureNames) != 82 {
		t.Errorf("%s: %d feature names, want 82 (SPEC-ground-truth.md)", group, len(meta.FeatureNames))
	}
	for _, slug := range meta.Pages {
		t.Run(group+"/"+slug, func(t *testing.T) {
			dir := filepath.Join(goldenDir, group, slug)
			for _, e := range goldenEngines {
				for _, run := range []string{"a", "b"} {
					checkCapture(t, filepath.Join(dir, e+"-"+run+".json"), slug, titleIsSlug)
					checkProbes(t, filepath.Join(dir, e+"-"+run+".probes.json.gz"))
				}
				checkPNG(t, filepath.Join(dir, e+"-a.png"))
			}
			for _, pair := range goldenPairs {
				checkFindings(t, filepath.Join(dir, "findings-"+pair+".json"), slug, pair, len(meta.FeatureNames))
			}
		})
	}
}

func checkCapture(t *testing.T, path, slug string, titleIsSlug bool) {
	t.Helper()
	var c struct {
		Meta struct {
			URL   string  `json:"url"`
			Title string  `json:"title"`
			VW    float64 `json:"vw"`
		} `json:"meta"`
		Root struct {
			T string `json:"t"`
		} `json:"root"`
	}
	readJSON(t, path, &c)
	if c.Root.T != "html" || c.Meta.URL == "" || c.Meta.VW != 1280 {
		t.Errorf("%s: root %q, url %q, viewport width %v; want an html root, a url and 1280", path, c.Root.T, c.Meta.URL, c.Meta.VW)
	}
	if titleIsSlug && c.Meta.Title != slug {
		t.Errorf("%s: title %q, want %q", path, c.Meta.Title, slug)
	}
}

func checkProbes(t *testing.T, path string) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	var probes map[string]json.RawMessage
	if err := json.NewDecoder(zr).Decode(&probes); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	// The probe ids of inject/probes.js.
	for _, id := range []string{"lh", "font", "controls", "all"} {
		if _, ok := probes[id]; !ok {
			t.Errorf("%s: no %q probe", path, id)
		}
	}
}

func checkPNG(t *testing.T, path string) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(b, pngSignature) {
		t.Errorf("%s: not a PNG", path)
	}
}

func checkFindings(t *testing.T, path, slug, pair string, nFeatures int) {
	t.Helper()
	var doc struct {
		Slug     string `json:"slug"`
		Pair     string `json:"pair"`
		Findings []struct {
			ID       string    `json:"id"`
			Kinds    []string  `json:"kinds"`
			Sel      string    `json:"sel"`
			Side     string    `json:"side"`
			Tier     string    `json:"tier"`
			D        any       `json:"d"`
			Features []float64 `json:"features"`
		} `json:"findings"`
	}
	readJSON(t, path, &doc)
	if doc.Slug != slug || doc.Pair != pair {
		t.Errorf("%s: slug %q pair %q, want %q %q", path, doc.Slug, doc.Pair, slug, pair)
	}
	for _, f := range doc.Findings {
		switch {
		case f.ID == "" || len(f.Kinds) == 0 || f.Sel == "":
			t.Errorf("%s: finding without id, kinds or selector: %+v", path, f)
		case f.Side != "A" && f.Side != "B":
			t.Errorf("%s: %s: side %q", path, f.ID, f.Side)
		case f.Tier != "report" && f.Tier != "info":
			t.Errorf("%s: %s: tier %q", path, f.ID, f.Tier)
		case len(f.Features) != nFeatures:
			t.Errorf("%s: %s: %d features, want %d", path, f.ID, len(f.Features), nFeatures)
		}
	}
}

func readGoldenMeta(t *testing.T, dir string) goldenMeta {
	t.Helper()
	var m goldenMeta
	readJSON(t, filepath.Join(dir, "meta.json"), &m)
	return m
}

func readJSON(t *testing.T, path string, v any) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
}
