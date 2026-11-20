package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/GerMP21/Omnisnap/internal/schema"
	"github.com/GerMP21/Omnisnap/internal/schema/schematest"
)

// Task 89: a browser that crashes is relaunched and the run retried once;
// crashing again takes the engine out of the job with one message, and the
// rest of the job goes on. Never `open firefox: connection closed` page after
// page.
func TestCrashedEngineLeavesTheJobWithOneMessage(t *testing.T) {
	m := &fakeMachine{crash: map[string]bool{"firefox": true}}
	m.browsers = append(m.browsers, blink("chromium"), blink("chrome"), gecko("firefox"))
	out := t.TempDir()
	code, stdout, stderr := run(t, m, "https://www.youtube.com/", "https://www.ui.com/", "--out", out,
		"--engines", "chromium,chrome,firefox")
	if code != ExitOK {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if n := strings.Count(stdout, "out of the job"); n != 1 {
		t.Errorf("%d loss messages; want exactly one:\n%s", n, stdout)
	}
	if !strings.Contains(stdout, "firefox crashed twice on ") {
		t.Errorf("stdout does not name the crash:\n%s", stdout)
	}
	if strings.Count(stdout, "firefox    run") != 1 {
		t.Errorf("want one firefox capture line (the lost one), later captures skipped quietly:\n%s", stdout)
	}
	if strings.Contains(stdout, "open firefox") {
		t.Errorf("a capture ran in the dead browser:\n%s", stdout)
	}
	launches := 0
	for _, id := range m.launched {
		if id == "firefox" {
			launches++
		}
	}
	if launches != 2 {
		t.Errorf("firefox launched %d times; want 2 (the first browser, one relaunch)", launches)
	}
	// Every launched browser is closed, the replacement included.
	if len(m.closed) != len(m.launched) {
		t.Errorf("launched %v, closed %v", m.launched, m.closed)
	}
	s := readSummary(t, out)
	var chromium, firefox int
	for _, r := range s.Results {
		switch r.Engine.Engine {
		case "chromium":
			if r.Status == schema.StatusOK {
				chromium++
			}
		case "firefox":
			if r.Status != schema.StatusFailed || len(r.Warnings) == 0 || !strings.Contains(r.Warnings[0], "crashed twice") {
				t.Errorf("firefox result %+v; want failed, the warning naming the crash", r)
			}
			firefox++
		}
	}
	if chromium != 2 || firefox != 2 {
		t.Errorf("chromium ok on %d pages, firefox results %d; want chromium on both pages and a failed firefox result for each", chromium, firefox)
	}
}

// --json: the loss is one valid engine "failed" event, after firefox's failed
// capture event.
func TestCrashedEngineJSONEvent(t *testing.T) {
	m := &fakeMachine{crash: map[string]bool{"firefox": true}}
	m.browsers = append(m.browsers, blink("chromium"), gecko("firefox"))
	code, stdout, stderr := run(t, m, "https://www.youtube.com/", "https://www.ui.com/", "--out", t.TempDir(), "--json")
	if code != ExitOK {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	failed := 0
	for _, l := range lines(stdout) {
		schematest.Validate(t, "progress", []byte(l))
		var ev struct{ Type, Engine, Status, Error string }
		_ = json.Unmarshal([]byte(l), &ev)
		if ev.Type == "engine" && ev.Status == "failed" {
			failed++
			if ev.Engine != "firefox" || !strings.Contains(ev.Error, "out of the job") {
				t.Errorf("engine event %s; want firefox's loss", l)
			}
		}
	}
	if failed != 1 {
		t.Errorf("%d engine failed events; want 1:\n%s", failed, stdout)
	}
}
