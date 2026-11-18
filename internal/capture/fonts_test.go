package capture

import (
	"context"
	"strings"
	"testing"
)

type fakeEval struct{ out string }

func (f fakeEval) Evaluate(context.Context, string) (string, error) { return f.out, nil }

func report(sans string, w float64, missing bool) FontReport {
	return FontReport{
		SansSerif: ResolvedFont{Width: w, Font: sans},
		Serif:     ResolvedFont{Width: 100, Font: "Liberation Serif"},
		Monospace: ResolvedFont{Width: 120, Font: "Noto Sans Mono"},
		Default:   ResolvedFont{Width: 100, Font: "Liberation Sans"},
		Scripts:   map[string]ScriptCoverage{"Japanese": {Width: 80, Missing: missing}},
	}
}

func TestFontsParsesTheProbe(t *testing.T) {
	got, err := Fonts(context.Background(), fakeEval{out: `{"sans-serif":{"width":100.5,"font":"Liberation Sans"},"serif":{"width":100,"font":"Liberation Serif"},"monospace":{"width":120,"font":"Noto Sans Mono"},"default":{"width":100,"font":"Liberation Sans"},"inlineHeight":24,"scripts":{"Japanese":{"width":80,"missing":true}}}`})
	if err != nil {
		t.Fatal(err)
	}
	if got.SansSerif.Font != "Liberation Sans" || got.SansSerif.Width != 100.5 || !got.Scripts["Japanese"].Missing {
		t.Errorf("Fonts = %+v", got)
	}
}

func TestFontWarningsParity(t *testing.T) {
	base := report("Liberation Sans", 100, false)
	same := map[string]FontReport{"chrome": base, "firefox": report("Liberation Sans", 100.2, false)}
	if w := FontWarnings("chrome", base, same); len(w) != 0 {
		t.Errorf("warnings %v; want none within 0.5 px", w)
	}
	// A different font, or a width off by more than half a pixel, is a
	// mismatch that changes every text width.
	diff := map[string]FontReport{"chrome": base, "firefox": report("DejaVu Sans", 104, false)}
	w := FontWarnings("chrome", base, diff)
	if len(w) != 1 || !strings.Contains(w[0], "DejaVu Sans") || !strings.Contains(w[0], "Liberation Sans") || !strings.Contains(w[0], "firefox") {
		t.Errorf("warnings %v; want one naming both fonts and the engine", w)
	}
	// Missing glyphs are warned per engine and script.
	missing := map[string]FontReport{"chrome": base, "firefox": report("Liberation Sans", 100, true)}
	w = FontWarnings("chrome", base, missing)
	if len(w) != 1 || !strings.Contains(w[0], "Japanese") || !strings.Contains(w[0], "firefox") {
		t.Errorf("warnings %v; want the missing Japanese font", w)
	}
}

func TestFontTableMarksMismatches(t *testing.T) {
	base := report("Liberation Sans", 100, false)
	table := FontTable("chrome", base, map[string]FontReport{
		"chrome":  base,
		"firefox": report("DejaVu Sans", 104, false),
	})
	if !strings.Contains(table, "chrome") || !strings.Contains(table, "firefox") {
		t.Errorf("table misses an engine:\n%s", table)
	}
	if !strings.Contains(table, "≠") {
		t.Errorf("table does not mark the mismatch:\n%s", table)
	}
	if strings.Count(table, "≠") != 1 {
		t.Errorf("table marks %d rows; want only sans-serif", strings.Count(table, "≠"))
	}
}
