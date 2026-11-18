package appearance

import (
	"image"
	_ "image/png"
	"os"
	"testing"
)

// Task 49: the port must agree with the spike's ink.mjs on a golden pair.
// Measured 2026-11-18 with the spike's current lib/ink.mjs on these PNGs:
// agreement 0.5799690299745603, ink 18082.
func TestInkParityWithTheSpike(t *testing.T) {
	load := func(p string) image.Image {
		f, err := os.Open(p)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		img, _, err := image.Decode(f)
		if err != nil {
			t.Fatal(err)
		}
		return img
	}
	dir := "../../testdata/golden/planted/03-overflow"
	a := load(dir + "/chrome-a.png")
	b := load(dir + "/firefox-a.png")
	got, ok := InkEvidence(a, b, [4]float64{122, 243.9, 1036, 445}, [4]float64{122, 243.9, 1600, 445})
	t.Logf("table#target: %+v ok=%v (spike 0.579969, ink 18082)", got, ok)
	if diff := got.Agreement - 0.5799690299745603; diff > 0.01 || diff < -0.01 {
		t.Errorf("agreement %v vs spike 0.579969", got.Agreement)
	}
	if got.Ink != 18082 {
		t.Errorf("ink %d, want 18082", got.Ink)
	}
}
