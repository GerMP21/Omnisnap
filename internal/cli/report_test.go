package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/GerMP21/Omnisnap/internal/schema"
	"github.com/GerMP21/Omnisnap/internal/viewer"
)

// Task 61: re-rendering an existing bundle reproduces the same report bytes.
func TestReportRerenders(t *testing.T) {
	dir := t.TempDir()
	b, err := os.ReadFile("../../testdata/viewer/m0.json")
	if err != nil {
		t.Fatal(err)
	}
	var s schema.Summary
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "summary.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := viewer.Write(dir, s); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(filepath.Join(dir, "report.html"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "report.html")); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := run(t, &fakeMachine{}, "report", dir)
	if code != ExitOK {
		t.Fatalf("exit %d, %s", code, stderr)
	}
	if !bytes.Contains([]byte(stdout), []byte("report.html")) {
		t.Errorf("stdout %q; want the report path", stdout)
	}
	second, err := os.ReadFile(filepath.Join(dir, "report.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Error("re-rendered report differs from the first render")
	}
	// A directory without a summary is a usage error, not a crash.
	if code, _, _ := run(t, &fakeMachine{}, "report", t.TempDir()); code != ExitUsage {
		t.Errorf("empty bundle: exit %d; want %d", code, ExitUsage)
	}
}
