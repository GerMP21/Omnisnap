package layoutdiff

import "sort"

// LineInfo is an element's text lines: distinct line tops and the median gap
// between consecutive lines (robust to paragraph gaps). Used by the
// knowledge base and by the page calibration model.
func LineInfo(n *Node) (lines int, pitch float64, ok bool) {
	var tops []float64
	var visit func(x *Node)
	visit = func(x *Node) {
		if x.N.Text != nil && !bool(x.N.Hid) {
			tops = append(tops, x.N.Text.LT...)
		}
		for _, c := range x.Kids {
			visit(c)
		}
	}
	visit(n)
	if len(tops) == 0 {
		return 0, 0, false
	}
	sort.Float64s(tops)
	var rows []float64
	for _, t := range tops {
		if len(rows) == 0 || t-rows[len(rows)-1] > 1 {
			rows = append(rows, t)
		}
	}
	if len(rows) < 3 {
		return 0, 0, false
	}
	gaps := make([]float64, 0, len(rows)-1)
	for i := 1; i < len(rows); i++ {
		gaps = append(gaps, rows[i]-rows[i-1])
	}
	return len(rows), Median(gaps), true
}

// Median of xs (0 for an empty slice).
func Median(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := append([]float64{}, xs...)
	sort.Float64s(s)
	m := len(s) / 2
	if len(s)%2 == 1 {
		return s[m]
	}
	return (s[m-1] + s[m]) / 2
}
