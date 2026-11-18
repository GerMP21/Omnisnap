package appearance

import (
	"image"

	"github.com/GerMP21/Omnisnap/internal/layoutdiff"
)

// Graphic is a matched graphics element drawn differently.
type Graphic struct {
	Node *layoutdiff.Node
	SSIM float64
}

// Detect finds graphics whose geometry matched but whose pixels differ beyond
// the threshold: an image swapped, an SVG fill, a canvas drawn differently.
// Elements with any geometric change are layout-diff's business; dynamic
// regions are ignored.
func Detect(A, B *layoutdiff.Tree, M *layoutdiff.Match, deltas map[int]layoutdiff.Delta, maskA, maskB map[int]bool, imgA, imgB image.Image) []Graphic {
	if imgA == nil || imgB == nil {
		return nil
	}
	var out []Graphic
	for _, a := range A.All {
		if !Graphics[a.N.T] || bool(a.N.Hid) || a.N.B[2]*a.N.B[3] == 0 {
			continue
		}
		b := M.PairOf[a.UID]
		if b == nil || bool(b.N.Hid) {
			continue
		}
		if d, ok := deltas[a.UID]; ok && d.Any {
			continue // geometry changed: layout-diff explains it
		}
		if maskA[a.UID] || maskB[b.UID] {
			continue
		}
		s, ok := Similarity(imgA, imgB, a.N.B, b.N.B)
		if !ok || s >= ssimThreshold {
			continue
		}
		out = append(out, Graphic{Node: a, SSIM: s})
	}
	return out
}
