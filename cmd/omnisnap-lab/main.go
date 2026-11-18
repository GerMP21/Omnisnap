// Command omnisnap-lab is the ground-truth tooling (Task 69): it is never part
// of a release. It shares the planted server and the capture pipeline with the
// product, and adds the crawl list, the crawler and mutation capture.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/GerMP21/Omnisnap/internal/groundtruth"
)

// Version is the lab build; the release tooling never ships this binary.
var Version = "dev"

const usage = `usage: omnisnap-lab <command>

Commands:
  version
  serve  --dir testdata/planted --addr 127.0.0.1:8099
  sites  --n 500 --out crawl-sites.json [--scan 6000]
  crawl  --list crawl-sites.json --out crawl-out [--limit N] [--shards N --shard I] [--engines chrome,firefox]
  mutate --bundles <dirs> [--engines chrome,firefox] [--per-engine 4] [--only slug] [--require-effective]
  dataset --bundles <dirs> --group planted|crawl|mut --label -1|0|1 --out dataset.csv

The lab is not shipped; the release tooling builds only ./cmd/omnisnap.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	switch os.Args[1] {
	case "version":
		fmt.Println("omnisnap-lab " + Version)
	case "serve":
		if err := serve(os.Args[2:]); err != nil {
			fail(err)
		}
	case "sites":
		if err := sites(ctx, os.Args[2:]); err != nil {
			fail(err)
		}
	case "crawl":
		if err := crawl(ctx, os.Args[2:]); err != nil {
			fail(err)
		}
	case "mutate":
		if err := mutate(ctx, os.Args[2:]); err != nil {
			fail(err)
		}
	case "dataset":
		if err := datasetExport(os.Args[2:]); err != nil {
			fail(err)
		}
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "omnisnap-lab:", err)
	os.Exit(1)
}

// serve serves the planted pages for manual crawls and captures.
func serve(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	dir := fs.String("dir", "testdata/planted", "planted pages directory")
	addr := fs.String("addr", "127.0.0.1:8099", "listen address")
	if err := fs.Parse(args); err != nil {
		return err
	}
	h, err := groundtruth.NewHandler(*dir)
	if err != nil {
		return err
	}
	srv := &http.Server{Addr: *addr, Handler: h, ReadHeaderTimeout: 10 * time.Second}
	fmt.Println("serving", *dir, "on http://"+*addr)
	return srv.ListenAndServe()
}

// sites builds the crawl list (Task 72): Tranco top-1M, the name filters, then
// homepage probes.
func sites(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("sites", flag.ContinueOnError)
	n := fs.Int("n", 500, "accepted sites to produce")
	scan := fs.Int("scan", 6000, "ranks to consider at most")
	out := fs.String("out", "crawl-sites.json", "output list")
	workers := fs.Int("workers", 16, "concurrent homepage probes")
	if err := fs.Parse(args); err != nil {
		return err
	}
	ranked, err := groundtruth.FetchTranco(ctx, *scan)
	if err != nil {
		return err
	}
	filter := groundtruth.NewSiteFilter(nil)
	var candidates []groundtruth.Site
	for _, r := range ranked {
		if d, ok := filter.Accept(r.Domain); ok {
			candidates = append(candidates, groundtruth.Site{URL: "https://" + d + "/", Rank: r.Rank, Slug: groundtruth.Slug(d)})
		}
	}
	list, err := groundtruth.ProbeSites(ctx, candidates, *n, *workers, filter.Rejected)
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(*out, append(b, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Printf("accepted %d of %d probed; wrote %s\n", len(list.Sites), list.Probed, *out)
	return nil
}

// crawl runs the crawl (Task 73) with the guards and resume.
func crawl(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("crawl", flag.ContinueOnError)
	list := fs.String("list", "crawl-sites.json", "crawl list")
	out := fs.String("out", "crawl-out", "bundle directory")
	limit := fs.Int("limit", 0, "sites this run (0: all)")
	shards := fs.Int("shards", 1, "number of shards")
	shard := fs.Int("shard", 0, "this shard (0-based)")
	engines := fs.String("engines", "", "engines to capture (default: every installed one)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	b, err := os.ReadFile(*list)
	if err != nil {
		return err
	}
	var doc groundtruth.SiteList
	if err := json.Unmarshal(b, &doc); err != nil {
		return err
	}
	plan := groundtruth.CrawlPlan(doc.Sites, groundtruth.CrawlOptions{
		Out: *out, Limit: *limit, Shards: *shards, Shard: *shard,
	})
	fmt.Printf("crawling %d site(s) of %d into %s\n", len(plan), len(doc.Sites), *out)
	captureSite, closer, err := newCapturer(*out, *engines)
	if err != nil {
		return err
	}
	defer closer()
	res, err := groundtruth.Crawl(ctx, plan, groundtruth.CrawlOptions{Out: *out, Limit: *limit, Shards: *shards, Shard: *shard},
		captureSite, func(format string, a ...any) { fmt.Printf(format+"\n", a...) })
	fmt.Printf("captured %d, skipped %d, failed %d\n", res.Captured, res.Skipped, res.Failed)
	return err
}

// dataset exports training rows (Task 74) from lab bundles.
func datasetExport(args []string) error {
	fs := flag.NewFlagSet("dataset", flag.ContinueOnError)
	bundles := fs.String("bundles", "", "comma-separated bundle directories")
	group := fs.String("group", "crawl", "row group (planted, crawl, mut)")
	label := fs.Int("label", 0, "row label (1 injected bug, 0 natural, -1 planted)")
	out := fs.String("out", "dataset.csv", "CSV output")
	metaPath := fs.String("meta", "dataset-meta.json", "metadata output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *bundles == "" {
		return fmt.Errorf("dataset needs --bundles")
	}
	var rows []groundtruth.DatasetRow
	cleanPairs := 0
	for _, dir := range strings.Split(*bundles, ",") {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		part, err := groundtruth.RowsFromBundle(dir, *group, *label)
		if err != nil {
			return err
		}
		rows = append(rows, part...)
		if *label == 0 {
			n, err := groundtruth.CleanPairs(dir)
			if err != nil {
				return err
			}
			cleanPairs += n
		}
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		return err
	}
	f, err := os.Create(*out)
	if err != nil {
		return err
	}
	if err := groundtruth.WriteDataset(f, rows); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	m, err := os.Create(*metaPath)
	if err != nil {
		return err
	}
	if err := groundtruth.WriteDatasetMeta(m, groundtruth.DatasetMeta{CleanPairs: cleanPairs}); err != nil {
		_ = m.Close()
		return err
	}
	if err := m.Close(); err != nil {
		return err
	}
	fmt.Printf("wrote %d rows to %s (%d clean pairs)\n", len(rows), *out, cleanPairs)
	return nil
}
