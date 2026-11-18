package layoutdiff

import (
	"fmt"
	"math"
	"strings"
)

// Cause is one element where a difference starts.
type Cause struct {
	Side  string // "A" (baseline) or "B" (the other engine); "B" causes carry the B node
	Node  *Node
	Other *Node // the matched node on the other side, when any
	Kinds []string
	D     *Delta
	Note  string
	// BlockedBy names the hosts whose blocked resources this element (or a
	// descendant) uses: the usual explanation for a branded browser's cause.
	BlockedBy []string
}

// Symptom is an overflow that another element already explains.
type Symptom struct {
	Selector string
	Kind     string
}

// Analysis is one pair's result.
type Analysis struct {
	D         map[int]Delta // A uid -> delta
	Causes    []Cause
	Symptoms  []Symptom
	Explained int
	Masked    int
}

// Analyze decides which changed elements are causes and which only moved or
// resized because of something else (the spike's lib/align.mjs rules):
//
//   - a child whose parent changed width is reflowing: effect
//   - a box that grew because a child grew/wrapped/moved: effect
//   - an item that moved because a sibling resized or an earlier sibling moved
//     the same way: effect
//   - children that all moved with none resized: one cause on the parent
//     (padding, gap, order, justify)
//   - in flex/grid rows, a box stretched like its siblings: effect
//   - overflow explained by a descendant's width/overflow: symptom, not cause
//   - an element present in only one engine: cause
func Analyze(a, b *Tree, m *Match, maskA, maskB map[int]bool) *Analysis {
	res := &Analysis{D: map[int]Delta{}}
	for _, n := range a.All {
		if other := m.PairOf[n.UID]; other != nil {
			res.D[n.UID] = Deltas(n, other, m)
		}
	}
	unmatchedUnder := map[int]bool{} // A uids whose children differ in membership
	for _, n := range m.OnlyA {
		unmatchedUnder[n.Parent.UID] = true
	}
	for _, n := range m.OnlyB {
		if pa := m.RevPair[n.Parent.UID]; pa != nil {
			unmatchedUnder[pa.UID] = true
		}
	}

	sized := func(n *Node) bool {
		d, ok := res.D[n.UID]
		return ok && (d.RW != 0 || d.RH != 0 || d.DLN != 0)
	}
	moved := func(n *Node) bool {
		d, ok := res.D[n.UID]
		return ok && (d.DX != 0 || d.DY != 0)
	}
	// Inert: takes no space (zero area), or is transparent AND out of flow.
	// A transparent element that still occupies space is NOT inert: it pushes
	// its siblings around (Stripe: a fading-in panel).
	noBox := func(n *Node) bool {
		return n.N.B[2] == 0 || n.N.B[3] == 0 || (bool(n.N.Hid) && (n.N.P == "absolute" || n.N.P == "fixed"))
	}
	invisible := func(n *Node) bool {
		other := m.PairOf[n.UID]
		return noBox(n) && (other == nil || noBox(other))
	}
	isMasked := func(n *Node) bool {
		if maskA[n.UID] {
			return true
		}
		if other := m.PairOf[n.UID]; other != nil && maskB[other.UID] {
			return true
		}
		return false
	}

	type flags struct{ w, h, ox, oy bool }
	flagMemo := map[int]flags{}
	var descFlags func(n *Node) flags
	descFlags = func(n *Node) flags {
		if f, ok := flagMemo[n.UID]; ok {
			return f
		}
		var f flags
		for _, c := range n.Kids {
			if d, ok := res.D[c.UID]; ok {
				f.w = f.w || d.DW != 0
				f.h = f.h || d.DH != 0 || d.DLN != 0
				f.ox = f.ox || d.ovfIncludes("x")
				f.oy = f.oy || d.ovfIncludes("y")
			}
			cf := descFlags(c)
			f.w = f.w || cf.w
			f.h = f.h || cf.h
			f.ox = f.ox || cf.ox
			f.oy = f.oy || cf.oy
		}
		flagMemo[n.UID] = f
		return f
	}

	movesKids := map[int]bool{}
	for _, n := range a.All {
		if _, ok := res.D[n.UID]; !ok || unmatchedUnder[n.UID] {
			continue
		}
		var kids []*Node
		for _, c := range n.Kids {
			if _, ok := res.D[c.UID]; ok && !invisible(c) {
				kids = append(kids, c)
			}
		}
		if len(kids) == 0 {
			continue
		}
		anySized := false
		for _, k := range kids {
			anySized = anySized || sized(k)
		}
		if anySized {
			continue
		}
		m := 0
		for _, k := range kids {
			if moved(k) {
				m++
			}
		}
		if m >= 2 || (m == 1 && len(kids) == 1) {
			movesKids[n.UID] = true
		}
	}

	// Does child c keep the same offset or proportion to parent p in both
	// engines? Then its width just follows the parent (block fill, percentage
	// widths, SVG viewBox scaling).
	tracks := func(c, p *Node) bool {
		cb, pb := m.PairOf[c.UID], m.PairOf[p.UID]
		if cb == nil || pb == nil {
			return false
		}
		if math.Abs((pb.N.B[2]-cb.N.B[2])-(p.N.B[2]-c.N.B[2])) <= TolAbs {
			return true
		}
		return p.N.B[2] > 0 && pb.N.B[2] > 0 &&
			math.Abs(cb.N.B[2]/pb.N.B[2]-c.N.B[2]/p.N.B[2]) <= TolRel
	}
	textChanged := func(n *Node) bool {
		other := m.PairOf[n.UID]
		if other == nil || !n.HasText() || !other.HasText() {
			return false
		}
		return changed(n.TW(), other.TW(), TolAbs, TolRel) != 0
	}
	widthCause := func(n *Node, d Delta, p *Node, pd *Delta, kids, sibs []*Node) bool {
		if d.DW == 0 {
			return false
		}
		if pd != nil && pd.RW != 0 && tracks(n, p) {
			return false // follows its parent
		}
		for _, k := range kids {
			if kd := res.D[k.UID]; kd.RW != 0 && !tracks(k, n) {
				return false // pushed by its content
			}
		}
		if textChanged(n) {
			return true // its own text got wider or narrower
		}
		if pd != nil && math.Signbit(pd.RW) == math.Signbit(d.DW) && pd.RW != 0 && d.DW != 0 {
			return false // share of a resized parent (table cells, flex-grow)
		}
		if p != nil && regexpFlexGrid(p.N.D) {
			for _, s := range sibs {
				if sd, ok := res.D[s.UID]; ok && sd.RW != 0 {
					return false // redistributed by a sibling
				}
			}
		}
		if unmatchedUnder[n.UID] {
			return false // children added or removed
		}
		return true // explicitly sized differently
	}

	for _, n := range a.All {
		d, ok := res.D[n.UID]
		if !ok || invisible(n) {
			continue
		}
		if !d.Any && !movesKids[n.UID] {
			continue
		}
		p := n.Parent
		var pd *Delta
		if p != nil {
			if v, ok := res.D[p.UID]; ok {
				pd = &v
			}
		}
		parentReflow := pd != nil && pd.RW != 0
		rowish := p != nil && regexpFlexGrid(p.N.D)
		var sibs, kids []*Node
		if p != nil {
			for _, s := range p.Kids {
				if s != n {
					if _, ok := res.D[s.UID]; ok {
						sibs = append(sibs, s)
					}
				}
			}
		}
		for _, c := range n.Kids {
			if _, ok := res.D[c.UID]; ok {
				kids = append(kids, c)
			}
		}
		hasUnmatched := unmatchedUnder[n.UID]
		var kinds []string

		// Inline flow cascades forward.
		if p != nil && strings.HasPrefix(n.N.D, "inline") {
			idx := indexOf(p.Kids, n)
			if (pd != nil && (pd.DLN != 0 || textChanged(p))) ||
				anyAny(res, p.Kids[:idx]) {
				res.Explained++
				continue
			}
		}

		if d.Vis && !(pd != nil && pd.Vis) {
			if bool(n.N.Hid) {
				kinds = append(kinds, "hidden-in-"+a.Engine)
			} else {
				kinds = append(kinds, "hidden-in-"+b.Engine)
			}
		}
		if d.Tr {
			if bool(n.N.TR) {
				kinds = append(kinds, "truncated-in-"+a.Engine)
			} else {
				kinds = append(kinds, "truncated-in-"+b.Engine)
			}
		}
		if d.TI {
			kinds = append(kinds, "text-shifted")
		}
		if d.DLN != 0 && d.RW == 0 {
			kinds = append(kinds, "wrap")
		}
		if widthCause(n, d, p, pd, kids, sibs) {
			kinds = append(kinds, "width")
		}
		if d.DH != 0 && d.DLN == 0 && d.RW == 0 && !parentReflow && !anySized(res, kids) && !anyMoved(res, kids) && !hasUnmatched {
			stretched := rowish && anySameSignRH(res, sibs, d.DH)
			if !stretched {
				kinds = append(kinds, "height")
			}
		}
		if movesKids[n.UID] {
			kinds = append(kinds, "children-moved")
		}
		if (d.DX != 0 || d.DY != 0) && !parentReflow && !(p != nil && movesKids[p.UID]) {
			idx := 0
			if p != nil {
				idx = indexOf(p.Kids, n)
			}
			sibSized := false
			for _, s := range sibs {
				sibSized = sibSized || sized(s)
			}
			if p != nil && unmatchedUnder[p.UID] {
				sibSized = true
			}
			earlierSame := false
			for _, s := range sibs {
				if p != nil && indexOf(p.Kids, s) > idx {
					continue
				}
				sd, ok := res.D[s.UID]
				if !ok {
					continue
				}
				if d.DY != 0 && math.Signbit(sd.DY) == math.Signbit(d.DY) && sd.DY != 0 {
					earlierSame = true
				}
				if d.DX != 0 && math.Signbit(sd.DX) == math.Signbit(d.DX) && sd.DX != 0 {
					earlierSame = true
				}
			}
			xOwn := d.DX == 0 || (d.RW != 0 && math.Abs(d.DX) <= math.Abs(d.RW)+TolPos)
			yOwn := d.DY == 0 || (d.RH != 0 && math.Abs(d.DY) <= math.Abs(d.RH)+TolPos)
			if !sibSized && !earlierSame && !(xOwn && yOwn) {
				kinds = append(kinds, "position")
			}
		}
		if d.Ovf != "" {
			f := descFlags(n)
			sel := Selector(n)
			if n.N.T == "html" {
				res.Symptoms = append(res.Symptoms, Symptom{Selector: sel, Kind: "page-overflow-" + d.Ovf})
			} else {
				if d.ovfIncludes("x") {
					if !f.w && !f.ox {
						kinds = append(kinds, "overflow-x")
					} else {
						res.Symptoms = append(res.Symptoms, Symptom{Selector: sel, Kind: "overflow-x"})
					}
				}
				if d.ovfIncludes("y") {
					if !f.h && !f.oy {
						kinds = append(kinds, "overflow-y")
					} else {
						res.Symptoms = append(res.Symptoms, Symptom{Selector: sel, Kind: "overflow-y"})
					}
				}
			}
		}

		if len(kinds) == 0 {
			res.Explained++
			continue
		}
		if isMasked(n) {
			res.Masked++
			continue
		}
		note := ""
		for _, k := range kinds {
			if k == "height" || k == "children-moved" {
				note = pitchNote(n, m.PairOf[n.UID])
				break
			}
		}
		dd := d
		res.Causes = append(res.Causes, Cause{Side: "A", Node: n, Other: m.PairOf[n.UID], Kinds: kinds, D: &dd, Note: note})
	}

	// Elements present in one engine only, grouped by their (matched) parent.
	// Extras on BOTH sides, or more than 3, mean the content itself differs:
	// one finding on the parent, not N.
	type extras struct{ a, b []*Node }
	byParent := map[int]*extras{}
	bucket := func(uid int) *extras {
		e := byParent[uid]
		if e == nil {
			e = &extras{}
			byParent[uid] = e
		}
		return e
	}
	for _, n := range m.OnlyA {
		if noBox(n) {
			continue
		}
		if maskA[n.UID] {
			res.Masked++
			continue
		}
		if n.Parent != nil {
			bucket(n.Parent.UID).a = append(bucket(n.Parent.UID).a, n)
		}
	}
	for _, n := range m.OnlyB {
		if noBox(n) {
			continue
		}
		if maskB[n.UID] {
			res.Masked++
			continue
		}
		if n.Parent != nil {
			if pa := m.RevPair[n.Parent.UID]; pa != nil {
				bucket(pa.UID).b = append(bucket(pa.UID).b, n)
			}
		}
	}
	byUID := map[int]*Node{}
	for _, n := range a.All {
		byUID[n.UID] = n
	}
	for uid, e := range byParent {
		if (len(e.a) > 0 && len(e.b) > 0) || len(e.a)+len(e.b) > 3 {
			parent := byUID[uid]
			if parent == nil {
				continue
			}
			if isMasked(parent) {
				res.Masked++
				continue
			}
			var d *Delta
			if v, ok := res.D[uid]; ok {
				d = &v
			}
			res.Causes = append(res.Causes, Cause{Side: "A", Node: parent, Other: m.PairOf[uid], Kinds: []string{"children-differ"}, D: d,
				Note: fmt.Sprintf("%d child element(s) only in %s, %d only in %s", len(e.a), a.Engine, len(e.b), b.Engine)})
			continue
		}
		for _, n := range e.a {
			res.Causes = append(res.Causes, Cause{Side: "A", Node: n, Kinds: []string{"only-in-" + a.Engine}})
		}
		for _, n := range e.b {
			res.Causes = append(res.Causes, Cause{Side: "B", Node: n, Kinds: []string{"only-in-" + b.Engine}})
		}
	}
	// Task 44: one columns cause on the table instead of one per cell.
	TableColumns(res, a, b, m)
	return res
}

func regexpFlexGrid(display string) bool {
	return strings.Contains(display, "flex") || strings.Contains(display, "grid")
}

func anyAny(res *Analysis, nodes []*Node) bool {
	for _, n := range nodes {
		if d, ok := res.D[n.UID]; ok && d.Any {
			return true
		}
	}
	return false
}

func anySized(res *Analysis, nodes []*Node) bool {
	for _, n := range nodes {
		if d, ok := res.D[n.UID]; ok && (d.RW != 0 || d.RH != 0 || d.DLN != 0) {
			return true
		}
	}
	return false
}

func anyMoved(res *Analysis, nodes []*Node) bool {
	for _, n := range nodes {
		if d, ok := res.D[n.UID]; ok && (d.DX != 0 || d.DY != 0) {
			return true
		}
	}
	return false
}

func anySameSignRH(res *Analysis, nodes []*Node, dh float64) bool {
	for _, n := range nodes {
		if d, ok := res.D[n.UID]; ok && d.RH != 0 && math.Signbit(d.RH) == math.Signbit(dh) {
			return true
		}
	}
	return false
}

func indexOf(nodes []*Node, n *Node) int {
	for i, k := range nodes {
		if k == n {
			return i
		}
	}
	return -1
}
