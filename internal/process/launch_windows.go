//go:build windows

package process

// afterStart puts the browser in its own process group and in a kill-on-close
// Job Object. There is a small race: the browser could spawn a child before
// it is assigned; Chrome and Firefox do not spawn renderers in the first
// milliseconds, and the sweeper (Task 29) is the second line of defence.
func (h *Handle) afterStart() {
	if job, err := createJob(h.cmd.Process.Pid); err == nil {
		h.job = job
	}
}

// killTree closes the job (killing every process in it) and then the browser.
func (h *Handle) killTree() {
	closeJob(h.job)
	h.job = 0
	_ = h.cmd.Process.Kill()
}
