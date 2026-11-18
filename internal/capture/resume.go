package capture

import (
	"fmt"
	"os"
	"path/filepath"
)

// Resume (SPEC-capture.md): a page counts as done when every engine was
// attempted to the end — its last run captured or failed, or its run a failed
// (an engine that failed run a skips run b). An unreachable page is done too;
// --retry-failed retries it.

// PageState is what the bundle says about one page.
type PageState struct {
	Done        bool
	Unreachable bool
	Reason      string // why it is unreachable
}

// ResumeState reads <out>/<slug> for the job's engines and run count.
func ResumeState(out, slug string, engines []string, runs int) PageState {
	if f, err := ReadFailure(out, slug); err == nil {
		return PageState{Done: true, Unreachable: true, Reason: f.Reason}
	}
	if runs < 1 {
		runs = 1
	}
	for _, engine := range engines {
		if !engineDone(out, slug, engine, runs) {
			return PageState{}
		}
	}
	return PageState{Done: true}
}

// engineDone is true when the engine's runs ended: every run has a capture or
// an error file, or an earlier run failed (the rest are skipped).
func engineDone(out, slug, engine string, runs int) bool {
	any := false
	for i := range runs {
		letter := string(rune('a' + i))
		base := filepath.Join(out, slug, engine+"-"+letter)
		captured := exists(base + ".json")
		failed := exists(base + ".error.txt")
		if captured {
			any = true
			continue
		}
		if failed {
			return true // run 1..i are skipped by the failure policy
		}
		return false
	}
	return any
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// ErrResumeSkipped marks a page skipped by --resume; the CLI reports it and
// moves on.
var ErrResumeSkipped = fmt.Errorf("resume: page already done")
