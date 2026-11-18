package bidi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"

	"github.com/GerMP21/Omnisnap/internal/drivers"
	"github.com/GerMP21/Omnisnap/internal/drivers/wsrpc"
)

// Browser is a BiDi session with a running Firefox.
type Browser struct {
	c *wsrpc.Conn
}

// Dial connects to Firefox's remote agent (ws://host:port, as printed by
// "WebDriver BiDi listening on ...") and starts a BiDi session.
func Dial(ctx context.Context, wsURL string) (*Browser, error) {
	u, err := url.Parse(wsURL)
	if err != nil {
		return nil, fmt.Errorf("bidi: %w", err)
	}
	if u.Path == "" || u.Path == "/" {
		u.Path = "/session" // the endpoint for new sessions
	}
	c, err := wsrpc.Dial(ctx, u.String(), codec{})
	if err != nil {
		return nil, err
	}
	b := &Browser{c: c}
	if _, err := c.Call(ctx, "", "session.new", map[string]any{"capabilities": map[string]any{}}, nil, nil); err != nil {
		_ = c.Close()
		return nil, err
	}
	return b, nil
}

// Close ends the BiDi session and the connection; it does not stop Firefox.
func (b *Browser) Close(ctx context.Context) error {
	if _, err := b.c.Call(ctx, "", "session.end", nil, nil, nil); err != nil && !errors.Is(err, drivers.ErrCrashed) {
		_ = b.c.Close()
		return err
	}
	return b.c.Close()
}

// Session is one tab (a top-level browsing context).
type Session struct {
	c       *wsrpc.Conn
	context string
}

// NewSession opens a tab.
func (b *Browser) NewSession(ctx context.Context) (*Session, error) {
	var res struct {
		Context string `json:"context"`
	}
	if err := call(ctx, b.c, "browsingContext.create", map[string]any{"type": "tab"}, &res); err != nil {
		return nil, err
	}
	return &Session{c: b.c, context: res.Context}, nil
}

func call(ctx context.Context, c *wsrpc.Conn, method string, params, out any) error {
	res, err := c.Call(ctx, "", method, params, nil, nil)
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(res, out); err != nil {
		return fmt.Errorf("bidi: %s: decode result: %w", method, err)
	}
	return nil
}

// Navigate loads url and waits until it is complete (the load event).
func (s *Session) Navigate(ctx context.Context, url string) error {
	err := call(ctx, s.c, "browsingContext.navigate", map[string]any{"context": s.context, "url": url, "wait": "complete"}, nil)
	var be *bidiError
	if errors.As(err, &be) {
		// Firefox reports a failed load as a protocol error ("unknown error",
		// "Error: NS_ERROR_CONNECTION_REFUSED").
		return fmt.Errorf("bidi: navigate %s: %w: %s", url, drivers.ErrNavigation, be.Message)
	}
	return err
}

// Evaluate runs expr, awaiting a Promise. A string result is returned as is,
// anything else as JSON (like CDP's returnByValue).
func (s *Session) Evaluate(ctx context.Context, expr string) (string, error) {
	var res struct {
		Type             string          `json:"type"` // success | exception
		Result           json.RawMessage `json:"result"`
		ExceptionDetails struct {
			Text string `json:"text"`
		} `json:"exceptionDetails"`
	}
	params := map[string]any{
		"expression": expr, "target": map[string]any{"context": s.context},
		"awaitPromise": true, "resultOwnership": "none",
	}
	if err := call(ctx, s.c, "script.evaluate", params, &res); err != nil {
		return "", err
	}
	if res.Type == "exception" {
		// The page's own message, e.g. "ReferenceError: x is not defined".
		return "", fmt.Errorf("bidi: evaluate: %w: %s", drivers.ErrScript, res.ExceptionDetails.Text)
	}
	var v struct {
		Type  string          `json:"type"`
		Value json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(res.Result, &v); err != nil {
		return "", fmt.Errorf("bidi: evaluate: %w", err)
	}
	if v.Type == "string" {
		var str string
		if err := json.Unmarshal(v.Value, &str); err != nil {
			return "", fmt.Errorf("bidi: evaluate: %w", err)
		}
		return str, nil
	}
	return remoteJSON(res.Result)
}

// viewportProbe measures the viewport the page actually got.
const viewportProbe = "JSON.stringify([innerWidth, innerHeight, devicePixelRatio])"

// SetViewport sets the CSS viewport and returns what the page measures.
func (s *Session) SetViewport(ctx context.Context, w, h int, dpr float64) (drivers.Viewport, error) {
	params := map[string]any{"context": s.context, "viewport": map[string]any{"width": w, "height": h}, "devicePixelRatio": dpr}
	if err := call(ctx, s.c, "browsingContext.setViewport", params, nil); err != nil {
		return drivers.Viewport{}, err
	}
	raw, err := s.Evaluate(ctx, viewportProbe)
	if err != nil {
		return drivers.Viewport{}, err
	}
	var m [3]float64
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return drivers.Viewport{}, fmt.Errorf("bidi: viewport: %w", err)
	}
	return drivers.Viewport{Width: int(m[0]), Height: int(m[1]), DPR: m[2]}, nil
}

// Close closes the tab. After a lost connection there is nothing to close.
func (s *Session) Close(ctx context.Context) error {
	err := call(ctx, s.c, "browsingContext.close", map[string]any{"context": s.context}, nil)
	if errors.Is(err, drivers.ErrCrashed) {
		return nil
	}
	return err
}
