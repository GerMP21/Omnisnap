package layoutdiff

import (
	"math"
	"sort"
	"strings"
)

// Design intent (Task 46, SPEC-layout-diff.md concept 8): a relation between
// two elements (shared edge, equal size, equal gap, grid column, no overlap)
// is INTENDED when at least two engine families keep it within 1px. One engine
// breaking an intended relation clearly (>= 2px, or >= 8% for sizes) is a
// violation, whichever engine it is, Chrome included. Ported from
// lib/intent.mjs.
const (
	intentHold     = 1.0  // px: a relation holds in an engine
	intentBreak    = 2.0  // px: a relation is clearly broken
	intentRel      = 0.08 // Weber fraction for sizes
	intentMaxKids  = 60
	intentWindow   = 6 // compare each child with its next 6 siblings in big groups
	intentMaxMag   = 800
	intentMinMatch = 0.95
)

// Third is another engine family's tree and its match to A.
type Third struct {
	Engine string
	T      *Tree
	M      *Match
}

// Violation is one intended relation an engine breaks.
type Violation struct {
	Engine  string // "A" or "B" (a third engine's violations are its own pair's)
	Node    *Node
	Kinds   []string
	Mag     float64
	HoldsIn []string
	Others  int
}

// IntentResult is what the detector found.
type IntentResult struct {
	Violations []Violation
	Relations  int
	Reason     string
}

type engineBox struct {
	key  string
	name string
	box  func(a *Node) *[4]float64
}

type relation struct {
	kind    string
	value   func(a, b [4]float64) float64
	size    func(a [4]float64) float64
	overlap bool
}

func visibleIntent(n *Node) bool {
	return n != nil && !bool(n.N.Hid) && n.N.D != "inline" && n.N.B[2] >= 4 && n.N.B[3] >= 4
}

func intentSig(n *Node) string {
	classes := strings.Fields(n.N.C)
	sort.Strings(classes)
	if len(classes) > 3 {
		classes = classes[:3]
	}
	return n.N.T + "." + strings.Join(classes, ".")
}

func edgeLeft(b [4]float64) float64   { return b[0] }
func edgeRight(b [4]float64) float64  { return b[0] + b[2] }
func edgeTop(b [4]float64) float64    { return b[1] }
func edgeBottom(b [4]float64) float64 { return b[1] + b[3] }
func centerX(b [4]float64) float64    { return b[0] + b[2]/2 }
func centerY(b [4]float64) float64    { return b[1] + b[3]/2 }

func overlapAmount(p, q [4]float64) float64 {
	return math.Min(
		math.Min(p[0]+p[2], q[0]+q[2])-math.Max(p[0], q[0]),
		math.Min(p[1]+p[3], q[1]+q[3])-math.Max(p[1], q[1]))
}

// intentRelations lists the relations two boxes keep as named measurements. A
// relation holds when its measurement is ~0 (no-overlap: when they do not
// overlap). Equal sizes only between repeated components; equal HEIGHT only
// side by side, never down a column.
func intentRelations(x, y *Node, sameKind bool) []relation {
	edges := []struct {
		kind string
		f    func([4]float64) float64
	}{
		{"align-left", edgeLeft}, {"align-right", edgeRight}, {"align-top", edgeTop},
		{"align-bottom", edgeBottom}, {"align-center-x", centerX}, {"align-center-y", centerY},
	}
	var out []relation
	for _, e := range edges {
		f := e.f
		out = append(out, relation{kind: e.kind, value: func(a, b [4]float64) float64 { return f(a) - f(b) }})
	}
	if sameKind {
		out = append(out, relation{kind: "equal-width",
			value: func(a, b [4]float64) float64 { return a[2] - b[2] },
			size:  func(a [4]float64) float64 { return a[2] }})
		sideBySide := math.Min(x.N.B[1]+x.N.B[3], y.N.B[1]+y.N.B[3]) > math.Max(x.N.B[1], y.N.B[1])
		if sideBySide {
			out = append(out, relation{kind: "equal-height",
				value: func(a, b [4]float64) float64 { return a[3] - b[3] },
				size:  func(a [4]float64) float64 { return a[3] }})
		}
	}
	out = append(out, relation{kind: "no-overlap", overlap: true,
		value: func(a, b [4]float64) float64 { return math.Max(0, overlapAmount(a, b)) }})
	return out
}

// checkPairAt measures a pair of elements in every engine and reports the
// relations one engine breaks while at least two keep them.
func checkPairAt(x, y *Node, engines []engineBox, rels []relation, add func(v Violation, vx, vy *Node)) {
	boxes := make([][2]*[4]float64, len(engines))
	for e, eng := range engines {
		p, q := eng.box(x), eng.box(y)
		if p == nil || q == nil {
			return
		}
		boxes[e] = [2]*[4]float64{p, q}
	}
	for _, r := range rels {
		v := make([]float64, len(engines))
		holds := make([]bool, len(engines))
		nHold := 0
		for e := range engines {
			v[e] = r.value(*boxes[e][0], *boxes[e][1])
			if r.overlap {
				holds[e] = v[e] <= intentHold
			} else {
				holds[e] = math.Abs(v[e]) <= intentHold
			}
			if holds[e] {
				nHold++
			}
		}
		if nHold < 2 || nHold == len(engines) {
			continue
		}
		for e := range engines {
			if holds[e] {
				continue
			}
			mag := math.Abs(v[e])
			need := intentBreak
			if !r.overlap && r.size != nil {
				need = math.Max(intentBreak, intentRel*r.size(*boxes[e][0]))
			}
			if mag < need || mag > intentMaxMag {
				continue
			}
			if r.overlap && math.Min(math.Min(boxes[e][0][2], boxes[e][0][3]), math.Min(boxes[e][1][2], boxes[e][1][3])) < 4 {
				continue
			}
			var holdsIn []string
			for k, h := range holds {
				if h {
					holdsIn = append(holdsIn, engines[k].name)
				}
			}
			add(Violation{Engine: engines[e].key, Kinds: []string{r.kind}, Mag: mag, HoldsIn: holdsIn}, x, y)
		}
	}
}

// IntentViolations runs the detector. M is the A-B match; thirds must contain a
// different engine family (intent needs a third opinion).
func IntentViolations(A, B *Tree, M *Match, thirds []Third, maskA, maskB map[int]bool) IntentResult {
	if len(thirds) == 0 {
		return IntentResult{Reason: "needs a third engine family"}
	}
	low := len(M.PairOf) < int(math.Ceil(intentMinMatch*float64(len(A.All))))
	for _, t := range thirds {
		if low {
			break
		}
		low = len(t.M.PairOf) < int(math.Ceil(intentMinMatch*float64(len(A.All))))
	}
	if low {
		return IntentResult{Reason: "match rate below 95%: page states differ"}
	}
	nodeIn := []func(*Node) *Node{
		func(a *Node) *Node { return a },
		func(a *Node) *Node {
			b := M.PairOf[a.UID]
			if visibleIntent(b) && !maskB[b.UID] {
				return b
			}
			return nil
		},
	}
	for _, t := range thirds {
		tt := t
		nodeIn = append(nodeIn, func(a *Node) *Node {
			n := tt.M.PairOf[a.UID]
			if visibleIntent(n) {
				return n
			}
			return nil
		})
	}
	engines := []engineBox{
		{key: "A", name: A.Engine, box: func(a *Node) *[4]float64 { b := a.N.B; return &b }},
		{key: "B", name: B.Engine, box: func(a *Node) *[4]float64 {
			n := nodeIn[1](a)
			if n == nil {
				return nil
			}
			b := n.N.B
			return &b
		}},
	}
	for i, t := range thirds {
		i := i
		engines = append(engines, engineBox{key: "T", name: t.Engine, box: func(a *Node) *[4]float64 {
			n := nodeIn[2+i](a)
			if n == nil {
				return nil
			}
			b := n.N.B
			return &b
		}})
	}
	textEngines := make([]engineBox, len(engines))
	for i, e := range engines {
		i := i
		base := e
		textEngines[i] = engineBox{key: base.key, name: base.name, box: func(a *Node) *[4]float64 {
			n := nodeIn[i](a)
			if n == nil || n.TI() == nil || n.TX() == "" {
				return nil
			}
			b := [4]float64{n.N.B[0] + *n.TI(), n.N.B[1], 1, n.N.B[3]}
			return &b
		}}
	}
	ok := func(n *Node) bool { return visibleIntent(n) && !maskA[n.UID] }
	type rawViolation struct {
		v         Violation
		x, y      *Node
		blame     *Node
		holdsList []string
	}
	var raw []rawViolation
	add := func(v Violation, x, y *Node) { raw = append(raw, rawViolation{v: v, x: x, y: y}) }
	checked := 0
	for _, P := range A.All {
		var kids []*Node
		for _, k := range P.Kids {
			if ok(k) && len(kids) < intentMaxKids {
				kids = append(kids, k)
			}
		}
		if len(kids) < 2 {
			continue
		}
		// Siblings.
		for i := range kids {
			lim := len(kids)
			if len(kids) > 24 {
				lim = min(len(kids), i+1+intentWindow)
			}
			for j := i + 1; j < lim; j++ {
				checked++
				checkPairAt(kids[i], kids[j], engines,
					intentRelations(kids[i], kids[j], intentSig(kids[i]) == intentSig(kids[j])), add)
			}
		}
		// Rhythm: equal gaps between consecutive siblings (rows and columns).
		for i := 0; i+2 < len(kids); i++ {
			a, b, c := kids[i], kids[i+1], kids[i+2]
			row := math.Abs(a.N.B[1]-b.N.B[1]) <= intentHold && math.Abs(b.N.B[1]-c.N.B[1]) <= intentHold
			col := math.Abs(a.N.B[0]-b.N.B[0]) <= intentHold && math.Abs(b.N.B[0]-c.N.B[0]) <= intentHold
			if !row && !col {
				continue
			}
			boxes := make([][3]*[4]float64, len(engines))
			bad := false
			for e, eng := range engines {
				p, q, r := eng.box(a), eng.box(b), eng.box(c)
				boxes[e] = [3]*[4]float64{p, q, r}
				if p == nil || q == nil || r == nil {
					bad = true
					break
				}
			}
			if bad {
				continue
			}
			checked++
			v := make([]float64, len(engines))
			holds := make([]bool, len(engines))
			nHold := 0
			for e := range engines {
				p, q, r := *boxes[e][0], *boxes[e][1], *boxes[e][2]
				gapPQ := gapBox(p, q, row)
				gapQR := gapBox(q, r, row)
				v[e] = gapPQ - gapQR
				holds[e] = math.Abs(v[e]) <= intentHold
				if holds[e] {
					nHold++
				}
			}
			if nHold < 2 || nHold == len(engines) {
				continue
			}
			for e := range engines {
				if holds[e] || math.Abs(v[e]) < intentBreak || math.Abs(v[e]) > intentMaxMag {
					continue
				}
				// Blame the element whose gap differs from the consensus gap.
				var cons float64
				for k, h := range holds {
					if h {
						p, q := *boxes[k][0], *boxes[k][1]
						cons = gapBox(p, q, row)
						break
					}
				}
				p, q := *boxes[e][0], *boxes[e][1]
				culprit := b
				if math.Abs(gapBox(p, q, row)-cons) <= intentHold {
					culprit = c
				}
				vx := culprit
				vy := a
				if culprit == b {
					vy = a
				} else {
					vy = b
				}
				var holdsIn []string
				for k, h := range holds {
					if h {
						holdsIn = append(holdsIn, engines[k].name)
					}
				}
				kind := "equal-gap-y"
				if row {
					kind = "equal-gap-x"
				}
				raw = append(raw, rawViolation{
					v:     Violation{Engine: engines[e].key, Kinds: []string{kind}, Mag: math.Abs(v[e]), HoldsIn: holdsIn},
					x:     vx,
					y:     vy,
					blame: culprit,
				})
			}
		}
		// Repeated components: corresponding parts keep their edges aligned.
		bySig := map[string][]*Node{}
		var sigOrder []string
		for _, k := range kids {
			g := intentSig(k)
			if _, ok := bySig[g]; !ok {
				sigOrder = append(sigOrder, g)
			}
			bySig[g] = append(bySig[g], k)
		}
		for _, g := range sigOrder {
			reps := bySig[g]
			if len(reps) < 2 {
				continue
			}
			partsOf := make([][]*Node, len(reps))
			for i, r := range reps {
				var ps []*Node
				for _, k := range r.Kids {
					if ok(k) {
						ps = append(ps, k)
					}
				}
				partsOf[i] = ps
			}
			for i := 0; i+1 < len(reps); i++ {
				for j := i + 1; j < min(len(reps), i+1+intentWindow); j++ {
					pi, pj := partsOf[i], partsOf[j]
					if len(pi) < 1 || len(pi) != len(pj) {
						continue
					}
					differ := false
					for c := range pi {
						if intentSig(pi[c]) != intentSig(pj[c]) {
							differ = true
							break
						}
					}
					if differ {
						continue
					}
					sideBySide := math.Min(reps[i].N.B[1]+reps[i].N.B[3], reps[j].N.B[1]+reps[j].N.B[3]) > math.Max(reps[i].N.B[1], reps[j].N.B[1])
					edges := []string{"left", "right"}
					if sideBySide {
						edges = []string{"top", "bottom"}
					}
					var rels []relation
					for _, e := range edges {
						f := map[string]func([4]float64) float64{"left": edgeLeft, "right": edgeRight, "top": edgeTop, "bottom": edgeBottom}[e]
						kind := "column-" + e
						if sideBySide {
							kind = "repeat-" + e
						}
						rels = append(rels, relation{kind: kind, value: func(a, b [4]float64) float64 { return f(a) - f(b) }})
					}
					for c := range pi {
						checked++
						checkPairAt(pi[c], pj[c], engines, rels, add)
						if !sideBySide {
							checkPairAt(pi[c], pj[c], textEngines, []relation{{kind: "column-text",
								value: func(a, b [4]float64) float64 { return a[0] - b[0] }}}, add)
						}
					}
				}
			}
		}
	}

	// Blame: per broken relation, the element that MOVED (relative to its
	// parent) most between the violating engine and one that keeps it; ties go
	// to the element breaking more relations.
	count := map[string]int{}
	for _, rv := range raw {
		count[fmtKey(rv.v.Engine, rv.x.UID)]++
		count[fmtKey(rv.v.Engine, rv.y.UID)]++
	}
	relBox := func(n *Node, engine int) *[4]float64 {
		b := engines[engine].box(n)
		if b == nil {
			return nil
		}
		var p *[4]float64
		if n.Parent != nil {
			p = engines[engine].box(n.Parent)
		}
		if p == nil {
			return b
		}
		out := [4]float64{b[0] - p[0], b[1] - p[1], b[2], b[3]}
		return &out
	}
	movedAmount := func(n *Node, engine, ref int) float64 {
		x, y := relBox(n, engine), relBox(n, ref)
		if x == nil || y == nil {
			return 0
		}
		sum := 0.0
		for i := range 4 {
			sum += math.Abs(x[i] - y[i])
		}
		return sum
	}
	type perKey struct {
		engine  int
		node    *Node
		kinds   map[string]bool
		mag     float64
		holdsIn []string
		with    map[int]bool
	}
	per := map[string]*perKey{}
	var perOrder []string
	for _, rv := range raw {
		// The reference is an engine that keeps the relation: the box the
		// violator moved away from.
		engineIdx, ref := 0, -1
		for i := range engines {
			if engines[i].key == rv.v.Engine {
				engineIdx = i
			}
		}
		for _, name := range rv.v.HoldsIn {
			for i := range engines {
				if engines[i].name == name {
					ref = i
					break
				}
			}
			if ref >= 0 {
				break
			}
		}
		if ref < 0 || ref == engineIdx {
			continue
		}
		mx := movedAmount(rv.x, engineIdx, ref)
		my := movedAmount(rv.y, engineIdx, ref)
		culprit := rv.blame
		if culprit == nil {
			if math.Abs(mx-my) > intentHold {
				if mx > my {
					culprit = rv.x
				} else {
					culprit = rv.y
				}
			} else if count[fmtKey(rv.v.Engine, rv.x.UID)] >= count[fmtKey(rv.v.Engine, rv.y.UID)] {
				culprit = rv.x
			} else {
				culprit = rv.y
			}
		}
		key := fmtKey(rv.v.Engine, culprit.UID)
		entry := per[key]
		if entry == nil {
			entry = &perKey{engine: engineIdx, node: culprit, kinds: map[string]bool{}, holdsIn: rv.v.HoldsIn, with: map[int]bool{}}
			per[key] = entry
			perOrder = append(perOrder, key)
		}
		for _, k := range rv.v.Kinds {
			entry.kinds[k] = true
		}
		entry.mag = math.Max(entry.mag, rv.v.Mag)
		if rv.x == culprit {
			entry.with[rv.y.UID] = true
		} else {
			entry.with[rv.x.UID] = true
		}
	}
	var out []Violation
	for _, key := range perOrder {
		e := per[key]
		if engines[e.engine].key == "T" {
			continue // a third engine's violations are reported in its own pair
		}
		var kinds []string
		for k := range e.kinds {
			kinds = append(kinds, k)
		}
		sort.Strings(kinds)
		out = append(out, Violation{
			Engine: engines[e.engine].key, Node: e.node, Kinds: kinds,
			Mag: math.Round(e.mag*10) / 10, HoldsIn: e.holdsIn, Others: len(e.with),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Others != out[j].Others {
			return out[i].Others > out[j].Others
		}
		return out[i].Mag > out[j].Mag
	})
	return IntentResult{Violations: out, Relations: checked}
}

func gapBox(p, q [4]float64, row bool) float64 {
	if row {
		return q[0] - (p[0] + p[2])
	}
	return q[1] - (p[1] + p[3])
}

func fmtKey(engine string, uid int) string {
	return engine + "|" + itoa(uid)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := "0123456789"
	var b []byte
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		b = append([]byte{digits[n%10]}, b...)
		n /= 10
	}
	if neg {
		return "-" + string(b)
	}
	return string(b)
}

// IntentKind maps relation kinds to the cause kind (Task 48).
func IntentKind(kinds []string) string {
	has := func(p string) bool {
		for _, k := range kinds {
			if strings.HasPrefix(k, p) {
				return true
			}
		}
		return false
	}
	switch {
	case has("no-overlap"):
		return "intent-overlap"
	case has("equal-width") || has("equal-height"):
		return "intent-size"
	case has("column-") || has("repeat-"):
		return "intent-column"
	case has("equal-gap"):
		return "intent-gap"
	}
	return "intent-align"
}

// IntentText is the human phrase per relation kind.
var IntentText = map[string]string{
	"align-left": "left edge", "align-right": "right edge", "align-top": "top edge",
	"align-bottom": "bottom edge", "align-center-x": "horizontal centre", "align-center-y": "vertical centre",
	"equal-width": "equal width", "equal-height": "equal height", "no-overlap": "no overlap",
	"repeat-top": "shared top line", "repeat-bottom": "shared bottom line",
	"equal-gap-x": "equal horizontal gap", "equal-gap-y": "equal vertical gap",
	"column-left": "grid column (left)", "column-text": "text column", "column-right": "grid column (right)",
}
