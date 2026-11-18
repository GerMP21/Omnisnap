package appearance

import (
	"image"
	"image/color"
	"testing"
)

// canvas builds a white image with black pixels at the given points.
func canvas(w, h int, black ...[2]int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.White)
		}
	}
	for _, p := range black {
		img.Set(p[0], p[1], color.Black)
	}
	return img
}

func lines(w, h int) *image.RGBA {
	img := canvas(w, h)
	for y := 10; y < 30; y++ {
		for x := 5; x < 45; x++ {
			img.Set(x, y, color.Black)
		}
	}
	return img
}

func TestInkIdenticalCrops(t *testing.T) {
	a := lines(60, 60)
	box := [4]float64{0, 0, 60, 60}
	got, ok := InkEvidence(a, a, box, box)
	if !ok || got.Agreement != 1 || got.Ink == 0 {
		t.Errorf("identical crops: %+v, %v; want agreement 1", got, ok)
	}
}

func TestInkShiftedByOnePixel(t *testing.T) {
	a := lines(60, 60)
	b := canvas(60, 60)
	for y := 10; y < 30; y++ {
		for x := 6; x < 46; x++ { // shifted one pixel right
			b.Set(x, y, color.Black)
		}
	}
	box := [4]float64{0, 0, 60, 60}
	got, ok := InkEvidence(a, b, box, box)
	if !ok {
		t.Fatal("not comparable")
	}
	// One pixel of shift with a 1 px dilation: nearly all ink still agrees.
	if got.Agreement < 0.9 {
		t.Errorf("1 px shift: agreement %v; want >= 0.9", got.Agreement)
	}
}

func TestInkMissingCrop(t *testing.T) {
	a := lines(60, 60)
	empty := canvas(60, 60)
	box := [4]float64{0, 0, 60, 60}
	got, ok := InkEvidence(a, empty, box, box)
	if !ok {
		t.Fatal("not comparable")
	}
	if got.Agreement > 0.2 {
		t.Errorf("ink vs nothing: agreement %v; want near 0", got.Agreement)
	}
	// Nothing drawn in either: agreement 1, ink 0.
	got, ok = InkEvidence(empty, empty, box, box)
	if !ok || got.Agreement != 1 || got.Ink != 0 {
		t.Errorf("empty crops: %+v", got)
	}
}

// A bordered box: the border must not become the background (the spike's
// first version counted the whole interior as ink).
func TestInkBorderedBox(t *testing.T) {
	img := canvas(60, 60)
	for x := 5; x < 55; x++ {
		img.Set(x, 5, color.RGBA{200, 200, 200, 255})
		img.Set(x, 54, color.RGBA{200, 200, 200, 255})
	}
	box := [4]float64{0, 0, 60, 60}
	got, ok := InkEvidence(img, img, box, box)
	if !ok || got.Agreement != 1 {
		t.Errorf("bordered box: %+v, %v; want agreement 1", got, ok)
	}
	// The border is lighter than the mask threshold from white: little ink.
	if got.Ink > 60*60/2 {
		t.Errorf("ink %d; the border must not fill the crop", got.Ink)
	}
}

// Recoloured content is drawn where the other is not.
func TestInkRecoloured(t *testing.T) {
	a := lines(60, 60)
	b := canvas(60, 60)
	for y := 10; y < 30; y++ {
		for x := 5; x < 45; x++ {
			b.Set(x, y, color.RGBA{0, 0, 255, 255})
		}
	}
	box := [4]float64{0, 0, 60, 60}
	got, ok := InkEvidence(a, b, box, box)
	if !ok || got.Agreement < 0.95 {
		t.Errorf("recoloured same shape: %+v; want the ink to agree (it is a mask)", got)
	}
}

func TestInkBeyondTheScreenshot(t *testing.T) {
	a := lines(60, 60)
	box := [4]float64{0, 55, 60, 40}
	if _, ok := InkEvidence(a, a, box, box); ok {
		t.Error("crop beyond the screenshot was compared")
	}
}

func TestNoiseFloor(t *testing.T) {
	var samples []Ink
	for i := range 20 {
		samples = append(samples, Ink{Agreement: 0.80 + float64(i)*0.01, Ink: 40})
	}
	floor, n, measured := NoiseFloor(samples)
	if !measured || n != 20 || floor < 0.79 || floor > 0.86 {
		t.Errorf("floor %v, n %d, measured %v", floor, n, measured)
	}
	// Few samples: the default, not measured.
	if floor, n, measured := NoiseFloor(samples[:5]); measured || floor != 0.85 || n != 5 {
		t.Errorf("few samples: floor %v, n %d, measured %v", floor, n, measured)
	}
	// Low-ink samples are ignored.
	var low []Ink
	for range 20 {
		low = append(low, Ink{Agreement: 0.1, Ink: 3})
	}
	if floor, _, measured := NoiseFloor(low); measured || floor != 0.85 {
		t.Errorf("low-ink samples: floor %v, measured %v", floor, measured)
	}
}

// Two engines often report different box widths for the same element. The
// crops must be compared over a common extent: a narrower B box used to index
// past the wider A mask and panic the pipeline (found on 31-control-dashboard,
// firefox vs chrome).
func TestInkDifferentBoxWidths(t *testing.T) {
	a := lines(80, 60)
	b := lines(80, 60)
	wide := [4]float64{0, 0, 60, 60}
	narrow := [4]float64{0, 0, 20, 60}
	for _, tc := range []struct {
		name       string
		boxA, boxB [4]float64
	}{
		{"B narrower", wide, narrow},
		{"B wider", narrow, wide},
	} {
		got, ok := InkEvidence(a, b, tc.boxA, tc.boxB)
		if !ok {
			t.Errorf("%s: no evidence", tc.name)
			continue
		}
		// The same drawing inside both crops: the part beyond the narrower box
		// is outside one crop, so agreement stays high but ink counts differ.
		if got.Agreement < 0.5 || got.Ink == 0 {
			t.Errorf("%s: %+v; want a sane comparison", tc.name, got)
		}
	}
	// Heights too: the same crash path exists on the other axis.
	if _, ok := InkEvidence(a, b, [4]float64{0, 0, 40, 60}, [4]float64{0, 0, 40, 20}); !ok {
		t.Error("different heights: no evidence")
	}
}
