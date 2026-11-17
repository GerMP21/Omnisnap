//go:build browsers

package discovery

import (
	"context"
	"slices"
	"testing"
	"time"
)

// On a real machine: list what is found, and check the time budget
// (SPEC-discovery.md: < 300 ms on Linux and Windows, median of 10, warm cache).
func TestThisMachine(t *testing.T) {
	var times []time.Duration
	var found []Browser
	for range 10 {
		start := time.Now()
		got, err := Find(context.Background(), Options{})
		times = append(times, time.Since(start))
		if err != nil {
			t.Fatal(err)
		}
		found = got
	}
	slices.Sort(times)
	for _, b := range found {
		t.Logf("%-12s %-7s %-8s %-14s %-40s %s %v", b.ID, b.Family, b.Protocol, b.Version, b.Executable, b.Source, b.Notes)
	}
	if len(found) == 0 {
		t.Error("no browser found")
	}
	if median := times[len(times)/2]; median > 300*time.Millisecond {
		t.Errorf("median %v, want < 300 ms", median)
	} else {
		t.Logf("median %v over 10 runs", median)
	}
}
