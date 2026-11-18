//go:build browsers

package bidi

import (
	"bufio"
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
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GerMP21/Omnisnap/assets/inject"
	"github.com/GerMP21/Omnisnap/internal/drivers"
	"github.com/GerMP21/Omnisnap/internal/schema"
)

// The transcript scenarios against a real Firefox: what keeps the
// hand-written BiDi transcripts honest.

// lastFetch is when /slow last answered (unix ns).
var lastFetch atomic.Int64

func firefoxExecutable(t *testing.T) string {
	t.Helper()
	if p := os.Getenv("OMNISNAP_FIREFOX"); p != "" {
		return p
	}
	for _, name := range []string{"firefox", "firefox-esr"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	t.Skip("no Firefox found: set OMNISNAP_FIREFOX")
	return ""
}

// launch starts a headless Firefox with an empty profile and returns its BiDi
// URL, printed on stderr. A minimal launcher; the real one, with the spike's
// prefs, is the process module (Task 18).
func launch(t *testing.T) string {
	t.Helper()
	profile, err := os.MkdirTemp("", "omnisnap-bidi-test-")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(firefoxExecutable(t), "--headless", "--no-remote", "--remote-debugging-port=0", "--profile", profile, "about:blank")
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		for range 50 {
			if os.RemoveAll(profile) == nil {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	})
	found := make(chan string, 1)
	go func() {
		re := regexp.MustCompile(`WebDriver BiDi listening on (ws://\S+)`)
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			if m := re.FindStringSubmatch(sc.Text()); m != nil {
				found <- m[1]
			}
		}
	}()
	select {
	case ws := <-found:
		return ws
	case <-time.After(30 * time.Second):
		t.Fatal("Firefox did not report its BiDi endpoint")
		return ""
	}
}

func TestBrowserScenarios(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/page.html", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!doctype html><html><head><title>bidi</title><style>body{margin:0;font:16px/1.5 sans-serif}#box{width:200px;height:100px}</style></head>`+
			`<body><main><div id="box">Omnisnap</div><p>Text that wraps.</p></main></body></html>`)
	})
	mux.HandleFunc("/tall.html", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!doctype html><html><head><style>body{margin:0}#tall{height:5000px;background:linear-gradient(red,blue)}</style></head><body><div id="tall"></div></body></html>`)
	})
	// /refuse closes the connection without answering.
	mux.HandleFunc("/refuse", func(w http.ResponseWriter, _ *http.Request) {
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			_ = conn.Close()
		}
	})
	mux.HandleFunc("/preload.html", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!doctype html><html><head><script>
document.documentElement.dataset.preload = String(window.__omnisnapPreload);
</script></head><body>ok</body></html>`)
	})
	mux.HandleFunc("/fetches.html", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!doctype html><html><body><img src="/refuse"><script>
addEventListener('load', () => setTimeout(async () => {
  document.documentElement.dataset.preload = String(window.__omnisnapPreload);
  await fetch('/slow?n=1');
}, 100));
</script></body></html>`)
	})
	mux.HandleFunc("/slow", func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
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
	defer func() { close(hang); srv.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	b, err := Dial(ctx, launch(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := b.Close(c); err != nil {
			t.Errorf("browser close: %v", err)
		}
	}()
	session := func(t *testing.T) *Session {
		t.Helper()
		s, err := b.NewSession(ctx)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = s.Close(c)
		})
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
		if got, err := s.Evaluate(ctx, "({x: [1, 'a'], y: null})"); err != nil || got != `{"x":[1,"a"],"y":null}` {
			t.Errorf("Evaluate(object) = %q, %v", got, err)
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
		if !strings.Contains(c.Meta.UA, "Firefox/") {
			t.Errorf("ua %q: not Firefox", c.Meta.UA)
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
		shot, err := s.Screenshot(ctx, drivers.Rect{Width: doc[0], Height: doc[1]})
		if err != nil {
			t.Fatal(err)
		}
		img, err := png.Decode(bytes.NewReader(shot))
		if err != nil {
			t.Fatal(err)
		}
		if doc[1] != 5000 || img.Bounds().Dx() != int(doc[0]) || img.Bounds().Dy() != int(doc[1]) {
			t.Errorf("screenshot %dx%d, document %vx%v; want exactly docW x docH = 1280x5000", img.Bounds().Dx(), img.Bounds().Dy(), doc[0], doc[1])
		}
	})

	t.Run("preload runs before page scripts", func(t *testing.T) {
		s := session(t)
		if err := s.AddPreloadScript(ctx, "window.__omnisnapPreload = 'yes';"); err != nil {
			t.Fatal(err)
		}
		if err := s.Navigate(ctx, srv.URL+"/preload.html"); err != nil {
			t.Fatal(err)
		}
		// The page's own inline script copied the value at parse time: it can
		// only be 'yes' if the preload ran before it.
		if got, err := s.Evaluate(ctx, "document.documentElement.dataset.preload"); err != nil || got != "yes" {
			t.Errorf("preload before page scripts: %q, %v", got, err)
		}
	})

	t.Run("failed requests and network idle", func(t *testing.T) {
		s := session(t)
		lastFetch.Store(0)
		if err := s.Navigate(ctx, srv.URL+"/fetches.html"); err != nil {
			t.Fatal(err)
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

	t.Run("script error", func(t *testing.T) {
		_, err := session(t).Evaluate(ctx, "nope()")
		if !errors.Is(err, drivers.ErrScript) || !strings.Contains(err.Error(), "ReferenceError: nope is not defined") {
			t.Errorf("err = %v", err)
		}
	})

	t.Run("protocol error", func(t *testing.T) {
		_, err := session(t).SetViewport(ctx, -1, 800, 1)
		if err == nil || !strings.Contains(err.Error(), "invalid argument") {
			t.Errorf("err = %v", err)
		}
	})

	t.Run("navigation error", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		closed := "http://" + ln.Addr().String() + "/"
		_ = ln.Close()
		err = session(t).Navigate(ctx, closed)
		if !errors.Is(err, drivers.ErrNavigation) || !strings.Contains(err.Error(), "NS_ERROR_CONNECTION_REFUSED") {
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
