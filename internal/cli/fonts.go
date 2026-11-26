package cli

import (
	"context"
	"fmt"

	"github.com/GerMP21/Omnisnap/internal/capture"
	"github.com/GerMP21/Omnisnap/internal/discovery"
	"github.com/GerMP21/Omnisnap/internal/schema"
)

// runCheckFonts is `omnisnap check-fonts` (SPEC-capture.md, "Fonts"): one
// browser per family, the font probe on about:blank, a table and the parity
// warnings.
func runCheckFonts(ctx context.Context, r *reporter, e env, args []string) int {
	if len(args) > 0 {
		fmt.Fprintln(r.err, "omnisnap: check-fonts takes no arguments")
		return ExitUsage
	}
	found, err := e.find(ctx)
	if err != nil {
		r.fatal(fmt.Errorf("discovery: %w", err))
		return ExitInternal
	}
	// One browser per family: launching every Chromium variant would measure
	// the same fonts again.
	var chosen []discovery.Browser
	seen := map[schema.Family]bool{}
	for _, b := range found {
		if seen[b.Family] || !canDrive(b) {
			continue
		}
		seen[b.Family] = true
		chosen = append(chosen, b)
	}
	if len(chosen) == 0 {
		r.fatal(fmt.Errorf("%w: no engine found", errNoBrowser))
		return ExitNoBrowser
	}
	reports := map[string]capture.FontReport{}
	baseline := ""
	for _, b := range chosen {
		eng, err := e.launch(ctx, b)
		if err != nil {
			r.fatal(fmt.Errorf("%s: %w", b.ID, err))
			return ExitInternal
		}
		s, err := eng.NewSession(ctx)
		if err != nil {
			_ = eng.Close(context.WithoutCancel(ctx))
			r.fatal(fmt.Errorf("%s: %w", b.ID, err))
			return ExitInternal
		}
		report, ferr := capture.Fonts(ctx, s)
		_ = s.Close(context.WithoutCancel(ctx))
		_ = eng.Close(context.WithoutCancel(ctx))
		if ferr != nil {
			r.fatal(fmt.Errorf("%s: %w", b.ID, ferr))
			return ExitInternal
		}
		reports[b.ID] = report
		if baseline == "" && b.Family == schema.FamilyBlink {
			baseline = b.ID
		}
	}
	if baseline == "" {
		baseline = chosen[0].ID
	}
	fmt.Fprint(r.out, capture.FontTable(baseline, reports[baseline], reports))
	warnings := capture.FontWarnings(baseline, reports[baseline], reports)
	for _, w := range warnings {
		fmt.Fprintln(r.out, "warning: "+w)
	}
	if len(warnings) == 0 {
		fmt.Fprintf(r.out, "font parity: %s and the other engines resolve the generic families alike\n", baseline)
	}
	return ExitOK
}

// checkFonts is the job pre-flight: with font parity on, measure every running
// engine and report mismatches as warnings (never fatal: the captures are
// still useful, they just measure the environment too). It returns the
// baseline's report; base is that report from an earlier group, whose
// baseline browser is closed by now (nil in the first group).
func checkFonts(ctx context.Context, r *reporter, baseline string, base *capture.FontReport, engines []discovery.Browser, running map[string]engine) *capture.FontReport {
	reports := map[string]capture.FontReport{}
	for _, b := range engines {
		eng, ok := running[b.ID]
		if !ok {
			continue
		}
		s, err := eng.NewSession(ctx)
		if err != nil {
			continue
		}
		if report, ferr := capture.Fonts(ctx, s); ferr == nil {
			reports[b.ID] = report
		}
		_ = s.Close(context.WithoutCancel(ctx))
	}
	if base == nil {
		b, ok := reports[baseline]
		if !ok {
			return nil
		}
		base = &b
	}
	for _, w := range capture.FontWarnings(baseline, *base, reports) {
		r.event(nil, "  warning: "+w)
	}
	return base
}
