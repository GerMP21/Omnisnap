package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/GerMP21/Omnisnap/internal/schema"
	"github.com/GerMP21/Omnisnap/internal/viewer"
)

// runReport is `omnisnap report <bundle>` (Task 61): re-render report.html from
// an existing bundle. Deterministic: the same summary gives the same bytes.
func runReport(ctx context.Context, r *reporter, args []string) int {
	var pos []string
	for _, a := range args {
		if a == "--json" || a == "--quiet" {
			continue // accepted for symmetry, no effect on a local re-render
		}
		pos = append(pos, a)
	}
	if len(pos) != 1 {
		fmt.Fprintln(r.err, "omnisnap: report takes one bundle directory")
		return ExitUsage
	}
	dir := pos[0]
	b, err := os.ReadFile(filepath.Join(dir, "summary.json"))
	if err != nil {
		r.fatal(fmt.Errorf("report: %w (is %s an omnisnap bundle?)", err, dir))
		return ExitUsage
	}
	var s schema.Summary
	if err := json.Unmarshal(b, &s); err != nil {
		r.fatal(fmt.Errorf("report: summary.json: %w", err))
		return ExitInternal
	}
	if err := viewer.Write(dir, s); err != nil {
		r.fatal(err)
		return ExitInternal
	}
	fmt.Fprintln(r.out, "report:", filepath.Join(dir, "report.html"))
	return ExitOK
}
