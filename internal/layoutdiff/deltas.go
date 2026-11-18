package layoutdiff

import (
	"math"
	"strings"
)

// Delta is what changed for one matched element.
type Delta struct {
	DW, DH float64 // size change, rounded to 0.1 px (reported: relative tolerance)
	RW, RH float64 // size change, absolute tolerance only (explains other changes)
	DX, DY float64 // parent-relative position change
	DLN    int     // line-count change
	Ovf    string  // "x", "y", "xy": an overflow appeared or disappeared
	Vis    bool    // visibility flipped
	Tr     bool    // truncation appeared or disappeared
	TI     bool    // own text moved inside an unchanged box
	Any    bool
}

// changed is the spike's tolerance test: the difference must exceed the larger
// of an absolute and a relative limit. The result is rounded to 0.1 px.
func changed(va, vb, abs, rel float64) float64 {
	d := vb - va
	limit := math.Max(abs, rel*math.Max(math.Abs(va), math.Abs(vb)))
	if math.Abs(d) > limit {
		return jsRound(d*10) / 10
	}
	return 0
}

// jsRound rounds half up (JavaScript's Math.round), so parity holds for the
// negative deltas too.
func jsRound(x float64) float64 {
	return math.Floor(x + 0.5)
}

// boxParent is the nearest ancestor with a real box (display: contents has none).
func boxParent(n *Node) *Node {
	p := n.Parent
	for p != nil && p.N.B[2] == 0 && p.N.B[3] == 0 && p.Parent != nil {
		p = p.Parent
	}
	return p
}

// Deltas measures a matched pair.
func Deltas(a, b *Node, m *Match) Delta {
	an, bn := a.N, b.N
	inline := an.D == "inline" && bn.D == "inline"
	d := Delta{
		DW: changed(an.B[2], bn.B[2], TolAbs, TolRel),
		DH: changed(an.B[3], bn.B[3], TolAbs, TolRel),
		RW: changed(an.B[2], bn.B[2], TolAbs, 0),
		RH: changed(an.B[3], bn.B[3], TolAbs, 0),
	}
	if inline {
		d.DH, d.RH = 0, 0
	}
	if pa := boxParent(a); pa != nil {
		if pb := m.PairOf[pa.UID]; pb != nil {
			d.DX = changed(an.B[0]-pa.N.B[0], bn.B[0]-pb.N.B[0], TolPos, 0)
			yTol := TolPos
			if inline {
				yTol = math.Max(TolPos, 0.2*math.Max(an.B[3], bn.B[3]))
			}
			d.DY = changed(an.B[1]-pa.N.B[1], bn.B[1]-pb.N.B[1], yTol, 0)
		}
	}
	if a.LN() != 0 && b.LN() != 0 && a.LN() != b.LN() {
		d.DLN = b.LN() - a.LN()
	}
	d.Vis = bool(an.Hid) != bool(bn.Hid)
	d.Tr = bool(an.TR) != bool(bn.TR)
	// Own text moved inside an unchanged box (padding, indent), beyond what a
	// different text width explains for centred or right-aligned text.
	if a.TI() != nil && b.TI() != nil && d.DLN == 0 && d.RW == 0 &&
		math.Abs(*b.TI()-*a.TI()) >= 3 &&
		math.Abs(*b.TI()-*a.TI()) > math.Abs(b.TW()-a.TW())+1 {
		d.TI = true
	}
	oa, ob := [2]float64{}, [2]float64{}
	if an.O != nil {
		oa = *an.O
	}
	if bn.O != nil {
		ob = *bn.O
	}
	flips := func(x, y float64) bool {
		return (x > TolOvf) != (y > TolOvf) && math.Abs(x-y) > TolOvfDelta
	}
	if flips(oa[0], ob[0]) {
		d.Ovf += "x"
	}
	if flips(oa[1], ob[1]) {
		d.Ovf += "y"
	}
	d.Any = d.DW != 0 || d.DH != 0 || d.DX != 0 || d.DY != 0 || d.DLN != 0 || d.Ovf != "" || d.Vis || d.Tr || d.TI
	return d
}

// ovfIncludes answers Delta.Ovf.
func (d Delta) ovfIncludes(axis string) bool { return strings.Contains(d.Ovf, axis) }
