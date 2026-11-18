package cdp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/GerMP21/Omnisnap/internal/drivers"
	"github.com/GerMP21/Omnisnap/internal/drivers/wsrpc"
)

// Browser is a CDP connection to a running browser (its browser-level
// websocket, from DevToolsActivePort or /json/version).
type Browser struct {
	c *wsrpc.Conn
}

// Dial connects to a browser's DevTools websocket URL.
func Dial(ctx context.Context, wsURL string) (*Browser, error) {
	c, err := wsrpc.Dial(ctx, wsURL, codec{})
	if err != nil {
		return nil, err
	}
	return &Browser{c: c}, nil
}

// Close closes the connection; it does not stop the browser process.
func (b *Browser) Close() error { return b.c.Close() }

var _ drivers.Session = (*Session)(nil)

// Session is one page (target) of the browser, driven over a flattened
// CDP session on the browser's connection.
type Session struct {
	c        *wsrpc.Conn
	targetID string
	id       string // CDP sessionId

	crashed   chan struct{} // closed when the page crashes
	crashOnce sync.Once
	net       *network
	netSub    *wsrpc.Subscription // network events of the page and its child targets

	mu     sync.Mutex
	stops  map[string][]func() // per CDP session (the page's and its children's): end its subscriptions
	closed bool
}

// NewSession opens a blank page and attaches to it.
func (b *Browser) NewSession(ctx context.Context) (*Session, error) {
	var target struct {
		TargetID string `json:"targetId"`
	}
	if err := b.call(ctx, "Target.createTarget", map[string]any{"url": "about:blank"}, &target); err != nil {
		return nil, err
	}
	var attached struct {
		SessionID string `json:"sessionId"`
	}
	if err := b.call(ctx, "Target.attachToTarget", map[string]any{"targetId": target.TargetID, "flatten": true}, &attached); err != nil {
		return nil, err
	}
	s := &Session{c: b.c, targetID: target.TargetID, id: attached.SessionID, crashed: make(chan struct{}),
		net: newNetwork(), stops: map[string][]func(){}}
	// Subscribe before enabling the domains that send the events.
	crashes, stopCrash := s.c.Subscribe(s.id, "Inspector.targetCrashed")
	go func() {
		for range crashes {
			s.crashOnce.Do(func() { close(s.crashed) })
		}
	}()
	s.addStop(s.id, stopCrash)
	netSub, stopNet := s.c.SubscribeBuffered(s.id, networkEvents, networkMethods...)
	s.netSub = netSub
	go s.net.follow(netSub.C())
	s.addStop(s.id, stopNet)
	s.watchTargets(s.id)
	for _, m := range []string{"Inspector.enable", "Page.enable", "Network.enable"} {
		if err := s.call(ctx, m, nil, nil); err != nil {
			s.stop()
			return nil, err
		}
	}
	if err := s.call(ctx, "Target.setAutoAttach", autoAttach, nil); err != nil {
		s.stop()
		return nil, err
	}
	return s, nil
}

func (b *Browser) call(ctx context.Context, method string, params, out any) error {
	res, err := b.c.Call(ctx, "", method, params, nil, nil)
	if err != nil {
		return err
	}
	return decode(method, res, out)
}

func errCrashed() error { return fmt.Errorf("cdp: page: %w", drivers.ErrCrashed) }

func (s *Session) call(ctx context.Context, method string, params, out any) error {
	select {
	case <-s.crashed:
		return errCrashed()
	default:
	}
	res, err := s.c.Call(ctx, s.id, method, params, s.crashed, errCrashed)
	if err != nil {
		return err
	}
	return decode(method, res, out)
}

func decode(method string, res json.RawMessage, out any) error {
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(res, out); err != nil {
		return fmt.Errorf("cdp: %s: decode result: %w", method, err)
	}
	return nil
}

// stop ends every subscription of the page and its child targets.
func (s *Session) stop() {
	s.mu.Lock()
	s.closed = true
	all := s.stops
	s.stops = map[string][]func(){}
	s.mu.Unlock()
	for _, fs := range all {
		for _, f := range fs {
			f()
		}
	}
}

// Navigate loads url and waits for the load event.
func (s *Session) Navigate(ctx context.Context, url string) error {
	s.net.reset()
	// Subscribe first: the load event can arrive right after the reply.
	loaded, stop := s.c.Subscribe(s.id, "Page.loadEventFired")
	defer stop()
	var nav struct {
		ErrorText string `json:"errorText"`
	}
	if err := s.call(ctx, "Page.navigate", map[string]any{"url": url}, &nav); err != nil {
		return err
	}
	if nav.ErrorText != "" {
		return fmt.Errorf("cdp: navigate %s: %w: %s", url, drivers.ErrNavigation, nav.ErrorText)
	}
	select {
	case _, ok := <-loaded:
		if !ok {
			return s.c.Err()
		}
		return nil
	case <-ctx.Done():
		return s.c.CtxErr(ctx, "navigate "+url, ctx.Err())
	case <-s.crashed:
		return errCrashed()
	}
}

// Evaluate runs expr, awaiting a Promise. A string result is returned as is,
// anything else as JSON.
func (s *Session) Evaluate(ctx context.Context, expr string) (string, error) {
	var res struct {
		Result struct {
			Type  string          `json:"type"`
			Value json.RawMessage `json:"value"`
		} `json:"result"`
		ExceptionDetails *struct {
			Text      string `json:"text"`
			Exception *struct {
				Description string `json:"description"`
			} `json:"exception"`
		} `json:"exceptionDetails"`
	}
	params := map[string]any{"expression": expr, "awaitPromise": true, "returnByValue": true}
	if err := s.call(ctx, "Runtime.evaluate", params, &res); err != nil {
		return "", err
	}
	if d := res.ExceptionDetails; d != nil {
		msg := d.Text
		if d.Exception != nil && d.Exception.Description != "" {
			msg = d.Exception.Description // the page's own message, e.g. "ReferenceError: x is not defined"
		}
		return "", fmt.Errorf("cdp: evaluate: %w: %s", drivers.ErrScript, msg)
	}
	if res.Result.Type == "string" {
		var str string
		if err := json.Unmarshal(res.Result.Value, &str); err != nil {
			return "", fmt.Errorf("cdp: evaluate: %w", err)
		}
		return str, nil
	}
	if len(res.Result.Value) == 0 {
		return "null", nil // undefined, or a value that cannot be serialized
	}
	return string(res.Result.Value), nil
}

// viewportProbe measures the viewport the page actually got.
const viewportProbe = "JSON.stringify([innerWidth, innerHeight, devicePixelRatio])"

// SetViewport sets the CSS viewport and returns what the page measures.
func (s *Session) SetViewport(ctx context.Context, w, h int, dpr float64) (drivers.Viewport, error) {
	params := map[string]any{"width": w, "height": h, "deviceScaleFactor": dpr, "mobile": false}
	if err := s.call(ctx, "Emulation.setDeviceMetricsOverride", params, nil); err != nil {
		return drivers.Viewport{}, err
	}
	raw, err := s.Evaluate(ctx, viewportProbe)
	if err != nil {
		return drivers.Viewport{}, err
	}
	var m [3]float64
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return drivers.Viewport{}, fmt.Errorf("cdp: viewport: %w", err)
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

// Capabilities reports what a CDP session supports: everything the interface offers.
func (s *Session) Capabilities() drivers.Capabilities {
	return drivers.Capabilities{Preload: true, NetworkIdle: true, FullPageScreenshot: true}
}

// Close closes the page. After a lost connection there is nothing to close.
func (s *Session) Close(ctx context.Context) error {
	defer s.stop()
	_, err := s.c.Call(ctx, "", "Target.closeTarget", map[string]any{"targetId": s.targetID}, nil, nil)
	if errors.Is(err, drivers.ErrCrashed) {
		return nil
	}
	return err
}
