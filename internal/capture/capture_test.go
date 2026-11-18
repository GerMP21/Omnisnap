package capture

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GerMP21/Omnisnap/internal/drivers"
	"github.com/GerMP21/Omnisnap/internal/schema"
	"github.com/GerMP21/Omnisnap/internal/schema/schematest"
)

func opts(t *testing.T) Options {
	t.Helper()
	return Options{
		Out: t.TempDir(), Slug: "p", URL: "http://127.0.0.1/p.html",
		Engine: "chrome", Version: "157.0", Run: "a",
	}
}

func readCapture(t *testing.T, path string) schema.Capture {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	schematest.Validate(t, "capture", b)
	var c schema.Capture
	if err := json.Unmarshal(b, &c); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestCaptureOnePage(t *testing.T) {
	f := newFake()
	f.docH = []int{957, 1013} // the page grew after the screenshot
	f.failed = []drivers.FailedRequest{
		{URL: "https://ads.example.net/t.js", Err: "net::ERR_BLOCKED_BY_CLIENT"},
		{URL: "https://example.com/x", Err: "net::ERR_EMPTY_RESPONSE"},
	}
	o := opts(t)
	res, err := Page(context.Background(), f, o)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{
		"viewport 1280x800@1", "navigate http://127.0.0.1/p.html", "idle 500ms 8s",
		"stabilize", "idle 500ms 5s", "extract", "screenshot", "extract",
	}
	if strings.Join(f.calls, " | ") != strings.Join(want, " | ") {
		t.Errorf("calls:\n got %q\nwant %q", f.calls, want)
	}

	dir := filepath.Join(o.Out, "p")
	c := readCapture(t, filepath.Join(dir, "chrome-a.json"))
	m := c.Meta
	if m.VW != 1280 || m.VH != 800 || m.Engine != "chrome" || m.Version != "157.0" {
		t.Errorf("meta viewport %dx%d, engine %q %q", m.VW, m.VH, m.Engine, m.Version)
	}
	if m.Post.DocH != 1013 || m.Shot != (schema.Shot{W: 1280, H: 957}) {
		t.Errorf("post %+v, shot %+v; want the re-measured height and the screenshot size", m.Post, m.Shot)
	}
	if m.Stabilize.TimersCleared != 6 || len(m.Failed) != 2 || m.Blocked != 1 {
		t.Errorf("stabilize %+v, failed %d, blocked %d", m.Stabilize, len(m.Failed), m.Blocked)
	}
	img, err := os.Open(filepath.Join(dir, "chrome-a.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = img.Close() }()
	cfg, err := png.DecodeConfig(img)
	if err != nil || cfg.Width != 1280 || cfg.Height != 957 {
		t.Errorf("png %dx%d, %v", cfg.Width, cfg.Height, err)
	}
	if res.Nodes != 3 || res.Blocked != 1 {
		t.Errorf("result %+v", res)
	}
	assertOnly(t, dir, "chrome-a.json", "chrome-a.png")
}

// Screenshots are capped at 2560 x 16000 px (SPEC-bundle-schema.md).
func TestScreenshotIsCapped(t *testing.T) {
	f := newFake()
	f.docW, f.docH = 3000, []int{40000}
	if _, err := Page(context.Background(), f, opts(t)); err != nil {
		t.Fatal(err)
	}
	if got := f.clips[0]; got.Width != 2560 || got.Height != 16000 {
		t.Errorf("clip %+v; want 2560 x 16000", got)
	}
}

// Classic has no network events: a fixed settle instead of network idle.
func TestNoNetworkIdleMeansFixedSettle(t *testing.T) {
	f := newFake()
	f.caps.NetworkIdle = false
	o := opts(t)
	o.settle = 1 // 1ns: the test must not wait 1.5 s
	if _, err := Page(context.Background(), f, o); err != nil {
		t.Fatal(err)
	}
	for _, c := range f.calls {
		if strings.HasPrefix(c, "idle") {
			t.Errorf("WaitNetworkIdle called without the capability: %q", f.calls)
		}
	}
}

func TestFailureWritesErrorFileOnly(t *testing.T) {
	for name, setup := range map[string]func(*fakeSession){
		"navigation": func(f *fakeSession) {
			f.navErr = fmt.Errorf("cdp: navigate: %w: net::ERR_CONNECTION_REFUSED", drivers.ErrNavigation)
		},
		"screenshot": func(f *fakeSession) { f.shotErr = errors.New("cdp: Page.captureScreenshot: boom") },
	} {
		t.Run(name, func(t *testing.T) {
			f := newFake()
			setup(f)
			o := opts(t)
			_, err := Page(context.Background(), f, o)
			if err == nil {
				t.Fatal("want an error")
			}
			dir := filepath.Join(o.Out, "p")
			msg, rerr := os.ReadFile(filepath.Join(dir, "chrome-a.error.txt"))
			if rerr != nil || !strings.Contains(string(msg), err.Error()) {
				t.Errorf("error.txt = %q, %v; want the error", msg, rerr)
			}
			assertOnly(t, dir, "chrome-a.error.txt")
		})
	}
}

func TestWritePageJSON(t *testing.T) {
	out := t.TempDir()
	if err := WritePageJSON(out, "p", "http://127.0.0.1/p.html"); err != nil {
		t.Fatal(err)
	}
	var p struct{ Slug, URL, CapturedAt string }
	b, err := os.ReadFile(filepath.Join(out, "p", "page.json"))
	if err != nil || json.Unmarshal(b, &p) != nil || p.Slug != "p" || p.URL != "http://127.0.0.1/p.html" || p.CapturedAt == "" {
		t.Errorf("page.json = %s, %v", b, err)
	}
}

// assertOnly fails unless dir holds exactly these files.
func assertOnly(t *testing.T, dir string, names ...string) {
	t.Helper()
	entries, _ := os.ReadDir(dir)
	var got []string
	for _, e := range entries {
		got = append(got, e.Name())
	}
	if strings.Join(got, ",") != strings.Join(names, ",") {
		t.Errorf("%s holds %v; want %v", dir, got, names)
	}
}

// Review finding 1: every step has a deadline, even when the caller's
// context has none (a hung renderer must not stall the whole job).
func TestEveryStepHasADeadline(t *testing.T) {
	for _, step := range []string{"viewport", "screenshot"} {
		t.Run(step, func(t *testing.T) {
			f := newFake()
			f.hang = step
			o := opts(t)
			o.Timeouts.ScriptMS = 50
			done := make(chan error, 1)
			go func() { _, err := Page(context.Background(), f, o); done <- err }()
			select {
			case err := <-done:
				if !errors.Is(err, context.DeadlineExceeded) {
					t.Errorf("err = %v; want the step's deadline", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("Page hung: no deadline on " + step)
			}
		})
	}
}

// Review finding 2: a retry replaces the previous attempt's files, so the
// bundle never holds a capture and an error for the same run.
func TestRetryReplacesPreviousAttempt(t *testing.T) {
	o := opts(t)
	dir := filepath.Join(o.Out, "p")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "chrome-a.error.txt"), []byte("old failure\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Page(context.Background(), newFake(), o); err != nil {
		t.Fatal(err)
	}
	assertOnly(t, dir, "chrome-a.json", "chrome-a.png")

	f := newFake()
	f.navErr = fmt.Errorf("cdp: navigate: %w", drivers.ErrNavigation)
	if _, err := Page(context.Background(), f, o); err == nil {
		t.Fatal("want the navigation error")
	}
	assertOnly(t, dir, "chrome-a.error.txt")
}
