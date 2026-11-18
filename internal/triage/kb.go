package triage

import (
	"fmt"
	"math"
	"regexp"
	"strings"

	"github.com/GerMP21/Omnisnap/internal/layoutdiff"
	"github.com/GerMP21/Omnisnap/internal/schema"
)

// Knowledge base (SPEC-triage.md, Task 52): expected, systematic engine
// differences, as data plus one matcher. Ported from lib/triage.mjs.

var formControls = map[string]bool{"input": true, "select": true, "textarea": true, "button": true}

// dominantType is the typography most of an element's text uses.
func dominantType(n *layoutdiff.Node) (schema.LineHeight, float64, string, string, bool) {
	type count struct {
		lh     schema.LineHeight
		fs     float64
		ff, fw string
		weight int
	}
	counts := map[string]*count{}
	var visit func(x *layoutdiff.Node)
	visit = func(x *layoutdiff.Node) {
		if x.N.Text != nil && !bool(x.N.Hid) {
			t := x.N.Text
			key := fmt.Sprintf("%v|%v|%s|%s", t.LH, t.FS, t.FF, t.FW)
			c := counts[key]
			if c == nil {
				c = &count{lh: t.LH, fs: t.FS, ff: t.FF, fw: t.FW}
				counts[key] = c
			}
			w := t.LN
			if w == 0 {
				w = 1
			}
			c.weight += w
		}
		for _, k := range x.Kids {
			visit(k)
		}
	}
	visit(n)
	var best *count
	for _, c := range counts {
		if best == nil || c.weight > best.weight {
			best = c
		}
	}
	if best == nil {
		return schema.LineHeight{}, 0, "", "", false
	}
	return best.lh, best.fs, best.ff, best.fw, true
}

// lineInfo delegates to layoutdiff (the calibration model shares it).
func lineInfo(n *layoutdiff.Node) (int, float64, bool) { return layoutdiff.LineInfo(n) }

var longToken = regexp.MustCompile(`(://|\S{25,})`)

// Match finds the knowledge-base entry that explains a cause, if any.
func Match(c layoutdiff.Cause, ctx Context) *KB {
	if c.Side != "A" || c.Other == nil {
		return nil
	}
	a, b := c.Node, c.Other
	d := c.D
	if d == nil {
		d = &layoutdiff.Delta{}
	}
	lah, lafs, laff, lafw, oka := dominantType(a)
	lbh, lbfs, lbff, lbfw, okb := dominantType(b)
	engines := ctx.Engines

	// K1 / K2: line pitch differs, same text, same lines, same font.
	if oka && okb && lafs == lbfs && laff == lbff && lafw == lbfw && lah == lbh {
		la, lpa, oka := lineInfo(a)
		lb, lpb, okb := lineInfo(b)
		if oka && okb && la == lb {
			dp := lpb - lpa
			if !lah.Normal && lah.PX != math.Trunc(lah.PX) && math.Abs(dp) >= 0.05 && math.Abs(dp) <= 1 {
				height := "a small drift"
				if d.DH != 0 {
					height = fmt.Sprintf("%+.1fpx", d.DH)
				}
				return &KB{ID: "K1", Expected: true, Title: "Fractional line-height rounded differently",
					Explanation: fmt.Sprintf("line-height %vpx is laid out at %.2fpx per line in %s vs %.2fpx in %s; over %d lines that is %s",
						lah.PX, lpb, engines.B, lpa, engines.A, la, height),
					Fix: fmt.Sprintf("use a whole-pixel line-height, e.g. line-height: %dpx (or round(calc(...), 1px))", int(math.Round(lah.PX)))}
			}
			if lah.Normal && math.Abs(dp) >= 0.2 {
				return &KB{ID: "K2", Expected: true, Title: "line-height: normal resolves differently",
					Explanation: fmt.Sprintf("line-height: normal is derived from the font's metrics: %.2fpx per line in %s, %.2fpx in %s (%d lines)",
						lpa, engines.A, lpb, engines.B, la),
					Fix: "set an explicit numeric line-height"}
			}
		}
	}
	// K5: a long unbreakable token (URL, long word) breaks across lines differently.
	if d.DLN != 0 && longToken.MatchString(a.TX()) {
		tx := a.TX()
		if len(tx) > 40 {
			tx = tx[:40]
		}
		return &KB{ID: "K5", Expected: false, Title: "Long token (URL) breaks differently",
			Explanation: fmt.Sprintf("%q wraps to %d line(s) in %s and %d in %s: engines apply different line-break opportunities to long tokens",
				tx, a.LN(), engines.A, b.LN(), engines.B),
			Fix: "overflow-wrap: anywhere (or insert <wbr> break points)"}
	}
	// K8: native form control with a different default size.
	if formControls[a.N.T] && (d.DW != 0 || d.DH != 0) {
		parts := ""
		if d.DW != 0 {
			parts = fmt.Sprintf("width %+.1fpx", d.DW)
		}
		if d.DH != 0 {
			if parts != "" {
				parts += ", "
			}
			parts += fmt.Sprintf("height %+.1fpx", d.DH)
		}
		return &KB{ID: "K8", Expected: true, Title: "Form control default size differs",
			Explanation: fmt.Sprintf("<%s> has engine-specific default dimensions (%s)", a.N.T, parts),
			Fix:         "give the control an explicit width/height and box-sizing"}
	}
	// K4: own text measures differently (shaping/hinting), same font, same lines.
	if contains(c.Kinds, "width") && d.DLN == 0 && a.HasText() && b.HasText() &&
		a.TW() != 0 && b.TW() != 0 && oka && okb && lafs == lbfs && laff == lbff && lafw == lbfw {
		ratio := b.TW() / a.TW()
		boxChange := math.Abs(b.N.B[2] - a.N.B[2])
		// Table cells are exempt: an auto-layout column follows its widest text.
		explains := a.N.T == "td" || a.N.T == "th" || boxChange <= math.Abs(b.TW()-a.TW())+2
		if ratio > 0.95 && ratio < 1.05 && explains {
			return &KB{ID: "K4", Expected: true, Title: "Text advance differs",
				Explanation: fmt.Sprintf("the same text in the same font measures %gpx in %s and %gpx in %s (glyph advances, hinting)",
					a.TW(), engines.A, b.TW(), engines.B),
				Fix: "no fix needed unless it causes wrapping, clipping or overflow"}
		}
	}
	return nil
}

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

func hasPrefixAny(ss []string, prefixes ...string) bool {
	for _, s := range ss {
		for _, p := range prefixes {
			if strings.HasPrefix(s, p) {
				return true
			}
		}
	}
	return false
}
