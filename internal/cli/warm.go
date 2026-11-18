package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/GerMP21/Omnisnap/internal/branded"
	"github.com/GerMP21/Omnisnap/internal/discovery"
	"github.com/GerMP21/Omnisnap/internal/schema"
)

// runWarm is `omnisnap warm <browser>` (Task 65): build the branded browser's
// template profile so its blocking lists are active during captures.
func runWarm(ctx context.Context, r *reporter, e env, args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(r.err, "omnisnap: warm takes one browser id (brave, edge)")
		return ExitUsage
	}
	id := strings.ToLower(args[0])
	found, err := e.find(ctx)
	if err != nil {
		r.fatal(fmt.Errorf("discovery: %w", err))
		return ExitInternal
	}
	var b *discovery.Browser
	for i := range found {
		if found[i].ID == id {
			b = &found[i]
			break
		}
	}
	if b == nil {
		r.fatal(fmt.Errorf("%s is not installed", id))
		return ExitNoBrowser
	}
	if b.Family != schema.FamilyBlink {
		fmt.Fprintf(r.err, "omnisnap: %s is not a branded Blink browser\n", id)
		return ExitUsage
	}
	cacheDir, err := cacheDir(e)
	if err != nil {
		r.fatal(err)
		return ExitInternal
	}
	t, err := branded.Warm(ctx, branded.WarmOptions{
		Browser: id, Exe: b.Executable, Version: b.Version, Cache: filepath.Dir(cacheDir),
		Logf: func(format string, a ...any) { fmt.Fprintf(r.out, format+"\n", a...) },
	})
	if err != nil {
		r.fatal(err)
		return ExitInternal
	}
	fmt.Fprintf(r.out, "%s\ncaptures of %s now use a copy of this template\n", t.VersionLine(), id)
	return ExitOK
}
