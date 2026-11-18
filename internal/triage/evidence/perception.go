package evidence

import (
	"math"

	"github.com/GerMP21/Omnisnap/internal/layoutdiff"
)

// Perception (Task 55): model what the eye notices (psychophysics and
// Gestalt), comparing qualitative relations between neighbouring boxes instead
// of raw pixel deltas. Inline boxes are skipped: they flow with text.
// Ported from lib/perception.mjs.

// EventWeight ranks the events.
var EventWeight = map[string]float64{
	"overlap-created": 3, "alignment-broken": 2.5, "order-changed": 2.5,
	"protrudes-parent": 2, "spacing-changed": 1, "size-noticeable": 1,
}

// Event is one perceptual change.
type Event struct {
	Type string  `json:"type"`
	Edge string  `json:"edge,omitempty"`
	Axis string  `json:"axis,omitempty"`
	From float64 `json:"from,omitempty"`
	To   float64 `json:"to,omitempty"`
}

// Perception is the cause's perceptual evidence.
type Perception struct {
	Events []Event  `json:"events"`
	Score  *float64 `json:"score"`
}

func visibleP(n *layoutdiff.Node) bool {
	return n != nil && !bool(n.N.Hid) && n.N.B[2] > 0 && n.N.B[3] > 0 && n.N.D != "inline"
}

func inter(p, q [4]float64) float64 {
	return math.Max(0, math.Min(p[0]+p[2], q[0]+q[2])-math.Max(p[0], q[0])) *
		math.Max(0, math.Min(p[1]+p[3], q[1]+q[3])-math.Max(p[1], q[1]))
}

func gapOf(p, q [4]float64) float64 {
	dx := math.Max(0, math.Max(p[0], q[0])-math.Min(p[0]+p[2], q[0]+q[2]))
	dy := math.Max(0, math.Max(p[1], q[1])-math.Min(p[1]+p[3], q[1]+q[3]))
	return math.Hypot(dx, dy)
}

func edges() []struct {
	name string
	f    func([4]float64) float64
} {
	return []struct {
		name string
		f    func([4]float64) float64
	}{
		{"left", func(b [4]float64) float64 { return b[0] }},
		{"right", func(b [4]float64) float64 { return b[0] + b[2] }},
		{"top", func(b [4]float64) float64 { return b[1] }},
		{"bottom", func(b [4]float64) float64 { return b[1] + b[3] }},
		{"centerX", func(b [4]float64) float64 { return b[0] + b[2]/2 }},
		{"centerY", func(b [4]float64) float64 { return b[1] + b[3]/2 }},
	}
}

// pairEvents lists the relation changes between two element pairs.
func pairEvents(x, xb, y, yb *layoutdiff.Node) []Event {
	var ev []Event
	for _, e := range edges() {
		if math.Abs(e.f(x.N.B)-e.f(y.N.B)) <= 0.5 && math.Abs(e.f(xb.N.B)-e.f(yb.N.B)) >= 1.5 {
			ev = append(ev, Event{Type: "alignment-broken", Edge: e.name})
		}
	}
	minArea := math.Min(x.N.B[2]*x.N.B[3], y.N.B[2]*y.N.B[3])
	if inter(x.N.B, y.N.B) <= 1 && inter(xb.N.B, yb.N.B) >= math.Max(4, 0.01*minArea) {
		ev = append(ev, Event{Type: "overlap-created"})
	}
	sameRowA := inter([4]float64{0, x.N.B[1], 1, x.N.B[3]}, [4]float64{0, y.N.B[1], 1, y.N.B[3]}) > 0
	sameColA := inter([4]float64{x.N.B[0], 0, x.N.B[2], 1}, [4]float64{y.N.B[0], 0, y.N.B[2], 1}) > 0
	cx := func(b [4]float64) float64 { return b[0] + b[2]/2 }
	cy := func(b [4]float64) float64 { return b[1] + b[3]/2 }
	if sameRowA && math.Abs(cx(x.N.B)-cx(y.N.B)) > 2 && math.Abs(cx(xb.N.B)-cx(yb.N.B)) > 2 &&
		sign(cx(x.N.B)-cx(y.N.B)) != sign(cx(xb.N.B)-cx(yb.N.B)) {
		ev = append(ev, Event{Type: "order-changed", Axis: "x"})
	}
	if sameColA && math.Abs(cy(x.N.B)-cy(y.N.B)) > 2 && math.Abs(cy(xb.N.B)-cy(yb.N.B)) > 2 &&
		sign(cy(x.N.B)-cy(y.N.B)) != sign(cy(xb.N.B)-cy(yb.N.B)) {
		ev = append(ev, Event{Type: "order-changed", Axis: "y"})
	}
	gA, gB := gapOf(x.N.B, y.N.B), gapOf(xb.N.B, yb.N.B)
	if math.Abs(gB-gA) >= 4 && math.Max(gA, gB)/math.Max(1, math.Min(gA, gB)) >= 1.5 && math.Min(gA, gB) < 64 {
		ev = append(ev, Event{Type: "spacing-changed", From: math.Round(gA), To: math.Round(gB)})
	}
	return ev
}

func sign(f float64) int {
	switch {
	case f > 0:
		return 1
	case f < 0:
		return -1
	}
	return 0
}

func inside(c, p [4]float64) bool {
	return c[0] >= p[0]-1 && c[1] >= p[1]-1 && c[0]+c[2] <= p[0]+p[2]+1 && c[1]+c[3] <= p[1]+p[3]+1
}

// Events computes the perceptual evidence of one cause.
func Events(c layoutdiff.Cause, M *layoutdiff.Match) Perception {
	var a, b *layoutdiff.Node
	if c.Side == "A" {
		a, b = c.Node, c.Other
	} else {
		b = c.Node
	}
	if a == nil || b == nil {
		return Perception{Events: []Event{}} // presence: nothing to perceive geometrically
	}
	var events []Event
	seen := map[string]bool{}
	add := func(e Event) {
		key := e.Type + "|" + e.Edge + e.Axis
		if !seen[key] {
			seen[key] = true
			events = append(events, e)
		}
	}
	// 1. The cause against its siblings.
	if visibleP(a) && a.Parent != nil {
		for i, s := range a.Parent.Kids {
			if i >= 80 {
				break
			}
			sb := M.PairOf[s.UID]
			if s == a || !visibleP(s) || !visibleP(sb) {
				continue
			}
			for _, e := range pairEvents(a, b, s, sb) {
				add(e)
			}
		}
		if pb := M.PairOf[a.Parent.UID]; pb != nil && visibleP(a.Parent) && protrudes(a, a.Parent, b, pb) {
			add(Event{Type: "protrudes-parent"})
		}
	}
	// 2. For children causes, relations among its children.
	childrenKind := false
	for _, k := range c.Kinds {
		if len(k) >= 8 && k[:8] == "children" {
			childrenKind = true
		}
	}
	if childrenKind {
		var kids []*layoutdiff.Node
		for _, k := range a.Kids {
			if len(kids) >= 40 {
				break
			}
			if visibleP(k) && visibleP(M.PairOf[k.UID]) {
				kids = append(kids, k)
			}
		}
		for i := range kids {
			for j := i + 1; j < len(kids); j++ {
				for _, e := range pairEvents(kids[i], M.PairOf[kids[i].UID], kids[j], M.PairOf[kids[j].UID]) {
					add(e)
				}
			}
		}
	}
	// 3. Weber: is the size change itself noticeable?
	d := layoutdiff.Delta{}
	if c.D != nil {
		d = *c.D
	}
	if (math.Abs(d.DW) >= 2 && math.Abs(d.DW)/math.Max(1, a.N.B[2]) >= 0.08) ||
		(math.Abs(d.DH) >= 2 && math.Abs(d.DH)/math.Max(1, a.N.B[3]) >= 0.08) {
		add(Event{Type: "size-noticeable"})
	}
	score := 0.0
	for _, e := range events {
		score = math.Max(score, EventWeight[e.Type])
	}
	if events == nil {
		events = []Event{}
	}
	return Perception{Events: events, Score: &score}
}

func protrudes(child, parent, childB, parentB *layoutdiff.Node) bool {
	if inside(child.N.B, parent.N.B) && !inside(childB.N.B, parentB.N.B) {
		return math.Max(parentB.N.B[0]-childB.N.B[0],
			math.Max(childB.N.B[0]+childB.N.B[2]-parentB.N.B[0]-parentB.N.B[2],
				childB.N.B[1]+childB.N.B[3]-parentB.N.B[1]-parentB.N.B[3])) >= 2
	}
	return false
}
