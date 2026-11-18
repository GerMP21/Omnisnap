//go:build browsers && unix

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/GerMP21/Omnisnap/internal/drivers/cdp"
	"github.com/GerMP21/Omnisnap/internal/groundtruth"
	"github.com/GerMP21/Omnisnap/internal/process"
	"github.com/GerMP21/Omnisnap/internal/schema"
	"github.com/GerMP21/Omnisnap/internal/schema/schematest"
)

// The walking skeleton: omnisnap <url> on a planted page with the installed
// Blink browser writes a schema-valid bundle and a report that opens cleanly.
func TestWalkingSkeleton(t *testing.T) {
	srv, err := groundtruth.Start("../../testdata/planted")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = srv.Close() }()
	out := t.TempDir()
	var stdout, stderr bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	code := main0(ctx, []string{"omnisnap", srv.URL("01-card-width"), "--out", out, "--json"}, &stdout, &stderr, defaultEnv())
	if code != ExitOK {
		t.Fatalf("exit %d\nstderr: %s", code, stderr.String())
	}
	for _, l := range lines(stdout.String()) {
		schematest.Validate(t, "progress", []byte(l))
	}
	b, err := os.ReadFile(filepath.Join(out, "summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	schematest.Validate(t, "summary", b)
	var s schema.Summary
	_ = json.Unmarshal(b, &s)
	if len(s.Results) == 0 || s.Results[0].Status != schema.StatusOK || s.Results[0].Shots == nil {
		t.Fatalf("results %+v", s.Results)
	}
	t.Logf("%d result(s), skipped %+v", len(s.Results), s.Skipped)

	// Open the report it wrote: no console errors, one section per result.
	exe := s.Hosts[0].Engines[0].Executable
	proc, err := process.StartBlink(ctx, exe, "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = proc.Close(context.Background()) }()
	br, err := cdp.Dial(ctx, proc.WSURL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = br.Close() }()
	sess, err := br.NewSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_ = sess.AddPreloadScript(ctx, `window.__errors = []; addEventListener('error', (e) => window.__errors.push(String(e.message)));
const __ce = console.error; console.error = (...a) => { window.__errors.push(a.join(' ')); __ce(...a); };`)
	if err := sess.Navigate(ctx, "file://"+filepath.Join(out, "report.html")); err != nil {
		t.Fatal(err)
	}
	if errs, _ := sess.Evaluate(ctx, "JSON.stringify(window.__errors)"); errs != "[]" {
		t.Errorf("console errors: %s", errs)
	}
	loaded, _ := sess.Evaluate(ctx, `(async () => { await Promise.all([...document.images].map((i) => i.decode().catch(() => false))); return String([...document.images].every((i) => i.naturalWidth > 0)); })()`)
	if loaded != "true" {
		t.Errorf("screenshots did not load from the report's relative paths")
	}
}
