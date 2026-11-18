package bidi

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/GerMP21/Omnisnap/internal/drivers"
	"github.com/GerMP21/Omnisnap/internal/drivers/replaytest"
)

const transcripts = "../../../testdata/drivers/transcripts/bidi"

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
	if err := b.Close(ctx); err != nil {
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
	// In the transcript's order (not a map: its iteration order is random).
	for _, c := range []struct{ expr, want string }{
		{"1 + 1", "2"},
		{"Promise.resolve(JSON.stringify({a: 1}))", `{"a":1}`},
	} {
		if got, err := s.Evaluate(ctx, c.expr); err != nil || got != c.want {
			t.Errorf("Evaluate(%s) = %q, %v; want %q", c.expr, got, err, c.want)
		}
	}
	// BiDi serializes objects as key/value pairs; Evaluate returns JSON like CDP.
	if got, err := s.Evaluate(ctx, "({x: [1, 'a'], y: null})"); err != nil || got != `{"x":[1,"a"],"y":null}` {
		t.Errorf("Evaluate(object) = %q, %v", got, err)
	}
	vp, err := s.SetViewport(ctx, 1280, 800, 1)
	if err != nil || vp != (drivers.Viewport{Width: 1280, Height: 800, DPR: 1}) {
		t.Errorf("SetViewport = %+v, %v", vp, err)
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
	if err := s.Navigate(ctxFor(t, 200*time.Millisecond), "https://example.com/slow"); !errors.Is(err, drivers.ErrTimeout) {
		t.Errorf("err = %v; want ErrTimeout", err)
	}
	closeAll(t, r, b, s)
}

func TestNavigateError(t *testing.T) {
	r := replaytest.Replay(t, transcripts, "navigate-error.jsonl")
	b, s := open(t, r)
	err := s.Navigate(ctxFor(t, 5*time.Second), "http://127.0.0.1:59999/")
	if !errors.Is(err, drivers.ErrNavigation) || !strings.Contains(err.Error(), "NS_ERROR_CONNECTION_REFUSED") {
		t.Errorf("err = %v; want ErrNavigation with the engine's error", err)
	}
	closeAll(t, r, b, s)
}

func TestProtocolErrorKeepsTheMessage(t *testing.T) {
	r := replaytest.Replay(t, transcripts, "protocol-error.jsonl")
	b, s := open(t, r)
	_, err := s.SetViewport(ctxFor(t, 5*time.Second), -1, 800, 1)
	if err == nil || !strings.Contains(err.Error(), "invalid argument") || !strings.Contains(err.Error(), "positive integer") {
		t.Errorf("err = %v; want the protocol's error code and message", err)
	}
	closeAll(t, r, b, s)
}

func TestConnectionLost(t *testing.T) {
	r := replaytest.Replay(t, transcripts, "connection-lost.jsonl")
	b, s := open(t, r)
	if _, err := s.Evaluate(ctxFor(t, 5*time.Second), "location.href"); !errors.Is(err, drivers.ErrCrashed) {
		t.Errorf("err = %v; want ErrCrashed", err)
	}
	if err := b.Close(ctxFor(t, 5*time.Second)); err != nil {
		t.Errorf("close after loss: %v", err)
	}
	r.Consumed()
}

func TestRemoteValueToJSON(t *testing.T) {
	for in, want := range map[string]string{
		`{"type":"undefined"}`:                                   `null`,
		`{"type":"null"}`:                                        `null`,
		`{"type":"boolean","value":true}`:                        `true`,
		`{"type":"number","value":2.5}`:                          `2.5`,
		`{"type":"number","value":"NaN"}`:                        `null`,
		`{"type":"number","value":"-0"}`:                         `0`,
		`{"type":"bigint","value":"9007199254740993"}`:           `"9007199254740993"`,
		`{"type":"string","value":"a\"b"}`:                       `"a\"b"`,
		`{"type":"array","value":[{"type":"number","value":1}]}`: `[1]`,
		`{"type":"map","value":[[{"type":"number","value":1},{"type":"string","value":"x"}]]}`: `{"1":"x"}`,
		`{"type":"node","sharedId":"x"}`: `null`,
	} {
		got, err := remoteJSON([]byte(in))
		if err != nil || got != want {
			t.Errorf("remoteJSON(%s) = %s, %v; want %s", in, got, err, want)
		}
	}
}

func TestNoGoroutineLeak(t *testing.T) {
	before := runtime.NumGoroutine()
	for range 3 {
		r := replaytest.Replay(t, transcripts, "navigate-evaluate.jsonl")
		b, s := open(t, r)
		ctx := ctxFor(t, 5*time.Second)
		_ = s.Navigate(ctx, "https://example.com/")
		_, _ = s.Evaluate(ctx, "1 + 1")
		_, _ = s.Evaluate(ctx, "Promise.resolve(JSON.stringify({a: 1}))")
		_, _ = s.Evaluate(ctx, "({x: [1, 'a'], y: null})")
		_, _ = s.SetViewport(ctx, 1280, 800, 1)
		closeAll(t, r, b, s)
		r.Close()
	}
	deadline := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > before && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if n := runtime.NumGoroutine(); n > before {
		t.Errorf("%d goroutines after Close, %d before", n, before)
	}
}
