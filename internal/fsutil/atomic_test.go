package fsutil

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// A failure while writing leaves neither a partial file nor a temp file.
func TestWriteAtomicLeavesNoPartialFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "chrome-a.json")
	err := WriteAtomic(target, func(w io.Writer) error {
		if _, err := io.WriteString(w, `{"meta":`); err != nil {
			return err
		}
		return errors.New("crash mid-write")
	})
	if err == nil {
		t.Fatal("want the write error")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("dir holds %v after a failed write", entries)
	}
	if err := WriteAtomic(target, func(w io.Writer) error { _, err := io.WriteString(w, "{}"); return err }); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(target); err != nil || string(b) != "{}" {
		t.Errorf("got %q, %v", b, err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("dir holds %v; want only the file", entries)
	}
}
