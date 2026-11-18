package evidence

import (
	"math"

	"github.com/GerMP21/Omnisnap/internal/layoutdiff"
)

// Consensus (Task 56): with a third engine FAMILY (Blink, Gecko, WebKit; Blink
// counts once), decide who deviates for each cause: B is odd (the third
// agrees with the baseline), A is odd (it agrees with the engine), or every
// engine differs. Ported from lib/consensus.mjs.

// Verdicts.
const (
	ConsensusBaselineOdd = "baseline-odd"
	ConsensusEngineOdd   = "engine-odd"
	ConsensusAllDiffer   = "all-differ"
)

func closeEnough(p, q, rel, abs float64) bool {
	return math.Abs(p-q) <= math.Max(abs, rel*math.Max(math.Abs(p), math.Abs(q)))
}

// agree: the same rendered geometry (size, position relative to the parent,
// line count, visibility).
func agree(x, y *layoutdiff.Node) bool {
	if x == nil || y == nil {
		return false
	}
	if bool(x.N.Hid) != bool(y.N.Hid) {
		return false
	}
	if !closeEnough(x.N.B[2], y.N.B[2], 0.01, 2) || !closeEnough(x.N.B[3], y.N.B[3], 0.01, 2) {
		return false
	}
	if x.LN() != 0 && y.LN() != 0 && x.LN() != y.LN() {
		return false
	}
	if x.Parent != nil && y.Parent != nil {
		if !closeEnough(x.N.B[0]-x.Parent.N.B[0], y.N.B[0]-y.Parent.N.B[0], 0, 2) {
			return false
		}
		if !closeEnough(x.N.B[1]-x.Parent.N.B[1], y.N.B[1]-y.Parent.N.B[1], 0, 2) {
			return false
		}
	}
	return true
}

// ConsensusOf votes on a cause with every third family. The verdict is "" when
// no third family can decide.
func ConsensusOf(c layoutdiff.Cause, thirds []layoutdiff.Third) string {
	if len(thirds) == 0 {
		return ""
	}
	var a, b *layoutdiff.Node
	if c.Side == "A" {
		a, b = c.Node, c.Other
	} else {
		b = c.Node
	}
	var decided []string
	for _, t := range thirds {
		if a == nil {
			continue // cannot anchor without the baseline node
		}
		y := t.M.PairOf[a.UID]
		if b == nil { // present in A only
			if y != nil {
				decided = append(decided, ConsensusEngineOdd)
			} else {
				decided = append(decided, ConsensusBaselineOdd)
			}
			continue
		}
		if y == nil {
			continue // unknown
		}
		withA, withB := agree(a, y), agree(b, y)
		switch {
		case withA && !withB:
			decided = append(decided, ConsensusEngineOdd)
		case withB && !withA:
			decided = append(decided, ConsensusBaselineOdd)
		case !withA && !withB:
			decided = append(decided, ConsensusAllDiffer)
		}
	}
	if len(decided) == 0 {
		return ""
	}
	first := decided[0]
	for _, v := range decided[1:] {
		if v != first {
			return ConsensusAllDiffer // third families disagree
		}
	}
	return first
}
