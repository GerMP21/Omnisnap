package install

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A file that is still held when Uninstall starts, as firefox.exe is for a
// moment after its browser is killed, is removed once it is released. Go's
// os.Open on Windows does not share delete access, so an open handle makes
// RemoveAll fail as the dying content processes do.
func TestUninstallWaitsForAHeldFile(t *testing.T) {
	cache := t.TempDir()
	exe := filepath.Join(cache, "firefox", "153.8.0esr", "firefox", "firefox.exe")
	if err := os.MkdirAll(filepath.Dir(exe), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte("MZ"), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(exe)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(cache, "firefox")); err == nil {
		t.Fatal("RemoveAll succeeded with the file held: the test no longer reproduces the lock")
	}
	released := make(chan struct{})
	go func() {
		time.Sleep(200 * time.Millisecond)
		_ = f.Close()
		close(released)
	}()
	if err := Uninstall(cache); err != nil {
		t.Fatalf("Uninstall with the file released after 200 ms: %v", err)
	}
	<-released
	if _, err := os.Stat(filepath.Join(cache, "firefox")); !os.IsNotExist(err) {
		t.Errorf("the firefox dir is still there: %v", err)
	}
}

// A file held for longer than the retries still fails the uninstall, with the
// path in the error.
func TestUninstallGivesUpOnAFileStillHeld(t *testing.T) {
	cache := t.TempDir()
	exe := filepath.Join(cache, "firefox", "153.8.0esr", "firefox", "firefox.exe")
	if err := os.MkdirAll(filepath.Dir(exe), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte("MZ"), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(exe)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := Uninstall(cache); err == nil {
		t.Fatal("Uninstall succeeded with the file held throughout")
	}
}
