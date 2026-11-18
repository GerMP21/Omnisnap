package cli

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/GerMP21/Omnisnap/internal/schema"
)

// browserEntry is one line of `omnisnap browsers --json`: a host.json engine
// entry (it validates as one) plus what discovery knows.
type browserEntry struct {
	schema.HostEngine
	Name     string   `json:"name"`
	Protocol string   `json:"protocol"`
	Driver   string   `json:"driver,omitempty"`
	Source   string   `json:"source"`
	Notes    []string `json:"notes,omitempty"`
}

func runBrowsers(ctx context.Context, r *reporter, e env, args []string) int {
	asJSON := false
	for _, a := range args {
		if a != "--json" {
			fmt.Fprintf(r.err, "omnisnap: browsers: unexpected argument %q\n", a)
			return ExitUsage
		}
		asJSON = true
	}
	found, err := e.find(ctx)
	if err != nil {
		fmt.Fprintln(r.err, "omnisnap: discovery:", err)
		return ExitInternal
	}
	entries := []browserEntry{}
	for _, b := range found {
		entries = append(entries, browserEntry{
			HostEngine: schema.HostEngine{ID: b.ID, Family: b.Family, Version: b.Version, Executable: b.Executable, Profile: schema.ProfileFresh},
			Name:       b.Name, Protocol: string(b.Protocol), Driver: b.Driver, Source: string(b.Source), Notes: b.Notes,
		})
	}
	if asJSON {
		enc := json.NewEncoder(r.out)
		enc.SetIndent("", "  ")
		if err := enc.Encode(entries); err != nil {
			return ExitInternal
		}
		return ExitOK
	}
	if len(entries) == 0 {
		fmt.Fprintln(r.out, "no browsers found")
	}
	for i, b := range entries {
		v := b.Version
		if v == "" {
			v = "(version unknown)"
		}
		support := ""
		if !canDrive(found[i]) {
			support = "  (not supported yet)"
		}
		fmt.Fprintf(r.out, "%-12s %-24s %-7s %-16s %s%s\n", b.ID, b.Name, b.Family, v, b.Executable, support)
		for _, n := range b.Notes {
			fmt.Fprintln(r.out, "             note:", n)
		}
	}
	return ExitOK
}
