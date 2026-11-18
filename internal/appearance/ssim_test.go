package appearance

import (
	"image"
	"image/color"
	"math"
	"testing"
)

// noise returns a deterministic pseudo-random gray image.
func noise(w, h int, seed uint32) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	x := seed
	for y := range h {
		for xx := range w {
			x = x*1664525 + 1013904223
			v := uint8((x >> 16) & 0xff)
			img.Set(xx, y, color.RGBA{v, v, v, 255})
		}
	}
	return img
}

// block paints a rectangle.
func block(w, h int, r image.Rectangle, c color.Color) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.White)
		}
	}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			img.Set(x, y, c)
		}
	}
	return img
}

func TestSimilarityIdentical(t *testing.T) {
	img := noise(40, 40, 7)
	box := [4]float64{0, 0, 40, 40}
	s, ok := Similarity(img, img, box, box)
	if !ok || math.Abs(s-1) > 1e-9 {
		t.Errorf("identical: ssim %v, ok %v; want 1", s, ok)
	}
}

// A two-pixel offset is found by the search: the same drawing scores high.
func TestSimilarityShifted(t *testing.T) {
	a := block(60, 60, image.Rect(10, 10, 40, 40), color.Black)
	b := block(60, 60, image.Rect(12, 12, 42, 42), color.Black)
	boxA, boxB := [4]float64{0, 0, 60, 60}, [4]float64{0, 0, 60, 60}
	s, ok := Similarity(a, b, boxA, boxB)
	if !ok || s < 0.80 {
		t.Errorf("2 px shift: ssim %v; want high (the search compensates)", s)
	}
}

// Different content: low SSIM.
func TestSimilarityDifferentContent(t *testing.T) {
	a := block(60, 60, image.Rect(5, 5, 55, 25), color.Black)
	b := block(60, 60, image.Rect(5, 35, 55, 55), color.Black)
	box := [4]float64{0, 0, 60, 60}
	s, ok := Similarity(a, b, box, box)
	if !ok || s >= ssimThreshold {
		t.Errorf("different content: ssim %v; want below %v", s, ssimThreshold)
	}
}

// The offset search must not turn unrelated noise into a match.
func TestSimilarityUnrelatedNoise(t *testing.T) {
	a, b := noise(50, 50, 1), noise(50, 50, 999)
	box := [4]float64{0, 0, 50, 50}
	s, ok := Similarity(a, b, box, box)
	if !ok || s >= ssimThreshold {
		t.Errorf("unrelated noise: ssim %v; want below %v", s, ssimThreshold)
	}
}

func TestSimilarityOutsideTheScreenshot(t *testing.T) {
	img := noise(20, 20, 3)
	if _, ok := Similarity(img, img, [4]float64{0, 0, 40, 40}, [4]float64{0, 0, 40, 40}); ok {
		t.Error("a crop beyond the screenshot was compared")
	}
}
