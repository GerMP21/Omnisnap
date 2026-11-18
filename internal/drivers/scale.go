package drivers

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
)

// ScalePNG returns png scaled by f (nearest neighbour) — enough for the one
// place it is needed: a Classic driver on a 2x display (Safari on a Retina
// Mac) returns screenshots in device pixels, while the stitched image is in
// CSS px. Layout comparison only needs geometry, not resampling quality.
func ScalePNG(p []byte, f float64) ([]byte, error) {
	if f <= 0 {
		return nil, fmt.Errorf("drivers: scale: factor %v", f)
	}
	img, err := png.Decode(bytes.NewReader(p))
	if err != nil {
		return nil, fmt.Errorf("drivers: scale: %w", err)
	}
	b := img.Bounds()
	w, h := int(float64(b.Dx())*f+0.5), int(float64(b.Dy())*f+0.5)
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		sy := b.Min.Y + int(float64(y)/f)
		if sy >= b.Max.Y {
			sy = b.Max.Y - 1
		}
		for x := range w {
			sx := b.Min.X + int(float64(x)/f)
			if sx >= b.Max.X {
				sx = b.Max.X - 1
			}
			out.Set(x, y, img.At(sx, sy))
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, out); err != nil {
		return nil, fmt.Errorf("drivers: scale: encode: %w", err)
	}
	return buf.Bytes(), nil
}
