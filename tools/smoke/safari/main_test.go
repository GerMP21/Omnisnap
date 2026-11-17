package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakePNG is the PNG signature followed by junk: enough to tell a decoded
// screenshot from a still-encoded one.
var fakePNG = []byte("\x89PNG\r\n\x1a\nfake")

// fakeDriver implements the WebDriver Classic endpoints the smoke test uses
// and records the order of the calls.
func fakeDriver(t *testing.T) (*httptest.Server, *[]string) {
	t.Helper()
	var calls []string
	mux := http.NewServeMux()
	reply := func(w http.ResponseWriter, value any) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"value": value}); err != nil {
			t.Errorf("encode reply: %v", err)
		}
	}
	mux.HandleFunc("GET /status", func(w http.ResponseWriter, _ *http.Request) {
		calls = append(calls, "status")
		reply(w, map[string]any{"ready": true})
	})
	mux.HandleFunc("POST /session", func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, "new-session")
		var body struct {
			Capabilities struct {
				AlwaysMatch map[string]any `json:"alwaysMatch"`
			} `json:"capabilities"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Capabilities.AlwaysMatch["browserName"] != "safari" {
			t.Errorf("new session body: browserName = %v, err %v", body.Capabilities.AlwaysMatch["browserName"], err)
		}
		reply(w, map[string]any{
			"sessionId":    "s1",
			"capabilities": map[string]any{"browserName": "safari", "browserVersion": "26.1"},
		})
	})
	mux.HandleFunc("POST /session/s1/url", func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, "navigate")
		var body struct{ URL string }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || !strings.HasPrefix(body.URL, "http://127.0.0.1:") {
			t.Errorf("navigate url = %q, err %v", body.URL, err)
		}
		reply(w, nil)
	})
	mux.HandleFunc("POST /session/s1/execute/sync", func(w http.ResponseWriter, _ *http.Request) {
		calls = append(calls, "execute")
		reply(w, map[string]any{"title": pageTitle, "boxWidth": 200, "userAgent": "Safari", "devicePixelRatio": 2})
	})
	mux.HandleFunc("GET /session/s1/screenshot", func(w http.ResponseWriter, _ *http.Request) {
		calls = append(calls, "screenshot")
		reply(w, base64.StdEncoding.EncodeToString(fakePNG))
	})
	mux.HandleFunc("DELETE /session/s1", func(w http.ResponseWriter, _ *http.Request) {
		calls = append(calls, "delete-session")
		reply(w, nil)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &calls
}

func TestSmokeRunsTheWholeSequence(t *testing.T) {
	driver, calls := fakeDriver(t)
	page := httptest.NewServer(pageHandler())
	t.Cleanup(page.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, png, err := smoke(ctx, driver.URL, page.URL)
	if err != nil {
		t.Fatalf("smoke: %v", err)
	}

	want := []string{"status", "new-session", "navigate", "execute", "screenshot", "delete-session"}
	if strings.Join(*calls, ",") != strings.Join(want, ",") {
		t.Errorf("calls = %v, want %v", *calls, want)
	}
	if res.BrowserVersion != "26.1" {
		t.Errorf("BrowserVersion = %q, want 26.1", res.BrowserVersion)
	}
	if res.Page.Title != pageTitle || res.Page.BoxWidth != 200 {
		t.Errorf("Page = %+v, want title %q and box width 200", res.Page, pageTitle)
	}
	if !bytes.Equal(png, fakePNG) {
		t.Errorf("screenshot = %q, want decoded PNG bytes", png)
	}
	for _, step := range []string{"newSession", "navigate", "execute", "screenshot"} {
		if _, ok := res.TimingsMS[step]; !ok {
			t.Errorf("TimingsMS has no %q: %v", step, res.TimingsMS)
		}
	}
}

func TestSmokeReportsWebDriverErrors(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /status", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"value":{"ready":true}}`))
	})
	mux.HandleFunc("POST /session", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"value":{"error":"session not created","message":"You must enable 'Allow remote automation'"}}`))
	})
	driver := httptest.NewServer(mux)
	t.Cleanup(driver.Close)

	_, _, err := smoke(context.Background(), driver.URL, "http://127.0.0.1:1/")
	if err == nil || !strings.Contains(err.Error(), "session not created") || !strings.Contains(err.Error(), "Allow remote automation") {
		t.Fatalf("err = %v, want the WebDriver error and message", err)
	}
}

func TestPageHandlerServesTheTestPage(t *testing.T) {
	srv := httptest.NewServer(pageHandler())
	t.Cleanup(srv.Close)
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "<title>"+pageTitle+"</title>") || !strings.Contains(buf.String(), `id="box"`) {
		t.Errorf("page = %q, want the title and the #box element", buf.String())
	}
}
