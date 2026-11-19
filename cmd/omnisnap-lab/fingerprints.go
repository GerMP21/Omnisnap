package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// fingerprints builds the corpus frequency table from a crawl (Task 73, Task
// 76): how many pages show each cause class. The table ships as
// assets/model/fingerprints.json and is what turns "this difference also
// happens on 40% of other sites" into a number.
//
// The crawl bundles must have been diffed first (omnisnap-lab diff): the
// fingerprints live in the findings files, one per cause.
func fingerprints(args []string) error {
	fs := flag.NewFlagSet("fingerprints", flag.ContinueOnError)
	bundles := fs.String("bundles", "", "comma-separated bundle directories")
	out := fs.String("out", "assets/model/fingerprints.json", "where the table goes")
	include := fs.String("include", "", "file with the page slugs the table is built from (one per line); empty means every page")
	exclude := fs.String("exclude", "", "file with the page slugs to leave out of the table")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *bundles == "" {
		return fmt.Errorf("fingerprints needs --bundles")
	}
	// The table can be built from a slice of the corpus: the pages left out
	// are then measured against a table that never saw them, which is the
	// only way to know what the model does with a class it has not seen.
	keep, drop := map[string]bool{}, map[string]bool{}
	if *include != "" {
		for _, slug := range readSlugs(*include) {
			keep[slug] = true
		}
	}
	if *exclude != "" {
		for _, slug := range readSlugs(*exclude) {
			drop[slug] = true
		}
	}
	// n: how many causes carry the fingerprint; pages: how many pages show it.
	n := map[string]int{}
	pages := map[string]map[string]bool{}
	total := 0
	read := 0
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
			b, err := os.ReadFile(filepath.Join(dir, "findings.json"))
			if err != nil {
				continue // not a diffed bundle
			}
			var doc findingsDoc
			if err := json.Unmarshal(b, &doc); err != nil {
				return fmt.Errorf("fingerprints: %s: %w", dir, err)
			}
			if doc.Slug == "" {
				continue
			}
			if len(keep) > 0 && !keep[doc.Slug] {
				continue
			}
			if drop[doc.Slug] {
				continue
			}
			read++
			total++
			for _, pair := range doc.Pairs {
				for _, c := range pair.Causes {
					if c.Fingerprint == "" {
						continue
					}
					n[c.Fingerprint]++
					if pages[c.Fingerprint] == nil {
						pages[c.Fingerprint] = map[string]bool{}
					}
					pages[c.Fingerprint][doc.Slug] = true
				}
			}
		}
	}
	table := map[string]any{
		"total":        total,
		"fingerprints": map[string]any{},
	}
	fps := table["fingerprints"].(map[string]any)
	for fp, count := range n {
		fps[fp] = map[string]any{"n": count, "pages": len(pages[fp])}
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(table, "", " ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(*out, append(b, '\n'), 0o644); err != nil {
		return err
	}
	// The most common classes first: they are the engine's baseline behaviour,
	// and the ones a report should not call a bug.
	type row struct {
		fp    string
		n     int
		pages int
	}
	var rows []row
	for fp := range fps {
		rows = append(rows, row{fp, n[fp], len(pages[fp])})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].pages != rows[j].pages {
			return rows[i].pages > rows[j].pages
		}
		return rows[i].fp < rows[j].fp
	})
	fmt.Printf("%d page(s), %d fingerprint(s); wrote %s\n", total, len(rows), *out)
	for i, r := range rows {
		if i >= 10 {
			break
		}
		fmt.Printf("  %-60s %d occurrence(s) on %d page(s)\n", r.fp, r.n, r.pages)
	}
	return nil
}

// findingsDoc is the part of the findings file the table needs.
type findingsDoc struct {
	Slug  string `json:"slug"`
	Pairs []struct {
		Causes []struct {
			Fingerprint string `json:"fingerprint"`
		} `json:"causes"`
	} `json:"pairs"`
}

// readSlugs reads a file with one page slug per line.
func readSlugs(path string) []string {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out []string
	for _, l := range strings.Split(string(b), "\n") {
		l = strings.TrimSpace(l)
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}
