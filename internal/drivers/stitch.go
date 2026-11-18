package drivers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"math"
)

// Screenshot caps: a capture is never larger than 2560 x 16000 CSS px
// (SPEC-capture.md), whatever the page claims.
const (
	MaxShotWidth  = 2560
	MaxShotHeight = 16000
)

// Frame is one viewport-sized screenshot and the document y it was taken at.
type Frame struct {
	Y   int
	PNG []byte
}

// Stitch joins viewport-sized frames into one width x height image on a white
// background. It is the Go replacement for the spike's JS stitching (which
// made WebKit captures about 2.5x slower than the other engines): png.Decode
// and image/draw do the work natively.
func Stitch(width, height int, frames []Frame) ([]byte, error) {
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("drivers: stitch: %dx%d image", width, height)
	}
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(canvas, canvas.Bounds(), image.White, image.Point{}, draw.Src)
	for i, f := range frames {
		img, err := png.Decode(bytes.NewReader(f.PNG))
		if err != nil {
			return nil, fmt.Errorf("drivers: stitch: frame %d: %w", i, err)
		}
		b := img.Bounds()
		// The frame's rows go to canvas y = f.Y + (row - b.Min.Y); columns
		// start at the frame's left edge. Rows past the canvas and columns
		// past its width are dropped (a 2560 px cap, or DPR > 1).
		dst := image.Rect(0, f.Y, b.Dx(), f.Y+b.Dy()).Intersect(canvas.Bounds())
		if dst.Empty() {
			continue // entirely above or below the capture
		}
		src := image.Pt(b.Min.X+dst.Min.X, b.Min.Y+dst.Min.Y-f.Y)
		draw.Draw(canvas, dst, img, src, draw.Src)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, canvas); err != nil {
		return nil, fmt.Errorf("drivers: stitch: encode: %w", err)
	}
	return buf.Bytes(), nil
}

// The spike's scroll-and-stitch scripts (capture.mjs): fixed and sticky
// elements are tagged, then hidden once the first frame is taken, so a
// header appears once instead of on every frame, and restored afterwards.
const (
	markFixedScript = `(() => {
  for (const el of document.querySelectorAll('body *')) {
    const p = getComputedStyle(el).position;
    if (p === 'fixed' || p === 'sticky') el.setAttribute('data-omnisnap-fixed', '');
  }
  return 0;
})()`
	hideFixedScript = `(() => {
  const s = document.createElement('style');
  s.id = 'omnisnap-hide-fixed';
  s.textContent = '[data-omnisnap-fixed] { visibility: hidden !important; }';
  document.documentElement.append(s);
  return 0;
})()`
	restoreFixedScript = `(() => {
  document.getElementById('omnisnap-hide-fixed')?.remove();
  for (const el of document.querySelectorAll('[data-omnisnap-fixed]')) el.removeAttribute('data-omnisnap-fixed');
  scrollTo(0, 0);
  return 0;
})()`
	// scrollToScript scrolls and waits two animation frames, so the frame
	// captured is the one the page paints at that offset.
	scrollToScript = `new Promise((resolve) => {
  scrollTo(0, %d);
  const raf = window.__omnisnapRAF || requestAnimationFrame;
  raf(() => raf(() => resolve(Math.round(scrollY))));
})`
)

// EvalFunc runs a script in the page and returns its string result.
type EvalFunc func(ctx context.Context, expr string) (string, error)

// ShotFunc returns a PNG of the current viewport.
type ShotFunc func(ctx context.Context) ([]byte, error)

// ScrollAndStitch captures a whole page through a session that can only
// screenshot its viewport (WebDriver Classic; BiDi without document-origin
// support). docW and docH are the document's scroll size in CSS px; frames
// are taken every viewportHeight px until the page is covered.
func ScrollAndStitch(ctx context.Context, evaluate EvalFunc, shot ShotFunc, docW, docH float64, viewportHeight int) ([]byte, error) {
	if viewportHeight <= 0 {
		return nil, fmt.Errorf("drivers: stitch: viewport height %d", viewportHeight)
	}
	width := min(int(math.Ceil(docW)), MaxShotWidth)
	height := min(int(math.Ceil(docH)), MaxShotHeight)
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("drivers: stitch: document size %vx%v", docW, docH)
	}
	if _, err := evaluate(ctx, markFixedScript); err != nil {
		return nil, fmt.Errorf("drivers: stitch: mark fixed elements: %w", err)
	}
	defer func() {
		// Best effort: the page may be gone (navigation, crash).
		_, _ = evaluate(context.WithoutCancel(ctx), restoreFixedScript)
	}()

	var frames []Frame
	for y := 0; y < height; y += viewportHeight {
		raw, err := evaluate(ctx, fmt.Sprintf(scrollToScript, y))
		if err != nil {
			return nil, fmt.Errorf("drivers: stitch: scroll to %d: %w", y, err)
		}
		var scrollY int
		if err := json.Unmarshal([]byte(raw), &scrollY); err != nil {
			return nil, fmt.Errorf("drivers: stitch: scroll position %q: %w", raw, err)
		}
		if len(frames) == 1 {
			if _, err := evaluate(ctx, hideFixedScript); err != nil {
				return nil, fmt.Errorf("drivers: stitch: hide fixed elements: %w", err)
			}
		}
		frame, err := shot(ctx)
		if err != nil {
			return nil, fmt.Errorf("drivers: stitch: frame at %d: %w", y, err)
		}
		img, err := png.Decode(bytes.NewReader(frame))
		if err != nil {
			return nil, fmt.Errorf("drivers: stitch: frame at %d: %w", y, err)
		}
		frames = append(frames, Frame{Y: scrollY, PNG: frame})
		// A full-document screenshot (some drivers ignore the viewport) or
		// the last row: nothing left to scroll.
		if img.Bounds().Dy() > viewportHeight+1 || scrollY+img.Bounds().Dy() >= height {
			break
		}
	}
	return Stitch(width, height, frames)
}
