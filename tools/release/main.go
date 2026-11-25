// Command release cross-compiles Omnisnap for the five released targets,
// writes SHA256SUMS, and (outside --dry-run) prints the git commands for the
// tag and the GitHub release (SPEC-wrappers.md, Task 82).
//
// omnisnap-lab is never built: it is the lab binary (Task 69), not shipped.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
)

type target struct {
	os, arch string
}

// targets are the released builds: the three OSes, both Apple and Linux
// architectures, Windows on amd64.
var targets = []target{
	{"linux", "amd64"},
	{"linux", "arm64"},
	{"darwin", "amd64"},
	{"darwin", "arm64"},
	{"windows", "amd64"},
}

func main() {
	out := flag.String("out", "dist", "output directory")
	version := flag.String("version", "", "version to stamp (default: cli.Version from git describe or dev)")
	dry := flag.Bool("dry-run", false, "build and checksum only; do not tag or release")
	stage := flag.Bool("stage", false, "copy the binaries into packages/npm/platforms and packages/pypi/omnisnap/bin")
	flag.Parse()

	if *version == "" {
		*version = gitVersion()
	}
	if err := run(*out, *version, *dry, *stage); err != nil {
		fmt.Fprintln(os.Stderr, "release:", err)
		os.Exit(1)
	}
}

func run(out, version string, dry, stage bool) error {
	if stage {
		// Staging is for publishing: npm and PyPI must carry the tag's own
		// version, or the packages go out under the previous one (the
		// v0.1.0-rc.6 tag was cut before its bump, and its package files
		// still said rc.5). Checked before anything is built.
		if err := checkPackageVersions(".", version); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	if err := clearPrevious(out); err != nil {
		return err
	}
	var built []string
	for _, t := range targets {
		name := fmt.Sprintf("omnisnap_%s_%s_%s", version, t.os, t.arch)
		if t.os == "windows" {
			name += ".exe"
		}
		dest := filepath.Join(out, name)
		ldflags := "-s -w -X github.com/GerMP21/Omnisnap/internal/cli.Version=" + version
		cmd := exec.Command("go", "build", "-trimpath", "-ldflags", ldflags, "-o", dest, "github.com/GerMP21/Omnisnap/cmd/omnisnap")
		cmd.Env = append(os.Environ(), "GOOS="+t.os, "GOARCH="+t.arch, "CGO_ENABLED=0")
		cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
		fmt.Printf("building %s/%s -> %s\n", t.os, t.arch, dest)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("build %s/%s: %w", t.os, t.arch, err)
		}
		built = append(built, name)
		if stage {
			if err := stageBinary(dest, t); err != nil {
				return err
			}
		}
	}
	// SHA256SUMS, sorted by file name, like Mozilla's tables.
	sums, err := checksums(out, built)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(out, "SHA256SUMS"), []byte(sums), 0o644); err != nil {
		return err
	}
	// The host binary must report the stamped version.
	if host, ok := hostBinary(out, version); ok {
		b, err := exec.Command(host, "version").CombinedOutput()
		if err != nil {
			return fmt.Errorf("%s version: %v: %s", host, err, b)
		}
		if !strings.Contains(string(b), version) {
			return fmt.Errorf("%s version = %q; want %s", host, strings.TrimSpace(string(b)), version)
		}
	}
	fmt.Printf("%d binaries and SHA256SUMS in %s\n", len(built), out)
	if !dry {
		fmt.Printf("next:\n  git tag %s && git push origin %s\n  gh release create %s %s/* --generate-notes\n", version, version, version, out)
	}
	return nil
}

// clearPrevious removes an earlier build's binaries and SHA256SUMS from out:
// the release is published with `gh release create <tag> dist/*`, so a binary
// left over from another version would be uploaded beside this one, outside
// SHA256SUMS (v0.1.0-rc.1 shipped five vf638128-dirty binaries that way).
// Other files in out are left alone.
func clearPrevious(out string) error {
	entries, err := os.ReadDir(out)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if n := e.Name(); !e.IsDir() && (strings.HasPrefix(n, "omnisnap_") || n == "SHA256SUMS") {
			if err := os.Remove(filepath.Join(out, n)); err != nil {
				return err
			}
		}
	}
	return nil
}

// stageBinary copies a built binary into the packaging trees: the npm platform
// package and the PyPI wheel data directory.
func stageBinary(src string, t target) error {
	// The wrappers look the binary up by their own platform keys, which say
	// x64 where the release targets say amd64 (npm: @omnisnap/linux-x64;
	// PyPI: bin/linux-x64). Staging under the target's name would leave the
	// x64 packages empty.
	key := t.os + "-" + t.arch
	if t.arch == "amd64" {
		key = t.os + "-x64"
	}
	if t.os == "windows" {
		key = "win32-" + t.arch
		if t.arch == "amd64" {
			key = "win32-x64"
		}
	}
	exe := "omnisnap"
	if t.os == "windows" {
		exe = "omnisnap.exe"
	}
	dests := []string{
		filepath.Join("packages", "npm", "platforms", key, "bin", exe),
		filepath.Join("packages", "pypi", "omnisnap", "bin", key, exe),
	}
	for _, dst := range dests {
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := copyExecutable(src, dst); err != nil {
			return err
		}
	}
	return nil
}

func copyExecutable(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	outFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(outFile, in); err != nil {
		_ = outFile.Close()
		return err
	}
	return outFile.Close()
}

// checksums writes one "<sha256>  <name>" line per binary.
func checksums(dir string, names []string) (string, error) {
	sort.Strings(names)
	var b strings.Builder
	for _, name := range names {
		f, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			return "", err
		}
		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			_ = f.Close()
			return "", err
		}
		_ = f.Close()
		fmt.Fprintf(&b, "%s  %s\n", hex.EncodeToString(h.Sum(nil)), name)
	}
	return b.String(), nil
}

// hostBinary returns the built binary this machine can run, for the version
// check.
func hostBinary(dir, version string) (string, bool) {
	for _, t := range targets {
		if t.os == runtime.GOOS && t.arch == runtime.GOARCH {
			name := fmt.Sprintf("omnisnap_%s_%s_%s", version, t.os, t.arch)
			if t.os == "windows" {
				name += ".exe"
			}
			p := filepath.Join(dir, name)
			if _, err := os.Stat(p); err == nil {
				return p, true
			}
		}
	}
	return "", false
}

// gitVersion is the latest tag, or "dev" outside a tagged checkout.
func gitVersion() string {
	out, err := exec.Command("git", "describe", "--tags", "--always", "--dirty").Output()
	if err != nil {
		return "dev"
	}
	v := strings.TrimSpace(string(out))
	if v == "" {
		return "dev"
	}
	if !strings.HasPrefix(v, "v") && v != "dev" {
		v = "v" + v
	}
	return v
}

// releaseTag is a release tag: v<major>.<minor>.<patch>, optionally with an
// alpha, beta or rc pre-release number.
var releaseTag = regexp.MustCompile(`^v(\d+\.\d+\.\d+)(?:-(alpha|beta|rc)\.(\d+))?$`)

// packageVersions is a tag's version as npm writes it (the tag without its v)
// and as PyPI does (PEP 440: 0.1.0rc7, 0.1.0a1, 0.1.0b2).
// Source: https://packaging.python.org/en/latest/specifications/version-specifiers/#pre-releases
func packageVersions(tag string) (npm, pypi string, err error) {
	m := releaseTag.FindStringSubmatch(tag)
	if m == nil {
		return "", "", fmt.Errorf("%q is not a release tag (v1.2.3 or v1.2.3-rc.4): nothing to publish", tag)
	}
	base, pre, n := m[1], m[2], m[3]
	if pre == "" {
		return base, base, nil
	}
	short := map[string]string{"alpha": "a", "beta": "b", "rc": "rc"}[pre]
	return base + "-" + pre + "." + n, base + short + n, nil
}

// checkPackageVersions reports every package file under root whose version is
// not the tag's: the six npm package.json files (and the main package's pins
// on the platform packages), pyproject.toml and the Python __version__.
func checkPackageVersions(root, tag string) error {
	npm, pypi, err := packageVersions(tag)
	if err != nil {
		return err
	}
	var problems []string
	wrong := func(file, field, got, want string) {
		if got != want {
			problems = append(problems, fmt.Sprintf("%s: %s is %q, want %q", file, field, got, want))
		}
	}
	manifests, _ := filepath.Glob(filepath.Join(root, "packages", "npm", "platforms", "*", "package.json"))
	manifests = append(manifests, filepath.Join(root, "packages", "npm", "omnisnap", "package.json"))
	for _, path := range manifests {
		var pkg struct {
			Version  string            `json:"version"`
			Optional map[string]string `json:"optionalDependencies"`
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(b, &pkg); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		wrong(path, "version", pkg.Version, npm)
		for dep, v := range pkg.Optional {
			wrong(path, "optionalDependencies."+dep, v, npm)
		}
	}
	for _, f := range []struct{ path, pattern string }{
		{filepath.Join(root, "packages", "pypi", "pyproject.toml"), `(?m)^version\s*=\s*"([^"]*)"`},
		{filepath.Join(root, "packages", "pypi", "omnisnap", "__init__.py"), `(?m)^__version__\s*=\s*"([^"]*)"`},
	} {
		b, err := os.ReadFile(f.path)
		if err != nil {
			return err
		}
		m := regexp.MustCompile(f.pattern).FindSubmatch(b)
		if m == nil {
			problems = append(problems, f.path+": no version found")
			continue
		}
		wrong(f.path, "version", string(m[1]), pypi)
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return errors.New("the packages do not carry " + tag + " (bump them before tagging):\n  " + strings.Join(problems, "\n  "))
	}
	return nil
}
