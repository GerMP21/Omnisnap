package triage

import (
	"strings"

	"github.com/GerMP21/Omnisnap/internal/layoutdiff"
	"github.com/GerMP21/Omnisnap/internal/schema"
)

// tier.go: the Report/Info decision (Task 53).
//
// The rule is the spec's (SPEC-triage.md §3): Report = a hard consequence, or
// a consequence the knowledge base does not explain; Info = the rest. The
// tree model's score never sets the tier.
//
// Two exceptions, and they are rules rather than scores.
//
// Deliberate hiding: a
// hidden or clipped box whose content is pushed a whole viewport beyond it is
// not cutting anything off — it is the image-replacement pattern (the text
// that is moved out of sight on purpose, `text-indent: 100%`, `-9999px`). Such
// a cause is Info: nothing a person can see changed.
//
// Line-box metrics on a media box: a difference whose only kind and only
// consequence are appearance, on an image or svg, and which the line-height
// probe removes, is how blink and firefox lay out the strut around an inline
// image. The crawl measured it on 249 of 457 real sites, 17.7 causes per page,
// 4414 of 6142 Report causes — a person sees nothing. Info.

// deliberateHiding reports that the cause's overflow is content pushed out of
// sight on purpose rather than content cut off.
//
// It is the same threshold the consequence rule uses (SPEC-triage.md §2):
// below the reach the content is clipped (a hard consequence), at or beyond
// it the content is out of sight on purpose. The reach is the viewport width
// on the x axis and 2000 px on the y axis, the spike's approximation of "much
// more than a viewport" (the capture carries no viewport height).
func deliberateHiding(c layoutdiff.Cause, ctx Context) bool {
	var a, b *layoutdiff.Node
	if c.Side == "A" {
		a, b = c.Node, c.Other
	} else {
		b = c.Node
	}
	for _, axis := range []string{"x", "y"} {
		if !contains(c.Kinds, "overflow-"+axis) {
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
		for _, n := range []*layoutdiff.Node{a, b} {
			if overflowOf(n, i) < reach {
				continue
			}
			m := modeOf(n, i)
			if strings.Contains(m, "hidden") || strings.Contains(m, "clip") {
				return true
			}
		}
	}
	return false
}

// onlyOverflow reports that the cause has exactly one consequence and it is
// the soft overflow: nothing else changed, so there is nothing to report
// beyond the hiding itself.
func onlyOverflow(cons []string) bool {
	return len(cons) == 1 && cons[0] == "overflow"
}

// lineHeightOnMedia reports that the engine's own line-box metrics explain a
// media box's appearance: the only kind and consequence are appearance, the
// element is an image or svg, and the line-height probe removes the difference.
func lineHeightOnMedia(c layoutdiff.Cause, cons []string, mechanism string) bool {
	if mechanism != "lh" {
		return false
	}
	if len(cons) != 1 || cons[0] != "appearance" {
		return false
	}
	if len(c.Kinds) != 1 || c.Kinds[0] != "appearance" {
		return false
	}
	return c.Node != nil && c.Node.N != nil && roleOf(c.Node.N.T, "") == "media"
}

// tierOf decides Report or Info and the ranking weight (Task 53). mechanism is
// the diagnosis evidence.Mechanism reached for this cause ("" when there is
// none); only the demotion rules read it.
func tierOf(c layoutdiff.Cause, kb *KB, cons []string, mechanism string, ctx Context) Assessment {
	hardHit := false
	for _, k := range cons {
		if hard[k] {
			hardHit = true
			break
		}
	}
	hiding, demoted := false, false
	var tier schema.Tier
	switch {
	case hardHit:
		// A hard consequence is never demoted (SPEC-triage.md, Boundaries).
		tier = schemaTierReport
	case kb != nil && kb.Expected:
		tier = schemaTierInfo
	case lineHeightOnMedia(c, cons, mechanism):
		tier = schemaTierInfo
		demoted = true
	case onlyOverflow(cons) && deliberateHiding(c, ctx):
		tier = schemaTierInfo
		hiding = true
	case len(cons) > 0:
		tier = schemaTierReport
	default:
		tier = schemaTierInfo
	}
	weight := 1.0
	if hardHit {
		weight = 3
	} else if len(cons) > 0 {
		weight = 2
	}
	if kb != nil && kb.Expected {
		weight *= 0.5
	}
	if hiding || demoted {
		weight = 1
	}
	y := c.Node.N.B[1]
	if c.Side == "B" && c.Other != nil {
		y = c.Other.N.B[1]
	}
	visibility := 0.4
	if y < 800 {
		visibility = 1
	} else if y < 2400 {
		visibility = 0.7
	}
	return Assessment{KB: kb, Consequences: cons, Tier: tier, Weight: weight, Visibility: visibility, Hiding: hiding}
}
