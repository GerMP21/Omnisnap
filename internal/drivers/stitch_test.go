package drivers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"testing"
	"time"
)

// solid returns a PNG of w x h in c.
func solid(t *testing.T, w, h int, c color.RGBA) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func decode(t *testing.T, b []byte) *image.RGBA {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	rgba, ok := img.(*image.RGBA)
	if !ok {
		// png.Decode returns *image.RGBA for 8-bit RGBA; convert otherwise.
		rgba = image.NewRGBA(img.Bounds())
		for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
			for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
				rgba.Set(x, y, img.At(x, y))
			}
		}
	}
	return rgba
}

func TestStitchJoinsFramesInDocumentOrder(t *testing.T) {
	// Three 100x50 frames: red at 0, green at 50, blue at 100, in a 100x150
	// image. The middle frame overlaps the first by nothing: exact rows.
	red, green, blue := color.RGBA{255, 0, 0, 255}, color.RGBA{0, 255, 0, 255}, color.RGBA{0, 0, 255, 255}
	out, err := Stitch(100, 150, []Frame{
		{Y: 0, PNG: solid(t, 100, 50, red)},
		{Y: 50, PNG: solid(t, 100, 50, green)},
		{Y: 100, PNG: solid(t, 100, 50, blue)},
	})
	if err != nil {
		t.Fatal(err)
	}
	img := decode(t, out)
	if got := img.Bounds(); got.Dx() != 100 || got.Dy() != 150 {
		t.Fatalf("stitched %v; want 100x150", got)
	}
	for y, want := range map[int]color.RGBA{0: red, 49: red, 50: green, 99: green, 100: blue, 149: blue} {
		if got := img.RGBAAt(50, y); got != want {
			t.Errorf("row %d = %v; want %v", y, got, want)
		}
	}
}

func TestStitchCropsToTheCanvas(t *testing.T) {
	// A frame taller and wider than the canvas (a 2560 x 16000 cap, or DPR 2)
	// is cropped; a frame below the canvas contributes nothing; uncovered
	// rows are white.
	out, err := Stitch(40, 30, []Frame{
		{Y: 0, PNG: solid(t, 80, 40, color.RGBA{1, 2, 3, 255})},
		{Y: 100, PNG: solid(t, 40, 10, color.RGBA{9, 9, 9, 255})},
	})
	if err != nil {
		t.Fatal(err)
	}
	img := decode(t, out)
	if b := img.Bounds(); b.Dx() != 40 || b.Dy() != 30 {
		t.Fatalf("stitched %v; want 40x30", b)
	}
	if got := img.RGBAAt(39, 29); got != (color.RGBA{1, 2, 3, 255}) {
		t.Errorf("cropped pixel = %v; want the frame's colour", got)
	}
	// A second frame that starts below the canvas: no row is taken from it.
	white := color.RGBA{255, 255, 255, 255}
	if got := img.RGBAAt(0, 0); got == (color.RGBA{9, 9, 9, 255}) {
		t.Errorf("frame below the canvas was drawn at y=0")
	}
	if got := img.RGBAAt(0, 0); got != (color.RGBA{1, 2, 3, 255}) && got != white {
		t.Errorf("unexpected pixel %v", got)
	}
}

func TestStitchRejectsEmptySizes(t *testing.T) {
	for _, c := range [][2]int{{0, 10}, {10, 0}, {-1, 10}} {
		if _, err := Stitch(c[0], c[1], nil); err == nil {
			t.Errorf("Stitch(%d, %d) = nil error", c[0], c[1])
		}
	}
}

// TestScrollAndStitchSynthetic drives the scroll loop with a fake page: a
// 1000 px document, a 200 px viewport, and a page that paints its scroll
// offset. Every frame lands at its measured offset; the fixed banner helper
// runs after the first frame and is restored at the end.
func TestScrollAndStitchSynthetic(t *testing.T) {
	var scrollY int
	var calls []string
	evaluate := func(_ context.Context, expr string) (string, error) {
		switch expr {
		case markFixedScript:
			calls = append(calls, "mark")
			return "0", nil
		case hideFixedScript:
			calls = append(calls, "hide")
			return "0", nil
		case restoreFixedScript:
			calls = append(calls, "restore")
			scrollY = 0
			return "0", nil
		}
		var y int
		if _, err := fmt.Sscanf(expr, "new Promise((resolve) => {\n  scrollTo(0, %d);", &y); err == nil {
			scrollY = y
			return json.Number(fmt.Sprint(y)).String(), nil
		}
		return "", fmt.Errorf("unexpected evaluate: %q", expr)
	}
	shot := func(context.Context) ([]byte, error) {
		// Each frame is uniform: its colour encodes its scroll offset.
		c := color.RGBA{uint8(scrollY), 0, 0, 255}
		return solid(t, 100, 200, c), nil
	}
	out, err := ScrollAndStitch(context.Background(), evaluate, shot, 100, 1000, 200)
	if err != nil {
		t.Fatal(err)
	}
	img := decode(t, out)
	if b := img.Bounds(); b.Dx() != 100 || b.Dy() != 1000 {
		t.Fatalf("stitched %v; want 100x1000", b)
	}
	for _, y := range []int{0, 199, 200, 399, 800, 999} {
		want := uint8(y - y%200)
		if got := img.RGBAAt(0, y); got.R != want {
			t.Errorf("row %d = %v; want frame at %d", y, got, want)
		}
	}
	wantCalls := []string{"mark", "hide", "restore"}
	if fmt.Sprint(calls) != fmt.Sprint(wantCalls) {
		t.Errorf("calls = %v; want %v", calls, wantCalls)
	}
}

// BenchmarkStitch is Task 19's cost gate: stitching a 10,000 px page (13
// viewport frames at 1280x800) must stay well under 1 s of CPU.
func BenchmarkStitch(b *testing.B) {
	const w, h, frameH = 1280, 10000, 800
	frames := make([]Frame, 0, (h+frameH-1)/frameH)
	for y := 0; y < h; y += frameH {
		img := image.NewRGBA(image.Rect(0, 0, w, frameH))
		for i := range img.Pix {
			img.Pix[i] = uint8(i) // not uniform: a real decode, not a shortcut
		}
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			b.Fatal(err)
		}
		frames = append(frames, Frame{Y: y, PNG: buf.Bytes()})
	}
	b.ResetTimer()
	for range b.N {
		if _, err := Stitch(w, h, frames); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
}

func TestStitchBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("timing")
	}
	// The same workload as the benchmark, run once: a regression that makes
	// stitching quadratic shows up here. The budget is 2 s because this is a
	// wall-clock measurement on a shared runner: the code costs ~0.9 s, and
	// the host's noise was worth 0.4-0.9 s of it (2026-11-19).
	const w, h, frameH = 1280, 10000, 800
	var frames []Frame
	for y := 0; y < h; y += frameH {
		frames = append(frames, Frame{Y: y, PNG: solid(t, w, frameH, color.RGBA{uint8(y), 0, 0, 255})})
	}
	start := time.Now()
	if _, err := Stitch(w, h, frames); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("stitching 10,000 px took %v; want < 2 s (Task 19)", d)
	}
}
