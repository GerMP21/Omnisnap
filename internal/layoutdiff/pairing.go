package layoutdiff

import "math"

// PairRuns picks the runs whose page states match best (a slideshow may be on
// different slides in each engine). The cost is |ΔdocH| + 10·|Δnodes|; ties
// keep run a. The returned A2/B2 are the other runs, for the dynamic masks.
func PairRuns(a, a2, b, b2 *Tree) (A, A2, B, B2 *Tree, runA, runB string) {
	cost := func(x, y *Tree) float64 {
		if x == nil || y == nil {
			return math.Inf(1)
		}
		return math.Abs(float64(x.Meta.DocH-y.Meta.DocH)) + 10*math.Abs(float64(x.Meta.Nodes-y.Meta.Nodes))
	}
	best := cost(a, b)
	runA, runB = "a", "a"
	A, A2, B, B2 = a, a2, b, b2
	for _, ra := range []struct {
		name string
		x    *Tree
	}{{"a", a}, {"b", a2}} {
		for _, rb := range []struct {
			name string
			y    *Tree
		}{{"a", b}, {"b", b2}} {
			c := cost(ra.x, rb.y)
			if c < best-0.5 {
				best, runA, runB = c, ra.name, rb.name
			}
		}
	}
	if runA == "b" {
		A, A2 = a2, a
	}
	if runB == "b" {
		B, B2 = b2, b
	}
	return A, A2, B, B2, runA, runB
}
