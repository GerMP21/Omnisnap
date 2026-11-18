package capture

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/GerMP21/Omnisnap/internal/schema"
)

// write puts files into <out>/<slug>/.
func write(t *testing.T, out, slug string, files ...string) {
	t.Helper()
	dir := filepath.Join(out, slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestResumeState(t *testing.T) {
	const slug = "p"
	for _, tc := range []struct {
		name        string
		runs        int
		files       []string
		wantDone    bool
		wantUnreach bool
	}{
		{"both runs captured", 2, []string{"chrome-a.json", "chrome-a.png", "chrome-b.json", "chrome-b.png"}, true, false},
		{"run a only", 2, []string{"chrome-a.json", "chrome-a.png"}, false, false},
		{"run a failed: run b is skipped by policy", 2, []string{"chrome-a.error.txt"}, true, false},
		{"run b failed", 2, []string{"chrome-a.json", "chrome-b.error.txt"}, true, false},
		{"one run job", 1, []string{"chrome-a.json"}, true, false},
		{"nothing", 2, nil, false, false},
		{"unreachable", 2, []string{"failed.json"}, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := t.TempDir()
			write(t, out, slug, tc.files...)
			got := ResumeState(out, slug, []string{"chrome"}, tc.runs)
			if got.Done != tc.wantDone || got.Unreachable != tc.wantUnreach {
				t.Errorf("ResumeState = %+v; want done=%v unreachable=%v", got, tc.wantDone, tc.wantUnreach)
			}
		})
	}
}

// Every engine must be done, not just one.
func TestResumeStateNeedsEveryEngine(t *testing.T) {
	out := t.TempDir()
	write(t, out, "p", "chrome-a.json", "chrome-b.json")
	if st := ResumeState(out, "p", []string{"chrome", "firefox"}, 2); st.Done {
		t.Error("done with firefox never captured")
	}
	write(t, out, "p", "firefox-a.json", "firefox-b.json")
	if st := ResumeState(out, "p", []string{"chrome", "firefox"}, 2); !st.Done {
		t.Error("not done with every engine captured")
	}
}

// A retry of a page the policy skipped run b for stays not-done only if a run
// after the failure exists: the failure itself is an attempt.
func TestResumeSkipsUnreachable(t *testing.T) {
	out := t.TempDir()
	if err := WriteFailure(out, schema.PageRef{Slug: "p", URL: "https://example.com/"}, "baseline timed out"); err != nil {
		t.Fatal(err)
	}
	st := ResumeState(out, "p", []string{"chrome"}, 2)
	if !st.Done || !st.Unreachable {
		t.Errorf("ResumeState = %+v; want done and unreachable", st)
	}
	if st.Reason == "" {
		t.Error("no reason from failed.json")
	}
}
