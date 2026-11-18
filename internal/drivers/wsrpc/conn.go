// Package wsrpc is the websocket transport shared by the CDP and WebDriver
// BiDi drivers: one goroutine owns the reads, calls are matched to replies by
// id, events go to subscribers by route and method. A Codec handles each
// protocol's message format.
package wsrpc

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

// MaxMessage bounds one message. Results are large: extract.js returns up to
// 20,000 nodes and a full-page screenshot is base64 PNG (up to 2560 x 16000 px).
const MaxMessage = 1 << 30

// EventBuffer is how many unread events a subscription holds before newer
// ones are dropped. Subscribers wait for one or a few events.
const EventBuffer = 64

// Event is a protocol event. Route is what events are delivered by: the CDP
// sessionId; empty for BiDi, whose events name their browsing context in Params.
type Event struct {
	Method string
	Route  string
	Params json.RawMessage
}

// Reply is the answer to one call.
type Reply struct {
	Result json.RawMessage
	Err    error // the protocol's error, with its message
}

// Codec is one protocol's message format.
type Codec interface {
	// Name prefixes errors ("cdp", "bidi").
	Name() string
	// Encode builds the message for a call.
	Encode(id int64, route, method string, params any) ([]byte, error)
	// Decode classifies a message: a reply (id > 0) or an event. ok is false
	// for anything else, which is ignored.
	Decode(b []byte) (id int64, reply Reply, ev Event, ok bool)
}

// Conn is one websocket to a browser.
type Conn struct {
	ws     *websocket.Conn
	codec  Codec
	cancel context.CancelFunc // stops the reader
	done   chan struct{}      // closed when the reader exits

	mu      sync.Mutex
	nextID  int64
	pending map[int64]chan Reply
	subs    map[*Subscription]struct{}
	err     error // why the connection ended; set before done closes
}

// Subscription receives the events of some routes and methods.
type Subscription struct {
	routes  map[string]bool
	methods map[string]bool
	ch      chan Event
}

// C is the subscription's channel, closed by its cancel func or when the
// connection ends.
func (s *Subscription) C() <-chan Event { return s.ch }

// Dial connects to url.
func Dial(ctx context.Context, url string, codec Codec) (*Conn, error) {
	ws, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		return nil, fmt.Errorf("%s: dial %s: %w", codec.Name(), url, err)
	}
	ws.SetReadLimit(MaxMessage)
	rctx, cancel := context.WithCancel(context.Background())
	c := &Conn{
		ws:      ws,
		codec:   codec,
		cancel:  cancel,
		done:    make(chan struct{}),
		pending: map[int64]chan Reply{},
		subs:    map[*Subscription]struct{}{},
	}
	go c.read(rctx)
	return c, nil
}

func (c *Conn) read(ctx context.Context) {
	var err error
	defer func() { c.shutdown(err) }()
	for {
		var b []byte
		_, b, err = c.ws.Read(ctx)
		if err != nil {
			return
		}
		id, reply, ev, ok := c.codec.Decode(b)
		if !ok {
			continue // not a protocol message: ignore rather than kill the session
		}
		if id != 0 {
			c.mu.Lock()
			ch := c.pending[id]
			delete(c.pending, id)
			c.mu.Unlock()
			if ch != nil {
				ch <- reply // buffered: never blocks
			}
			continue
		}
		c.mu.Lock()
		for s := range c.subs {
			if s.routes[ev.Route] && s.methods[ev.Method] {
				select {
				case s.ch <- ev:
				default: // a subscriber that stopped reading must not stall the connection
				}
			}
		}
		c.mu.Unlock()
	}
}

// shutdown ends the connection: pending calls and subscriptions see it through done.
func (c *Conn) shutdown(cause error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return
	}
	c.err = fmt.Errorf("%s: connection closed: %w (%v)", c.codec.Name(), drivers.ErrCrashed, cause)
	for s := range c.subs {
		close(s.ch)
		delete(c.subs, s)
	}
	close(c.done)
}

// Err is why the connection ended (nil while it is open).
func (c *Conn) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

// Call sends method and waits for its result. abort, when it closes, fails
// the call with abortErr (a crashed page never answers); both may be nil.
func (c *Conn) Call(ctx context.Context, route, method string, params any, abort <-chan struct{}, abortErr func() error) (json.RawMessage, error) {
	c.mu.Lock()
	if c.err != nil {
		err := c.err
		c.mu.Unlock()
		return nil, err
	}
	c.nextID++
	id := c.nextID
	ch := make(chan Reply, 1)
	c.pending[id] = ch
	c.mu.Unlock()
	forget := func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}

	b, err := c.codec.Encode(id, route, method, params)
	if err != nil {
		forget()
		return nil, fmt.Errorf("%s: %s: %w", c.codec.Name(), method, err)
	}
	if err := c.ws.Write(ctx, websocket.MessageText, b); err != nil {
		forget()
		return nil, c.CtxErr(ctx, method, err)
	}
	select {
	case r := <-ch:
		if r.Err != nil {
			return nil, fmt.Errorf("%s: %s: %w", c.codec.Name(), method, r.Err)
		}
		return r.Result, nil
	case <-ctx.Done():
		forget()
		return nil, c.CtxErr(ctx, method, ctx.Err())
	case <-abort:
		forget()
		return nil, abortErr()
	case <-c.done:
		return nil, c.Err()
	}
}

// CtxErr maps a failure to the typed errors: a deadline is ErrTimeout, a
// closed connection ErrCrashed.
func (c *Conn) CtxErr(ctx context.Context, what string, err error) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("%s: %s: %w", c.codec.Name(), what, drivers.ErrTimeout)
	}
	if ctx.Err() != nil {
		return fmt.Errorf("%s: %s: %w", c.codec.Name(), what, ctx.Err())
	}
	select {
	case <-c.done:
		return c.Err()
	default:
		return fmt.Errorf("%s: %s: %w", c.codec.Name(), what, err)
	}
}

// Subscribe delivers route's events of the given methods until cancel.
func (c *Conn) Subscribe(route string, methods ...string) (<-chan Event, func()) {
	s, cancel := c.SubscribeBuffered(route, EventBuffer, methods...)
	return s.ch, cancel
}

// SubscribeBuffered is Subscribe with a buffer of n events, for a subscriber
// that must not miss any (network tracking: a page can start hundreds of
// requests at once). More routes can join it (AddRoute): their events then
// arrive on one channel in the order the browser sent them.
func (c *Conn) SubscribeBuffered(route string, n int, methods ...string) (*Subscription, func()) {
	s := &Subscription{routes: map[string]bool{route: true}, methods: map[string]bool{}, ch: make(chan Event, n)}
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
	return s, func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		if _, ok := c.subs[s]; ok {
			delete(c.subs, s)
			close(s.ch)
		}
	}
}

// AddRoute adds a route's events to subscription s.
func (c *Conn) AddRoute(s *Subscription, route string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	s.routes[route] = true
}

// RemoveRoute stops delivering a route's events to s. Events already queued
// are still delivered.
func (c *Conn) RemoveRoute(s *Subscription, route string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(s.routes, route)
}

// Close ends the connection and waits for the reader to exit. A connection
// the browser already dropped only has its socket released.
func (c *Conn) Close() error {
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
