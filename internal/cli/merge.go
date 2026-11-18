package cli

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/GerMP21/Omnisnap/internal/merge"
	"github.com/GerMP21/Omnisnap/internal/schema"
	"github.com/GerMP21/Omnisnap/internal/viewer"
)

// runMerge is `omnisnap merge <bundle>... -o <merged>` (Task 77).
func runMerge(ctx context.Context, r *reporter, args []string) int {
	out, rename := "", false
	var dirs []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-o", "--out":
			if i+1 >= len(args) {
				fmt.Fprintln(r.err, "omnisnap: -o needs a directory")
				return ExitUsage
			}
			i++
			out = args[i]
		case "--rename-hosts":
			rename = true
		default:
			dirs = append(dirs, args[i])
		}
	}
	if len(dirs) == 0 || out == "" {
		fmt.Fprintln(r.err, "omnisnap: merge needs bundle directories and -o <merged-dir>")
		return ExitUsage
	}
	var bundles []merge.Bundle
	for _, d := range dirs {
		b, err := merge.Read(d)
		if err != nil {
			r.fatal(err)
			return ExitUsage
		}
		bundles = append(bundles, b)
	}
	m, err := merge.Merge(bundles, merge.Options{Out: out, RenameHosts: rename})
	if err != nil {
		r.fatal(err)
		return ExitUsage
	}
	err = merge.Apply(m, out, func(dir string, s schema.Summary) error {
		if err := writeJSON(filepath.Join(dir, "summary.json"), s); err != nil {
			return err
		}
		return viewer.Write(dir, s)
	})
	if err != nil {
		r.fatal(err)
		return ExitInternal
	}
	if !r.quiet {
		fmt.Fprintf(r.out, "merged %d bundle(s), %d result(s), %d host(s): %s\n",
			len(bundles), len(m.Summary.Results), len(m.Summary.Hosts), filepath.Join(out, "report.html"))
	}
	return ExitOK
}
