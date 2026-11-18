package cdp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/GerMP21/Omnisnap/internal/drivers"
	"github.com/GerMP21/Omnisnap/internal/drivers/wsrpc"
)

// Limits on the failed-request list, as in the spike (capture.mjs): enough to
// explain a branded browser's blocking, small enough for every capture file.
const (
	maxFailed    = 300
	maxFailedURL = 200
)

// networkEvents buffers network events: they arrive in bursts.
const networkEvents = 4096

// network tracks a page's requests from Network.* events: what is in flight
// (for idle) and what failed (blocked requests explain branded browsers).
type network struct {
	mu       sync.Mutex
	urls     map[string]request // requestId -> request, while in flight
	last     time.Time          // last request start or end
	failures []drivers.FailedRequest
}

// request is one request in flight.
type request struct {
	url     string
	session string // the CDP session that reported it (the page, or a child target)
}

func newNetwork() *network {
	return &network{urls: map[string]request{}, last: time.Now()}
}

// follow consumes the network events of the page and its child targets
// (cross-site iframes, workers: each has its own CDP session) until the
// channel closes. One channel, in the browser's order: a child's request and
// the child's detach are seen in the order they happened, and navigation
// requests, which keep their id across sessions, can start in one session and
// end in another.
func (n *network) follow(events <-chan wsrpc.Event) {
	for ev := range events {
		var p struct {
			SessionID     string               `json:"sessionId"` // Target.detachedFromTarget: the child
			RequestID     string               `json:"requestId"`
			Request       struct{ URL string } `json:"request"`
			ErrorText     string               `json:"errorText"`
			BlockedReason string               `json:"blockedReason"`
		}
		if json.Unmarshal(ev.Params, &p) != nil {
			continue
		}
		switch ev.Method {
		case "Network.requestWillBeSent": // also each redirect, same requestId
			n.started(ev.Route, p.RequestID, p.Request.URL)
		case "Network.loadingFinished":
			n.finished(p.RequestID)
		case "Network.loadingFailed":
			n.failed(p.RequestID, p.ErrorText, p.BlockedReason)
		case "Target.detachedFromTarget":
			n.dropSession(p.SessionID)
		}
	}
}

func (n *network) started(session, id, url string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.urls[id] = request{url: url, session: session}
	n.last = time.Now()
}

// dropSession forgets the requests of a detached target: their end will
// never be reported.
func (n *network) dropSession(session string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	for id, r := range n.urls {
		if r.session == session {
			delete(n.urls, id)
		}
	}
	n.last = time.Now()
}

func (n *network) finished(id string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	delete(n.urls, id)
	n.last = time.Now()
}

func (n *network) failed(id, errText, blocked string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	r, ok := n.urls[id]
	delete(n.urls, id)
	n.last = time.Now()
	if !ok || len(n.failures) >= maxFailed {
		return
	}
	url := r.url
	if len(url) > maxFailedURL {
		url = url[:maxFailedURL]
	}
	if blocked != "" {
		errText = fmt.Sprintf("%s (blocked: %s)", errText, blocked)
	}
	n.failures = append(n.failures, drivers.FailedRequest{URL: url, Err: errText})
}

// reset starts a new page (on Navigate): it forgets the previous page's
// failures and its requests still in flight. Those end soon after, mostly
// cancelled (ERR_ABORTED); unknown ids are ignored, so they are neither
// reported against the new page nor able to hold up its network idle.
func (n *network) reset() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.failures = nil
	clear(n.urls)
	n.last = time.Now()
}

func (n *network) failedRequests() []drivers.FailedRequest {
	n.mu.Lock()
	defer n.mu.Unlock()
	return append([]drivers.FailedRequest(nil), n.failures...)
}

// idleFor reports how long nothing has been in flight (0 while something is).
func (n *network) idleFor() time.Duration {
	n.mu.Lock()
	defer n.mu.Unlock()
	if len(n.urls) > 0 {
		return 0
	}
	return time.Since(n.last)
}

// pollIdle is how often waitIdle looks at the in-flight count.
const pollIdle = 25 * time.Millisecond

// waitIdle returns nil once nothing has been in flight for idle, and
// ErrTimeout if that has not happened within limit.
func (n *network) waitIdle(ctx context.Context, idle, limit time.Duration) error {
	deadline := time.Now().Add(limit)
	tick := time.NewTicker(pollIdle)
	defer tick.Stop()
	for {
		if n.idleFor() >= idle {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("cdp: network not idle within %v: %w", limit, drivers.ErrTimeout)
		}
		select {
		case <-ctx.Done():
			if ctx.Err() == context.DeadlineExceeded {
				return fmt.Errorf("cdp: wait for network idle: %w", drivers.ErrTimeout)
			}
			return ctx.Err()
		case <-tick.C:
		}
	}
}
