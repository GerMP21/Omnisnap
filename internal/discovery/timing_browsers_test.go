//go:build browsers

package discovery

import (
	"context"
	"sort"
	"testing"
	"time"
)

// Task 32: discovery is file reading, so it is fast: < 300 ms on Linux and
// Windows, < 1 s on macOS (median of 10). CI runs this on all three OSes.
func TestDiscoveryTiming(t *testing.T) {
	budget := 300 * time.Millisecond
	if runtimeGOOS() == "darwin" {
		budget = time.Second
	}
	var samples []time.Duration
	for range 10 {
		start := time.Now()
		if _, err := Find(context.Background(), Options{}); err != nil {
			t.Fatal(err)
		}
		samples = append(samples, time.Since(start))
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	median := samples[len(samples)/2]
	if median > budget {
		t.Errorf("discovery median %v; want < %v", median, budget)
	}
	t.Logf("discovery median %v (budget %v)", median, budget)
}
