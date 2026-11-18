//go:build browsers

package drivers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/GerMP21/Omnisnap/assets/inject"
	"github.com/GerMP21/Omnisnap/internal/drivers"
	"github.com/GerMP21/Omnisnap/internal/drivers/bidi"
	"github.com/GerMP21/Omnisnap/internal/drivers/cdp"
	"github.com/GerMP21/Omnisnap/internal/process"
	"github.com/GerMP21/Omnisnap/internal/schema"
)

// The conformance suite of SPEC-drivers.md: one set of scenarios run against
// every engine family installed on this machine. A driver that drifts from the
// others fails here, before capture is affected.

// conformanceServer serves layout-neutral pages: explicit sizes, no text that
// could wrap differently, so the geometry tree is the same in every engine.
func conformanceServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/fixed.html", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!doctype html><html><head><style>
html,body{margin:0;padding:0}
#a{position:absolute;left:10px;top:20px;width:200px;height:100px;background:#eee}
#b{position:absolute;left:250px;top:20px;width:120px;height:300px;background:#ddd}
#c{position:absolute;left:10px;top:400px;width:360px;height:40px;background:#ccc}
</style></head><body><div id="a"></div><div id="b"></div><div id="c"></div></body></html>`)
	})
	mux.HandleFunc("/tall.html", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!doctype html><html><head><style>html,body{margin:0}#tall{height:5000px;background:linear-gradient(red,blue)}</style></head><body><div id="tall"></div></body></html>`)
	})
	hang := make(chan struct{})
	mux.HandleFunc("/hang", func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-hang:
		case <-r.Context().Done():
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(func() { close(hang); srv.Close() })
	return srv
}

// harness is one engine family, launched once for the whole suite.
type harness struct {
	name    string
	newSess func(t *testing.T, ctx context.Context) drivers.Session
	close   func()
}

// harnesses returns the engines installed on this machine.
func harnesses(t *testing.T) []harness {
	t.Helper()
	var out []harness
	if blink, exe := startBlink(t); blink != nil {
		t.Logf("blink: %s", exe)
		br, err := cdp.Dial(context.Background(), blink.WSURL)
		if err != nil {
			t.Fatalf("dial CDP: %v", err)
		}
		out = append(out, harness{
			name: "blink (cdp)",
			newSess: func(t *testing.T, ctx context.Context) drivers.Session {
				s, err := br.NewSession(ctx)
				if err != nil {
					t.Fatal(err)
				}
				return s
			},
			close: func() { _ = br.Close(); _ = blink.Close(context.Background()) },
		})
	}
	if gecko, exe := startGecko(t); gecko != nil {
		t.Logf("gecko: %s", exe)
		br, err := bidi.Dial(context.Background(), gecko.WSURL)
		if err != nil {
			t.Fatalf("dial BiDi: %v", err)
		}
		out = append(out, harness{
			name: "gecko (bidi)",
			newSess: func(t *testing.T, ctx context.Context) drivers.Session {
				s, err := br.NewSession(ctx)
				if err != nil {
					t.Fatal(err)
				}
				return s
			},
			close: func() {
				_ = br.Close(context.Background())
				_ = gecko.Close(context.Background())
			},
		})
	}
	if h := webkitHarness(t); h != nil {
		out = append(out, *h)
	}
	if len(out) == 0 {
		t.Skip("no engine installed: nothing to conform")
	}
	return out
}

// startBlink tries every candidate until one launches. A broken wrapper earlier
// on PATH (a snap stub named chromium, a browser whose sandbox cannot start)
// must not fail the suite when a working Chrome is installed.
func startBlink(t *testing.T) (*process.Blink, string) {
	t.Helper()
	for _, exe := range candidates("OMNISNAP_CHROME", "google-chrome-stable", "google-chrome", "chromium", "chromium-browser", "chrome") {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		b, err := process.StartBlink(ctx, exe, "")
		cancel()
		if err == nil {
			return b, exe
		}
		t.Logf("blink %s: %v (next candidate)", exe, err)
	}
	return nil, ""
}

func startGecko(t *testing.T) (*process.Gecko, string) {
	t.Helper()
	for _, exe := range candidates("OMNISNAP_FIREFOX", "firefox", "firefox-esr") {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		g, err := process.StartGecko(ctx, exe, "")
		cancel()
		if err == nil {
			return g, exe
		}
		t.Logf("gecko %s: %v (next candidate)", exe, err)
	}
	return nil, ""
}

// candidates lists every executable among names that exists (env wins, alone).
func candidates(env string, names ...string) []string {
	if p := os.Getenv(env); p != "" {
		return []string{p}
	}
	var out []string
	for _, name := range names {
		if p, err := exec.LookPath(name); err == nil {
			out = append(out, p)
		}
	}
	return out
}

func findExecutable(env string, names ...string) string {
	if p := os.Getenv(env); p != "" {
		return p
	}
	for _, name := range names {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	return ""
}

// treeShape is the geometry that must agree across engines.
type treeShape struct {
	T  string      `json:"t"`
	ID string      `json:"id,omitempty"`
	B  [4]float64  `json:"b"`
	K  []treeShape `json:"k,omitempty"`
}

func shape(n schema.Node) treeShape {
	s := treeShape{T: n.T, ID: n.ID, B: n.B}
	for _, k := range n.K {
		s.K = append(s.K, shape(k))
	}
	return s
}

func TestConformance(t *testing.T) {
	srv := conformanceServer(t)
	before := runtime.NumGoroutine()
	hs := harnesses(t)
	for _, h := range hs {
		t.Run(h.name, func(t *testing.T) {
			t.Run("viewport is exactly 1280x800", func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				s := h.newSess(t, ctx)
				defer closeSession(t, s)
				vp, err := s.SetViewport(ctx, 1280, 800, 1)
				if err != nil {
					t.Fatal(err)
				}
				if vp.Width != 1280 || vp.Height != 800 || vp.DPR != 1 {
					t.Errorf("SetViewport = %+v; want 1280x800 @1", vp)
				}
			})
			t.Run("extract.js gives the same geometry tree", func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				s := h.newSess(t, ctx)
				defer closeSession(t, s)
				if _, err := s.SetViewport(ctx, 1280, 800, 1); err != nil {
					t.Fatal(err)
				}
				if err := s.Navigate(ctx, srv.URL+"/fixed.html"); err != nil {
					t.Fatal(err)
				}
				raw, err := s.Evaluate(ctx, inject.Extract)
				if err != nil {
					t.Fatal(err)
				}
				var c schema.Capture
				if err := json.Unmarshal([]byte(raw), &c); err != nil {
					t.Fatalf("extract.js: %v", err)
				}
				got, _ := json.Marshal(shape(c.Root))
				// The page is layout-neutral: the three fixed boxes must be in
				// the same places, with the same nesting.
				// Both engines must produce this: the three boxes at their
				// exact positions, and the same ignored elements (no head, no
				// text). A driver that drops or moves a box fails the suite.
				want := `{"t":"html","b":[0,0,1280,0],"k":[{"t":"body","b":[0,0,1280,0],"k":[{"t":"div","id":"a","b":[10,20,200,100]},{"t":"div","id":"b","b":[250,20,120,300]},{"t":"div","id":"c","b":[10,400,360,40]}]}]}`
				if string(got) != want {
					t.Errorf("extract.js tree:\n%s\nwant\n%s", got, want)
				}
			})
			t.Run("full-page screenshot is docW x docH", func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
				defer cancel()
				s := h.newSess(t, ctx)
				defer closeSession(t, s)
				if _, err := s.SetViewport(ctx, 1280, 800, 1); err != nil {
					t.Fatal(err)
				}
				if err := s.Navigate(ctx, srv.URL+"/tall.html"); err != nil {
					t.Fatal(err)
				}
				raw, err := s.Evaluate(ctx, "JSON.stringify([document.documentElement.scrollWidth, document.documentElement.scrollHeight])")
				if err != nil {
					t.Fatal(err)
				}
				var doc [2]float64
				_ = json.Unmarshal([]byte(raw), &doc)
				shot, err := s.Screenshot(ctx, drivers.Rect{Width: doc[0], Height: doc[1]})
				if err != nil {
					t.Fatal(err)
				}
				img, err := png.Decode(bytes.NewReader(shot))
				if err != nil {
					t.Fatal(err)
				}
				if img.Bounds().Dx() != int(doc[0]) || img.Bounds().Dy() != int(doc[1]) {
					t.Errorf("screenshot %v; want %vx%v", img.Bounds(), doc[0], doc[1])
				}
			})
			t.Run("typed errors", func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				s := h.newSess(t, ctx)
				defer closeSession(t, s)
				if _, err := s.Evaluate(ctx, "nope()"); !errors.Is(err, drivers.ErrScript) || !strings.Contains(err.Error(), "nope is not defined") {
					t.Errorf("script error = %v; want ErrScript with the page's message", err)
				}
				closed := closedPort(t)
				if err := s.Navigate(ctx, closed); !errors.Is(err, drivers.ErrNavigation) {
					t.Errorf("navigation to %s = %v; want ErrNavigation", closed, err)
				}
				if err := s.Navigate(ctx, srv.URL+"/tall.html"); err != nil {
					t.Fatal(err)
				}
				short, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
				defer cancel()
				_, err := s.Evaluate(short, "new Promise(() => {})")
				if !errors.Is(err, drivers.ErrTimeout) && !errors.Is(err, drivers.ErrScript) {
					t.Errorf("hung script = %v; want ErrTimeout (or the driver's script timeout as ErrScript)", err)
				}
			})
			t.Run("capabilities are reported honestly", func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				s := h.newSess(t, ctx)
				defer closeSession(t, s)
				c := s.Capabilities()
				switch {
				case strings.HasPrefix(h.name, "webkit"):
					if c.Preload || c.NetworkIdle || c.FullPageScreenshot {
						t.Errorf("Classic capabilities = %+v; want all false", c)
					}
				default:
					if !c.Preload || !c.NetworkIdle || !c.FullPageScreenshot {
						t.Errorf("%s capabilities = %+v; want all true", h.name, c)
					}
				}
			})
		})
	}
	// Everything is closed: no goroutine may outlive its browser.
	for _, h := range hs {
		h.close()
	}
	deadline := time.Now().Add(5 * time.Second)
	for runtime.NumGoroutine() > before && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if n := runtime.NumGoroutine(); n > before {
		t.Errorf("%d goroutines after Close, %d before (Task 22: no leaks)", n, before)
	}
}

func closeSession(t *testing.T, s drivers.Session) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.Close(ctx); err != nil {
		t.Errorf("session close: %v", err)
	}
}

func closedPort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	return "http://" + addr + "/"
}
