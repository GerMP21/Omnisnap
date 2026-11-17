package cdp

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/GerMP21/Omnisnap/internal/drivers"
)

// open dials the replayer and opens a session, as capture would.
func open(t *testing.T, r *replayer) (*Browser, *Session) {
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

func closeAll(t *testing.T, r *replayer, b *Browser, s *Session) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.Close(ctx); err != nil {
		t.Errorf("session close: %v", err)
	}
	if err := b.Close(); err != nil {
		t.Errorf("browser close: %v", err)
	}
	r.consumed()
}

func ctxFor(t *testing.T, d time.Duration) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), d)
	t.Cleanup(cancel)
	return ctx
}

func TestNavigateEvaluateViewport(t *testing.T) {
	r := replay(t, "navigate-evaluate.jsonl")
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
	r := replay(t, "script-error.jsonl")
	b, s := open(t, r)
	_, err := s.Evaluate(ctxFor(t, 5*time.Second), "nope()")
	if !errors.Is(err, drivers.ErrScript) || !strings.Contains(err.Error(), "ReferenceError: nope is not defined") {
		t.Errorf("err = %v; want ErrScript with the page's message", err)
	}
	closeAll(t, r, b, s)
}

func TestNavigateTimeout(t *testing.T) {
	r := replay(t, "navigate-timeout.jsonl")
	b, s := open(t, r)
	err := s.Navigate(ctxFor(t, 200*time.Millisecond), "https://example.com/slow")
	if !errors.Is(err, drivers.ErrTimeout) {
		t.Errorf("err = %v; want ErrTimeout", err)
	}
	closeAll(t, r, b, s)
}

func TestNavigateError(t *testing.T) {
	r := replay(t, "navigate-error.jsonl")
	b, s := open(t, r)
	err := s.Navigate(ctxFor(t, 5*time.Second), "http://127.0.0.1:59999/")
	if !errors.Is(err, drivers.ErrNavigation) || !strings.Contains(err.Error(), "ERR_CONNECTION_REFUSED") {
		t.Errorf("err = %v; want ErrNavigation with the engine's error text", err)
	}
	closeAll(t, r, b, s)
}

func TestCrashFailsThePendingCall(t *testing.T) {
	r := replay(t, "crash.jsonl")
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
	r := replay(t, "protocol-error.jsonl")
	b, s := open(t, r)
	_, err := s.SetViewport(ctxFor(t, 5*time.Second), -1, 800, 1)
	if err == nil || !strings.Contains(err.Error(), "Invalid parameters") || !strings.Contains(err.Error(), "int32 value expected") {
		t.Errorf("err = %v; want the protocol's message and data", err)
	}
	closeAll(t, r, b, s)
}

func TestConnectionLost(t *testing.T) {
	r := replay(t, "connection-lost.jsonl")
	b, s := open(t, r)
	_, err := s.Evaluate(ctxFor(t, 5*time.Second), "location.href")
	if !errors.Is(err, drivers.ErrCrashed) {
		t.Errorf("err = %v; want ErrCrashed when the browser connection drops", err)
	}
	if err := b.Close(); err != nil {
		t.Errorf("close after loss: %v", err)
	}
	r.consumed()
}

// No goroutine outlives Close (a crude check until goleak is approved, Task 22).
func TestNoGoroutineLeak(t *testing.T) {
	before := runtime.NumGoroutine()
	for range 3 {
		r := replay(t, "navigate-evaluate.jsonl")
		b, s := open(t, r)
		ctx := ctxFor(t, 5*time.Second)
		if err := s.Navigate(ctx, "https://example.com/"); err != nil {
			t.Fatal(err)
		}
		_, _ = s.Evaluate(ctx, "1 + 1")
		_, _ = s.Evaluate(ctx, "Promise.resolve(JSON.stringify({a: 1}))")
		_, _ = s.SetViewport(ctx, 1280, 800, 1)
		closeAll(t, r, b, s)
		r.srv.Close()
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
