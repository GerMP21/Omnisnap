package schema

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// consumerFields are the summary.json fields each consumer reads, from its
// spec. A path segment ending in [] is an array: the rest of the path must be
// present in at least one element. Presence means the key exists; null counts.
var consumerFields = map[string][]string{
	"viewer (SPEC-viewer.md)": {
		"hosts[].id",
		"results[].host.id", "results[].page.slug", "results[].page.url", "results[].engine.engine",
		"results[].baseline.engine", "results[].status", "results[].warnings",
		"results[].stats.report", "results[].stats.info", "results[].stats.matchRate",
		"results[].shots.baseline.src", "results[].shots.baseline.w", "results[].shots.baseline.h",
		"results[].shots.engine.src", "results[].shots.engine.w", "results[].shots.engine.h",
		"results[].findings",
		"results[].causes[].id", "results[].causes[].tier", "results[].causes[].title", "results[].causes[].count",
		"results[].causes[].consequences", "results[].causes[].selector",
		"results[].causes[].boxes.baseline", "results[].causes[].boxes.engine", "results[].causes[].delta.dw",
		"results[].causes[].kb", "results[].causes[].score", "results[].causes[].instances",
		"results[].causes[].evidence.mechanism", "results[].causes[].evidence.consensus",
		"results[].causes[].evidence.perception", "results[].causes[].evidence.ink", "results[].causes[].evidence.intent",
		"results[].intent[].engine", "results[].intent[].selector", "results[].intent[].relations",
		"results[].intent[].px", "results[].intent[].keptBy",
		"results[].dynamic.baseline", "results[].dynamic.engine",
		"results[].blocked.baseline", "results[].blocked.engine",
		"results[].audit.baseline", "results[].audit.engine",
	},
	"merge (SPEC-merge.md)": {
		"$schema", "tool.version", "job.pages", "job.viewport", "hosts[].id",
		"results[].host.os", "results[].host.arch", "results[].host.id",
		"results[].page.slug", "results[].page.url", "results[].engine.engine",
		"results[].shots.baseline.src", "results[].shots.engine.src", "results[].findings",
		"pages[].slug", "pages[].report",
	},
	"ci-action (SPEC-ci-action.md)": {
		"pages[].slug", "pages[].url", "pages[].report",
		"results[].host.id", "results[].engine.engine", "results[].status", "results[].stats.report",
		"results[].causes[].tier", "results[].causes[].title", "results[].causes[].selector", "results[].causes[].score",
		"skipped[].host", "skipped[].engine", "skipped[].reason",
	},
	"wrappers (SPEC-wrappers.md, planted gates)": {
		"tool.version", "generated",
		"results[].page.slug", "results[].engine.engine", "results[].status", "results[].stats.report",
		"results[].causes[].tier", "results[].causes[].selector", "results[].causes[].kb",
		"results[].intent[].selector",
	},
}

const fullSummary = "../../testdata/schema/summary/valid/full.json"

func TestSummaryHasEveryConsumerField(t *testing.T) {
	in, err := os.ReadFile(fullSummary)
	if err != nil {
		t.Fatal(err)
	}
	var s Summary
	if err := json.Unmarshal(in, &s); err != nil {
		t.Fatal(err)
	}
	out, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	for name, doc := range map[string][]byte{"fixture": in, "Go round trip": out} {
		var v any
		if err := json.Unmarshal(doc, &v); err != nil {
			t.Fatal(err)
		}
		for consumer, paths := range consumerFields {
			for _, p := range paths {
				if !hasPath(v, strings.Split(p, ".")) {
					t.Errorf("%s: %s reads %s, which is missing", name, consumer, p)
				}
			}
		}
	}
}

func hasPath(v any, path []string) bool {
	if len(path) == 0 {
		return true
	}
	m, ok := v.(map[string]any)
	if !ok {
		return false
	}
	key, each := strings.CutSuffix(path[0], "[]")
	child, ok := m[key]
	if !ok {
		return false
	}
	if !each {
		return hasPath(child, path[1:])
	}
	items, _ := child.([]any)
	for _, it := range items {
		if hasPath(it, path[1:]) {
			return true
		}
	}
	return false
}

func TestHasPath(t *testing.T) {
	var v any
	_ = json.Unmarshal([]byte(`{"a":[{"b":null},{"c":{"d":1}}],"e":[]}`), &v)
	for p, want := range map[string]bool{"a[].b": true, "a[].c.d": true, "a[].x": false, "e[].y": false, "a": true, "z": false} {
		if got := hasPath(v, strings.Split(p, ".")); got != want {
			t.Errorf("hasPath(%s) = %v, want %v", p, got, want)
		}
	}
}

func TestSummaryFixturesRoundTrip(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join(fixtureDir, "summary", "valid", "*.json"))
	if len(files) == 0 {
		t.Fatal("no summary fixtures")
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			in, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			var s Summary
			if err := json.Unmarshal(in, &s); err != nil {
				t.Fatal(err)
			}
			out, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			if !sameJSON(t, in, out) {
				t.Errorf("round trip changed the summary:\n in: %s\nout: %s", in, out)
			}
			validate(t, "summary", s)
		})
	}
}

// A Go Summary built field by field (not decoded) must validate: nil slices
// become [], nullable fields become null.
func TestSummaryZeroValuesValidate(t *testing.T) {
	s := NewSummary("0.1.0", Job{Pages: []PageRef{{URL: "http://localhost:3000/"}}})
	s.Results = append(s.Results, Result{
		Host:     Machine{OS: "linux", Arch: "amd64"},
		Page:     ResultPage{Slug: "home", URL: "http://localhost:3000/"},
		Baseline: ResultBaseline{Engine: "chrome"},
		Engine:   ResultEngine{Engine: "firefox", Family: FamilyGecko, Profile: ProfileFresh},
		Status:   StatusOK,
		Causes:   []Cause{{ID: "x", Tier: TierInfo, Title: "t", Kinds: []string{"width"}, Selector: "div", Count: 1}},
	})
	validate(t, "summary", s)
}
