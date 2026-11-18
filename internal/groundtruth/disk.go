package groundtruth

import (
	"fmt"
	"os"
)

// Disk guards (Task 73, SPEC-ground-truth.md): a crawl writes hundreds of MB
// per site; on a developer machine (WSL: an 8 GB VM, a virtual disk on C: that
// never shrinks) that is a real hazard. Warn below 20 GB free, stop below 5 GB
// on the filesystem holding the output.

// DiskGuard holds the thresholds (GB) and the free-space lookup, injectable in
// tests.
type DiskGuard struct {
	WarnGB float64
	StopGB float64
	Free   func(dir string) (uint64, error) // default: the OS lookup
}

// DiskDecision is what the guard says.
type DiskDecision struct {
	FreeGB float64
	Stop   bool
	Warn   bool
	Err    error
}

// Check measures the output filesystem.
func (g *DiskGuard) Check(dir string) DiskDecision {
	warn, stop := g.WarnGB, g.StopGB
	if warn == 0 {
		warn = 20
	}
	if stop == 0 {
		stop = 5
	}
	free := g.Free
	if free == nil {
		free = FreeBytes
	}
	n, err := free(dir)
	if err != nil {
		return DiskDecision{Err: fmt.Errorf("groundtruth: disk: %w", err)}
	}
	gb := float64(n) / (1 << 30)
	return DiskDecision{FreeGB: gb, Warn: gb < warn, Stop: gb < stop}
}

// EnsureOut makes sure the output directory exists before measuring it.
func EnsureOut(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("groundtruth: %w", err)
	}
	return nil
}
