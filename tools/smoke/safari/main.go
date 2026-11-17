// Command safari is the Task 2 smoke test: it proves Safari runs unattended
// through safaridriver (WebDriver Classic) on a GitHub-hosted macOS runner.
// It opens a session, loads a local page, evaluates a script, takes a
// screenshot, and writes result.json and screenshot.png to -out.
//
// It is throwaway tooling: it uses only the standard library and shares no
// code with the drivers module (SPEC-drivers.md), which comes in Task 20.
package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

const pageTitle = "Omnisnap Safari smoke"

// The #box width is fixed so the script result proves layout was read, not
// just that a script ran.
const page = `<!doctype html>
<html><head><meta charset="utf-8"><title>` + pageTitle + `</title>
<style>body{margin:0;font:16px/1.5 sans-serif}#box{width:200px;height:100px;background:#36c;color:#fff}</style>
</head><body><div id="box">Omnisnap</div><p>Safari smoke test page.</p></body></html>`

const script = `const b = document.getElementById('box').getBoundingClientRect();
return {title: document.title, boxWidth: b.width, userAgent: navigator.userAgent, devicePixelRatio: window.devicePixelRatio};`

// Result is written to result.json and read by the workflow summary.
type Result struct {
	OK             bool             `json:"ok"`
	Error          string           `json:"error,omitempty"`
	BrowserVersion string           `json:"browserVersion,omitempty"`
	Page           PageInfo         `json:"page"`
	TimingsMS      map[string]int64 `json:"timingsMs"`
}

// PageInfo is what the evaluated script returns.
type PageInfo struct {
	Title            string  `json:"title"`
	BoxWidth         float64 `json:"boxWidth"`
	UserAgent        string  `json:"userAgent"`
	DevicePixelRatio float64 `json:"devicePixelRatio"`
}

func main() {
	driverPath := flag.String("driver", "safaridriver", "safaridriver executable")
	out := flag.String("out", "smoke-out", "output directory")
	timeout := flag.Duration("timeout", 2*time.Minute, "overall timeout")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	res, png, err := run(ctx, *driverPath)
	if err != nil {
		res.Error = err.Error()
	}
	res.OK = err == nil
	if werr := write(*out, res, png); werr != nil {
		fmt.Fprintln(os.Stderr, "safari smoke:", werr)
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "safari smoke: FAILED:", err)
		os.Exit(1)
	}
	fmt.Printf("safari smoke: OK, Safari %s, timings %v ms\n", res.BrowserVersion, res.TimingsMS)
}

// run starts the page server and safaridriver, then runs the smoke sequence.
func run(ctx context.Context, driverPath string) (Result, []byte, error) {
	pageLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return Result{}, nil, fmt.Errorf("page server: %w", err)
	}
	pageSrv := &http.Server{Handler: pageHandler(), ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = pageSrv.Serve(pageLn) }()
	defer func() { _ = pageSrv.Close() }()

	port, err := freePort()
	if err != nil {
		return Result{}, nil, err
	}
	start := time.Now()
	cmd := exec.Command(driverPath, "-p", strconv.Itoa(port))
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Start(); err != nil {
		return Result{}, nil, fmt.Errorf("start %s: %w", driverPath, err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	res, png, err := smoke(ctx, fmt.Sprintf("http://127.0.0.1:%d", port), "http://"+pageLn.Addr().String()+"/")
	if res.TimingsMS != nil {
		res.TimingsMS["total"] = time.Since(start).Milliseconds()
	}
	return res, png, err
}

// smoke waits for the driver, then runs session → navigate → execute →
// screenshot → delete, timing each step.
func smoke(ctx context.Context, driverURL, pageURL string) (Result, []byte, error) {
	res := Result{TimingsMS: map[string]int64{}}
	wd := &webdriver{base: driverURL, client: &http.Client{}}
	step := func(name string, f func() error) error {
		t0 := time.Now()
		err := f()
		res.TimingsMS[name] = time.Since(t0).Milliseconds()
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		return nil
	}

	if err := step("driverReady", func() error { return wd.waitReady(ctx) }); err != nil {
		return res, nil, err
	}

	var session struct {
		SessionID    string `json:"sessionId"`
		Capabilities struct {
			BrowserVersion string `json:"browserVersion"`
		} `json:"capabilities"`
	}
	caps := map[string]any{"capabilities": map[string]any{"alwaysMatch": map[string]any{"browserName": "safari"}}}
	if err := step("newSession", func() error { return wd.call(ctx, http.MethodPost, "/session", caps, &session) }); err != nil {
		return res, nil, err
	}
	res.BrowserVersion = session.Capabilities.BrowserVersion
	sess := "/session/" + session.SessionID
	defer func() {
		// Safari allows one automation session at a time: always end it.
		_ = step("deleteSession", func() error {
			return wd.call(context.WithoutCancel(ctx), http.MethodDelete, sess, nil, nil)
		})
	}()

	if err := step("navigate", func() error {
		return wd.call(ctx, http.MethodPost, sess+"/url", map[string]string{"url": pageURL}, nil)
	}); err != nil {
		return res, nil, err
	}
	if err := step("execute", func() error {
		return wd.call(ctx, http.MethodPost, sess+"/execute/sync", map[string]any{"script": script, "args": []any{}}, &res.Page)
	}); err != nil {
		return res, nil, err
	}
	var encoded string
	if err := step("screenshot", func() error { return wd.call(ctx, http.MethodGet, sess+"/screenshot", nil, &encoded) }); err != nil {
		return res, nil, err
	}
	png, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return res, nil, fmt.Errorf("screenshot: decode: %w", err)
	}
	return res, png, nil
}

func pageHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, page)
	})
}

type webdriver struct {
	base   string
	client *http.Client
}

// waitReady polls /status: safaridriver accepts connections a moment after
// it starts.
func (wd *webdriver) waitReady(ctx context.Context) error {
	for {
		var status struct {
			Ready bool `json:"ready"`
		}
		err := wd.call(ctx, http.MethodGet, "/status", nil, &status)
		if err == nil && status.Ready {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("driver not ready (last error: %v): %w", err, ctx.Err())
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// call sends one WebDriver command and decodes the response's "value" into
// out (when out is non-nil). WebDriver errors keep their code and message.
func (wd *webdriver) call(ctx context.Context, method, path string, body, out any) error {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, wd.base+path, reqBody)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := wd.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	var envelope struct {
		Value json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("HTTP %d, invalid JSON: %.200s", resp.StatusCode, raw)
	}
	if resp.StatusCode >= 300 {
		var wdErr struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal(envelope.Value, &wdErr)
		return fmt.Errorf("HTTP %d: %s: %s", resp.StatusCode, wdErr.Error, wdErr.Message)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(envelope.Value, out)
}

func freePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("free port: %w", err)
	}
	defer func() { _ = ln.Close() }()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

func write(dir string, res Result, png []byte) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "result.json"), append(b, '\n'), 0o644); err != nil {
		return err
	}
	if len(png) == 0 {
		return nil
	}
	return os.WriteFile(filepath.Join(dir, "screenshot.png"), png, 0o644)
}
