package capture

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
)

// Font parity (SPEC-capture.md, "Fonts"; spike fontcheck.mjs): if engines
// resolve `sans-serif` (or serif, monospace) to different fonts, every text
// width differs and the diff measures the environment instead of the page.
// Launch settings point fontconfig engines at Blink's fonts (process); this
// probe verifies it, and flags scripts no installed font covers (CJK boxes
// differ per engine, spike lesson 2).

// fontProbe measures the generic families' resolved fonts and script
// coverage. Only names that measure the same with two different fallbacks
// behind them are trusted as "resolved": a missing font silently falls back.
const fontProbe = `(() => {
  const ctx = document.createElement('canvas').getContext('2d');
  const sample = 'The quick brown fox jumps over the lazy dog 0123456789';
  const width = (font, text) => { ctx.font = '16px ' + font; return Math.round(ctx.measureText(text).width * 10) / 10; };
  const candidates = ['Noto Sans', 'DejaVu Sans', 'Liberation Sans', 'Noto Serif', 'DejaVu Serif', 'Liberation Serif', 'Noto Sans Mono', 'DejaVu Sans Mono', 'Liberation Mono'];
  const named = candidates.filter((f) => width('"' + f + '", monospace') === width('"' + f + '", serif'));
  const widths = Object.fromEntries(named.map((f) => [f, width('"' + f + '"')]));
  const resolve = (generic) => {
    const w = width(generic);
    const hit = named.find((f) => Math.abs(widths[f] - w) < 0.05);
    return { width: w, font: hit || 'unknown' };
  };
  const span = document.createElement('span');
  span.textContent = 'Docs';
  span.style.font = '16px/1.5 sans-serif';
  document.body.append(span);
  const height = span.getBoundingClientRect().height;
  span.remove();
  const scripts = {
    'Japanese': '日本語のテキスト',
    'Simplified Chinese': '简体中文文本',
    'Hangul': '한국어 텍스트',
    'Arabic': 'نص عربي',
  };
  const coverage = {};
  for (const [name, text] of Object.entries(scripts)) {
    const n = [...text].length;
    const tofu = '\uFFFD'.repeat(n);
    coverage[name] = { width: width('sans-serif', text), missing: Math.abs(width('sans-serif', text) - width('sans-serif', tofu)) < 0.5 };
  }
  return JSON.stringify({
    'sans-serif': resolve('sans-serif'),
    'serif': resolve('serif'),
    'monospace': resolve('monospace'),
    'default': resolve(getComputedStyle(document.body).fontFamily),
    'inlineHeight': height,
    'scripts': coverage,
  });
})()`

// ResolvedFont is what a generic family resolved to, measured.
type ResolvedFont struct {
	Width float64 `json:"width"`
	Font  string  `json:"font"`
}

// ScriptCoverage says whether a script has an installed font.
type ScriptCoverage struct {
	Width   float64 `json:"width"`
	Missing bool    `json:"missing"`
}

// FontReport is one engine's answer to the font probe.
type FontReport struct {
	SansSerif    ResolvedFont              `json:"sans-serif"`
	Serif        ResolvedFont              `json:"serif"`
	Monospace    ResolvedFont              `json:"monospace"`
	Default      ResolvedFont              `json:"default"`
	InlineHeight float64                   `json:"inlineHeight"`
	Scripts      map[string]ScriptCoverage `json:"scripts"`
}

// evaluator is what Fonts needs: a page to run the probe on.
type evaluator interface {
	Evaluate(ctx context.Context, expr string) (string, error)
}

// Fonts runs the probe on a session's current page.
func Fonts(ctx context.Context, s evaluator) (FontReport, error) {
	raw, err := s.Evaluate(ctx, fontProbe)
	if err != nil {
		return FontReport{}, fmt.Errorf("fonts: %w", err)
	}
	var r FontReport
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		return FontReport{}, fmt.Errorf("fonts: probe result: %w", err)
	}
	return r, nil
}

// fontWidthTolerance is the spike's parity threshold in px.
const fontWidthTolerance = 0.5

// FontWarnings compares each engine's report with the baseline's: a different
// font (or a width off by more than half a pixel) for a generic family, and
// any script with no installed font. Deterministic order (by engine, then
// family/script) so job warnings do not churn.
func FontWarnings(baselineFont string, base FontReport, engines map[string]FontReport) []string {
	names := make([]string, 0, len(engines))
	for name := range engines {
		names = append(names, name)
	}
	sort.Strings(names)
	var warnings []string
	families := []struct {
		name string
		a, b ResolvedFont
	}{
		{"sans-serif", base.SansSerif, FontReport{}.SansSerif},
		{"serif", base.Serif, FontReport{}.Serif},
		{"monospace", base.Monospace, FontReport{}.Monospace},
	}
	for _, engine := range names {
		if engine == baselineFont {
			continue
		}
		r := engines[engine]
		got := []ResolvedFont{r.SansSerif, r.Serif, r.Monospace}
		for i, f := range families {
			g := got[i]
			if f.a.Font != g.Font || math.Abs(f.a.Width-g.Width) > fontWidthTolerance {
				warnings = append(warnings, fmt.Sprintf(
					"font parity: %s resolves %s to %s (%g px), %s to %s (%g px); text widths will differ",
					baselineFont, f.name, f.a.Font, f.a.Width, engine, g.Font, g.Width))
			}
		}
		scripts := make([]string, 0, len(r.Scripts))
		for script := range r.Scripts {
			scripts = append(scripts, script)
		}
		sort.Strings(scripts)
		for _, script := range scripts {
			if r.Scripts[script].Missing {
				warnings = append(warnings, fmt.Sprintf(
					"missing font: %s has no font for %s; its glyphs render differently in every engine", engine, script))
			}
		}
	}
	return warnings
}

// FontTable renders reports for `omnisnap check-fonts`: one line per engine
// and generic family, with a marker when it differs from the baseline.
func FontTable(baselineFont string, base FontReport, engines map[string]FontReport) string {
	names := make([]string, 0, len(engines))
	for name := range engines {
		names = append(names, name)
	}
	sort.Strings(names)
	var b strings.Builder
	fmt.Fprintf(&b, "%-16s %-12s %-22s %8s\n", "engine", "family", "font", "width")
	rows := func(name string, r FontReport) []struct {
		family string
		f      ResolvedFont
	} {
		return []struct {
			family string
			f      ResolvedFont
		}{{"sans-serif", r.SansSerif}, {"serif", r.Serif}, {"monospace", r.Monospace}, {"default", r.Default}}
	}
	for _, name := range names {
		for _, row := range rows(name, engines[name]) {
			mark := " "
			if name != baselineFont {
				var bf ResolvedFont
				switch row.family {
				case "sans-serif":
					bf = base.SansSerif
				case "serif":
					bf = base.Serif
				case "monospace":
					bf = base.Monospace
				default:
					bf = base.Default
				}
				if bf.Font != row.f.Font || math.Abs(bf.Width-row.f.Width) > fontWidthTolerance {
					mark = "≠"
				}
			}
			fmt.Fprintf(&b, "%-16s %-12s %-22s %8g %s\n", name, row.family, row.f.Font, row.f.Width, mark)
		}
	}
	return b.String()
}
