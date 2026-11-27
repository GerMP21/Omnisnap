package capture

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"

	"github.com/GerMP21/Omnisnap/internal/schema"
)

// The not-painted rule (backlog B17; the rule and its numbers are the
// owner's, 2026-11-27, ZANZIBAR-56). On macOS runner image 20261117.0516.1,
// Safari 27.2 laid out every page but its screenshots held no text and no
// images, only box shadows and scrollbars; compared as they were, they made
// false "Drawn differently" causes. A browser that painted none of the text
// the DOM says is visible produced no evidence, so the capture fails.
//
// Measured on the release demo and self-test bundles: good captures have ink
// in 77-100% of their visible text boxes (a page with collapsed sections is
// the low end), the blank Safari ones in 0%.
const (
	// inkRange: a box has ink when its pixels span more than this many
	// levels (0-255) in some color channel. Box shadows and gradients stay
	// under it; text over its background does not.
	inkRange = 24
	// paintMinBoxes: a page with fewer visible text boxes is not judged.
	paintMinBoxes = 10
	// paintMinShare: under this share of inked boxes, nothing was painted.
	paintMinShare = 0.10
	// paintMinSide: smaller boxes (px) are too small to judge.
	paintMinSide = 4
)

// ErrUnpainted is a capture whose browser painted none of the page's text.
var ErrUnpainted = errors.New("the browser painted none of the page's text")

// checkPainted applies the not-painted rule to a screenshot (document
// coordinates, origin at the top left) and the tree extracted with it. The
// visible text boxes are the elements with own text, not hidden, at least
// paintMinSide px on each side and wholly inside the screenshot.
func checkPainted(shot []byte, root schema.Node) error {
	img, err := png.Decode(bytes.NewReader(shot))
	if err != nil {
		return fmt.Errorf("decode screenshot: %w", err)
	}
	bounds := img.Bounds()
	total, inked := 0, 0
	var walk func(n *schema.Node)
	walk = func(n *schema.Node) {
		if n.Text != nil && !bool(n.Hid) {
			if r, ok := textRect(n.B, bounds); ok {
				total++
				if hasInk(img, r) {
					inked++
				}
			}
		}
		for i := range n.K {
			walk(&n.K[i])
		}
	}
	walk(&root)
	if total >= paintMinBoxes && float64(inked) < paintMinShare*float64(total) {
		return fmt.Errorf("%w: %d of %d visible text boxes have ink in the screenshot", ErrUnpainted, inked, total)
	}
	return nil
}

// textRect is a node's border box as pixels, when it is large enough and
// wholly inside the screenshot.
func textRect(b [4]float64, bounds image.Rectangle) (image.Rectangle, bool) {
	x, y, w, h := b[0], b[1], b[2], b[3]
	if w < paintMinSide || h < paintMinSide || x < 0 || y < 0 {
		return image.Rectangle{}, false
	}
	r := image.Rect(int(x), int(y), int(x+w), int(y+h))
	if !r.In(bounds) {
		return image.Rectangle{}, false
	}
	return r, true
}

// hasInk reports whether the pixels in r span more than inkRange levels in
// some channel. It stops at the first pixel that settles it.
func hasInk(img image.Image, r image.Rectangle) bool {
	var lo, hi [3]uint32
	first := true
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			cr, cg, cb, _ := img.At(x, y).RGBA()
			c := [3]uint32{cr >> 8, cg >> 8, cb >> 8}
			if first {
				lo, hi, first = c, c, false
				continue
			}
			for i := range c {
				lo[i], hi[i] = min(lo[i], c[i]), max(hi[i], c[i])
				if hi[i]-lo[i] > inkRange {
					return true
				}
			}
		}
	}
	return false
}
