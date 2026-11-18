// Package process starts browsers in a known, isolated state and always
// cleans them up: every child process and every temporary profile
// (SPEC-process.md).
package process

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
)

// maxStderr bounds the browser output kept for error messages.
const maxStderr = 16 << 10

// Handle is a running browser process with its temp dir. Close it with defer.
type Handle struct {
	cmd    *exec.Cmd
	tmp    tempDir
	out    *tailBuffer
	exit   chan struct{} // closed when the process has exited
	reaped bool          // the exit watcher already reaped it (valid after exit closes)

	job uintptr // Windows: the Job Object handle (kill on close)

	closeOnce sync.Once
	closeErr  error
}

// spawn starts exe in its own process group with a fresh temp dir under root
// ("" = the OS temp dir). args may refer to the dir through argsFor.
func spawn(ctx context.Context, root, exe string, args []string, argsFor func(tempDir) []string) (*Handle, error) {
	var setup func(tempDir) ([]string, []string)
	if argsFor != nil {
		setup = func(d tempDir) ([]string, []string) { return argsFor(d), nil }
	}
	return spawnEnv(ctx, root, exe, args, setup)
}

// spawnEnv is spawn with a setup hook that may also add environment variables
// (appended to os.Environ()): the private Wayland display, a profile path.
func spawnEnv(ctx context.Context, root, exe string, args []string, setup func(tempDir) (args, env []string)) (*Handle, error) {
	tmp, err := newTempDir(root)
	if err != nil {
		return nil, err
	}
	var env []string
	if setup != nil {
		var extraArgs []string
		extraArgs, env = setup(tmp)
		args = append(extraArgs, args...)
	}
	cmd := exec.Command(exe, args...)
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	out := &tailBuffer{max: maxStderr}
	cmd.Stdout, cmd.Stderr = out, out
	setProcessGroup(cmd)
	if err := ctx.Err(); err != nil {
		_ = tmp.remove()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		_ = tmp.remove()
		return nil, fmt.Errorf("process: start %s: %w", exe, err)
	}
	h := &Handle{cmd: cmd, tmp: tmp, out: out, exit: make(chan struct{})}
	h.afterStart()
	// The sweeper needs the browser's process group id: owner.lock names the
	// dead omnisnap process, not the group its browsers lead (SPEC-process.md,
	// cleanup layer 3).
	_ = os.WriteFile(filepath.Join(tmp.dir, groupLock), []byte(strconv.Itoa(cmd.Process.Pid)+"\n"), 0o600)
	go func() {
		// Where the platform can, observe the exit without reaping: the
		// zombie keeps holding the pid, so the group kill in Close can never
		// reach a recycled process group id.
		if err := waitExitedNoReap(cmd.Process.Pid); err != nil {
			_ = cmd.Wait()
			h.reaped = true
		}
		close(h.exit)
	}()
	return h, nil
}

// ProfileDir is the browser's profile inside the temp dir. Warming a branded
// browser's template copies it before Close deletes the temp dir.
func (h *Handle) ProfileDir() string { return h.tmp.profile() }

// Dir is the launch's temp dir (owner.lock and profile).
func (h *Handle) Dir() string { return h.tmp.dir }

// PID is the browser's process id (and its process group id).
func (h *Handle) PID() int { return h.cmd.Process.Pid }

// Close kills the whole process tree and deletes the temp dir. It is
// idempotent. No graceful shutdown: the profile is thrown away, so there is
// nothing to flush, and Chrome takes seconds to act on SIGTERM.
func (h *Handle) Close(ctx context.Context) error {
	h.closeOnce.Do(func() {
		var errs []error
		// The tree: a process group on Unix, a Job Object on Windows.
		h.killTree()
		select {
		case <-h.exit:
			if !h.reaped {
				_ = h.cmd.Wait() // only now may the pid be reused
			}
		case <-ctx.Done():
			errs = append(errs, fmt.Errorf("process: %d did not exit: %w", h.PID(), ctx.Err()))
		}
		if err := h.tmp.remove(); err != nil {
			errs = append(errs, err)
		}
		h.closeErr = errors.Join(errs...)
	})
	return h.closeErr
}

// output returns the end of the browser's stdout and stderr, for errors.
func (h *Handle) output() string { return h.out.String() }

// tailBuffer keeps the last max bytes written to it.
type tailBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
	max int
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf.Write(p)
	if extra := t.buf.Len() - t.max; extra > 0 {
		t.buf.Next(extra)
	}
	return len(p), nil
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.buf.String()
}
