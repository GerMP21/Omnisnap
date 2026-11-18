package pipeline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/GerMP21/Omnisnap/internal/schema"
)

// Task 64: the per-pair time budget. Diff + triage on the largest golden pair
// must stay under 2.5 s (and triage under 300 ms per pair); a regression fails
// CI (the benchmark is run with -bench . in the M1 pipeline job).
func benchPair(tb testing.TB, slug string) Inputs {
	tb.Helper()
	load := func(engine, run string) *schema.Capture {
		b, err := os.ReadFile(filepath.Join("../../testdata/golden/planted", slug, engine+"-"+run+".json"))
		if err != nil {
			tb.Fatal(err)
		}
		var c schema.Capture
		if err := json.Unmarshal(b, &c); err != nil {
			tb.Fatal(err)
		}
		return &c
	}
	return Inputs{
		SlugBase: slug + "@firefox", BaselineEngine: "chrome", Engine: "firefox",
		A: load("chrome", "a"), A2: load("chrome", "b"),
		B: load("firefox", "a"), B2: load("firefox", "b"),
		Thirds: []ThirdCapture{{Engine: "webkit", A: load("webkit", "a"), B: load("webkit", "b")}},
	}
}

// The largest golden page by node count (15-kb-line-height-normal: 217 nodes).
func BenchmarkLargestPair(b *testing.B) {
	in := benchPair(b, "15-kb-line-height-normal")
	b.ResetTimer()
	for range b.N {
		_ = Run(in)
	}
}

func TestLargestPairBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("timing")
	}
	in := benchPair(t, "15-kb-line-height-normal")
	start := nowMS()
	pair := Run(in)
	d := nowMS() - start
	if d > 2500 {
		t.Errorf("diff + triage took %d ms; want <= 2500 ms (Task 64)", d)
	}
	t.Logf("largest golden pair: %d causes in %d ms (match rate %.3f)", len(pair.Causes), d, pair.Stats.MatchRate)
}

// 10,000-element trees: Task 48's budget is <= 2 s for the pair.
func TestTenThousandElements(t *testing.T) {
	if testing.Short() {
		t.Skip("performance")
	}
	build := func(shift float64) *schema.Capture {
		root := schema.Node{T: "html", B: [4]float64{0, 0, 1280, 800}, D: "block"}
		for i := range 100 {
			row := schema.Node{T: "div", C: "row", B: [4]float64{0, float64(i * 40), 1280, 40}, D: "block"}
			for j := range 100 {
				cell := schema.Node{T: "div", C: "cell", B: [4]float64{float64(j * 12), float64(i * 40), 10, 20}, D: "block"}
				if i%17 == 0 && j%13 == 0 {
					cell.B[2] += shift // a difference every so often
				}
				row.K = append(row.K, cell)
			}
			root.K = append(root.K, row)
		}
		c := &schema.Capture{Meta: schema.Meta{VW: 1280, VH: 800, CW: 1280, DocW: 1280, DocH: 4000, Nodes: 10001}, Root: root}
		return c
	}
	in := Inputs{SlugBase: "big@firefox", BaselineEngine: "chrome", Engine: "firefox", A: build(0), B: build(3)}
	start := nowMS()
	pair := Run(in)
	d := nowMS() - start
	if d > 2000 {
		t.Errorf("10k elements: %d ms; want <= 2000 ms (Task 48)", d)
	}
	t.Logf("10k elements: %d causes in %d ms (match rate %.3f)", len(pair.Causes), d, pair.Stats.MatchRate)
}

// Task 66: a cause on an element that loads a resource from a blocked host
// carries blockedBy naming it.
func TestBlockedAttribution(t *testing.T) {
	img := func(w float64) schema.Node {
		v := schema.Node{T: "img", ID: "ad", U: "https://ads.example.net/ad.png",
			B: [4]float64{10, 10, w, 50}, D: "block"}
		return v
	}
	base := &schema.Capture{Meta: schema.Meta{VW: 1280, VH: 800, CW: 1280, DocW: 1280, DocH: 400}, Root: schema.Node{T: "html", B: [4]float64{0, 0, 1280, 400}, D: "block", K: []schema.Node{img(300)}}}
	eng := &schema.Capture{
		Meta: schema.Meta{VW: 1280, VH: 800, CW: 1280, DocW: 1280, DocH: 400,
			Failed: []schema.FailedRequest{{URL: "https://ads.example.net/ad.png", Err: "net::ERR_BLOCKED_BY_CLIENT"}}},
		Root: schema.Node{T: "html", B: [4]float64{0, 0, 1280, 400}, D: "block", K: []schema.Node{img(120)}}}
	pair := Run(Inputs{SlugBase: "x@firefox", BaselineEngine: "chrome", Engine: "firefox", A: base, B: eng})
	found := false
	for _, c := range pair.Causes {
		if c.Selector == "img#ad" {
			found = true
			if len(c.BlockedBy) != 1 || c.BlockedBy[0] != "ads.example.net" {
				t.Errorf("blockedBy = %v; want ads.example.net", c.BlockedBy)
			}
		}
	}
	if !found {
		t.Fatalf("no cause on img#ad: %+v", pair.Causes)
	}
}
