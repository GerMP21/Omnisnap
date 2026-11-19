package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/GerMP21/Omnisnap/internal/capture"
	"github.com/GerMP21/Omnisnap/internal/discovery"
	"github.com/GerMP21/Omnisnap/internal/drivers"
	"github.com/GerMP21/Omnisnap/internal/groundtruth"
	"github.com/GerMP21/Omnisnap/internal/layoutdiff"
	"github.com/GerMP21/Omnisnap/internal/pipeline"
	"github.com/GerMP21/Omnisnap/internal/schema"
)

// mutate (Task 70/71) turns clean bundles into mutant bundles: for every base
// page it injects the scheduled bugs in every engine, then labels each mutant
// against the clean run of the same engine. The labels land in the mutant's
// directory, where the dataset export picks them up.
//
// A crawl is long and the sites are other people's: a page that times out or
// refuses the mutation is reported and skipped rather than ending the run, and
// a page/engine whose mutants are already on disk is skipped, so the same
// command resumes an interrupted batch.
func mutate(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("mutate", flag.ContinueOnError)
	bundles := fs.String("bundles", "", "comma-separated bundle directories with clean captures")
	engines := fs.String("engines", "", "comma-separated engine ids or families (default: every installed)")
	perEngine := fs.Int("per-engine", groundtruth.DefaultMutantsPerEngine, "mutants per page and engine")
	requireEffective := fs.Bool("require-effective", false, "retry the operator rotation until the mutant moves something (training only: it biases the set)")
	only := fs.String("only", "", "only base slugs containing this")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *bundles == "" {
		return fmt.Errorf("mutate needs --bundles")
	}
	running, err := mutantEngines(ctx, *engines)
	if err != nil {
		return err
	}
	defer closeEngines(running)
	if len(running) == 0 {
		return fmt.Errorf("no browser could be launched")
	}

	var total mutStats
	failures, skipped := 0, 0
	for _, root := range strings.Split(*bundles, ",") {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		dirs, err := pageDirs(root)
		if err != nil {
			return err
		}
		for _, dir := range dirs {
			base, err := readPageMeta(dir)
			if err != nil {
				fmt.Printf("  %s: %v\n", filepath.Base(dir), err)
				failures++
				continue
			}
			if base.Group == "mut" {
				continue // already a mutant
			}
			if *only != "" && !strings.Contains(base.Slug, *only) {
				continue
			}
			for _, e := range running {
				if have := existingMutants(dir, base.Slug, e.id); have >= *perEngine {
					fmt.Printf("  %s/%s: %d mutant(s) already there, skipped\n", base.Slug, e.id, have)
					skipped++
					continue
				}
				st, err := mutateOne(ctx, dir, base, e, *perEngine, *requireEffective)
				total.pages += st.pages
				total.mutants += st.mutants
				total.effective += st.effective
				total.labeled += st.labeled
				if err != nil {
					fmt.Printf("  %s/%s: %v\n", base.Slug, e.id, err)
					failures++
				}
			}
		}
	}
	if total.mutants == 0 {
		if skipped > 0 && failures == 0 {
			// Fully resumed: nothing to do, and that is not a failure.
			fmt.Printf("mutate: %d page/engine pair(s) already mutated, nothing to do\n", skipped)
			return nil
		}
		return fmt.Errorf("no mutant was captured (%d page/engine pair(s) failed; do the bundles have <engine>-a.json and -b.json?)", failures)
	}
	fmt.Printf("mutate: %d page/engine pairs, %d mutants, %d effective, %d labeled\n",
		total.pages, total.mutants, total.effective, total.labeled)
	if skipped > 0 {
		fmt.Printf("mutate: %d page/engine pair(s) skipped, already mutated\n", skipped)
	}
	if failures > 0 {
		fmt.Printf("mutate: %d page/engine pair(s) failed and are retried by rerunning the command\n", failures)
	}
	return nil
}

// mutStats is one page/engine pair's contribution.
type mutStats struct {
	pages, mutants, effective, labeled int
}

// mutateOne injects and labels one page's mutants in one engine.
func mutateOne(ctx context.Context, dir string, base pageMeta, e labEngine, perEngine int, requireEffective bool) (mutStats, error) {
	var st mutStats
	clean, clean2, ok, err := cleanPair(dir, e.id)
	if err != nil {
		return st, err
	}
	if !ok {
		return st, fmt.Errorf("no clean pair")
	}
	if !groundtruth.Eligible(clean, clean2) {
		return st, fmt.Errorf("the two runs disagree (%d vs %d nodes)", clean.Meta.Nodes, clean2.Meta.Nodes)
	}
	mask := layoutdiff.DynamicMask(layoutdiff.Prep(clean, ""), layoutdiff.Prep(clean2, ""))
	sess, err := e.new(ctx)
	if err != nil {
		return st, err
	}
	results, err := groundtruth.CaptureMutants(ctx, sess, groundtruth.MutateOptions{
		Out: dir, BaseSlug: base.Slug, URL: base.URL, Engine: e.id,
		PerEngine: perEngine, RequireEffective: requireEffective,
		// Same evidence as a clean capture: screenshot and probes are
		// taken while the bug is applied, so the mutant's causes
		// carry the same features as the clean run.
		Evidence: func(ctx context.Context, s drivers.Session, c *schema.Capture, basePath string) error {
			return capture.Evidence(ctx, s, capture.Options{
				Out: dir, Slug: base.Slug, Engine: e.id, Version: e.version, Run: "a",
			}, c, basePath)
		},
		Logf: func(f string, a ...any) { fmt.Printf("  "+f+"\n", a...) },
	})
	_ = sess.Close(context.WithoutCancel(ctx))
	if err != nil {
		return st, err
	}
	st.pages++
	for _, r := range results {
		st.mutants++
		if r.Err != nil {
			fmt.Printf("  %s: %v\n", r.Mutant.Slug, r.Err)
			continue
		}
		label, err := groundtruth.LabelMutant(clean, r.Capture, mask)
		if err != nil {
			fmt.Printf("  %s: %v\n", r.Mutant.Slug, err)
			continue
		}
		if label.Effective {
			st.effective++
		}
		if len(label.Positives) > 0 {
			st.labeled++
		}
		causes, note := mutantFindings(dir, base, e.id, r, clean, clean2, label)
		if note != "" {
			fmt.Printf("  %s: %s\n", r.Mutant.Slug, note)
		}
		if err := groundtruth.WriteLabels(dir, r.Mutant, r.Capture.Meta.Mutation, label, causes); err != nil {
			return st, err
		}
	}
	return st, nil
}

// existingMutants counts the mutant directories already written for one
// page/engine, so an interrupted batch resumes where it stopped.
func existingMutants(dir, slug, engine string) int {
	prefix := slug + "~" + engine + "~"
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), prefix) {
			n++
		}
	}
	return n
}

// pageDirs accepts either one page directory or a bundle root full of pages.
func pageDirs(root string) ([]string, error) {
	if _, err := os.Stat(filepath.Join(root, "page.json")); err == nil {
		return []string{root}, nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("mutate: %w", err)
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		if _, err := os.Stat(filepath.Join(dir, "page.json")); err == nil {
			out = append(out, dir)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s has no page.json and no page subdirectories", root)
	}
	return out, nil
}

// pageMeta is the page.json a capture writes.
type pageMeta struct {
	Slug  string `json:"slug"`
	URL   string `json:"url"`
	Group string `json:"group"`
}

func readPageMeta(dir string) (pageMeta, error) {
	b, err := os.ReadFile(filepath.Join(dir, "page.json"))
	if err != nil {
		return pageMeta{}, fmt.Errorf("mutate: %w", err)
	}
	var m pageMeta
	if err := json.Unmarshal(b, &m); err != nil {
		return pageMeta{}, fmt.Errorf("mutate: %s/page.json: %w", dir, err)
	}
	if m.URL == "" {
		return pageMeta{}, fmt.Errorf("mutate: %s/page.json has no url", dir)
	}
	return m, nil
}

// cleanPair reads the two clean runs of one engine.
func cleanPair(dir, engine string) (a, b *schema.Capture, ok bool, err error) {
	read := func(run string) (*schema.Capture, error) {
		p := filepath.Join(dir, engine+"-"+run+".json")
		raw, err := os.ReadFile(p)
		if os.IsNotExist(err) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		c := &schema.Capture{}
		if err := json.Unmarshal(raw, c); err != nil {
			return nil, fmt.Errorf("mutate: %s: %w", p, err)
		}
		return c, nil
	}
	a, err = read("a")
	if err != nil || a == nil {
		return nil, nil, false, err
	}
	b, err = read("b")
	if err != nil || b == nil {
		// One run is enough for the label; without a second run there is no
		// dynamic mask, so the page is skipped rather than mislabeled.
		return nil, nil, false, err
	}
	return a, b, true, nil
}

// mutantEngines launches one browser per wanted family, in discovery order.
func mutantEngines(ctx context.Context, wanted string) ([]labEngine, error) {
	found, err := discovery.Find(ctx, discovery.Options{})
	if err != nil {
		return nil, err
	}
	want := map[string]bool{}
	for _, e := range strings.Split(wanted, ",") {
		if e = strings.TrimSpace(e); e != "" {
			want[e] = true
		}
	}
	var out []labEngine
	seen := map[schema.Family]bool{}
	for _, b := range found {
		if seen[b.Family] {
			continue
		}
		if len(want) > 0 && !want[b.ID] && !want[string(b.Family)] {
			continue
		}
		e, err := launchLabEngine(ctx, b)
		if err != nil {
			fmt.Printf("  %s: %v (skipped)\n", b.ID, err)
			continue
		}
		seen[b.Family] = true
		out = append(out, e)
	}
	return out, nil
}

func closeEngines(es []labEngine) {
	for i := len(es) - 1; i >= 0; i-- {
		es[i].close()
	}
}

// mutantFindings diffs a mutant against a baseline engine's clean run, writes
// the mutant's findings.json and returns the positive cause ids. A note
// explains why it was skipped (a mutant without a baseline still gets labels).
func mutantFindings(pageDir string, base pageMeta, engine string, r groundtruth.MutantResult, clean, clean2 *schema.Capture, label groundtruth.MutantLabel) ([]string, string) {
	caps, err := pageCaptures(pageDir)
	if err != nil {
		return nil, "findings skipped: no captures"
	}
	baselineID := pickBaselineExcept(caps, engine)
	if baselineID == "" {
		return nil, "findings skipped: no baseline engine"
	}
	bA, bA2 := caps[baselineID][0], caps[baselineID][1]
	if bA == nil {
		return nil, "findings skipped: baseline has no run a"
	}
	mutantDir := filepath.Join(pageDir, r.Mutant.Slug)
	pair, err := compare(pageDir, mutantDir, base.Slug, baselineID, engine, bA, bA2, r.Capture, nil, caps)
	if err != nil {
		return nil, "findings: " + err.Error()
	}
	if _, err := pipeline.WriteFindings(pageDir, r.Mutant.Slug, base.URL, []pipeline.FindingsPair{pair}); err != nil {
		return nil, "findings: " + err.Error()
	}
	// A cause the clean cross-engine diff already had existed before the
	// injection: never positive.
	cleanKeys := map[string]bool{}
	if cp, err := compare(pageDir, pageDir, base.Slug, baselineID, engine, bA, bA2, clean, clean2, caps); err == nil {
		for _, c := range cp.Causes {
			cleanKeys[groundtruth.CauseKey(c.Cause)] = true
		}
	}
	refs := make([]groundtruth.CauseRef, 0, len(pair.Causes))
	for _, c := range pair.Causes {
		ref := groundtruth.CauseRef{Cause: c.Cause, Side: c.Side}
		if c.Node != nil {
			ref.Node, ref.HasNode = *c.Node, true
		}
		refs = append(refs, ref)
	}
	pos := groundtruth.PositiveCauses(groundtruth.CauseJoin{
		Causes: refs, Label: label, Mutant: r.Capture, Clean: clean,
		Baseline: bA, CleanKeys: cleanKeys,
	})
	ids := make([]string, 0, len(pos))
	for id := range pos {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids, ""
}

// pickBaselineExcept prefers a Blink engine other than the mutated one.
func pickBaselineExcept(caps map[string][2]*schema.Capture, engine string) string {
	for _, id := range []string{"chrome", "chromium", "brave", "edge"} {
		if id == engine {
			continue
		}
		if c, ok := caps[id]; ok && c[0] != nil {
			return id
		}
	}
	for _, id := range sortedEngines(caps) {
		if id != engine && caps[id][0] != nil {
			return id
		}
	}
	return ""
}
