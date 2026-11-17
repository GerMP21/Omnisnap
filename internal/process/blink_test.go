package process

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// The launch flags are the spec's, exactly: they change what is measured
// (SPEC-process.md, "Ask first").
func TestBlinkArgsMatchTheSpec(t *testing.T) {
	spec, err := os.ReadFile("../../specs/SPEC-process.md")
	if err != nil {
		t.Fatal(err)
	}
	var row string
	for _, line := range strings.Split(string(spec), "\n") {
		if strings.HasPrefix(line, "| Blink |") {
			row = line
		}
	}
	if row == "" {
		t.Fatal("no Blink row in SPEC-process.md")
	}
	var want []string
	for _, m := range regexp.MustCompile("`([^`]+)`").FindAllStringSubmatch(row, -1) {
		want = append(want, strings.ReplaceAll(m[1], "<temp>", "/tmp/p"))
	}
	if got := blinkArgs("/tmp/p"); !slices.Equal(got, want) {
		t.Errorf("blinkArgs = %q\nspec      = %q", got, want)
	}
}

func TestNewTempDirHasOwnerLock(t *testing.T) {
	root := t.TempDir()
	d, err := newTempDir(root)
	if err != nil {
		t.Fatal(err)
	}
	base := filepath.Base(d.dir)
	if !strings.HasPrefix(base, "omnisnap-"+strconv.Itoa(os.Getpid())+"-") {
		t.Errorf("dir %s: want omnisnap-<pid>-<n>", base)
	}
	lock, err := os.ReadFile(filepath.Join(d.dir, ownerLock))
	if err != nil || strings.TrimSpace(string(lock)) != strconv.Itoa(os.Getpid()) {
		t.Errorf("owner.lock = %q, %v; want this pid", lock, err)
	}
	if info, err := os.Stat(d.profile()); err != nil || !info.IsDir() {
		t.Errorf("profile dir: %v", err)
	}
	if err := d.remove(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(d.dir); !os.IsNotExist(err) {
		t.Errorf("dir still exists after remove: %v", err)
	}
}

func TestParseDevToolsActivePort(t *testing.T) {
	got, err := parseDevToolsActivePort("43117\n/devtools/browser/0f2c-aa\n")
	if err != nil || got != "ws://127.0.0.1:43117/devtools/browser/0f2c-aa" {
		t.Errorf("got %q, %v", got, err)
	}
	for _, bad := range []string{"", "43117\n", "x\n/devtools/browser/1\n", "43117\nbrowser\n"} {
		if _, err := parseDevToolsActivePort(bad); err == nil {
			t.Errorf("%q: want an error", bad)
		}
	}
}
