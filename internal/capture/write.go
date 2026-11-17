package capture

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// writeAtomic writes path through a temp file in the same directory and a
// rename, so a crash never leaves a partial file under the final name.
func writeAtomic(path string, write func(*os.File) error) (err error) {
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("capture: %w", err)
	}
	defer func() {
		if err != nil {
			_ = f.Close()
			_ = os.Remove(f.Name())
		}
	}()
	if err = write(f); err != nil {
		return fmt.Errorf("capture: write %s: %w", filepath.Base(path), err)
	}
	if err = f.Close(); err != nil {
		return fmt.Errorf("capture: %w", err)
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return fmt.Errorf("capture: %w", err)
	}
	return nil
}

// WritePageJSON writes <out>/<slug>/page.json: url, slug and capture time.
func WritePageJSON(out, slug, url string) error {
	dir := filepath.Join(out, slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("capture: %w", err)
	}
	page := map[string]string{"slug": slug, "url": url, "capturedAt": time.Now().UTC().Format(time.RFC3339)}
	return writeAtomic(filepath.Join(dir, "page.json"), func(f *os.File) error { return json.NewEncoder(f).Encode(page) })
}
