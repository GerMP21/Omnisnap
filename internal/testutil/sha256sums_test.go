package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadSHA256Sums(t *testing.T) {
	path := filepath.Join(t.TempDir(), "SHA256SUMS")
	content := "aa11  a.js\nbb22  b.js\n\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got := ReadSHA256Sums(t, path)
	if len(got) != 2 || got["a.js"] != "aa11" || got["b.js"] != "bb22" {
		t.Errorf("got %v", got)
	}
}
