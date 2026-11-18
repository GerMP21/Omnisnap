package classic

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/GerMP21/Omnisnap/internal/drivers"
)

var _ drivers.Session = (*Session)(nil)

// Session is one WebDriver Classic session: a browser tab.
type Session struct {
	c       *Client
	id      string
	browser string
	version string

	vw, vh int     // last measured viewport, for stitching
	dpr    float64 // devicePixelRatio from the last measurement
}

// NewSession creates a browser session and applies the timeouts of
// SPEC-drivers.md: script 60 s, pageLoad 30 s, no implicit wait.
func (c *Client) NewSession(ctx context.Context) (*Session, error) {
	var v struct {
		SessionID    string `json:"sessionId"`
		Capabilities struct {
			BrowserName    string `json:"browserName"`
			BrowserVersion string `json:"browserVersion"`
		} `json:"capabilities"`
	}
	if err := c.call(ctx, http.MethodPost, "/session", map[string]any{"capabilities": c.Capabilities}, &v); err != nil {
		return nil, err
	}
	s := &Session{c: c, id: v.SessionID, browser: v.Capabilities.BrowserName, version: v.Capabilities.BrowserVersion}
	if err := s.command(ctx, http.MethodPost, "/timeouts", map[string]any{"script": 60000, "pageLoad": 30000, "implicit": 0}, nil); err != nil {
		return nil, err
	}
	// The spike sizes the window on a blank document, where layout costs
	// nothing (capture.mjs: navigate('about:blank') before the resize loop).
	if err := s.command(ctx, http.MethodPost, "/url", map[string]any{"url": "about:blank"}, nil); err != nil {
		_ = s.Close(ctx)
		return nil, err
	}
	return s, nil
}

// Browser is the browserName the driver reported, e.g. "MiniBrowser 2.48".
func (s *Session) Browser() string {
	if s.version == "" {
		return s.browser
	}
	return s.browser + " " + s.version
}

func (s *Session) command(ctx context.Context, method, route string, body, out any) error {
	return s.c.call(ctx, method, "/session/"+s.id+route, body, out)
}

// Navigate loads url and waits for the pageLoad timeout.
func (s *Session) Navigate(ctx context.Context, url string) error {
	err := s.command(ctx, http.MethodPost, "/url", map[string]any{"url": url}, nil)
	if err == nil {
		return nil
	}
	mapped := mapError(err)
	if errors.Is(mapped, drivers.ErrTimeout) {
		return mapped
	}
	var pe *protocolError
	if errors.As(err, &pe) {
		// Any other failure to load the page is a navigation error, with the
		// engine's own message (the spike saw "unknown error" here).
		return fmt.Errorf("classic: navigate %s: %w: %s", url, drivers.ErrNavigation, pe.wd.Message)
	}
	return mapped
}

// evaluate wraps expr in the async promise form: /execute/async always awaits
// the callback, whether or not the implementation awaits returned Promises
// (SPEC-drivers.md, "Evaluate (await promise)").
const evaluateScript = `const done = arguments[arguments.length - 1];
Promise.resolve((
%s
)).then(done, (e) => done(JSON.stringify({ omnisnapError: String(e) })));`

// Evaluate runs expr in the page, awaiting it if it is a Promise. A string
// result is returned as is; any other value as its JSON encoding.
func (s *Session) Evaluate(ctx context.Context, expr string) (string, error) {
	var v any
	err := s.command(ctx, http.MethodPost, "/execute/async", map[string]any{
		"script": fmt.Sprintf(evaluateScript, expr), "args": []any{},
	}, &v)
	if err != nil {
		mapped := mapError(err)
		var pe *protocolError
		if errors.Is(mapped, drivers.ErrScript) && errors.As(err, &pe) {
			return "", fmt.Errorf("classic: evaluate: %w: %s", drivers.ErrScript, pe.wd.Message)
		}
		return "", mapped
	}
	switch x := v.(type) {
	case nil:
		return "null", nil
	case string:
		// The wrapper's rejection path: the page's own message.
		var e struct {
			OmnisnapError string `json:"omnisnapError"`
		}
		if json.Unmarshal([]byte(x), &e) == nil && e.OmnisnapError != "" {
			return "", fmt.Errorf("classic: evaluate: %w: %s", drivers.ErrScript, e.OmnisnapError)
		}
		return x, nil
	default:
		b, err := json.Marshal(x)
		if err != nil {
			return "", fmt.Errorf("classic: evaluate: %w", err)
		}
		return string(b), nil
	}
}

// AddPreloadScript: Classic has no preload scripts. Capture adapts through
// Capabilities, and callers get ErrUnsupported.
func (s *Session) AddPreloadScript(context.Context, string) error {
	return fmt.Errorf("classic: AddPreloadScript: %w", drivers.ErrUnsupported)
}

// WaitNetworkIdle: Classic has no network events. Capture uses fixed settle
// waits instead.
func (s *Session) WaitNetworkIdle(context.Context, time.Duration, time.Duration) error {
	return fmt.Errorf("classic: WaitNetworkIdle: %w", drivers.ErrUnsupported)
}

// FailedRequests: Classic reports nothing.
func (s *Session) FailedRequests() []drivers.FailedRequest { return nil }

// Capabilities reports what Classic offers: no preload, no network events, and
// viewport-only screenshots (the session stitches them itself).
func (s *Session) Capabilities() drivers.Capabilities {
	return drivers.Capabilities{Preload: false, NetworkIdle: false, FullPageScreenshot: false}
}

// viewportProbe measures the viewport the page actually got.
const viewportProbe = "JSON.stringify([innerWidth, innerHeight, devicePixelRatio])"

// measure reads the viewport from the page.
func (s *Session) measure(ctx context.Context) (drivers.Viewport, error) {
	raw, err := s.Evaluate(ctx, viewportProbe)
	if err != nil {
		return drivers.Viewport{}, err
	}
	var m [3]float64
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return drivers.Viewport{}, fmt.Errorf("classic: viewport: %w", err)
	}
	return drivers.Viewport{Width: int(m[0]), Height: int(m[1]), DPR: m[2]}, nil
}

// maxViewportAttempts is the spike's measure-and-correct loop bound.
const maxViewportAttempts = 5

// resizeSettle is how long the window manager gets to apply a resize (the
// spike's sleep(300): resizes are asynchronous, and an unsized viewport
// breaks vh units).
const resizeSettle = 300 * time.Millisecond

// SetViewport sizes the window so the viewport is exactly w x h: window chrome
// varies between drivers and displays, so it measures and corrects. dpr is
// ignored (Classic cannot emulate devicePixelRatio); the measured value is
// returned.
func (s *Session) SetViewport(ctx context.Context, w, h int, dpr float64) (drivers.Viewport, error) {
	_ = dpr
	var fit drivers.Viewport
	for range maxViewportAttempts {
		vp, err := s.measure(ctx)
		if err != nil {
			return drivers.Viewport{}, err
		}
		fit = vp
		if vp.Width == w && vp.Height == h {
			break
		}
		var rect struct {
			Width  int `json:"width"`
			Height int `json:"height"`
		}
		if err := s.command(ctx, http.MethodGet, "/window/rect", nil, &rect); err != nil {
			return drivers.Viewport{}, err
		}
		// Grow or shrink the window by the viewport's error.
		err = s.command(ctx, http.MethodPost, "/window/rect", map[string]any{
			"width": rect.Width + w - vp.Width, "height": rect.Height + h - vp.Height,
		}, nil)
		if err != nil {
			return drivers.Viewport{}, err
		}
		select {
		case <-ctx.Done():
			return drivers.Viewport{}, fmt.Errorf("classic: set viewport: %w", ctx.Err())
		case <-time.After(resizeSettle):
		}
	}
	s.vw, s.vh, s.dpr = fit.Width, fit.Height, fit.DPR
	if s.dpr == 0 {
		s.dpr = 1
	}
	return fit, nil
}

// viewportShot is one viewport-sized screenshot, in device pixels.
func (s *Session) viewportShot(ctx context.Context) ([]byte, error) {
	var b64 string
	if err := s.command(ctx, http.MethodGet, "/screenshot", nil, &b64); err != nil {
		return nil, err
	}
	png, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("classic: screenshot: %w", err)
	}
	return png, nil
}

// Screenshot returns a PNG of clip (document coordinates) by scrolling and
// stitching viewport screenshots: Classic cannot capture beyond the viewport.
func (s *Session) Screenshot(ctx context.Context, clip drivers.Rect) ([]byte, error) {
	vh := s.vh
	if vh == 0 {
		vp, err := s.measure(ctx)
		if err != nil {
			return nil, err
		}
		vh, s.dpr = vp.Height, vp.DPR
	}
	if s.dpr == 0 {
		s.dpr = 1
	}
	shot := func(ctx context.Context) ([]byte, error) {
		png, err := s.viewportShot(ctx)
		if err != nil {
			return nil, err
		}
		if s.dpr != 1 {
			// A Retina display returns device pixels; the canvas is CSS px.
			return drivers.ScalePNG(png, 1/s.dpr)
		}
		return png, nil
	}
	return drivers.ScrollAndStitch(ctx, s.Evaluate, shot, clip.Width, clip.Height, vh)
}

// Close deletes the session. A driver that already lost it is not an error.
func (s *Session) Close(ctx context.Context) error {
	err := s.command(ctx, http.MethodDelete, "", nil, nil)
	if err == nil {
		return nil
	}
	if errors.Is(mapError(err), drivers.ErrCrashed) {
		return nil
	}
	return err
}

// mapError turns a WebDriver error reply into the driver's typed errors.
func mapError(err error) error {
	var pe *protocolError
	if !errors.As(err, &pe) {
		return err
	}
	var typed error
	switch pe.wd.Code {
	case "timeout", "script timeout":
		typed = drivers.ErrTimeout
	case "invalid session id", "no such session":
		typed = drivers.ErrCrashed
	case "javascript error":
		typed = drivers.ErrScript
	case "unknown command", "unsupported operation":
		typed = drivers.ErrUnsupported
	}
	if typed == nil {
		return err
	}
	return fmt.Errorf("classic: %w: %s", typed, pe.wd.Message)
}
