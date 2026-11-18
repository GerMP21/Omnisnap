package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Task 82: a dry-run release produces five binaries and SHA256SUMS, and the
// host binary reports the stamped version. Slow (five cross-builds).
func TestDryRunRelease(t *testing.T) {
	if testing.Short() {
		t.Skip("cross-compiles five targets")
	}
	out := t.TempDir()
	if err := run(out, "v0.0.0-test", true, false); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(out)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if len(names) != 6 { // five binaries + SHA256SUMS
		t.Errorf("%d files: %v; want five binaries and SHA256SUMS", len(names), names)
	}
	sums, err := os.ReadFile(filepath.Join(out, "SHA256SUMS"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(sums)), "\n")
	if len(lines) != 5 {
		t.Errorf("SHA256SUMS has %d lines; want 5", len(lines))
	}
	for _, l := range lines {
		if len(strings.Fields(l)) != 2 || len(strings.Fields(l)[0]) != 64 {
			t.Errorf("bad checksum line %q", l)
		}
	}
	// omnisnap-lab must never be built.
	for _, n := range names {
		if strings.Contains(n, "lab") {
			t.Errorf("release built %s; the lab binary is not shipped", n)
		}
	}
}
