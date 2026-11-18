package capture

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/GerMP21/Omnisnap/internal/fsutil"
)

// WritePageJSON writes <out>/<slug>/page.json: url, slug and capture time.
func WritePageJSON(out, slug, url string) error {
	dir := filepath.Join(out, slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("capture: %w", err)
	}
	page := map[string]string{"slug": slug, "url": url, "capturedAt": time.Now().UTC().Format(time.RFC3339)}
	return fsutil.WriteAtomic(filepath.Join(dir, "page.json"), func(f io.Writer) error { return json.NewEncoder(f).Encode(page) })
}
