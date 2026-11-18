package cdp

import (
	"context"
	"encoding/json"
	"time"
)

// Child targets: cross-site iframes run in their own renderer process, with
// their own CDP session. Their requests must count for failed requests and
// network idle (ads and embeds are mostly cross-site), as they did through
// Puppeteer in the spike. Without this, the iframe's document request starts
// on the page's session and ends on the iframe's, so the page never looks idle.

// autoAttach makes new child targets wait until they are set up, so their
// first requests are not missed.
var autoAttach = map[string]any{"autoAttach": true, "waitForDebuggerOnStart": true, "flatten": true}

// childSetup bounds the calls that set up one child target.
const childSetup = 10 * time.Second

func (s *Session) addStop(session string, f func()) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		f()
		return
	}
	s.stops[session] = append(s.stops[session], f)
	s.mu.Unlock()
}

// networkMethods are the events network tracking follows, in one stream for
// the page and its children (detaches included, to drop a child's requests in order).
var networkMethods = []string{
	"Network.requestWillBeSent", "Network.loadingFinished", "Network.loadingFailed", "Target.detachedFromTarget",
}

// watchTargets handles the children attached to one CDP session.
func (s *Session) watchTargets(session string) {
	events, stop := s.c.Subscribe(session, "Target.attachedToTarget", "Target.detachedFromTarget")
	s.addStop(session, stop)
	go func() {
		for ev := range events {
			var p struct {
				SessionID string `json:"sessionId"`
			}
			if json.Unmarshal(ev.Params, &p) != nil || p.SessionID == "" {
				continue
			}
			if ev.Method == "Target.attachedToTarget" {
				s.attachChild(p.SessionID)
			} else {
				s.detachChild(p.SessionID)
			}
		}
	}()
}

// attachChild tracks a new child target's network and its own children, then
// lets it run. Every child must be resumed, whatever its type: it waits.
func (s *Session) attachChild(child string) {
	s.c.AddRoute(s.netSub, child)
	s.addStop(child, func() { s.c.RemoveRoute(s.netSub, child) })
	s.watchTargets(child)
	ctx, cancel := context.WithTimeout(context.Background(), childSetup)
	defer cancel()
	// Errors are ignored: a target without a network domain still runs.
	_, _ = s.c.Call(ctx, child, "Network.enable", nil, nil, nil)
	_, _ = s.c.Call(ctx, child, "Target.setAutoAttach", autoAttach, nil, nil)
	_, _ = s.c.Call(ctx, child, "Runtime.runIfWaitingForDebugger", nil, nil, nil)
}

// detachChild ends a child's subscriptions. Its requests in flight are
// dropped by network tracking, which sees the detach in order.
func (s *Session) detachChild(child string) {
	s.mu.Lock()
	fs := s.stops[child]
	delete(s.stops, child)
	s.mu.Unlock()
	for _, f := range fs {
		f()
	}
}
