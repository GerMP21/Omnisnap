//go:build browsers

package branded_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GerMP21/Omnisnap/internal/branded"
	"github.com/GerMP21/Omnisnap/internal/discovery"
	"github.com/GerMP21/Omnisnap/internal/drivers/cdp"
	"github.com/GerMP21/Omnisnap/internal/process"
)

// The branded checks. A fresh branded browser has no filter lists and blocks
// nothing; a warmed template does (SPEC-branded.md, Task 65). Edge is the
// exception: its tracking prevention is off under --headless, so it is
// measured with a window (Task 68).

func findBrowser(t *testing.T, id string) (discovery.Browser, bool) {
	t.Helper()
	found, err := discovery.Find(context.Background(), discovery.Options{})
	if err != nil {
		t.Fatalf("discovery: %v", err)
	}
	for _, b := range found {
		if b.ID == id {
			return b, true
		}
	}
	return discovery.Browser{}, false
}

// trackerServer serves testdata/branded over HTTP.
func trackerServer(t *testing.T) *httptest.Server {
	t.Helper()
	dir, err := filepath.Abs("../../testdata/branded")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.FileServer(http.Dir(dir)))
	t.Cleanup(srv.Close)
	return srv
}

// blockedCount opens the tracker page and counts the requests the browser
// itself refused (net::ERR_BLOCKED_BY_CLIENT), with their URLs for the log.
func blockedCount(t *testing.T, b discovery.Browser, template, page string, visible bool) (int, []string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var p *process.Blink
	var err error
	switch {
	case visible:
		p, err = process.StartBlinkVisible(ctx, b.Executable, "")
	case template != "":
		p, err = process.StartBlinkTemplate(ctx, b.Executable, "", template)
	default:
		p, err = process.StartBlink(ctx, b.Executable, "")
	}
	if err != nil {
		return 0, nil, fmt.Errorf("start %s: %w", b.ID, err)
	}
	defer func() { _ = p.Close(context.Background()) }()
	c, err := cdp.Dial(ctx, p.WSURL)
	if err != nil {
		return 0, nil, fmt.Errorf("dial %s: %w", b.ID, err)
	}
	defer func() { _ = c.Close() }()
	s, err := c.NewSession(ctx)
	if err != nil {
		return 0, nil, fmt.Errorf("session: %w", err)
	}
	if err := s.Navigate(ctx, page); err != nil {
		return 0, nil, fmt.Errorf("navigate: %w", err)
	}
	// Let the subresources finish; offline they fail, and that is not a block.
	_ = s.WaitNetworkIdle(ctx, 300*time.Millisecond, 15*time.Second)
	var blocked []string
	for _, f := range s.FailedRequests() {
		if strings.Contains(strings.ToUpper(f.Err), "BLOCKED_BY_CLIENT") {
			blocked = append(blocked, f.URL)
		}
	}
	return len(blocked), blocked, nil
}

func TestWarmTemplateBlocksTrackers(t *testing.T) {
	b, ok := findBrowser(t, "brave")
	if !ok {
		t.Skip("Brave is not installed")
	}
	srv := trackerServer(t)
	fresh, freshURLs, err := blockedCount(t, b, "", srv.URL+"/trackers.html", false)
	if err != nil {
		t.Fatal(err)
	}
	if fresh != 0 {
		t.Errorf("fresh Brave blocked %d request(s); the spike measured 0", fresh)
	}
	cache := t.TempDir()
	if _, err := branded.Warm(context.Background(), branded.WarmOptions{
		Browser: "brave", Exe: b.Executable, Version: b.Version, Cache: cache,
		Logf: t.Logf,
	}); err != nil {
		t.Skipf("warm failed (the lists need the network): %v", err)
	}
	tpl, err := branded.Find(cache, "brave", b.Version, time.Now())
	if err != nil || tpl == nil {
		t.Fatalf("no fresh template after Warm: %v", err)
	}
	warmed, warmedURLs, err := blockedCount(t, b, tpl.Dir, srv.URL+"/trackers.html", false)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Brave %s: fresh blocked %d %v, warmed blocked %d %v", b.Version, fresh, freshURLs, warmed, warmedURLs)
	if warmed == 0 {
		t.Fatal("warmed Brave blocked nothing; the template has no filter lists")
	}
}

func TestEdgeTrackingPrevention(t *testing.T) {
	b, ok := findBrowser(t, "edge")
	if !ok {
		t.Skip("Edge is not installed")
	}
	srv := trackerServer(t)
	headless, headlessURLs, err := blockedCount(t, b, "", srv.URL+"/trackers.html", false)
	if err != nil {
		t.Fatal(err)
	}
	visible, visibleURLs, err := blockedCount(t, b, "", srv.URL+"/trackers.html", true)
	if err != nil {
		// A runner without a desktop cannot answer the headed question; the log
		// says so instead of turning the lane red.
		t.Skipf("headed Edge: %v", err)
	}
	t.Logf("Edge %s: blocked headless %d %v, headed %d %v", b.Version, headless, headlessURLs, visible, visibleURLs)
	if headless != 0 {
		t.Logf("Edge blocked %d headless; the spike measured 0", headless)
	}
	if visible == 0 {
		t.Skip("Edge blocked nothing headed either; do not claim Edge support yet (Task 68)")
	}
}
