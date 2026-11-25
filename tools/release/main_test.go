package main

import (
	"encoding/json"
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
	// An earlier build left in the directory must not be published with this
	// one (`gh release create <tag> dist/*`); an unrelated file stays.
	for _, stale := range []string{"omnisnap_vold-dirty_linux_amd64", "omnisnap_vold-dirty_windows_amd64.exe"} {
		if err := os.WriteFile(filepath.Join(out, stale), []byte("old"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(out, "NOTES.txt"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run(out, "v0.0.0-test", true, false); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(out, "NOTES.txt")); err != nil {
		t.Errorf("an unrelated file was removed: %v", err)
	}
	entries, err := os.ReadDir(out)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
		if strings.Contains(e.Name(), "vold") {
			t.Errorf("stale %s still in the output: it would be published", e.Name())
		}
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

// The staged binaries must land where the wrappers look for them: the npm
// platform packages are @omnisnap/<os>-x64 and the PyPI wheel data is
// bin/<os>-x64, while the release targets say amd64.
func TestStageUsesWrapperPlatformKeys(t *testing.T) {
	src := filepath.Join(t.TempDir(), "omnisnap")
	if err := os.WriteFile(src, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(wd) }()

	for _, tc := range []struct {
		target target
		key    string
	}{
		{target{"linux", "amd64"}, "linux-x64"},
		{target{"linux", "arm64"}, "linux-arm64"},
		{target{"darwin", "amd64"}, "darwin-x64"},
		{target{"darwin", "arm64"}, "darwin-arm64"},
		{target{"windows", "amd64"}, "win32-x64"},
	} {
		if err := stageBinary(src, tc.target); err != nil {
			t.Fatal(err)
		}
		exe := "omnisnap"
		if tc.target.os == "windows" {
			exe = "omnisnap.exe"
		}
		for _, dst := range []string{
			filepath.Join("packages", "npm", "platforms", tc.key, "bin", exe),
			filepath.Join("packages", "pypi", "omnisnap", "bin", tc.key, exe),
		} {
			if _, err := os.Stat(dst); err != nil {
				t.Errorf("%s: %v", dst, err)
			}
		}
	}
}

func TestPackageVersions(t *testing.T) {
	for _, c := range []struct{ tag, npm, pypi string }{
		{"v0.1.0-rc.7", "0.1.0-rc.7", "0.1.0rc7"},
		{"v1.2.3", "1.2.3", "1.2.3"},
		{"v2.0.0-beta.1", "2.0.0-beta.1", "2.0.0b1"},
		{"v2.0.0-alpha.3", "2.0.0-alpha.3", "2.0.0a3"},
	} {
		npm, pypi, err := packageVersions(c.tag)
		if err != nil || npm != c.npm || pypi != c.pypi {
			t.Errorf("packageVersions(%q) = %q, %q, %v; want %q, %q", c.tag, npm, pypi, err, c.npm, c.pypi)
		}
	}
	for _, bad := range []string{"dev", "0.1.0", "v0.1.0-rc7", "v0.1.0-rc.7-3-gabcdef", "v0.1.0-dirty"} {
		if _, _, err := packageVersions(bad); err == nil {
			t.Errorf("packageVersions(%q) accepted a non-release version", bad)
		}
	}
}

// The repository's own packages carry one version: the one the release
// workflow will publish for the current tag (the rc.6 mistake, caught).
func TestCheckPackageVersions(t *testing.T) {
	root := filepath.Join("..", "..")
	b, err := os.ReadFile(filepath.Join(root, "packages", "npm", "omnisnap", "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	var pkg struct{ Version string }
	if err := json.Unmarshal(b, &pkg); err != nil {
		t.Fatal(err)
	}
	tag := "v" + pkg.Version
	if err := checkPackageVersions(root, tag); err != nil {
		t.Errorf("the packages disagree with each other: %v", err)
	}
	// A tag the packages were not bumped for is refused, naming the files.
	err = checkPackageVersions(root, "v9.9.9")
	if err == nil || !strings.Contains(err.Error(), "pyproject.toml") || !strings.Contains(err.Error(), "package.json") {
		t.Errorf("a stale bump was not reported with its files: %v", err)
	}
}
