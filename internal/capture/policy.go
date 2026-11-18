package capture

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/GerMP21/Omnisnap/internal/drivers"
	"github.com/GerMP21/Omnisnap/internal/fsutil"
	"github.com/GerMP21/Omnisnap/internal/schema"
)

// The failure policy of SPEC-capture.md, live pages only (planted pages are
// always fully captured: Plan.FullCapture):
//
//   - the baseline's run a fails with a timeout or a network error, or returns
//     fewer than botWallNodes elements (a challenge page) -> the page is
//     unreachable: failed.json, no other engine captures it, run b is skipped;
//   - an engine that failed run a skips its run b.

// botWallNodes: below this element count the baseline did not get the page
// (the spike: challenge pages render a handful of nodes).
const botWallNodes = 50

// Failure is <slug>/failed.json: why a page is unreachable.
type Failure struct {
	Slug   string    `json:"slug"`
	URL    string    `json:"url"`
	Reason string    `json:"reason"`
	At     time.Time `json:"at"`
}

// unreachable reports whether the baseline's run a means the page cannot be
// compared: a timeout or navigation error, or a bot wall. allowSmall drops the
// size rule, not the error rules (a small page still has to load).
func unreachable(res Result, err error, allowSmall bool) (string, bool) {
	switch {
	case err == nil && !allowSmall && res.Nodes < botWallNodes:
		return fmt.Sprintf("baseline rendered %d elements: a bot wall or a broken page", res.Nodes), true
	case errors.Is(err, drivers.ErrTimeout):
		return "baseline timed out: " + err.Error(), true
	case errors.Is(err, drivers.ErrNavigation):
		return "baseline could not load the page: " + err.Error(), true
	}
	return "", false
}

// WriteFailure writes <out>/<slug>/failed.json atomically.
func WriteFailure(out string, page schema.PageRef, reason string) error {
	dir := filepath.Join(out, page.Slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("capture: %w", err)
	}
	f := Failure{Slug: page.Slug, URL: page.URL, Reason: reason, At: time.Now().UTC()}
	return fsutil.WriteAtomic(filepath.Join(dir, "failed.json"), func(w io.Writer) error {
		return json.NewEncoder(w).Encode(f)
	})
}

// ReadFailure reads <out>/<slug>/failed.json; fs.ErrNotExist when the page is
// not marked unreachable.
func ReadFailure(out, slug string) (Failure, error) {
	b, err := os.ReadFile(filepath.Join(out, slug, "failed.json"))
	if err != nil {
		return Failure{}, err
	}
	var f Failure
	if err := json.Unmarshal(b, &f); err != nil {
		return Failure{}, fmt.Errorf("capture: %s/failed.json: %w", slug, err)
	}
	return f, nil
}
