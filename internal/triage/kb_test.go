package triage

import (
	"testing"

	"github.com/GerMP21/Omnisnap/internal/layoutdiff"
	"github.com/GerMP21/Omnisnap/internal/schema"
)

// k4Candidate builds a cause with a width change and the same text metrics.
func k4Candidate(tag string, boxBefore, boxAfter, textBefore, textAfter float64) layoutdiff.Cause {
	ti := 0.0
	a := &layoutdiff.Node{N: &schema.Node{T: tag, B: [4]float64{0, 0, boxBefore, 100}, D: "block",
		Text: &schema.Text{TX: "x", LN: 1, TW: textBefore, TI: &ti, LH: schema.LineHeight{PX: 20}, FS: 16, FF: "Arial"}}}
	b := &layoutdiff.Node{N: &schema.Node{T: tag, B: [4]float64{0, 0, boxAfter, 100}, D: "block",
		Text: &schema.Text{TX: "x", LN: 1, TW: textAfter, TI: &ti, LH: schema.LineHeight{PX: 20}, FS: 16, FF: "Arial"}}}
	d := layoutdiff.Delta{DW: boxAfter - boxBefore}
	return layoutdiff.Cause{Side: "A", Node: a, Other: b, Kinds: []string{"width"}, D: &d}
}

// K4 explains a box that shrank around the same text, but not a box smaller
// than its text change explains (mutation testing: a width bug was explained
// away here) and not a generic shrunken paragraph.
func TestK4Bound(t *testing.T) {
	ctx := Context{}
	// Text advance changed and the box changed with it: explained.
	if kb := Match(k4Candidate("div", 200, 194, 150, 145), ctx); kb == nil || kb.ID != "K4" {
		t.Errorf("K4 not matched: %+v", kb)
	}
	// A 100 px box change with a 2 px text change: not text advance.
	if kb := Match(k4Candidate("div", 200, 100, 150, 148), ctx); kb != nil && kb.ID == "K4" {
		t.Errorf("K4 explained a layout change: %+v", kb)
	}
	// Table cells are exempt: an auto-layout column follows its widest text.
	if kb := Match(k4Candidate("td", 200, 100, 150, 148), ctx); kb == nil || kb.ID != "K4" {
		t.Errorf("K4 did not exempt a table cell: %+v", kb)
	}
}
