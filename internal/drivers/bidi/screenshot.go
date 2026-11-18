package bidi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/GerMP21/Omnisnap/internal/drivers"
)

// Screenshot returns a PNG of clip (document coordinates).
//
// The BiDi way is one document-origin capture with a box clip: the whole page
// at once, however tall. Firefox ESR builds without `origin: document` answer
// "unsupported operation"; then the page is scrolled and stitched in Go
// (drivers.ScrollAndStitch), as for WebDriver Classic.
// Source: https://w3c.github.io/webdriver-bidi/#command-browsingContext-captureScreenshot
func (s *Session) Screenshot(ctx context.Context, clip drivers.Rect) ([]byte, error) {
	png, err := s.capture(ctx, "document", clip)
	if err == nil {
		return png, nil
	}
	if !isUnsupported(err) {
		return nil, err
	}
	s.stitch = true
	vh, err := s.viewportHeight(ctx)
	if err != nil {
		return nil, err
	}
	return drivers.ScrollAndStitch(ctx, s.Evaluate, s.viewportShot, clip.Width, clip.Height, vh)
}

// viewportShot captures just the viewport (the origin of a stitched frame).
func (s *Session) viewportShot(ctx context.Context) ([]byte, error) {
	return s.capture(ctx, "viewport", drivers.Rect{})
}

func (s *Session) capture(ctx context.Context, origin string, clip drivers.Rect) ([]byte, error) {
	params := map[string]any{
		"context": s.context,
		"origin":  origin,
		"format":  map[string]any{"type": "image/png"},
	}
	if clip.Width > 0 && clip.Height > 0 {
		params["clip"] = map[string]any{
			"type": "box", "x": clip.X, "y": clip.Y, "width": clip.Width, "height": clip.Height,
		}
	}
	var res struct {
		Data string `json:"data"`
	}
	if err := call(ctx, s.c, "browsingContext.captureScreenshot", params, &res); err != nil {
		return nil, err
	}
	png, err := base64.StdEncoding.DecodeString(res.Data)
	if err != nil {
		return nil, fmt.Errorf("bidi: screenshot: %w", err)
	}
	return png, nil
}

// isUnsupported reports whether the browser answered "unsupported operation"
// (or "unknown command": the method does not exist at all).
func isUnsupported(err error) bool {
	var be *bidiError
	if errors.As(err, &be) {
		return be.Code == "unsupported operation" || be.Code == "unknown command"
	}
	return false
}

func (s *Session) viewportHeight(ctx context.Context) (int, error) {
	raw, err := s.Evaluate(ctx, "JSON.stringify([innerHeight])")
	if err != nil {
		return 0, err
	}
	var m [1]float64
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return 0, fmt.Errorf("bidi: viewport height: %w", err)
	}
	return int(m[0]), nil
}
