//go:build unix

package process

// afterStart: the process group was set before Start (setProcessGroup).
func (h *Handle) afterStart() {}

// killTree kills the browser's process group, even after the parent exited:
// children can outlive it.
func (h *Handle) killTree() { killGroup(h.cmd) }
