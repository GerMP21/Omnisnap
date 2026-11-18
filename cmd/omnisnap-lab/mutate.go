package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/GerMP21/Omnisnap/internal/capture"
	"github.com/GerMP21/Omnisnap/internal/discovery"
	"github.com/GerMP21/Omnisnap/internal/drivers"
	"github.com/GerMP21/Omnisnap/internal/groundtruth"
	"github.com/GerMP21/Omnisnap/internal/layoutdiff"
	"github.com/GerMP21/Omnisnap/internal/schema"
)

// mutate (Task 70/71) turns clean bundles into mutant bundles: for every base
// page it injects the scheduled bugs in every engine, then labels each mutant
// against the clean run of the same engine. The labels land in the mutant's
// directory, where the dataset export picks them up.
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

	pages := 0
	mutants, effective, labeled := 0, 0, 0
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
				return err
			}
			if base.Group == "mut" {
				continue // already a mutant
			}
			if *only != "" && !strings.Contains(base.Slug, *only) {
				continue
			}
			for _, e := range running {
				clean, clean2, ok, err := cleanPair(dir, e.id)
				if err != nil {
					return err
				}
				if !ok {
					fmt.Printf("  %s/%s: no clean pair, skipped\n", base.Slug, e.id)
					continue
				}
				if !groundtruth.Eligible(clean, clean2) {
					fmt.Printf("  %s/%s: the two runs disagree (%d vs %d nodes), skipped\n",
						base.Slug, e.id, clean.Meta.Nodes, clean2.Meta.Nodes)
					continue
				}
				mask := layoutdiff.DynamicMask(layoutdiff.Prep(clean, ""), layoutdiff.Prep(clean2, ""))
				sess, err := e.new(ctx)
				if err != nil {
					return fmt.Errorf("%s: %w", e.id, err)
				}
				results, err := groundtruth.CaptureMutants(ctx, sess, groundtruth.MutateOptions{
					Out: dir, BaseSlug: base.Slug, URL: base.URL, Engine: e.id,
					PerEngine: *perEngine, RequireEffective: *requireEffective,
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
					return fmt.Errorf("%s/%s: %w", base.Slug, e.id, err)
				}
				pages++
				for _, r := range results {
					mutants++
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
						effective++
					}
					if len(label.Positives) > 0 {
						labeled++
					}
					if err := groundtruth.WriteLabels(dir, r.Mutant, r.Capture.Meta.Mutation, label); err != nil {
						return err
					}
				}
			}
		}
	}
	if mutants == 0 {
		return fmt.Errorf("no mutant was captured (do the bundles have <engine>-a.json and -b.json?)")
	}
	fmt.Printf("mutate: %d page/engine pairs, %d mutants, %d effective, %d labeled\n",
		pages, mutants, effective, labeled)
	return nil
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
		return nil, fmt.Errorf("mutate: %s has no page.json and no page subdirectories", root)
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
