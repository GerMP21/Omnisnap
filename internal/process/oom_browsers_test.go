//go:build linux && browsers

package process

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/GerMP21/Omnisnap/internal/discovery"
	"github.com/GerMP21/Omnisnap/internal/drivers"
	"github.com/GerMP21/Omnisnap/internal/drivers/bidi"
	"github.com/GerMP21/Omnisnap/internal/drivers/cdp"
	"github.com/GerMP21/Omnisnap/internal/drivers/classic"
	"github.com/GerMP21/Omnisnap/internal/schema"
)

// Change cap-live-browsers, task 2.4: with real browsers open on a page, every
// process of every launched tree (renderers, content and web processes, the
// drivers, the private Weston) reads oom_score_adj 1000 after ApplyOOMScore;
// the test process and a process Omnisnap did not start keep theirs.
func TestOOMScoreOnRealBrowsers(t *testing.T) {
	orig := oomScore(t, os.Getpid())
	other := exec.Command("sleep", "120")
	if err := other.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = other.Process.Kill(); _ = other.Wait() }()
	otherBefore := oomScore(t, other.Process.Pid)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!doctype html><title>oom</title><p>A page, so the browsers start their renderers.</p>`)
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	var opened []string
	open := func(name string, s drivers.Session, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("%s: session: %v", name, err)
		}
		if err := s.Navigate(ctx, srv.URL); err != nil {
			t.Fatalf("%s: navigate: %v", name, err)
		}
		opened = append(opened, name)
		t.Cleanup(func() { _ = s.Close(context.Background()) })
	}
	if exe := lookFor("OMNISNAP_CHROME", "google-chrome-stable", "google-chrome", "chromium", "chrome"); exe != "" {
		b, err := StartBlink(ctx, exe, "")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = b.Close(context.Background()) })
		c, err := cdp.Dial(ctx, b.WSURL)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = c.Close() })
		s, err := c.NewSession(ctx)
		open("chrome", s, err)
	}
	if exe := lookFor("OMNISNAP_FIREFOX", "firefox", "firefox-esr"); exe != "" {
		g, err := StartGecko(ctx, exe, "")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = g.Close(context.Background()) })
		c, err := bidi.Dial(ctx, g.WSURL)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = c.Close(context.Background()) })
		s, err := c.NewSession(ctx)
		open("firefox", s, err)
	}
	if found, err := discovery.Find(ctx, discovery.Options{}); err == nil {
		for _, b := range found {
			if b.Family != schema.FamilyWebKit || b.ID == "wpe" || b.Driver == "" {
				continue
			}
			w, err := StartWebKit(ctx, WebKitOptions{Driver: b.Driver, Browser: b.Executable})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = w.Close(context.Background()) })
			c := classic.New(w.URL)
			c.Capabilities = w.Capabilities
			s, err := c.NewSession(ctx)
			open(b.ID, s, err)
			break
		}
	}
	if len(opened) == 0 {
		t.Skip("no browser installed")
	}

	if err := ApplyOOMScore(); err != nil {
		t.Fatalf("ApplyOOMScore: %v", err)
	}
	checked := 0
	liveHandles.Range(func(k, _ any) bool {
		h := k.(*Handle)
		for _, pid := range append([]int{h.PID()}, descendants(t, h.PID())...) {
			got, ok := readScore(pid)
			if !ok {
				continue // exited since the scan
			}
			checked++
			if got != oomScoreMax {
				t.Errorf("%s (pid %d, tree of %s): oom_score_adj %d; want %d", comm(pid), pid, comm(h.PID()), got, oomScoreMax)
			}
		}
		return true
	})
	t.Logf("%s open: %d processes checked", strings.Join(opened, ", "), checked)
	if got := oomScore(t, os.Getpid()); got != orig {
		t.Errorf("the test process: %d; want %d", got, orig)
	}
	if got := oomScore(t, other.Process.Pid); got != otherBefore {
		t.Errorf("a process Omnisnap did not start: %d; want %d", got, otherBefore)
	}
}

func lookFor(env string, names ...string) string {
	if p := os.Getenv(env); p != "" {
		return p
	}
	for _, n := range names {
		if p, err := exec.LookPath(n); err == nil {
			return p
		}
	}
	return ""
}

func readScore(pid int) (int, bool) {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/oom_score_adj")
	if err != nil {
		return 0, false
	}
	v, err := strconv.Atoi(strings.TrimSpace(string(b)))
	return v, err == nil
}

func comm(pid int) string {
	b, _ := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/comm")
	return strings.TrimSpace(string(b))
}
