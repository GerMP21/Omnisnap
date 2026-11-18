//go:build browsers && unix

package groundtruth

import (
	"context"
	"testing"
	"time"

	"github.com/GerMP21/Omnisnap/internal/discovery"
	"github.com/GerMP21/Omnisnap/internal/drivers"
	"github.com/GerMP21/Omnisnap/internal/drivers/bidi"
	"github.com/GerMP21/Omnisnap/internal/drivers/cdp"
	"github.com/GerMP21/Omnisnap/internal/layoutdiff"
	"github.com/GerMP21/Omnisnap/internal/process"
	"github.com/GerMP21/Omnisnap/internal/schema"
)

// Task 70/71 smoke: 2 pages x 2 engines x 4 mutants on real browsers. Every
// mutant must be effective (the label finds a changed neighbourhood) and get a
// positive label — Task 70 wants all 16 effective, Task 71 at least 80%
// labeled.
func TestMutateOnTwoEngines(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	srv, err := Start("../../testdata/planted")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = srv.Close() }()

	pages := []string{"03-overflow", "31-control-dashboard"} // >= 50 nodes
	engines := mutateEngines(t, ctx)
	if len(engines) == 0 {
		t.Skip("no Blink or Gecko browser installed")
	}

	// Task 70/71 smoke, both policies (SPEC-ground-truth-mutant-policy.md):
	// the default records every scheduled mutant even when nothing moves; the
	// opt-in retries until the label sees a change.
	var totalDefault, totalRequired, effDefault, effRequired, positive int
	for _, e := range engines {
		sess, closeEngine := e.start(t, ctx)
		for _, page := range pages {
			t.Run(e.id+"/"+page, func(t *testing.T) {
				url := srv.URL(page)
				opts := BaselineOptions{URL: url, Engine: e.id}
				// Two clean runs of the same engine give the dynamic mask.
				clean, err := CaptureBaseline(ctx, sess, opts)
				if err != nil {
					t.Fatal(err)
				}
				clean2, err := CaptureBaseline(ctx, sess, opts)
				if err != nil {
					t.Fatal(err)
				}
				if !Eligible(clean, clean2) {
					t.Fatalf("the engine renders two different states: %d vs %d nodes", clean.Meta.Nodes, clean2.Meta.Nodes)
				}
				mask := layoutdiff.DynamicMask(layoutdiff.Prep(clean, ""), layoutdiff.Prep(clean2, ""))

				capturePass := func(t *testing.T, requireEffective bool) {
					results, err := CaptureMutants(ctx, sess, MutateOptions{
						Out: t.TempDir(), BaseSlug: page, URL: url, Engine: e.id,
						PerEngine: 4, RequireEffective: requireEffective, Logf: t.Logf,
					})
					if err != nil {
						t.Fatal(err)
					}
					if len(results) != 4 {
						t.Fatalf("%d mutants, want 4", len(results))
					}
					if requireEffective {
						totalRequired += len(results)
					} else {
						totalDefault += len(results)
					}
					for _, r := range results {
						if r.Err != nil {
							t.Errorf("%s: %v", r.Mutant.Slug, r.Err)
							continue
						}
						if r.Capture == nil || r.Capture.Meta.Mutation == nil {
							t.Errorf("%s: no mutation marker", r.Mutant.Slug)
							continue
						}
						label, err := LabelMutant(clean, r.Capture, mask)
						if err != nil {
							t.Errorf("%s: %v", r.Mutant.Slug, err)
							continue
						}
						if !label.Effective {
							if requireEffective {
								t.Errorf("%s (%s): the retry left a mutation that moved nothing (changed %d page-wide)",
									r.Mutant.Slug, r.Capture.Meta.Mutation.Op, label.Changed)
								continue
							}
							// The default policy keeps it; it is a hard negative
							// for the dataset, not an error.
							t.Logf("%s: %s moved nothing (recorded)", r.Mutant.Slug, r.Capture.Meta.Mutation.Op)
							continue
						}
						if requireEffective {
							effRequired++
							if r.Capture.Meta.Mutation.Attempts < 1 {
								t.Errorf("%s: no attempts recorded", r.Mutant.Slug)
							}
						} else {
							effDefault++
							if r.Capture.Meta.Mutation.Attempts != 0 {
								t.Errorf("%s: attempts = %d in the default policy", r.Mutant.Slug, r.Capture.Meta.Mutation.Attempts)
							}
						}
						if requireEffective && len(label.Positives) > 0 {
							positive++
						}
						t.Logf("%s (retry=%v): %s on %s, %d positive, %d changed, attempts %d",
							r.Mutant.Slug, requireEffective, r.Capture.Meta.Mutation.Op, label.Target,
							len(label.Positives), label.Changed, r.Capture.Meta.Mutation.Attempts)
					}
				}
				capturePass(t, false)
				capturePass(t, true)
			})
		}
		closeEngine()
	}
	if totalDefault < 16 || totalRequired < 16 {
		t.Errorf("captured %d default and %d retried mutants, want at least 16 each", totalDefault, totalRequired)
	}
	if effRequired != totalRequired {
		t.Errorf("%d/%d mutants effective with retries, want all", effRequired, totalRequired)
	}
	if ratio := float64(positive) / float64(totalRequired); ratio < 0.8 {
		t.Errorf("%.0f%% of effective mutants produced a positive (want >= 80%%)", ratio*100)
	}
	t.Logf("default: %d captured, %d effective; retries: %d/%d effective, %d positive",
		totalDefault, effDefault, effRequired, totalRequired, positive)

}

type mutateEngine struct {
	id      string
	family  schema.Family
	version string
	start   func(t *testing.T, ctx context.Context) (drivers.Session, func())
}

// mutateEngines launches the first Blink and the first Gecko browser found.
// WebKitGTK and Safari have their own lanes; the mutation smoke needs two
// engines, not every engine.
func mutateEngines(t *testing.T, ctx context.Context) []mutateEngine {
	t.Helper()
	found, err := discovery.Find(ctx, discovery.Options{})
	if err != nil {
		t.Fatal(err)
	}
	var out []mutateEngine
	for _, b := range found {
		switch {
		case b.Family == schema.FamilyBlink && !hasMutateFamily(out, schema.FamilyBlink):
			proc, err := process.StartBlink(ctx, b.Executable, "")
			if err != nil {
				t.Fatalf("start Blink: %v", err)
			}
			br, err := cdp.Dial(ctx, proc.WSURL)
			if err != nil {
				t.Fatalf("dial CDP: %v", err)
			}
			out = append(out, mutateEngine{
				id: b.ID, family: b.Family, version: b.Version,
				start: func(t *testing.T, ctx context.Context) (drivers.Session, func()) {
					s, err := br.NewSession(ctx)
					if err != nil {
						t.Fatal(err)
					}
					return s, func() { _ = br.Close(); _ = proc.Close(context.Background()) }
				},
			})
		case b.Family == schema.FamilyGecko && !hasMutateFamily(out, schema.FamilyGecko):
			proc, err := process.StartGecko(ctx, b.Executable, "")
			if err != nil {
				t.Fatalf("start Gecko: %v", err)
			}
			br, err := bidi.Dial(ctx, proc.WSURL)
			if err != nil {
				t.Fatalf("dial BiDi: %v", err)
			}
			out = append(out, mutateEngine{
				id: b.ID, family: b.Family, version: b.Version,
				start: func(t *testing.T, ctx context.Context) (drivers.Session, func()) {
					s, err := br.NewSession(ctx)
					if err != nil {
						t.Fatal(err)
					}
					return s, func() { _ = br.Close(context.Background()); _ = proc.Close(context.Background()) }
				},
			})
		}
	}
	return out
}

func hasMutateFamily(ms []mutateEngine, fam schema.Family) bool {
	for _, m := range ms {
		if m.family == fam {
			return true
		}
	}
	return false
}
