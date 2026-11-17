// Package viewer renders report.html from a summary (SPEC-viewer.md): one
// self-contained file that works offline from file://. Screenshots are
// referenced by relative path, never inlined.
package viewer

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/GerMP21/Omnisnap/internal/schema"
)

var (
	//go:embed template.html
	templateHTML string
	//go:embed app.js
	appJS string
	//go:embed style.css
	styleCSS string
)

// Render writes the report for s. The summary is a JSON data island:
// encoding/json escapes <, > and &, so no page text can close the script tag,
// and app.js inserts every string with textContent.
func Render(w io.Writer, s schema.Summary) error {
	data, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("viewer: %w", err)
	}
	page := strings.NewReplacer(
		"{{style}}", styleCSS,
		"{{data}}", string(data),
		"{{app}}", appJS,
	).Replace(templateHTML)
	_, err = io.WriteString(w, page)
	return err
}

// Write renders <dir>/report.html through a temp file and a rename.
func Write(dir string, s schema.Summary) (err error) {
	f, err := os.CreateTemp(dir, ".report.html.tmp-*")
	if err != nil {
		return fmt.Errorf("viewer: %w", err)
	}
	defer func() {
		if err != nil {
			_ = f.Close()
			_ = os.Remove(f.Name())
		}
	}()
	if err = Render(f, s); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return fmt.Errorf("viewer: %w", err)
	}
	return os.Rename(f.Name(), filepath.Join(dir, "report.html"))
}
