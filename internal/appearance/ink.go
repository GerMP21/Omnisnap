// Package appearance measures what geometry cannot see (SPEC-appearance-diff.md):
// crops each element at its own position in each engine's screenshot
// (piecewise registration through the matched DOM boxes), compares INK MASKS
// instead of pixel intensities, and detects graphics drawn differently with
// SSIM.
package appearance

import (
	"image"
	"image/color"
	"math"
)

// maxPixels: larger crops are sampled every second pixel (the spike's cap).
const maxPixels = 400000

// luma is the perceptual luminance of a pixel.
func luma(c color.Color) float64 {
	r, g, b, _ := c.RGBA()
	return 0.299*float64(r>>8) + 0.587*float64(g>>8) + 0.114*float64(b>>8)
}

// crop is a registered crop: luminance samples and whether each lies inside
// the screenshot.
type crop struct {
	lum    []float64
	inside []bool
	w, h   int
	step   int
}

// cropOf samples box [x, y, w, h] at step. Rows and columns outside the image
// are marked not inside (a page taller than the screenshot is skipped whole by
// the callers, but a box can still stick out).
func cropOf(img image.Image, box [4]float64, step int) crop {
	if step < 1 {
		step = 1
	}
	w := int(math.Ceil(box[2] / float64(step)))
	h := int(math.Ceil(box[3] / float64(step)))
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	c := crop{lum: make([]float64, w*h), inside: make([]bool, w*h), w: w, h: h, step: step}
	b := img.Bounds()
	for r := range h {
		y := int(math.Round(box[1])) + r*step
		for col := range w {
			x := int(math.Round(box[0])) + col*step
			i := r*w + col
			if x < b.Min.X || y < b.Min.Y || x >= b.Max.X || y >= b.Max.Y {
				continue
			}
			c.lum[i] = luma(img.At(x, y))
			c.inside[i] = true
		}
	}
	return c
}

// mask is where something is drawn: pixels that differ from the crop's
// background by more than 32 luma. The background is the histogram mode
// (4-level bins), so a border or an edge line cannot win the vote.
func (c crop) mask() (m []bool, ink int) {
	var hist [64]int
	for i, v := range c.lum {
		if !c.inside[i] {
			continue
		}
		bin := int(v) >> 2
		if bin > 63 {
			bin = 63
		}
		hist[bin]++
	}
	mode := 63
	for k := range hist {
		if hist[k] > hist[mode] {
			mode = k
		}
	}
	bg := float64(mode*4 + 2)
	m = make([]bool, len(c.lum))
	for i, v := range c.lum {
		if c.inside[i] && math.Abs(v-bg) > 32 {
			m[i] = true
			ink++
		}
	}
	return m, ink
}

// dilate expands the mask by one pixel in eight directions.
func dilate(m []bool, w, h int) []bool {
	out := make([]bool, len(m))
	for r := range h {
		for c := range w {
			if !m[r*w+c] {
				continue
			}
			for dr := -1; dr <= 1; dr++ {
				for dc := -1; dc <= 1; dc++ {
					rr, cc := r+dr, c+dc
					if rr >= 0 && cc >= 0 && rr < h && cc < w {
						out[rr*w+cc] = true
					}
				}
			}
		}
	}
	return out
}

// Ink is one element's ink evidence.
type Ink struct {
	Agreement  float64 // 0..1: how much of the two inks coincide
	Ink        int     // ink pixels in both crops
	Floor      float64 // the page's noise floor
	Observable bool    // agreement below the floor
}

// InkEvidence compares the ink of two registered crops. ok is false when
// there is nothing to compare (no images, a degenerate box, a crop beyond the
// screenshot).
func InkEvidence(imgA, imgB image.Image, boxA, boxB [4]float64) (Ink, bool) {
	if imgA == nil || imgB == nil {
		return Ink{}, false
	}
	w := math.Max(boxA[2], boxB[2])
	h := math.Max(boxA[3], boxB[3])
	if w < 2 || h < 2 {
		return Ink{}, false
	}
	if boxA[1]+h > float64(imgA.Bounds().Max.Y) || boxB[1]+h > float64(imgB.Bounds().Max.Y) {
		return Ink{}, false
	}
	step := 1
	if w*h > maxPixels {
		step = 2
	}
	ca := cropOf(imgA, boxA, step)
	cb := cropOf(imgB, boxB, step)
	ma, na := ca.mask()
	mb, nb := cb.mask()
	if na+nb < 8 {
		return Ink{Agreement: 1, Ink: 0}, true // nothing drawn in either
	}
	da := dilate(ma, ca.w, ca.h)
	db := dilate(mb, cb.w, cb.h)
	hit := 0
	for i := range ma {
		if ma[i] && db[i] {
			hit++
		}
		if mb[i] && da[i] {
			hit++
		}
	}
	return Ink{Agreement: float64(hit) / float64(na+nb), Ink: na + nb}, true
}

// NoiseFloor is the page's rasterization noise: the 5th percentile of ink
// agreement over unchanged elements with enough ink, clamped to [0.5, 0.97].
// Fewer than 10 samples: the default 0.85, not measured.
func NoiseFloor(samples []Ink) (floor float64, n int, measured bool) {
	var xs []float64
	for _, s := range samples {
		if s.Ink >= 20 {
			xs = append(xs, s.Agreement)
		}
	}
	n = len(xs)
	if n < 10 {
		return 0.85, n, false
	}
	sortFloats(xs)
	p5 := xs[int(float64(n)*0.05)]
	if p5 < 0.5 {
		p5 = 0.5
	}
	if p5 > 0.97 {
		p5 = 0.97
	}
	return p5, n, true
}

func sortFloats(xs []float64) {
	// Small slices: insertion sort avoids the sort package's overhead here.
	for i := 1; i < len(xs); i++ {
		v := xs[i]
		j := i - 1
		for j >= 0 && xs[j] > v {
			xs[j+1] = xs[j]
			j--
		}
		xs[j+1] = v
	}
}
