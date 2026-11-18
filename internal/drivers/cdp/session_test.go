package cdp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/GerMP21/Omnisnap/internal/drivers"
	"github.com/GerMP21/Omnisnap/internal/drivers/replaytest"
)

const transcripts = "../../../testdata/drivers/transcripts/cdp"

// open dials the replayer and opens a session, as capture would.
func open(t *testing.T, r *replaytest.Replayer) (*Browser, *Session) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	b, err := Dial(ctx, r.URL())
	if err != nil {
		t.Fatal(err)
	}
	s, err := b.NewSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return b, s
}

func closeAll(t *testing.T, r *replaytest.Replayer, b *Browser, s *Session) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.Close(ctx); err != nil {
		t.Errorf("session close: %v", err)
	}
	if err := b.Close(); err != nil {
		t.Errorf("browser close: %v", err)
	}
	r.Consumed()
}

func ctxFor(t *testing.T, d time.Duration) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), d)
	t.Cleanup(cancel)
	return ctx
}

func TestNavigateEvaluateViewport(t *testing.T) {
	r := replaytest.Replay(t, transcripts, "navigate-evaluate.jsonl")
	b, s := open(t, r)
	ctx := ctxFor(t, 5*time.Second)

	if err := s.Navigate(ctx, "https://example.com/"); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Evaluate(ctx, "1 + 1"); err != nil || got != "2" {
		t.Errorf("Evaluate(1 + 1) = %q, %v; want 2 (non-strings as JSON)", got, err)
	}
	if got, err := s.Evaluate(ctx, "Promise.resolve(JSON.stringify({a: 1}))"); err != nil || got != `{"a":1}` {
		t.Errorf("Evaluate(promise) = %q, %v; want the string as is", got, err)
	}
	vp, err := s.SetViewport(ctx, 1280, 800, 1)
	if err != nil || vp != (drivers.Viewport{Width: 1280, Height: 800, DPR: 1}) {
		t.Errorf("SetViewport = %+v, %v; want 1280x800 at 1", vp, err)
	}
	closeAll(t, r, b, s)
}

func TestEvaluateScriptError(t *testing.T) {
	r := replaytest.Replay(t, transcripts, "script-error.jsonl")
	b, s := open(t, r)
	_, err := s.Evaluate(ctxFor(t, 5*time.Second), "nope()")
	if !errors.Is(err, drivers.ErrScript) || !strings.Contains(err.Error(), "ReferenceError: nope is not defined") {
		t.Errorf("err = %v; want ErrScript with the page's message", err)
	}
	closeAll(t, r, b, s)
}

func TestNavigateTimeout(t *testing.T) {
	r := replaytest.Replay(t, transcripts, "navigate-timeout.jsonl")
	b, s := open(t, r)
	err := s.Navigate(ctxFor(t, 200*time.Millisecond), "https://example.com/slow")
	if !errors.Is(err, drivers.ErrTimeout) {
		t.Errorf("err = %v; want ErrTimeout", err)
	}
	closeAll(t, r, b, s)
}

func TestNavigateError(t *testing.T) {
	r := replaytest.Replay(t, transcripts, "navigate-error.jsonl")
	b, s := open(t, r)
	err := s.Navigate(ctxFor(t, 5*time.Second), "http://127.0.0.1:59999/")
	if !errors.Is(err, drivers.ErrNavigation) || !strings.Contains(err.Error(), "ERR_CONNECTION_REFUSED") {
		t.Errorf("err = %v; want ErrNavigation with the engine's error text", err)
	}
	closeAll(t, r, b, s)
}

func TestCrashFailsThePendingCall(t *testing.T) {
	r := replaytest.Replay(t, transcripts, "crash.jsonl")
	b, s := open(t, r)
	_, err := s.Evaluate(ctxFor(t, 5*time.Second), "while (true) {}")
	if !errors.Is(err, drivers.ErrCrashed) {
		t.Errorf("err = %v; want ErrCrashed", err)
	}
	// Later calls fail fast instead of waiting for their deadline.
	if _, err := s.Evaluate(ctxFor(t, 5*time.Second), "1"); !errors.Is(err, drivers.ErrCrashed) {
		t.Errorf("after the crash: err = %v; want ErrCrashed", err)
	}
	closeAll(t, r, b, s)
}

func TestProtocolErrorKeepsTheMessage(t *testing.T) {
	r := replaytest.Replay(t, transcripts, "protocol-error.jsonl")
	b, s := open(t, r)
	_, err := s.SetViewport(ctxFor(t, 5*time.Second), -1, 800, 1)
	if err == nil || !strings.Contains(err.Error(), "Invalid parameters") || !strings.Contains(err.Error(), "int32 value expected") {
		t.Errorf("err = %v; want the protocol's message and data", err)
	}
	closeAll(t, r, b, s)
}

func TestConnectionLost(t *testing.T) {
	r := replaytest.Replay(t, transcripts, "connection-lost.jsonl")
	b, s := open(t, r)
	_, err := s.Evaluate(ctxFor(t, 5*time.Second), "location.href")
	if !errors.Is(err, drivers.ErrCrashed) {
		t.Errorf("err = %v; want ErrCrashed when the browser connection drops", err)
	}
	if err := b.Close(); err != nil {
		t.Errorf("close after loss: %v", err)
	}
	r.Consumed()
}

// No goroutine outlives Close (a crude check until goleak is approved, Task 22).
func TestNoGoroutineLeak(t *testing.T) {
	before := runtime.NumGoroutine()
	for range 3 {
		r := replaytest.Replay(t, transcripts, "navigate-evaluate.jsonl")
		b, s := open(t, r)
		ctx := ctxFor(t, 5*time.Second)
		if err := s.Navigate(ctx, "https://example.com/"); err != nil {
			t.Fatal(err)
		}
		_, _ = s.Evaluate(ctx, "1 + 1")
		_, _ = s.Evaluate(ctx, "Promise.resolve(JSON.stringify({a: 1}))")
		_, _ = s.SetViewport(ctx, 1280, 800, 1)
		closeAll(t, r, b, s)
		r.Close()
	}
	deadline := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > before && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if n := runtime.NumGoroutine(); n > before {
		buf := make([]byte, 1<<16)
		t.Errorf("%d goroutines after Close, %d before\n%s", n, before, buf[:runtime.Stack(buf, true)])
	}
}

func TestScreenshot(t *testing.T) {
	r := replaytest.Replay(t, transcripts, "screenshot.jsonl")
	b, s := open(t, r)
	png, err := s.Screenshot(ctxFor(t, 5*time.Second), drivers.Rect{Width: 2, Height: 3})
	if err != nil {
		t.Fatal(err)
	}
	img, err := decodePNG(png)
	if err != nil || img.Dx() != 2 || img.Dy() != 3 {
		t.Errorf("screenshot: %v, %v; want a decoded 2x3 PNG", img, err)
	}
	closeAll(t, r, b, s)
}

func TestPreload(t *testing.T) {
	r := replaytest.Replay(t, transcripts, "preload.jsonl")
	b, s := open(t, r)
	if err := s.AddPreloadScript(ctxFor(t, 5*time.Second), "window.__omnisnap = 1"); err != nil {
		t.Fatal(err)
	}
	closeAll(t, r, b, s)
}

func TestNetworkFailedRequestsAndIdle(t *testing.T) {
	r := replaytest.Replay(t, transcripts, "network.jsonl")
	b, s := open(t, r)
	ctx := ctxFor(t, 5*time.Second)
	if err := s.Navigate(ctx, "https://example.com/"); err != nil {
		t.Fatal(err)
	}
	// Every request in the transcript has ended: idle after 50 ms of quiet.
	if err := s.WaitNetworkIdle(ctx, 50*time.Millisecond, 2*time.Second); err != nil {
		t.Fatalf("WaitNetworkIdle: %v", err)
	}
	got := s.FailedRequests()
	want := []drivers.FailedRequest{
		{URL: "https://ads.example.net/tag.js", Err: "net::ERR_BLOCKED_BY_CLIENT (blocked: inspector)"},
		{URL: "https://example.com/api/refused", Err: "net::ERR_EMPTY_RESPONSE"},
	}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("FailedRequests = %+v\nwant %+v", got, want)
	}
	if c := s.Capabilities(); !c.Preload || !c.NetworkIdle || !c.FullPageScreenshot {
		t.Errorf("Capabilities = %+v; CDP supports all", c)
	}
	closeAll(t, r, b, s)
}

func TestWaitNetworkIdleTimesOut(t *testing.T) {
	n := newNetwork()
	n.started("S1", "R1", "https://example.com/never")
	err := n.waitIdle(context.Background(), 20*time.Millisecond, 100*time.Millisecond)
	if !errors.Is(err, drivers.ErrTimeout) {
		t.Errorf("err = %v; want ErrTimeout while a request stays in flight", err)
	}
}

func TestFailedRequestsAreCapped(t *testing.T) {
	n := newNetwork()
	long := "https://example.com/" + strings.Repeat("x", 300)
	for i := range maxFailed + 10 {
		id := fmt.Sprint(i)
		n.started("S1", id, long)
		n.failed(id, "net::ERR_FAILED", "")
	}
	got := n.failedRequests()
	if len(got) != maxFailed || len(got[0].URL) != maxFailedURL {
		t.Errorf("%d failed requests, URL length %d; want %d and %d (spike limits)", len(got), len(got[0].URL), maxFailed, maxFailedURL)
	}
}

func decodePNG(b []byte) (image.Rectangle, error) {
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		return image.Rectangle{}, err
	}
	return img.Bounds(), nil
}

// Review finding 6: requests of the previous page that end after a new
// Navigate (cancelled: ERR_ABORTED) belong to no page: not reported as
// failures of the new one, and never holding up its network idle.
func TestNavigateForgetsThePreviousPagesRequests(t *testing.T) {
	n := newNetwork()
	n.started("S1", "R9", "https://example.com/old/stream")
	n.reset()
	n.failed("R9", "net::ERR_ABORTED", "")
	if got := n.failedRequests(); len(got) != 0 {
		t.Errorf("FailedRequests = %+v; want none from the previous page", got)
	}
	n.started("S1", "R8", "https://example.com/old/longpoll")
	n.reset()
	if err := n.waitIdle(context.Background(), 20*time.Millisecond, time.Second); err != nil {
		t.Errorf("WaitNetworkIdle: %v; a previous page's request must not block idle", err)
	}
}

// Review finding 5: child targets (cross-site iframes) are set up, then
// resumed; their requests count, and a detached child's do not linger.
func TestChildTargets(t *testing.T) {
	r := replaytest.Replay(t, transcripts, "oopif.jsonl")
	b, s := open(t, r)
	ctx := ctxFor(t, 5*time.Second)
	if err := s.Navigate(ctx, "https://example.com/"); err != nil {
		t.Fatal(err)
	}
	// Idle only once the iframe's document ended on its own session, and
	// despite the detached iframe's long poll.
	if err := s.WaitNetworkIdle(ctx, 100*time.Millisecond, 3*time.Second); err != nil {
		t.Fatalf("WaitNetworkIdle: %v", err)
	}
	got := s.FailedRequests()
	if len(got) != 1 || got[0].URL != "https://ads.example.net/pixel.gif" {
		t.Errorf("FailedRequests = %+v; want the iframe's blocked pixel", got)
	}
	closeAll(t, r, b, s)
}
