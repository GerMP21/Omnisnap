package layoutdiff

import (
	"fmt"
	"math"
	"net/url"
	"strings"

	"github.com/GerMP21/Omnisnap/internal/schema"
)

// Pair is everything one comparison needs beyond the captures: the two runs
// per engine chosen for comparison, the dynamic masks, and the other runs for
// the "page changes over time" warning.
type Pair struct {
	A, A2 *Tree
	B, B2 *Tree
	MaskA map[int]bool
	MaskB map[int]bool
	RunA  string
	RunB  string
}

// Warnings is the comparison's caveats and dynamic regions (the spike's
// diff.mjs "warnings" section).
type Warnings struct {
	List          []string
	Notes         []string
	DynamicA      []string // selectors, top 5
	DynamicB      []string
	DynamicBoxA   [][4]float64 // top 20
	DynamicBoxB   [][4]float64
	BlockedA      []string // hosts whose requests the baseline blocked
	BlockedB      []string
	StateGap      bool // the engines rendered different page states
	SecondRunGone bool
}

// WarningsOf builds the warnings for one comparison.
func WarningsOf(p Pair) Warnings {
	var w Warnings
	a, b := p.A, p.B
	if p.A2 == nil || p.B2 == nil {
		w.List = append(w.List, "second run missing: dynamic content is not masked")
		w.SecondRunGone = true
	}
	if a.Meta.CW != b.Meta.CW {
		w.List = append(w.List, fmt.Sprintf(
			"layout viewport differs: %s %dpx vs %s %dpx (scrollbars?) - widths everywhere will differ",
			a.Engine, a.Meta.CW, b.Engine, b.Meta.CW))
	}
	if a.Meta.Truncated || b.Meta.Truncated {
		w.List = append(w.List, "node cap hit: tree truncated")
	}
	for _, t := range []*Tree{a, b} {
		if math.Abs(float64(t.Meta.Post.DocH-t.Meta.DocH)) > 2 || t.Meta.Post.Nodes != t.Meta.Nodes {
			w.List = append(w.List, fmt.Sprintf(
				"%s: the page changed during capture (height %d -> %dpx, %d -> %d elements): screenshot and geometry may not match",
				t.Engine, t.Meta.DocH, t.Meta.Post.DocH, t.Meta.Nodes, t.Meta.Post.Nodes))
		}
	}
	w.DynamicA, w.DynamicBoxA = topMasked(a, p.MaskA)
	w.DynamicB, w.DynamicBoxB = topMasked(b, p.MaskB)

	bigGap := func(x, y int) bool {
		fx, fy := float64(x), float64(y)
		return math.Abs(fx-fy)/math.Max(fx, fy) > 0.02 && math.Abs(fx-fy) > 200
	}
	nodeGap := math.Abs(float64(a.Meta.Nodes-b.Meta.Nodes)) / math.Max(float64(a.Meta.Nodes), float64(b.Meta.Nodes))
	selfDrift := (p.A2 != nil && bigGap(a.Meta.DocH, p.A2.Meta.DocH)) ||
		(p.B2 != nil && bigGap(b.Meta.DocH, p.B2.Meta.DocH))
	if (nodeGap > 0.01 && absInt(a.Meta.Nodes-b.Meta.Nodes) > 10) || bigGap(a.Meta.DocH, b.Meta.DocH) {
		w.StateGap = true
		regions := append(append([]string{}, w.DynamicA...), w.DynamicB...)
		regions = unique(regions)
		if len(regions) > 4 {
			regions = regions[:4]
		}
		if selfDrift {
			aHeights := fmt.Sprintf("%d", a.Meta.DocH)
			if p.A2 != nil {
				aHeights += fmt.Sprintf("/%d", p.A2.Meta.DocH)
			}
			bHeights := fmt.Sprintf("%d", b.Meta.DocH)
			if p.B2 != nil {
				bHeights += fmt.Sprintf("/%d", p.B2.Meta.DocH)
			}
			see := strings.Join(regions, ", ")
			if see == "" {
				see = "see masked count"
			}
			w.List = append(w.List, fmt.Sprintf(
				"the page changes over time: two runs of the same engine differ (page height %s vs %spx). Changing regions are ignored: %s. Clock control would make them comparable.",
				aHeights, bHeights, see))
		} else {
			w.List = append(w.List, fmt.Sprintf(
				"engines rendered different page states: %d vs %d elements, page height %d vs %dpx (timers, carousels, A/B or UA-dependent content); findings may describe the state gap, not layout",
				a.Meta.Nodes, b.Meta.Nodes, a.Meta.DocH, b.Meta.DocH))
		}
	}
	if strings.Join(a.Meta.Fonts.Failed, ",") != strings.Join(b.Meta.Fonts.Failed, ",") {
		w.List = append(w.List, fmt.Sprintf("web fonts failed differently: %s [%s] vs %s [%s]",
			a.Engine, strings.Join(a.Meta.Fonts.Failed, ", "),
			b.Engine, strings.Join(b.Meta.Fonts.Failed, ", ")))
	}
	w.BlockedA = blockedHosts(a.Meta)
	w.BlockedB = blockedHosts(b.Meta)
	if len(w.BlockedB) > 0 && len(w.BlockedA) == 0 {
		hosts := w.BlockedB
		more := ""
		if len(hosts) > 8 {
			hosts, more = hosts[:8], ", ..."
		}
		w.List = append(w.List, fmt.Sprintf("%s blocked requests %s loaded: %s%s",
			b.Engine, a.Engine, strings.Join(hosts, ", "), more))
	}
	for _, t := range []*Tree{a, b} {
		if t.Meta.ShotNote != "" {
			w.Notes = append(w.Notes, t.Engine+": "+t.Meta.ShotNote)
		}
	}
	return w
}

// topMasked lists the topmost masked elements with a box: the regions to name
// and shade, never every child.
func topMasked(t *Tree, mask map[int]bool) ([]string, [][4]float64) {
	var selectors []string
	var boxes [][4]float64
	for _, n := range t.All {
		if !mask[n.UID] || (n.Parent != nil && mask[n.Parent.UID]) || n.N.B[2]*n.N.B[3] == 0 {
			continue
		}
		if len(selectors) < 5 {
			selectors = append(selectors, Selector(n))
		}
		if len(boxes) < 20 {
			boxes = append(boxes, n.N.B)
		}
	}
	return selectors, boxes
}

// blockedHosts lists distinct hosts whose requests the browser itself blocked
// (Brave Shields, Edge tracking prevention): the usual cause behind branded
// browser differences.
func blockedHosts(m schema.Meta) []string {
	var hosts []string
	for _, f := range m.Failed {
		if !strings.Contains(strings.ToUpper(f.Err), "BLOCKED_BY_CLIENT") {
			continue
		}
		host := f.URL
		if u, err := url.Parse(f.URL); err == nil && u.Host != "" {
			host = u.Host
		}
		hosts = append(hosts, host)
	}
	return unique(hosts)
}

func unique(ss []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range ss {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
