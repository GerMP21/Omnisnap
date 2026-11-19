package triage

import (
	"math"
	"strings"

	"github.com/GerMP21/Omnisnap/internal/layoutdiff"
	"github.com/GerMP21/Omnisnap/internal/schema"
)

const (
	schemaTierReport = schema.TierReport
	schemaTierInfo   = schema.TierInfo
)

// Hard consequences are always reported. Overflow is hard only when it is
// perceivable: content CLIPPED (overflow hidden/clip) or a SCROLLBAR
// (auto/scroll). Visible overflow that collides with nothing is a soft
// consequence.
var hard = map[string]bool{
	"presence": true, "visibility": true, "clipped": true,
	"scrollbar": true, "protrusion": true, "truncation": true,
}

// overflowOf is how far the content overflows its box on one axis.
func overflowOf(n *layoutdiff.Node, axis int) float64 {
	if n == nil || n.N.O == nil {
		return 0
	}
	return n.N.O[axis]
}

// modeOf is the overflow mode on one axis: "visible", "hidden", "clip",
// "auto", "scroll" (the capture carries them per axis, "x/y").
func modeOf(n *layoutdiff.Node, axis int) string {
	if n == nil || n.N.OF == "" {
		return "visible"
	}
	parts := strings.Split(n.N.OF, "/")
	if axis >= len(parts) || parts[axis] == "" {
		return "visible"
	}
	return parts[axis]
}

// consequences lists what a person can see change (Task 53).
func consequences(c layoutdiff.Cause, ctx Context) []string {
	var out []string
	seen := map[string]bool{}
	add := func(s string) {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	var a, b *layoutdiff.Node
	if c.Side == "A" {
		a, b = c.Node, c.Other
	} else {
		b = c.Node
	}
	kinds := c.Kinds
	d := layoutdiff.Delta{}
	if c.D != nil {
		d = *c.D
	}
	if hasPrefixAny(kinds, "only-in-") || contains(kinds, "children-differ") {
		add("presence")
	}
	if hasPrefixAny(kinds, "hidden-in-") {
		add("visibility")
	}
	if contains(kinds, "columns") {
		add("structural")
	}
	if contains(kinds, "text-shifted") {
		add("content-moved")
	}
	// A graphic drawn differently (appearance diff): visible content changed.
	if contains(kinds, "appearance") {
		add("appearance")
	}
	if hasPrefixAny(kinds, "intent-") {
		add("intent-broken")
	}

	overflow := overflowOf
	mode := modeOf
	for _, axis := range []string{"x", "y"} {
		if !contains(kinds, "overflow-"+axis) {
			continue
		}
		i := 0
		reach := float64(ctx.VWA)
		if axis == "y" {
			i = 1
			reach = 2000
		}
		if ctx.VWB > ctx.VWA && i == 0 {
			reach = float64(ctx.VWB)
		}
		sides := []*layoutdiff.Node{}
		if overflow(a, i) > 0 {
			sides = append(sides, a)
		}
		if overflow(b, i) > 0 {
			sides = append(sides, b)
		}
		clipped, scrollbar := false, false
		for _, n := range sides {
			m := mode(n, i)
			if (strings.Contains(m, "hidden") || strings.Contains(m, "clip")) && overflow(n, i) < reach {
				clipped = true
			} else if strings.Contains(m, "auto") || strings.Contains(m, "scroll") {
				scrollbar = true
			}
		}
		switch {
		case clipped:
			add("clipped")
		case scrollbar:
			add("scrollbar")
		default:
			add("overflow")
		}
	}
	if a != nil && b != nil {
		outA := a.N.B[0]+a.N.B[2] > float64(ctx.VWA)+1
		outB := b.N.B[0]+b.N.B[2] > float64(ctx.VWB)+1
		if outA != outB {
			add("protrusion")
		}
		if bool(a.N.TR) != bool(b.N.TR) {
			add("truncation")
		}
	}
	if d.DLN != 0 {
		add("line-count")
	}
	moved := func(n *layoutdiff.Node) bool {
		if n == nil || !bool(n.N.IA) || n.N.D == "inline" {
			return false
		}
		dd, ok := ctx.D[n.UID]
		if !ok {
			return false
		}
		return math.Max(math.Abs(dd.DX), math.Abs(dd.DY)) >= 4
	}
	if moved(a) {
		add("interactive-moved")
	}
	if a != nil && contains(kinds, "children-moved") {
		for _, k := range a.Kids {
			if moved(k) {
				add("interactive-moved")
				break
			}
		}
	}
	big := func(delta, size float64) bool {
		return math.Abs(delta) >= math.Max(8, 0.1*size)
	}
	if a != nil && (big(d.DW, a.N.B[2]) || big(d.DH, a.N.B[3]) || big(d.DX, a.N.B[2]) || big(d.DY, a.N.B[3])) {
		add("structural")
	}
	if a != nil && contains(kinds, "children-moved") {
		for _, k := range a.Kids {
			kd, ok := ctx.D[k.UID]
			if !ok || k.N.D == "inline" {
				continue
			}
			if big(kd.DX, k.N.B[2]) || big(kd.DY, k.N.B[3]) {
				add("structural")
				break
			}
		}
	}
	return out
}
