// Package testutil holds helpers shared by tests.
package testutil

import (
	"os"
	"strings"
	"testing"
)

// ReadSHA256Sums parses a manifest in sha256sum's output format
// ("<hex>  <name>" per line, so `sha256sum -c` checks it too) into name → hex.
func ReadSHA256Sums(t testing.TB, path string) map[string]string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	sums := map[string]string{}
	for _, line := range strings.Split(string(b), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if len(fields) != 2 {
			t.Fatalf("%s: malformed line %q", path, line)
		}
		sums[strings.TrimPrefix(fields[1], "*")] = fields[0]
	}
	return sums
}
