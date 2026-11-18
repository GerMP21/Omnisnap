package classic

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GerMP21/Omnisnap/internal/drivers"
)

const transcripts = "../../../testdata/drivers/transcripts/classic"

// step is one line of a recorded WebDriver Classic exchange: the request the
// client must send and the driver's reply.
type step struct {
	Method string          `json:"method"`
	Path   string          `json:"path"`
	Body   json.RawMessage `json:"body,omitempty"` // optional: the request body to expect
	Status int             `json:"status"`
	Value  json.RawMessage `json:"value"`
}

// driverServer replays a transcript over HTTP.
type driverServer struct {
	t     *testing.T
	steps []step
	mu    sync.Mutex
	pos   int
	srv   *httptest.Server
}

func replay(t *testing.T, name string) (*driverServer, *Client) {
	t.Helper()
	f, err := os.Open(filepath.Join(transcripts, name))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	var steps []step
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var s step
		if err := json.Unmarshal([]byte(line), &s); err != nil {
			t.Fatalf("%s: %v: %s", name, err, line)
		}
		steps = append(steps, s)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	d := &driverServer{t: t, steps: steps}
	d.srv = httptest.NewServer(http.HandlerFunc(d.serve))
	t.Cleanup(d.srv.Close)
	return d, New(d.srv.URL)
}

func (d *driverServer) serve(w http.ResponseWriter, r *http.Request) {
	var body bytes.Buffer
	_, _ = body.ReadFrom(r.Body)
	d.mu.Lock()
	if d.pos >= len(d.steps) {
		d.mu.Unlock()
		d.t.Errorf("driver got a request past the end of the transcript: %s %s", r.Method, r.URL.Path)
		http.Error(w, `{"value":{"error":"unknown command"}}`, http.StatusNotFound)
		return
	}
	s := d.steps[d.pos]
	d.pos++
	d.mu.Unlock()
	if r.Method != s.Method || r.URL.Path != s.Path {
		d.t.Errorf("step %d: driver got %s %s; want %s %s", d.pos-1, r.Method, r.URL.Path, s.Method, s.Path)
	}
	if len(s.Body) > 0 {
		var got, want any
		_ = json.Unmarshal(body.Bytes(), &got)
		_ = json.Unmarshal(s.Body, &want)
		if !sameJSON(got, want) {
			d.t.Errorf("step %d: body\n  %s\nwant\n  %s", d.pos-1, body.Bytes(), s.Body)
		}
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(s.Status)
	if s.Value == nil {
		_, _ = w.Write([]byte(`{"value":null}`))
		return
	}
	_, _ = w.Write([]byte(`{"value":` + string(s.Value) + `}`))
}

func sameJSON(a, b any) bool {
	ab, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	return bytes.Equal(ab, bb)
}

// consumed fails unless every recorded exchange was used.
func (d *driverServer) consumed() {
	d.t.Helper()
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.pos != len(d.steps) {
		d.t.Errorf("transcript stopped at step %d of %d", d.pos, len(d.steps))
	}
}

func ctxFor(t *testing.T, d time.Duration) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), d)
	t.Cleanup(cancel)
	return ctx
}

func TestNewSessionAndClose(t *testing.T) {
	d, c := replay(t, "session.jsonl")
	s, err := c.NewSession(ctxFor(t, 5*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Browser(); got != "MiniBrowser 2.48" {
		t.Errorf("Browser() = %q", got)
	}
	if c := s.Capabilities(); c.Preload || c.NetworkIdle || c.FullPageScreenshot {
		t.Errorf("Capabilities = %+v; Classic supports none of them", c)
	}
	if err := s.AddPreloadScript(context.Background(), "x"); !errors.Is(err, drivers.ErrUnsupported) {
		t.Errorf("AddPreloadScript = %v; want ErrUnsupported", err)
	}
	if err := s.WaitNetworkIdle(context.Background(), time.Millisecond, time.Millisecond); !errors.Is(err, drivers.ErrUnsupported) {
		t.Errorf("WaitNetworkIdle = %v; want ErrUnsupported", err)
	}
	if got := s.FailedRequests(); got != nil {
		t.Errorf("FailedRequests = %v; Classic reports nothing", got)
	}
	// 1280x800 at the first measure: no resize needed.
	if err := s.Close(ctxFor(t, 5*time.Second)); err != nil {
		t.Errorf("close: %v", err)
	}
	d.consumed()
}

func TestEvaluate(t *testing.T) {
	d, c := replay(t, "evaluate.jsonl")
	s, err := c.NewSession(ctxFor(t, 5*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	ctx := ctxFor(t, 5*time.Second)
	for _, tc := range []struct{ expr, want string }{
		{"1 + 1", "2"},
		{"JSON.stringify({a: 1})", `{"a":1}`},
		{"({x: [1, 'a'], y: null})", `{"x":[1,"a"],"y":null}`},
		{"undefined", "null"},
	} {
		if got, err := s.Evaluate(ctx, tc.expr); err != nil || got != tc.want {
			t.Errorf("Evaluate(%s) = %q, %v; want %q", tc.expr, got, err, tc.want)
		}
	}
	// An exception reached the wrapper's rejection path.
	if _, err := s.Evaluate(ctx, "nope()"); !errors.Is(err, drivers.ErrScript) || !strings.Contains(err.Error(), "ReferenceError: nope is not defined") {
		t.Errorf("Evaluate(nope()) = %v; want ErrScript with the page's message", err)
	}
	// The driver itself rejected the script (500 javascript error).
	if _, err := s.Evaluate(ctx, "for (;;) {}"); !errors.Is(err, drivers.ErrScript) || !strings.Contains(err.Error(), "ScriptTimeout") {
		t.Errorf("Evaluate(infinite) = %v; want ErrScript with the driver's message", err)
	}
	if err := s.Close(ctx); err != nil {
		t.Errorf("close: %v", err)
	}
	d.consumed()
}

func TestNavigateTimeoutAndError(t *testing.T) {
	for _, tc := range []struct {
		name    string
		wantErr error
		message string
	}{
		{"navigate-timeout.jsonl", drivers.ErrTimeout, "Timed out waiting for page load"},
		{"navigate-error.jsonl", drivers.ErrNavigation, "Could not connect to the server"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, c := replay(t, tc.name)
			s, err := c.NewSession(ctxFor(t, 5*time.Second))
			if err != nil {
				t.Fatal(err)
			}
			err = s.Navigate(ctxFor(t, 5*time.Second), "https://example.com/slow")
			if !errors.Is(err, tc.wantErr) || !strings.Contains(err.Error(), tc.message) {
				t.Errorf("Navigate = %v; want %v with %q", err, tc.wantErr, tc.message)
			}
			if err := s.Close(ctxFor(t, 5*time.Second)); err != nil {
				t.Errorf("close: %v", err)
			}
			d.consumed()
		})
	}
}

// TestViewportCorrection follows the spike's loop: measure, resize the window
// by the error, measure again.
func TestViewportCorrection(t *testing.T) {
	d, c := replay(t, "viewport.jsonl")
	s, err := c.NewSession(ctxFor(t, 5*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	vp, err := s.SetViewport(ctxFor(t, 10*time.Second), 1280, 800, 1)
	if err != nil {
		t.Fatal(err)
	}
	if vp != (drivers.Viewport{Width: 1280, Height: 800, DPR: 1}) {
		t.Errorf("SetViewport = %+v; want the corrected 1280x800", vp)
	}
	if err := s.Close(ctxFor(t, 5*time.Second)); err != nil {
		t.Errorf("close: %v", err)
	}
	d.consumed()
}

func TestScreenshotStitchesTheViewport(t *testing.T) {
	d, c := replay(t, "screenshot.jsonl")
	ctx := ctxFor(t, 10*time.Second)
	s, err := c.NewSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetViewport(ctx, 64, 48, 1); err != nil {
		t.Fatal(err)
	}
	shot, err := s.Screenshot(ctx, drivers.Rect{Width: 64, Height: 72})
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(shot))
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 64 || b.Dy() != 72 {
		t.Errorf("stitched screenshot %v; want 64x72", b)
	}
	// The second frame (green) covers rows 48..71; the first (red) 0..47.
	if got := img.At(10, 10); got != (color.RGBA{255, 0, 0, 255}) {
		t.Errorf("row 10 = %v; want the first frame's red", got)
	}
	if got := img.At(10, 60); got != (color.RGBA{0, 255, 0, 255}) {
		t.Errorf("row 60 = %v; want the second frame's green", got)
	}
	if err := s.Close(ctx); err != nil {
		t.Errorf("close: %v", err)
	}
	d.consumed()
}

// frameB64 is a solid 64x48 PNG, base64: one viewport frame.
func frameB64(t *testing.T, c color.RGBA) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 64, 48))
	for y := range 48 {
		for x := range 64 {
			img.SetRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}
