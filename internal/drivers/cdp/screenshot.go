package cdp

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/GerMP21/Omnisnap/internal/drivers"
)

// Screenshot returns a PNG of clip (document coordinates). With
// captureBeyondViewport the whole clip is rendered in one shot, however tall,
// without scrolling.
func (s *Session) Screenshot(ctx context.Context, clip drivers.Rect) ([]byte, error) {
	params := map[string]any{
		"format": "png",
		"clip": map[string]any{
			"x": clip.X, "y": clip.Y, "width": clip.Width, "height": clip.Height, "scale": 1,
		},
		"captureBeyondViewport": true,
		"fromSurface":           true,
	}
	var res struct {
		Data string `json:"data"`
	}
	if err := s.call(ctx, "Page.captureScreenshot", params, &res); err != nil {
		return nil, err
	}
	png, err := base64.StdEncoding.DecodeString(res.Data)
	if err != nil {
		return nil, fmt.Errorf("cdp: screenshot: %w", err)
	}
	return png, nil
}
