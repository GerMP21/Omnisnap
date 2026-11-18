// Package fsutil holds the file helpers shared by the modules that write the
// bundle.
package fsutil

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// WriteAtomic writes path through a temp file in the same directory and a
// rename, so a crash never leaves a partial file under the final name.
func WriteAtomic(path string, write func(io.Writer) error) (err error) {
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = f.Close()
			_ = os.Remove(f.Name())
		}
	}()
	if err = write(f); err != nil {
		return fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
