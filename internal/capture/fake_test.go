package capture

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"sync"
	"time"

	"github.com/GerMP21/Omnisnap/assets/inject"
	"github.com/GerMP21/Omnisnap/internal/drivers"
)

// fakeSession is a scripted drivers.Session that records its calls.
type fakeSession struct {
	mu       sync.Mutex
	calls    []string
	caps     drivers.Capabilities
	viewport drivers.Viewport
	docW     int
	docH     []int // docH per extract call; the last repeats
	extracts int
	failed   []drivers.FailedRequest
	navErr   error
	shotErr  error
	hang     string // "viewport" or "screenshot": that call blocks until its context ends
	clips    []drivers.Rect
}

func newFake() *fakeSession {
	return &fakeSession{
		caps:     drivers.Capabilities{Preload: true, NetworkIdle: true, FullPageScreenshot: true},
		viewport: drivers.Viewport{Width: 1280, Height: 800, DPR: 1},
		docW:     1280,
		docH:     []int{957},
	}
}

func (f *fakeSession) record(s string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, s)
}

func (f *fakeSession) Navigate(_ context.Context, url string) error {
	f.record("navigate " + url)
	return f.navErr
}

func (f *fakeSession) WaitNetworkIdle(_ context.Context, idle, limit time.Duration) error {
	f.record(fmt.Sprintf("idle %v %v", idle, limit))
	return nil
}

func (f *fakeSession) Evaluate(_ context.Context, expr string) (string, error) {
	switch expr {
	case inject.Stabilize:
		f.record("stabilize")
		return `{"ms":187,"finished":0,"frozen":0,"timersCleared":6,"scrolledTo":957,"pendingImages":0}`, nil
	case inject.Extract:
		f.record("extract")
		h := f.docH[min(f.extracts, len(f.docH)-1)]
		f.extracts++
		return extractJSON(f.viewport, f.docW, h), nil
	}
	f.record("evaluate")
	return "null", nil
}

func (f *fakeSession) AddPreloadScript(context.Context, string) error { return nil }

func (f *fakeSession) SetViewport(ctx context.Context, w, h int, dpr float64) (drivers.Viewport, error) {
	f.record(fmt.Sprintf("viewport %dx%d@%v", w, h, dpr))
	if f.hang == "viewport" {
		<-ctx.Done()
		return drivers.Viewport{}, ctx.Err()
	}
	return f.viewport, nil
}

func (f *fakeSession) Screenshot(ctx context.Context, clip drivers.Rect) ([]byte, error) {
	f.record("screenshot")
	if f.hang == "screenshot" {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	f.clips = append(f.clips, clip)
	if f.shotErr != nil {
		return nil, f.shotErr
	}
	w, h := int(clip.Width), int(clip.Height)
	if w*h > 4<<20 {
		w, h = 1, 1 // tests of huge clips check the clip, not the pixels: encoding them takes seconds
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, w, h)))
	return buf.Bytes(), nil
}

func (f *fakeSession) FailedRequests() []drivers.FailedRequest { return f.failed }
func (f *fakeSession) Capabilities() drivers.Capabilities      { return f.caps }
func (f *fakeSession) Close(context.Context) error             { return nil }

// extractJSON is what extract.js returns: meta without the pipeline's fields.
func extractJSON(vp drivers.Viewport, docW, docH int) string {
	b, _ := json.Marshal(map[string]any{
		"meta": map[string]any{
			"url": "http://127.0.0.1/p.html", "title": "p", "ua": "UA", "vw": vp.Width, "vh": vp.Height, "cw": vp.Width,
			"dpr": vp.DPR, "docW": docW, "docH": docH, "nodes": 3, "truncated": false,
			"fonts": map[string]any{"loaded": []string{}, "failed": []string{}},
		},
		"root": map[string]any{"t": "html", "b": []float64{0, 0, float64(docW), float64(docH)}, "d": "block",
			"k": []any{map[string]any{"t": "body", "b": []float64{0, 0, float64(docW), float64(docH)}, "d": "block"}}},
	})
	return string(b)
}
