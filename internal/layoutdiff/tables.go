package layoutdiff

import (
	"math"
	"strings"
)

// TableColumns replaces per-cell width causes with one cause on the table when
// the columns were sized differently (table-layout, column rules): the cells'
// widths change far more than any text in the table does, while the table
// keeps its width. Auto-layout columns that follow their widest text (text
// advance, K4) stay as they are.
func TableColumns(res *Analysis, a, b *Tree, m *Match) {
	tableOf := func(n *Node) *Node {
		for x, i := n.Parent, 0; x != nil && i < 6; x, i = x.Parent, i+1 {
			if x.N.T == "table" {
				return x
			}
		}
		return nil
	}
	isCell := func(n *Node) bool { return n.N.T == "td" || n.N.T == "th" }
	byTable := map[int][]int{} // table uid -> indices into res.Causes
	for i, c := range res.Causes {
		if c.Side != "A" || !contains(c.Kinds, "width") || !isCell(c.Node) {
			continue
		}
		if t := tableOf(c.Node); t != nil {
			byTable[t.UID] = append(byTable[t.UID], i)
		}
	}
	drop := map[int]bool{}
	byUID := map[int]*Node{}
	for _, n := range a.All {
		byUID[n.UID] = n
	}
	for uid, idx := range byTable {
		t := byUID[uid]
		if t == nil {
			continue
		}
		tb := m.PairOf[uid]
		if len(idx) < 2 || tb == nil || math.Abs(tb.N.B[2]-t.N.B[2]) > TolAbs {
			continue
		}
		var cells []*Node
		for _, n := range Subtree(t) {
			if isCell(n) && m.PairOf[n.UID] != nil {
				cells = append(cells, n)
			}
		}
		maxDw := 0.0
		for _, n := range cells {
			maxDw = math.Max(maxDw, math.Abs(m.PairOf[n.UID].N.B[2]-n.N.B[2]))
		}
		maxDtw := 0.0
		for _, n := range cells {
			other := m.PairOf[n.UID]
			if n.HasText() && other.HasText() {
				maxDtw = math.Max(maxDtw, math.Abs(other.TW()-n.TW()))
			}
		}
		if maxDw <= math.Max(8, 3*maxDtw+2) {
			continue
		}
		for _, i := range idx {
			drop[i] = true
		}
		var d *Delta
		if v, ok := res.D[uid]; ok {
			d = &v
		}
		res.Causes = append(res.Causes, Cause{Side: "A", Node: t, Other: tb, Kinds: []string{"columns"}, D: d,
			Note: "column widths differ by up to " + itoaRound(maxDw) + "px while the cells' text differs by at most " + itoaRound(maxDtw) + "px"})
	}
	if len(drop) == 0 {
		return
	}
	var kept []Cause
	for i, c := range res.Causes {
		if !drop[i] {
			kept = append(kept, c)
		}
	}
	res.Causes = kept
}

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

// itoaRound prints a px value rounded to an integer, as the spike's note does.
func itoaRound(f float64) string {
	n := int(math.Round(f))
	neg := n < 0
	if neg {
		n = -n
	}
	digits := "0123456789"
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{digits[n%10]}, b...)
		n /= 10
	}
	if neg {
		return "-" + string(b)
	}
	return string(b)
}

var _ = strings.TrimSpace
