// Package cdp drives Blink browsers (Chrome, Chromium, Edge, Brave, Opera,
// Vivaldi) over the Chrome DevTools Protocol, directly: no chromedriver.
package cdp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"

	"github.com/coder/websocket"

	"github.com/GerMP21/Omnisnap/internal/drivers"
)

// maxMessage bounds one CDP message. Results are large: extract.js returns up
// to 20,000 nodes and a full-page screenshot is base64 PNG (up to 2560 x 16000 px).
const maxMessage = 1 << 30

// eventBuffer is how many unread events a subscription holds before newer
// ones are dropped. Subscribers wait for one or a few events.
const eventBuffer = 64

// conn is one websocket to a browser. One goroutine owns the reads; calls are
// matched to responses by id; events go to subscribers by session and method.
type conn struct {
	ws     *websocket.Conn
	cancel context.CancelFunc // stops the reader
	done   chan struct{}      // closed when the reader exits

	mu      sync.Mutex
	nextID  int64
	pending map[int64]chan response
	subs    map[*subscription]struct{}
	err     error // why the connection ended; set before done closes
}

type response struct {
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    string `json:"data"`
}

func (e *rpcError) Error() string {
	if e.Data != "" {
		return fmt.Sprintf("%s (%d): %s", e.Message, e.Code, e.Data)
	}
	return fmt.Sprintf("%s (%d)", e.Message, e.Code)
}

// event is a CDP event.
type event struct {
	Method    string          `json:"method"`
	SessionID string          `json:"sessionId"`
	Params    json.RawMessage `json:"params"`
}

type subscription struct {
	sessionID string
	methods   map[string]bool
	ch        chan event
}

func dial(ctx context.Context, url string) (*conn, error) {
	ws, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		return nil, fmt.Errorf("cdp: dial %s: %w", url, err)
	}
	ws.SetReadLimit(maxMessage)
	rctx, cancel := context.WithCancel(context.Background())
	c := &conn{
		ws:      ws,
		cancel:  cancel,
		done:    make(chan struct{}),
		pending: map[int64]chan response{},
		subs:    map[*subscription]struct{}{},
	}
	go c.read(rctx)
	return c, nil
}

func (c *conn) read(ctx context.Context) {
	var err error
	defer func() { c.shutdown(err) }()
	for {
		var b []byte
		_, b, err = c.ws.Read(ctx)
		if err != nil {
			return
		}
		var m struct {
			ID int64 `json:"id"`
			response
			event
		}
		if json.Unmarshal(b, &m) != nil {
			continue // not a CDP message: ignore rather than kill the session
		}
		if m.ID != 0 {
			c.mu.Lock()
			ch := c.pending[m.ID]
			delete(c.pending, m.ID)
			c.mu.Unlock()
			if ch != nil {
				ch <- m.response // buffered: never blocks
			}
			continue
		}
		c.mu.Lock()
		for s := range c.subs {
			if s.sessionID == m.SessionID && s.methods[m.Method] {
				select {
				case s.ch <- m.event:
				default: // a subscriber that stopped reading must not stall the connection
				}
			}
		}
		c.mu.Unlock()
	}
}

// shutdown ends the connection: pending calls and subscriptions see it through done.
func (c *conn) shutdown(cause error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return
	}
	c.err = fmt.Errorf("cdp: connection closed: %w (%v)", drivers.ErrCrashed, cause)
	for s := range c.subs {
		close(s.ch)
		delete(c.subs, s)
	}
	close(c.done)
}

// call sends method and waits for its result. abort, when it closes, fails the
// call with abortErr (a crashed page never answers).
func (c *conn) call(ctx context.Context, sessionID, method string, params any, abort <-chan struct{}, abortErr func() error) (json.RawMessage, error) {
	c.mu.Lock()
	if c.err != nil {
		err := c.err
		c.mu.Unlock()
		return nil, err
	}
	c.nextID++
	id := c.nextID
	ch := make(chan response, 1)
	c.pending[id] = ch
	c.mu.Unlock()
	forget := func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}

	msg := map[string]any{"id": id, "method": method}
	if params != nil {
		msg["params"] = params
	}
	if sessionID != "" {
		msg["sessionId"] = sessionID
	}
	b, err := json.Marshal(msg)
	if err != nil {
		forget()
		return nil, fmt.Errorf("cdp: %s: %w", method, err)
	}
	if err := c.ws.Write(ctx, websocket.MessageText, b); err != nil {
		forget()
		return nil, c.ctxErr(ctx, method, err)
	}
	select {
	case r := <-ch:
		if r.Error != nil {
			return nil, fmt.Errorf("cdp: %s: %w", method, r.Error)
		}
		return r.Result, nil
	case <-ctx.Done():
		forget()
		return nil, c.ctxErr(ctx, method, ctx.Err())
	case <-abort:
		forget()
		return nil, abortErr()
	case <-c.done:
		return nil, c.err
	}
}

// ctxErr maps a failure to the typed errors: a deadline is ErrTimeout, a
// closed connection ErrCrashed.
func (c *conn) ctxErr(ctx context.Context, method string, err error) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("cdp: %s: %w", method, drivers.ErrTimeout)
	}
	if ctx.Err() != nil {
		return fmt.Errorf("cdp: %s: %w", method, ctx.Err())
	}
	select {
	case <-c.done:
		return c.err
	default:
		return fmt.Errorf("cdp: %s: %w", method, err)
	}
}

// subscribe delivers sessionID's events of the given methods until cancel.
// The channel is closed by cancel or when the connection ends.
func (c *conn) subscribe(sessionID string, methods ...string) (<-chan event, func()) {
	return c.subscribeBuffered(sessionID, eventBuffer, methods...)
}

// subscribeBuffered is subscribe with a buffer of n events, for a subscriber
// that must not miss any (network tracking: a page can start hundreds of
// requests at once).
func (c *conn) subscribeBuffered(sessionID string, n int, methods ...string) (<-chan event, func()) {
	s := &subscription{sessionID: sessionID, methods: map[string]bool{}, ch: make(chan event, n)}
	for _, m := range methods {
		s.methods[m] = true
	}
	c.mu.Lock()
	if c.err != nil {
		close(s.ch)
	} else {
		c.subs[s] = struct{}{}
	}
	c.mu.Unlock()
	return s.ch, func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		if _, ok := c.subs[s]; ok {
			delete(c.subs, s)
			close(s.ch)
		}
	}
}

// close ends the connection and waits for the reader to exit. A connection
// the browser already dropped only has its socket released.
func (c *conn) close() error {
	var err error
	select {
	case <-c.done:
		err = c.ws.CloseNow()
	default:
		err = c.ws.Close(websocket.StatusNormalClosure, "")
	}
	c.cancel()
	<-c.done
	if err == nil || errors.Is(err, net.ErrClosed) || websocket.CloseStatus(err) != -1 {
		return nil
	}
	return err
}
