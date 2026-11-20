package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"strings"
	"sync"
	"time"

	"github.com/GerMP21/Omnisnap/assets/inject"
	"github.com/GerMP21/Omnisnap/internal/discovery"
	"github.com/GerMP21/Omnisnap/internal/drivers"
	"github.com/GerMP21/Omnisnap/internal/schema"
)

// fakeMachine has the given browsers installed; launching and capturing are
// simulated. It records every navigation as "<engine> <url>".
type fakeMachine struct {
	browsers []discovery.Browser
	failNav  map[string]bool // engine id -> every navigation fails
	crash    map[string]bool // engine id -> every navigation crashes the browser
	panicOn  bool            // discovery panics (an internal error)
	cacheDir string          // the browser cache for install/uninstall tests

	mu       sync.Mutex
	navs     []string
	launched []string
	closed   []string
}

func (m *fakeMachine) env() env {
	return env{
		find: func(context.Context) ([]discovery.Browser, error) {
			if m.panicOn {
				panic("boom")
			}
			return m.browsers, nil
		},
		cache: func() (string, error) { return m.cacheDir, nil },
		launch: func(_ context.Context, b discovery.Browser) (engine, error) {
			m.mu.Lock()
			m.launched = append(m.launched, b.ID)
			m.mu.Unlock()
			return &fakeEngine{m: m, id: b.ID}, nil
		},
	}
}

func blink(id string) discovery.Browser {
	return discovery.Browser{ID: id, Name: id, Family: schema.FamilyBlink, Protocol: discovery.ProtocolCDP,
		Executable: "/usr/bin/" + id, Version: "157.0", Source: discovery.SourcePath}
}

func gecko(id string) discovery.Browser {
	return discovery.Browser{ID: id, Name: id, Family: schema.FamilyGecko, Protocol: discovery.ProtocolBiDi,
		Executable: "/usr/bin/" + id, Version: "160.0", Source: discovery.SourcePath}
}

type fakeEngine struct {
	m    *fakeMachine
	id   string
	dead bool // crashed: refuses new pages, as a real dead browser does
}

func (e *fakeEngine) NewSession(context.Context) (drivers.Session, error) {
	if e.dead {
		return nil, fmt.Errorf("cdp: connection closed: %w", drivers.ErrCrashed)
	}
	return &fakeSession{e: e}, nil
}

func (e *fakeEngine) Close(context.Context) error {
	e.m.mu.Lock()
	defer e.m.mu.Unlock()
	e.m.closed = append(e.m.closed, e.id)
	return nil
}

type fakeSession struct{ e *fakeEngine }

func (s *fakeSession) Navigate(_ context.Context, url string) error {
	s.e.m.mu.Lock()
	s.e.m.navs = append(s.e.m.navs, s.e.id+" "+url)
	fail := s.e.m.failNav[s.e.id]
	crash := s.e.m.crash[s.e.id]
	s.e.m.mu.Unlock()
	if crash {
		s.e.dead = true
		return fmt.Errorf("cdp: connection closed: %w (EOF)", drivers.ErrCrashed)
	}
	if fail {
		return fmt.Errorf("fake: %w: net::ERR_CONNECTION_REFUSED", drivers.ErrNavigation)
	}
	return nil
}

func (s *fakeSession) WaitNetworkIdle(context.Context, time.Duration, time.Duration) error {
	return nil
}

func (s *fakeSession) Evaluate(_ context.Context, expr string) (string, error) {
	switch expr {
	case inject.Stabilize:
		return `{"ms":1,"finished":0,"frozen":0,"timersCleared":0,"scrolledTo":0,"pendingImages":0}`, nil
	case inject.Probes:
		return `{"installed":true}`, nil
	case inject.Extract:
		b, _ := json.Marshal(map[string]any{
			"meta": map[string]any{"url": "u", "title": "t", "ua": "UA", "vw": 1280, "vh": 800, "cw": 1280, "dpr": 1,
				"docW": 1280, "docH": 900, "nodes": 120, "truncated": false, "fonts": map[string]any{"loaded": []string{}, "failed": []string{}}},
			"root": map[string]any{"t": "html", "b": []float64{0, 0, 1280, 900}, "d": "block"},
		})
		return string(b), nil
	}
	if strings.HasPrefix(expr, "window.__omnisnapProbe.apply(") {
		return `{"id":"lh","lineHeightsPinned":1}`, nil
	}
	if strings.HasPrefix(expr, "window.__omnisnapProbe.revert()") {
		return "reverted", nil
	}
	if strings.Contains(expr, "measureText") {
		// Font probe: Firefox resolves sans-serif to another font.
		sans := `{"width":100,"font":"Liberation Sans"}`
		if s.e.id == "firefox" {
			sans = `{"width":104,"font":"DejaVu Sans"}`
		}
		return `{"sans-serif":` + sans + `,"serif":{"width":100,"font":"Liberation Serif"},"monospace":{"width":120,"font":"Noto Sans Mono"},"default":{"width":100,"font":"Liberation Sans"},"inlineHeight":24,"scripts":{"Japanese":{"width":80,"missing":false}}}`, nil
	}
	return "null", nil
}

func (s *fakeSession) AddPreloadScript(context.Context, string) error { return nil }
func (s *fakeSession) SetViewport(_ context.Context, w, h int, dpr float64) (drivers.Viewport, error) {
	return drivers.Viewport{Width: w, Height: h, DPR: dpr}, nil
}
func (s *fakeSession) Screenshot(_ context.Context, clip drivers.Rect) ([]byte, error) {
	var buf bytes.Buffer
	err := png.Encode(&buf, image.NewGray(image.Rect(0, 0, int(clip.Width), int(clip.Height))))
	return buf.Bytes(), err
}
func (s *fakeSession) FailedRequests() []drivers.FailedRequest {
	return []drivers.FailedRequest{{URL: "https://ads.example.net/t.js", Err: "net::ERR_BLOCKED_BY_CLIENT"}}
}
func (s *fakeSession) Capabilities() drivers.Capabilities {
	return drivers.Capabilities{Preload: true, NetworkIdle: true, FullPageScreenshot: true}
}
func (s *fakeSession) Close(context.Context) error { return nil }

// lines splits output into non-empty lines.
func lines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}
