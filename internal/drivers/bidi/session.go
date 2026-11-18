package bidi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sync"
	"time"

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

	net      *network
	netSub   func() // ends the event subscription
	netOK    bool   // network events are enabled (Firefox emits them only when subscribed)
	subID    string // BiDi subscription of this tab's events, for unsubscribe
	stitch   bool   // no document-origin screenshots: scroll and stitch
	closeMux sync.Mutex
}

var _ drivers.Session = (*Session)(nil)

// NewSession opens a tab and subscribes to its network events.
func (b *Browser) NewSession(ctx context.Context) (*Session, error) {
	var res struct {
		Context string `json:"context"`
	}
	if err := call(ctx, b.c, "browsingContext.create", map[string]any{"type": "tab"}, &res); err != nil {
		return nil, err
	}
	s := &Session{c: b.c, context: res.Context, net: newNetwork()}
	// Subscribe before enabling the events, so none is lost.
	events, stop := b.c.SubscribeBuffered("", networkEvents, networkMethods...)
	s.netSub = stop
	go s.net.follow(events.C())
	var subscribed struct {
		Subscription string `json:"subscription"`
	}
	err := call(ctx, b.c, "session.subscribe", map[string]any{"events": networkMethods, "contexts": []string{res.Context}}, &subscribed)
	if err != nil {
		// Without network events there is no idle signal or failure list;
		// capture adapts through Capabilities. The tab is still usable.
		stop()
		s.netSub = nil
	} else {
		s.netOK = true
		s.subID = subscribed.Subscription
	}
	return s, nil
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
	s.net.reset()
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

// WaitNetworkIdle returns once nothing has been in flight for idle, or
// ErrTimeout after limit.
func (s *Session) WaitNetworkIdle(ctx context.Context, idle, limit time.Duration) error {
	return s.net.waitIdle(ctx, idle, limit)
}

// FailedRequests lists the requests that failed since the last Navigate.
func (s *Session) FailedRequests() []drivers.FailedRequest { return s.net.failedRequests() }

// Capabilities reports what a BiDi session supports. FullPageScreenshot turns
// false once a document-origin capture was refused and the session switched to
// stitching (the capture still gets a full page).
func (s *Session) Capabilities() drivers.Capabilities {
	return drivers.Capabilities{Preload: true, NetworkIdle: s.netOK, FullPageScreenshot: !s.stitch}
}

// Close unsubscribes and closes the tab. After a lost connection there is
// nothing to close.
func (s *Session) Close(ctx context.Context) error {
	s.closeMux.Lock()
	defer s.closeMux.Unlock()
	if s.netSub != nil {
		s.netSub()
		s.netSub = nil
	}
	if s.subID != "" {
		_ = call(ctx, s.c, "session.unsubscribe", map[string]any{"subscriptions": []string{s.subID}}, nil)
		s.subID = ""
	}
	err := call(ctx, s.c, "browsingContext.close", map[string]any{"context": s.context}, nil)
	if errors.Is(err, drivers.ErrCrashed) {
		return nil
	}
	return err
}
