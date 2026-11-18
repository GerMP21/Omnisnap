package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image"
	_ "image/png" // screenshots
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/GerMP21/Omnisnap/internal/pipeline"
	"github.com/GerMP21/Omnisnap/internal/schema"
	"github.com/GerMP21/Omnisnap/internal/triage/evidence"
)

// diff (Task 74's prerequisite): run the M1 comparison over bundle directories
// the crawler wrote and write findings.json next to the captures. Without it
// the lab has geometry and screenshots but nothing the dataset can label.
func diffBundles(args []string) error {
	fs := flag.NewFlagSet("diff", flag.ContinueOnError)
	bundles := fs.String("bundles", "", "comma-separated bundle directories (or page directories)")
	baseline := fs.String("baseline", "", "baseline engine id (default: the first Blink capture found)")
	only := fs.String("only", "", "only page slugs containing this")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *bundles == "" {
		return fmt.Errorf("diff needs --bundles")
	}
	pages, engines := 0, 0
	for _, root := range strings.Split(*bundles, ",") {
		dirs, err := pageDirs(strings.TrimSpace(root))
		if err != nil {
			return err
		}
		for _, dir := range dirs {
			base, err := readPageMeta(dir)
			if err != nil {
				return err
			}
			if base.Group == "mut" {
				continue // mutants get their findings from mutate
			}
			if *only != "" && !strings.Contains(base.Slug, *only) {
				continue
			}
			pairs, err := diffPage(dir, base, *baseline)
			if err != nil {
				fmt.Printf("  %s: %v\n", base.Slug, err)
				continue
			}
			if len(pairs) == 0 {
				continue
			}
			// WriteFindings joins out + slug: hand it the parent so the file
			// lands next to the captures.
			root, slug := filepath.Dir(strings.TrimSuffix(dir, "/")), filepath.Base(strings.TrimSuffix(dir, "/"))
			if _, err := pipeline.WriteFindings(root, slug, base.URL, pairs); err != nil {
				return err
			}
			pages++
			engines += len(pairs)
		}
	}
	fmt.Printf("diff: %d pages, %d engine pairs\n", pages, engines)
	return nil
}

// diffPage compares one page's engines against its baseline.
func diffPage(dir string, base pageMeta, baselineID string) ([]pipeline.FindingsPair, error) {
	caps, err := pageCaptures(dir)
	if err != nil {
		return nil, err
	}
	baselineID = pickBaseline(caps, baselineID)
	if baselineID == "" {
		return nil, fmt.Errorf("no baseline capture")
	}
	a, a2 := caps[baselineID][0], caps[baselineID][1]
	if a == nil {
		return nil, fmt.Errorf("baseline %s has no run a", baselineID)
	}
	var pairs []pipeline.FindingsPair
	for _, engine := range sortedEngines(caps) {
		if engine == baselineID || caps[engine][0] == nil {
			continue
		}
		pair, err := compare(dir, dir, base.Slug, baselineID, engine, a, a2, caps[engine][0], caps[engine][1], caps)
		if err != nil {
			return pairs, err
		}
		pairs = append(pairs, pair)
	}
	return pairs, nil
}

// compare runs one engine pair. The two sides may live in different
// directories (a mutant's capture next to the clean baseline's).
func compare(dirA, dirB, slug, baselineID, engine string, a, a2, b, b2 *schema.Capture, all map[string][2]*schema.Capture) (pipeline.FindingsPair, error) {
	var thirds []pipeline.ThirdCapture
	for id, c := range all {
		if id == baselineID || id == engine || c[0] == nil {
			continue
		}
		thirds = append(thirds, pipeline.ThirdCapture{Engine: id, A: c[0], B: c[1]})
	}
	sort.Slice(thirds, func(i, j int) bool { return thirds[i].Engine < thirds[j].Engine })
	pair := pipeline.Run(pipeline.Inputs{
		SlugBase: slug + "@" + engine, BaselineEngine: baselineID, Engine: engine,
		A: a, A2: a2, B: b, B2: b2, Thirds: thirds,
		ProbesA: readProbesFile(dirA, baselineID, "a"),
		ProbesB: readProbesFile(dirB, engine, "a"),
		ImagesA: readImageFile(dirA, baselineID, "a"),
		ImagesB: readImageFile(dirB, engine, "a"),
	})
	symptoms := make([]pipeline.Symptom, len(pair.Symptoms))
	for i, s := range pair.Symptoms {
		symptoms[i] = pipeline.Symptom{Selector: s.Selector, Kind: s.Kind}
	}
	warnings := pair.Warnings
	if b2 == nil {
		warnings = append(warnings, "one run: dynamic content not masked")
	}
	return pipeline.FindingsPair{
		Engine: engine, Baseline: baselineID,
		Causes:   pipeline.CausesWithRefs(pair.Causes, pair.CauseRefs),
		Symptoms: symptoms, Warnings: warnings, Dynamic: pair.Dynamic,
		Blocked: pair.Blocked, Stats: pair.Stats, Features: pair.Features,
	}, nil
}

// pageCaptures reads every <engine>-{a,b}.json in a page directory.
func pageCaptures(dir string) (map[string][2]*schema.Capture, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := map[string][2]*schema.Capture{}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, "-a.json") && !strings.HasSuffix(name, "-b.json") {
			continue
		}
		run := "a"
		if strings.HasSuffix(name, "-b.json") {
			run = "b"
		}
		engine := strings.TrimSuffix(strings.TrimSuffix(name, "-a.json"), "-b.json")
		c, err := readCaptureFile(filepath.Join(dir, name))
		if err != nil || c == nil {
			continue
		}
		slot := out[engine]
		if run == "a" {
			slot[0] = c
		} else {
			slot[1] = c
		}
		out[engine] = slot
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no captures in %s", dir)
	}
	return out, nil
}

// pickBaseline prefers a Blink engine (the plan's baseline rule) and otherwise
// takes the first id in sorted order, so a bundle diffed without flags is
// deterministic.
func pickBaseline(caps map[string][2]*schema.Capture, want string) string {
	if want != "" {
		if c, ok := caps[want]; ok && c[0] != nil {
			return want
		}
		return ""
	}
	preference := []string{"chrome", "chromium", "brave", "edge"}
	for _, id := range preference {
		if c, ok := caps[id]; ok && c[0] != nil {
			return id
		}
	}
	ids := sortedEngines(caps)
	if len(ids) == 0 {
		return ""
	}
	return ids[0]
}

func sortedEngines(caps map[string][2]*schema.Capture) []string {
	ids := make([]string, 0, len(caps))
	for id := range caps {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func readCaptureFile(path string) (*schema.Capture, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	c := &schema.Capture{}
	if err := json.Unmarshal(b, c); err != nil {
		return nil, err
	}
	return c, nil
}

func readImageFile(dir, engine, run string) image.Image {
	f, err := os.Open(filepath.Join(dir, engine+"-"+run+".png"))
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil
	}
	return img
}

func readProbesFile(dir, engine, run string) map[string]*schema.Capture {
	f, err := os.Open(filepath.Join(dir, engine+"-"+run+".probes.json.gz"))
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	m, err := evidence.ReadProbes(f)
	if err != nil {
		return nil
	}
	return m
}
