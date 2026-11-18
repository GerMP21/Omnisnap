// Package drivers is the one interface capture uses for every engine
// (SPEC-drivers.md). Each engine family speaks its own protocol directly, with
// no proxy driver: CDP for Blink (drivers/cdp), WebDriver BiDi for Gecko,
// WebDriver Classic for WebKit.
package drivers

import (
	"context"
	"time"
)

// Session is one page (tab) in a running browser.
type Session interface {
	// Navigate loads url and waits for the load event.
	Navigate(ctx context.Context, url string) error
	// WaitNetworkIdle returns nil once no request has been in flight for
	// idle, ErrTimeout if that does not happen within limit (callers usually
	// note it and go on). ErrUnsupported on Classic.
	WaitNetworkIdle(ctx context.Context, idle, limit time.Duration) error
	// Evaluate runs expr in the page, awaiting it if it is a Promise. A string
	// result is returned as is (the injected scripts return JSON strings);
	// any other value as its JSON encoding.
	Evaluate(ctx context.Context, expr string) (string, error)
	// AddPreloadScript runs src before any page script on every navigation.
	// ErrUnsupported on Classic.
	AddPreloadScript(ctx context.Context, src string) error
	// SetViewport sets the CSS viewport and returns the size obtained.
	SetViewport(ctx context.Context, w, h int, dpr float64) (Viewport, error)
	// Screenshot returns a PNG of clip, in document coordinates.
	Screenshot(ctx context.Context, clip Rect) ([]byte, error)
	// FailedRequests lists the requests that failed since the last Navigate.
	FailedRequests() []FailedRequest
	// Capabilities says what this session supports; capture adapts to it.
	Capabilities() Capabilities
	// Close closes the page.
	Close(ctx context.Context) error
}

// Viewport is a CSS viewport as measured in the page.
type Viewport struct {
	Width  int     `json:"width"`
	Height int     `json:"height"`
	DPR    float64 `json:"dpr"`
}

// Rect is a region in document coordinates (CSS px).
type Rect struct {
	X, Y, Width, Height float64
}

// FailedRequest is a request that failed while the page loaded.
type FailedRequest struct {
	URL string `json:"url"`
	Err string `json:"err"` // the engine's error text, e.g. net::ERR_BLOCKED_BY_CLIENT
}

// Capabilities are the optional features of a session.
type Capabilities struct {
	Preload            bool // AddPreloadScript works
	NetworkIdle        bool // WaitNetworkIdle and FailedRequests work
	FullPageScreenshot bool // one screenshot covers the document; otherwise capture stitches
	// NavigationErrors is true when Navigate reports a failed load as
	// ErrNavigation. WebKitGTK's Classic driver accepts the command and shows
	// its own error page, so Classic reports false.
	NavigationErrors bool
}
