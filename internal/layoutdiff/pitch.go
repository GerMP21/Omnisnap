package layoutdiff

import (
	"fmt"
	"sort"
)

// linePitch is the distance between consecutive text lines inside an element.
// Engines round fractional line-heights differently, and the difference
// compounds with every line.
func linePitch(n *Node) (pitch float64, lines int, ok bool) {
	if n.TX() != "" && len(n.Kids) == 0 && n.LN() > 1 {
		return n.N.B[3] / float64(n.LN()), n.LN(), true
	}
	var centers []float64
	var visit func(x *Node)
	visit = func(x *Node) {
		if x.TX() != "" && !bool(x.N.Hid) && x.N.B[3] > 0 {
			centers = append(centers, x.N.B[1]+x.N.B[3]/2)
		}
		for _, k := range x.Kids {
			visit(k)
		}
	}
	visit(n)
	sort.Float64s(centers)
	var rows []float64
	for _, c := range centers {
		if len(rows) == 0 || c-rows[len(rows)-1] > 4 {
			rows = append(rows, c)
		}
	}
	if len(rows) < 3 {
		return 0, 0, false
	}
	return (rows[len(rows)-1] - rows[0]) / float64(len(rows)-1), len(rows), true
}

// pitchNote explains a height difference that comes from fractional
// line-height rounding: "line pitch 22.0 -> 22.7px over 35 lines".
func pitchNote(a, b *Node) string {
	if b == nil {
		return ""
	}
	pa, la, oka := linePitch(a)
	pb, lb, okb := linePitch(b)
	if !oka || !okb || la != lb || absFloat(pa-pb) < 0.2 {
		return ""
	}
	return fmt.Sprintf("line pitch %.1f -> %.1fpx over %d lines (fractional line-height rounded differently)", pa, pb, la)
}

func absFloat(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}
