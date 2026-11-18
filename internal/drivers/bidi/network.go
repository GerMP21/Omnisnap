package bidi

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/GerMP21/Omnisnap/internal/drivers"
	"github.com/GerMP21/Omnisnap/internal/drivers/wsrpc"
)

// networkMethods are the BiDi events that say when a request starts and ends
// (SPEC-drivers.md, "Network idle / failures"). Firefox only emits them after
// session.subscribe names them.
var networkMethods = []string{
	"network.beforeRequestSent",
	"network.responseCompleted",
	"network.fetchError",
}

// Limits on the failed-request list, as in the spike and the CDP driver: enough
// to explain a branded browser's blocking, small enough for every capture file.
const (
	maxFailed    = 300
	maxFailedURL = 200
)

// networkEvents buffers network events: they arrive in bursts.
const networkEvents = 4096

// pollIdle is how often waitIdle looks at the in-flight count.
const pollIdle = 25 * time.Millisecond

// network tracks a page's requests from network.* events: what is in flight
// (for idle) and what failed (blocked requests explain branded browsers).
type network struct {
	mu       sync.Mutex
	urls     map[string]string // request id -> url, while in flight
	last     time.Time         // last request start or end
	failures []drivers.FailedRequest
}

func newNetwork() *network {
	return &network{urls: map[string]string{}, last: time.Now()}
}

// follow consumes network events until the channel closes.
func (n *network) follow(events <-chan wsrpc.Event) {
	for ev := range events {
		var p struct {
			Request struct {
				Request string `json:"request"` // the request id
				URL     string `json:"url"`
			} `json:"request"`
			ErrorText string `json:"errorText"`
		}
		if json.Unmarshal(ev.Params, &p) != nil {
			continue
		}
		id := p.Request.Request
		if id == "" {
			continue
		}
		switch ev.Method {
		case "network.beforeRequestSent":
			n.started(id, p.Request.URL)
		case "network.responseCompleted":
			n.finished(id)
		case "network.fetchError":
			n.failed(id, p.ErrorText)
		}
	}
}

func (n *network) started(id, url string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.urls[id] = url
	n.last = time.Now()
}

func (n *network) finished(id string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	delete(n.urls, id)
	n.last = time.Now()
}

func (n *network) failed(id, errText string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	url, ok := n.urls[id]
	delete(n.urls, id)
	n.last = time.Now()
	if !ok || len(n.failures) >= maxFailed {
		return
	}
	if len(url) > maxFailedURL {
		url = url[:maxFailedURL]
	}
	n.failures = append(n.failures, drivers.FailedRequest{URL: url, Err: errText})
}

// reset starts a new page (on Navigate): it forgets the previous page's
// failures and its requests still in flight. Those end soon after, mostly
// cancelled; unknown ids are ignored, so they are neither reported against the
// new page nor able to hold up its network idle.
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
			return fmt.Errorf("bidi: network not idle within %v: %w", limit, drivers.ErrTimeout)
		}
		select {
		case <-ctx.Done():
			if ctx.Err() == context.DeadlineExceeded {
				return fmt.Errorf("bidi: wait for network idle: %w", drivers.ErrTimeout)
			}
			return ctx.Err()
		case <-tick.C:
		}
	}
}
