//go:build browsers

package cdp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GerMP21/Omnisnap/assets/inject"
	"github.com/GerMP21/Omnisnap/internal/drivers"
	"github.com/GerMP21/Omnisnap/internal/schema"
)

// The same scenarios as the transcript tests, against a real Blink browser:
// this is what keeps the hand-written transcripts honest.

// chromeExecutable finds a Blink browser: $OMNISNAP_CHROME, else the first on PATH.
func chromeExecutable(t *testing.T) string {
	t.Helper()
	if p := os.Getenv("OMNISNAP_CHROME"); p != "" {
		return p
	}
	for _, name := range []string{"chromium", "google-chrome-stable", "google-chrome", "chromium-browser", "chrome"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	t.Skip("no Blink browser found: set OMNISNAP_CHROME")
	return ""
}

// launch starts a headless browser with a throwaway profile and returns its
// DevTools websocket URL. A minimal launcher for this test; the real one is
// the process module (Task 12).
func launch(t *testing.T) string {
	t.Helper()
	// Not t.TempDir: the browser's child processes outlive the killed parent
	// for a moment and keep writing the profile. Tearing down the whole tree
	// is the process module's job (Task 12); here, removal retries briefly.
	profile, err := os.MkdirTemp("", "omnisnap-cdp-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for range 50 {
			if os.RemoveAll(profile) == nil {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Errorf("profile %s not removed", profile)
	})
	cmd := exec.Command(chromeExecutable(t),
		"--headless=new", "--remote-debugging-port=0", "--user-data-dir="+profile,
		"--no-first-run", "--no-default-browser-check", "--disable-gpu", "about:blank")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	// The browser writes its port and websocket path to DevToolsActivePort.
	file := filepath.Join(profile, "DevToolsActivePort")
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		f, err := os.Open(file)
		if err == nil {
			sc := bufio.NewScanner(f)
			var lines []string
			for sc.Scan() {
				lines = append(lines, sc.Text())
			}
			_ = f.Close()
			if len(lines) >= 2 {
				return fmt.Sprintf("ws://127.0.0.1:%s%s", lines[0], lines[1])
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("browser did not write DevToolsActivePort")
	return ""
}

// lastFetch is when /slow last answered (unix ns).
var lastFetch atomic.Int64

func pageServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/page.html", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!doctype html><html><head><title>cdp</title><style>body{margin:0;font:16px/1.5 sans-serif}#box{width:200px;height:100px}</style></head>`+
			`<body><main><div id="box">Omnisnap</div><p>Text that wraps.</p><a href="/x">link</a></main></body></html>`)
	})
	mux.HandleFunc("/tall.html", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!doctype html><html><head><style>body{margin:0}#tall{height:5000px;background:linear-gradient(red,blue)}</style></head><body><div id="tall"></div></body></html>`)
	})
	// /refuse closes the connection without answering: net::ERR_EMPTY_RESPONSE.
	mux.HandleFunc("/refuse", func(w http.ResponseWriter, _ *http.Request) {
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			_ = conn.Close()
		}
	})
	mux.HandleFunc("/fetches.html", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!doctype html><html><body><img src="/refuse"><script>
addEventListener('load', () => setTimeout(async () => {
  await fetch('/slow?n=1'); await fetch('/slow?n=2');
}, 200));
</script></body></html>`)
	})
	mux.HandleFunc("/slow", func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(300 * time.Millisecond)
		fmt.Fprint(w, "ok")
		lastFetch.Store(time.Now().UnixNano())
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

func TestBrowserScenarios(t *testing.T) {
	srv := pageServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	b, err := Dial(ctx, launch(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := b.Close(); err != nil {
			t.Errorf("browser close: %v", err)
		}
	}()
	session := func(t *testing.T) *Session {
		t.Helper()
		s, err := b.NewSession(ctx)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = s.Close(context.Background()) })
		return s
	}

	t.Run("navigate, viewport, evaluate extract.js", func(t *testing.T) {
		s := session(t)
		vp, err := s.SetViewport(ctx, 1280, 800, 1)
		if err != nil || vp != (drivers.Viewport{Width: 1280, Height: 800, DPR: 1}) {
			t.Fatalf("SetViewport = %+v, %v", vp, err)
		}
		if err := s.Navigate(ctx, srv.URL+"/page.html"); err != nil {
			t.Fatal(err)
		}
		if got, err := s.Evaluate(ctx, "1 + 1"); err != nil || got != "2" {
			t.Errorf("Evaluate(1 + 1) = %q, %v", got, err)
		}
		if got, err := s.Evaluate(ctx, "Promise.resolve(JSON.stringify({a: 1}))"); err != nil || got != `{"a":1}` {
			t.Errorf("Evaluate(promise) = %q, %v", got, err)
		}
		raw, err := s.Evaluate(ctx, inject.Extract)
		if err != nil {
			t.Fatal(err)
		}
		var c schema.Capture
		if err := json.Unmarshal([]byte(raw), &c); err != nil {
			t.Fatalf("extract.js result: %v", err)
		}
		box := findID(c.Root, "box")
		if c.Root.T != "html" || c.Meta.VW != 1280 || box == nil || box.B[2] != 200 {
			t.Errorf("extract.js: root %q, vw %d, #box %+v; want html, 1280, a 200 px box", c.Root.T, c.Meta.VW, box)
		}
	})

	t.Run("script error", func(t *testing.T) {
		_, err := session(t).Evaluate(ctx, "nope()")
		if !errors.Is(err, drivers.ErrScript) || !strings.Contains(err.Error(), "ReferenceError: nope is not defined") {
			t.Errorf("err = %v", err)
		}
	})

	t.Run("navigation error", func(t *testing.T) {
		// A free port nobody listens on (port 1 would be refused earlier, as ERR_UNSAFE_PORT).
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		closed := "http://" + ln.Addr().String() + "/"
		_ = ln.Close()
		err = session(t).Navigate(ctx, closed)
		if !errors.Is(err, drivers.ErrNavigation) || !strings.Contains(err.Error(), "ERR_CONNECTION_REFUSED") {
			t.Errorf("err = %v", err)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		short, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
		defer cancel()
		if err := session(t).Navigate(short, srv.URL+"/hang"); !errors.Is(err, drivers.ErrTimeout) {
			t.Errorf("err = %v", err)
		}
	})

	t.Run("protocol error", func(t *testing.T) {
		_, err := session(t).SetViewport(ctx, -1, 800, 1)
		if err == nil || !strings.Contains(err.Error(), "Emulation.setDeviceMetricsOverride") {
			t.Errorf("err = %v", err)
		}
	})

	t.Run("full-page screenshot of a 5000 px page", func(t *testing.T) {
		s := session(t)
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
		png, err := s.Screenshot(ctx, drivers.Rect{Width: doc[0], Height: doc[1]})
		if err != nil {
			t.Fatal(err)
		}
		img, err := decodePNG(png)
		if err != nil {
			t.Fatal(err)
		}
		if doc[1] != 5000 || img.Dx() != int(doc[0]) || img.Dy() != int(doc[1]) {
			t.Errorf("screenshot %dx%d, document %vx%v; want exactly docW x docH = 1280x5000", img.Dx(), img.Dy(), doc[0], doc[1])
		}
	})

	t.Run("failed requests and network idle", func(t *testing.T) {
		s := session(t)
		if err := s.AddPreloadScript(ctx, "window.__preloaded = 'yes'"); err != nil {
			t.Fatal(err)
		}
		if err := s.Navigate(ctx, srv.URL+"/fetches.html"); err != nil {
			t.Fatal(err)
		}
		if got, err := s.Evaluate(ctx, "window.__preloaded"); err != nil || got != "yes" {
			t.Errorf("preload: %q, %v", got, err)
		}
		if err := s.WaitNetworkIdle(ctx, 500*time.Millisecond, 10*time.Second); err != nil {
			t.Fatal(err)
		}
		if last := lastFetch.Load(); last == 0 || time.Now().UnixNano() < last {
			t.Errorf("WaitNetworkIdle returned before the page's last fetch")
		}
		var refused bool
		for _, f := range s.FailedRequests() {
			if strings.HasSuffix(f.URL, "/refuse") && f.Err != "" {
				refused = true
			}
		}
		if !refused {
			t.Errorf("FailedRequests = %+v; want the refused /refuse", s.FailedRequests())
		}
	})

	// Last: it kills every renderer. A renderer dying (OOM killer, a GPU
	// fault) must fail calls at once, not at their deadline. Page.crash and
	// chrome://crash do not crash anything in Chromium 157 under CDP.
	t.Run("crash", func(t *testing.T) {
		s := session(t)
		if err := s.Navigate(ctx, srv.URL+"/page.html"); err != nil {
			t.Fatal(err)
		}
		var info struct {
			ProcessInfo []struct {
				Type string `json:"type"`
				ID   int    `json:"id"`
			} `json:"processInfo"`
		}
		if err := b.call(ctx, "SystemInfo.getProcessInfo", nil, &info); err != nil {
			t.Fatal(err)
		}
		for _, p := range info.ProcessInfo {
			if p.Type == "renderer" {
				if proc, err := os.FindProcess(p.ID); err == nil {
					_ = proc.Kill()
				}
			}
		}
		short, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		start := time.Now()
		if _, err := s.Evaluate(short, "1"); !errors.Is(err, drivers.ErrCrashed) {
			t.Errorf("after the renderer died: err = %v", err)
		}
		if d := time.Since(start); d > 5*time.Second {
			t.Errorf("ErrCrashed took %v: want it on the crash event, not a deadline", d)
		}
	})
}

func findID(n schema.Node, id string) *schema.Node {
	if n.ID == id {
		return &n
	}
	for _, k := range n.K {
		if f := findID(k, id); f != nil {
			return f
		}
	}
	return nil
}
