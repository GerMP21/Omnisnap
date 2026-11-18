package appearance

import (
	"image"
	"math"
)

// SSIM (Task 50): compare the luma of two registered crops with a ±3 px
// offset search, so a one-pixel registration error does not read as a drawn
// difference. A global SSIM is enough here: the crops are single elements, not
// photographs, and the threshold only decides whether an image/SVG/canvas is
// drawn differently.

// maxOffset is the search radius in pixels.
const maxOffset = 3

// ssimThreshold: below this a matched graphic is drawn differently (initial
// value from the spec; changing it needs approval).
const ssimThreshold = 0.90

// grayCrop samples a box at full resolution.
func grayCrop(img image.Image, box [4]float64) ([]float64, int, int, bool) {
	w := int(math.Round(box[2]))
	h := int(math.Round(box[3]))
	if w < 2 || h < 2 {
		return nil, 0, 0, false
	}
	b := img.Bounds()
	out := make([]float64, w*h)
	x0, y0 := int(math.Round(box[0])), int(math.Round(box[1]))
	for r := range h {
		for c := range w {
			x, y := x0+c, y0+r
			if x < b.Min.X || y < b.Min.Y || x >= b.Max.X || y >= b.Max.Y {
				return nil, 0, 0, false // outside the screenshot: skip the element
			}
			out[r*w+c] = luma(img.At(x, y))
		}
	}
	return out, w, h, true
}

// ssimOf computes the global SSIM of two equal-sized windows.
func ssimOf(a, b []float64) float64 {
	n := float64(len(a))
	if n == 0 || len(b) != len(a) {
		return 0
	}
	var ma, mb float64
	for i := range a {
		ma += a[i]
		mb += b[i]
	}
	ma /= n
	mb /= n
	var va, vb, cov float64
	for i := range a {
		da, db := a[i]-ma, b[i]-mb
		va += da * da
		vb += db * db
		cov += da * db
	}
	va /= n
	vb /= n
	cov /= n
	const c1 = 6.5025  // (0.01 * 255)^2
	const c2 = 58.5225 // (0.03 * 255)^2
	return ((2*ma*mb + c1) * (2*cov + c2)) / ((ma*ma + mb*mb + c1) * (va + vb + c2))
}

// Similarity is the best SSIM between two registered crops over the offset
// search. ok is false when a crop is degenerate or outside the screenshot.
func Similarity(imgA, imgB image.Image, boxA, boxB [4]float64) (float64, bool) {
	if imgA == nil || imgB == nil {
		return 0, false
	}
	// The window is inset by the search radius so every shifted window stays
	// inside the screenshots.
	w := int(math.Min(math.Round(boxA[2]), math.Round(boxB[2]))) - 2*maxOffset
	h := int(math.Min(math.Round(boxA[3]), math.Round(boxB[3]))) - 2*maxOffset
	if w < 2 || h < 2 {
		return 0, false
	}
	best := 0.0
	found := false
	for dy := -maxOffset; dy <= maxOffset; dy++ {
		for dx := -maxOffset; dx <= maxOffset; dx++ {
			ga, _, _, okA := grayCrop(imgA, [4]float64{boxA[0] + maxOffset, boxA[1] + maxOffset, float64(w), float64(h)})
			if !okA {
				continue
			}
			gb, _, _, okB := grayCrop(imgB, [4]float64{
				boxB[0] + maxOffset + float64(dx), boxB[1] + maxOffset + float64(dy), float64(w), float64(h)})
			if !okB {
				continue
			}
			if s := ssimOf(ga, gb); !found || s > best {
				best, found = s, true
			}
		}
	}
	if !found {
		return 0, false
	}
	return best, true
}

// Graphics are the tags whose drawn content geometry cannot see.
var Graphics = map[string]bool{
	"img": true, "svg": true, "canvas": true, "video": true, "picture": true, "iframe": true,
}

// appearanceTags returns the matched graphics worth comparing.
func appearanceTags(tag string) bool { return Graphics[tag] }
