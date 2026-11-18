// Package classic drives WebKit over WebDriver Classic: plain HTTP, no driver
// library (SPEC-drivers.md). Classic has no network events, no preload scripts
// and viewport-only screenshots, so the session reports that honestly through
// Capabilities and stitches full-page images in Go.
package classic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is a WebDriver Classic endpoint: WebKitWebDriver (WebKitGTK) or
// safaridriver (Safari). It is the driver process's HTTP server, already
// running.
type Client struct {
	base string
	http *http.Client
	// Capabilities are the requested session capabilities (browserName and
	// the driver-specific browser options).
	Capabilities map[string]any
}

// New returns a client for a running driver at base (e.g.
// "http://127.0.0.1:9515"). Call Status to wait until it listens.
func New(base string) *Client {
	return &Client{
		base: base,
		http: &http.Client{Timeout: 2 * time.Minute}, // /url waits out the pageLoad timeout
		Capabilities: map[string]any{
			"alwaysMatch": map[string]any{},
		},
	}
}

// Status waits for the driver to answer GET /status. It is called once after
// the driver process starts.
func (c *Client) Status(ctx context.Context) error {
	var v struct {
		Ready bool `json:"ready"`
	}
	if err := c.call(ctx, http.MethodGet, "/status", nil, &v); err != nil {
		return err
	}
	if !v.Ready {
		return fmt.Errorf("classic: driver at %s reports not ready", c.base)
	}
	return nil
}

// wdError is a WebDriver error response: {"value":{"error","message","stacktrace"}}.
type wdError struct {
	Code    string `json:"error"`
	Message string `json:"message"`
}

func (e *wdError) Error() string { return e.Code + ": " + e.Message }

// call performs one command. out receives the response's "value".
func (c *Client) call(ctx context.Context, method, path string, body, out any) error {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("classic: %s %s: %w", method, path, err)
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, r)
	if err != nil {
		return fmt.Errorf("classic: %s %s: %w", method, path, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
	}
	res, err := c.http.Do(req)
	if err != nil {
		return c.ctxErr(ctx, method, path, err)
	}
	defer func() { _ = res.Body.Close() }()
	var envelope struct {
		Value json.RawMessage `json:"value"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 64<<20)).Decode(&envelope); err != nil {
		return fmt.Errorf("classic: %s %s: decode %d: %w", method, path, res.StatusCode, err)
	}
	if res.StatusCode >= 400 {
		var we wdError
		if json.Unmarshal(envelope.Value, &we) != nil || we.Code == "" {
			return fmt.Errorf("classic: %s %s: HTTP %d: %s", method, path, res.StatusCode, envelope.Value)
		}
		return &protocolError{status: res.StatusCode, wd: &we}
	}
	if out == nil || len(envelope.Value) == 0 {
		return nil
	}
	if err := json.Unmarshal(envelope.Value, out); err != nil {
		return fmt.Errorf("classic: %s %s: decode value: %w", method, path, err)
	}
	return nil
}

func (c *Client) ctxErr(ctx context.Context, method, path string, err error) error {
	if ctx.Err() != nil {
		return fmt.Errorf("classic: %s %s: %w", method, path, ctx.Err())
	}
	return fmt.Errorf("classic: %s %s: %w", method, path, err)
}

// protocolError is a WebDriver error reply, kept as is so each command can map
// it to the driver's typed errors.
type protocolError struct {
	status int
	wd     *wdError
}

func (e *protocolError) Error() string { return fmt.Sprintf("classic: %s", e.wd) }

func (e *protocolError) Unwrap() error { return e.wd }
