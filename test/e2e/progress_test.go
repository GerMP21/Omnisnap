//go:build e2e

package e2e

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/GerMP21/Omnisnap/internal/cli"
	"github.com/GerMP21/Omnisnap/internal/discovery"
	"github.com/GerMP21/Omnisnap/internal/groundtruth"
)

// Change cap-live-browsers, task 3.4: a job within the live-browser cap emits
// the progress stream it emitted before the cap existed. The reference is the
// stream of the code before the change (testdata/progress/README.md), for
// Chrome, Firefox and WebKitGTK on the planted suite: three engines, the
// default cap, one group. It needs the three engines installed, so it runs on
// a developer machine and is skipped where one is missing.

const progressRef = "../../testdata/progress/three-engines-planted.ndjson"

func TestProgressStreamWithinTheCap(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e capture suite")
	}
	found, err := discovery.Find(t.Context(), discovery.Options{})
	if err != nil {
		t.Fatal(err)
	}
	have := map[string]bool{}
	for _, b := range found {
		have[b.ID] = true
	}
	for _, id := range []string{"chrome", "firefox", "webkitgtk"} {
		if !have[id] {
			t.Skipf("needs chrome, firefox and webkitgtk; %s is not installed", id)
		}
	}
	pages, err := groundtruth.LoadPlanted("../../testdata/planted")
	if err != nil {
		t.Fatal(err)
	}
	srv, err := groundtruth.Start("../../testdata/planted")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = srv.Close() }()
	type pageRef struct {
		URL  string `json:"url"`
		Slug string `json:"slug"`
	}
	job := struct {
		Pages    []pageRef `json:"pages"`
		Engines  []string  `json:"engines"`
		Baseline string    `json:"baseline"`
		Output   string    `json:"output"`
	}{Engines: []string{"chrome", "firefox", "webkitgtk"}, Baseline: "chrome", Output: filepath.Join(t.TempDir(), "out")}
	for _, p := range pages {
		job.Pages = append(job.Pages, pageRef{URL: srv.URL(p.Slug), Slug: p.Slug})
	}
	jobFile := filepath.Join(t.TempDir(), "job.json")
	b, _ := json.Marshal(job)
	if err := os.WriteFile(jobFile, b, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := cli.Main([]string{"omnisnap", "run", jobFile, "--json", "--allow-small-pages"}, &stdout, &stderr); code != cli.ExitOK {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	ref, err := os.ReadFile(progressRef)
	if err != nil {
		t.Fatal(err)
	}
	got, want := normalizeProgress(t, stdout.Bytes()), normalizeProgress(t, ref)
	if len(got) != len(want) {
		t.Errorf("%d normalized events; the reference has %d", len(got), len(want))
	}
	for i := range min(len(got), len(want)) {
		if got[i] != want[i] {
			t.Fatalf("event %d:\n got  %s\n want %s", i, got[i], want[i])
		}
	}
}

var hostPort = regexp.MustCompile(`^http://[^/]+`)

// normalizeProgress drops what varies between runs and machines (timestamps,
// durations, browser versions, the output path, the server's port) and, within
// a page, lists each protocol lane's events in their own order: the CDP/BiDi
// lane and the Classic lane run in parallel, so how their events interleave
// changes from run to run, while each lane's order does not.
func normalizeProgress(t *testing.T, raw []byte) []string {
	t.Helper()
	var out, page []string
	flush := func() {
		// Stable: within a lane the order is kept.
		sort.SliceStable(page, func(i, j int) bool { return laneOf(page[i]) < laneOf(page[j]) })
		out = append(out, page...)
		page = nil
	}
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var e map[string]any
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			t.Fatalf("%s: %v", sc.Text(), err)
		}
		for _, k := range []string{"ts", "ms", "version"} {
			delete(e, k)
		}
		if _, ok := e["output"]; ok {
			e["output"] = "<out>"
		}
		if _, ok := e["summary"]; ok {
			e["summary"] = "<out>/summary.json"
		}
		if u, ok := e["url"].(string); ok {
			e["url"] = hostPort.ReplaceAllString(u, "")
		}
		line, _ := json.Marshal(e) // map keys sort, so both sides serialize alike
		switch e["type"] {
		case "capture":
			page = append(page, string(line))
			continue
		case "page-done":
			flush()
		}
		out = append(out, string(line))
	}
	flush()
	return out
}

// laneOf is a capture event's protocol lane: WebKit engines run over Classic.
func laneOf(line string) string {
	for _, id := range []string{`"engine":"webkitgtk"`, `"engine":"wpe"`, `"engine":"safari"`} {
		if strings.Contains(line, id) {
			return "classic"
		}
	}
	return "cdp+bidi"
}
