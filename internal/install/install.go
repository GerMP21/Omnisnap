package install

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// Options configure Firefox.
type Options struct {
	Channel Channel
	Version string // "" = resolve from Mozilla's version feed
	Cache   string // the browsers cache dir; "" = CacheDir(nil)
	Client  *http.Client
	// Log is told what is downloaded, from where, and its size, before the
	// download starts (SPEC-install.md: "Always").
	Log      func(format string, args ...any)
	Progress Progress

	// Base overrides Mozilla's release host; Key overrides the pinned key.
	// Both are for tests only (the shipped zero values are Mozilla's).
	Base string
	Key  []byte
}

// Result describes an installed build.
type Result struct {
	Version    string
	Dir        string
	Executable string
	URL        string
	Already    bool // it was already installed
}

// Firefox downloads, verifies and unpacks the requested Firefox into the
// cache. Nothing is written into the cache before both the SHA-512 and the
// OpenPGP signature verify.
func Firefox(ctx context.Context, o Options) (Result, error) {
	if o.Client == nil {
		o.Client = &http.Client{Timeout: 30 * time.Minute}
	}
	if o.Channel == "" {
		o.Channel = Release
	}
	if o.Base == "" {
		o.Base = releaseBase
	}
	key := o.Key
	if key == nil {
		key = mozillaKey
	}
	logf := o.Log
	if logf == nil {
		logf = func(string, ...any) {}
	}
	cache := o.Cache
	if cache == "" {
		var err error
		if cache, err = CacheDir(nil); err != nil {
			return Result{}, err
		}
	}
	target := HostTarget()
	version := o.Version
	if version == "" {
		var err error
		if version, err = ResolveVersion(ctx, o.Client, o.Channel); err != nil {
			return Result{}, err
		}
	}
	dir := versionDir(cache, version)
	exe := executablePath(dir)
	if _, err := os.Stat(exe); err == nil {
		return Result{Version: version, Dir: dir, Executable: exe, Already: true}, nil
	}
	relPath, err := archivePath(version, target)
	if err != nil {
		return Result{}, err
	}
	urlPath, err := archiveURL(o.Base, version, target)
	if err != nil {
		return Result{}, err
	}
	name := archiveName(version, target)
	dirURL := releaseDir(o.Base, version)
	sumsURL := dirURL + "SHA512SUMS"
	sigURL := sumsURL + ".asc"

	// Disk space: the download and the unpacked tree (2x the archive).
	size := remoteSize(ctx, o.Client, urlPath)
	if size > 0 {
		logf("downloading %s (%s) from %s", name, HumanBytes(size), urlPath)
		if free, err := freeDisk(cache); err == nil && free < uint64(2*size) {
			return Result{}, fmt.Errorf("install: %s free in the cache is less than twice the %s download", HumanBytes(int64(free)), HumanBytes(size))
		}
	} else {
		logf("downloading %s from %s", name, urlPath)
	}

	archive := filepath.Join(downloadsDir(cache), name)
	if err := download(ctx, o.Client, urlPath, archive, o.Progress); err != nil {
		return Result{}, err
	}
	cleanup := func() {
		_ = os.Remove(archive)
	}
	sums, err := fetch(ctx, o.Client, sumsURL)
	if err != nil {
		cleanup()
		return Result{}, err
	}
	sig, err := fetch(ctx, o.Client, sigURL)
	if err != nil {
		cleanup()
		return Result{}, err
	}
	if err := VerifyDetached(sums, sig, key); err != nil {
		cleanup()
		return Result{}, fmt.Errorf("%w (the SHA512SUMS signature did not verify: nothing was installed)", err)
	}
	// SHA512SUMS lists the archive by its path in the release directory, not by
	// the local file name (macOS and Windows are renamed on download).
	if err := verifyArchive(archive, sums, relPath); err != nil {
		cleanup()
		return Result{}, err
	}
	logf("verified %s: SHA-512 and Mozilla's signature", name)

	tmp, err := os.MkdirTemp(cache, "install-")
	if err != nil {
		cleanup()
		return Result{}, fmt.Errorf("install: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	if err := unpackArchive(archive, tmp); err != nil {
		cleanup()
		return Result{}, err
	}
	if err := finalize(tmp, dir); err != nil {
		cleanup()
		return Result{}, err
	}
	execPath := executablePath(dir)
	if _, err := os.Stat(execPath); err != nil {
		return Result{}, fmt.Errorf("install: the archive has no browser at %s", execPath)
	}
	src := Source{Version: version, Channel: o.Channel, URL: urlPath, InstalledAt: time.Now().UTC(), Verified: "sha512+openpgp"}
	if err := writeSource(dir, src); err != nil {
		_ = os.RemoveAll(dir)
		return Result{}, err
	}
	cleanup()
	return Result{Version: version, Dir: dir, Executable: execPath, URL: urlPath}, nil
}

// freeDisk is freeBytes, replaceable in tests to simulate a full drive.
var freeDisk = freeBytes

// remoteSize asks for the archive's size; -1 when the server does not say.
func remoteSize(ctx context.Context, client *http.Client, url string) int64 {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return -1
	}
	res, err := client.Do(req)
	if err != nil {
		return -1
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return -1
	}
	return res.ContentLength
}

// finalize moves the unpacked tree into its final layout.
func finalize(tmp, dir string) error {
	var src, dst string
	switch runtime.GOOS {
	case "darwin":
		src, dst = filepath.Join(tmp, "Firefox.app"), filepath.Join(dir, "Firefox.app")
	case "windows":
		exe, err := findFile(tmp, "firefox.exe")
		if err != nil {
			return fmt.Errorf("install: %w", err)
		}
		src, dst = filepath.Dir(exe), filepath.Join(dir, "firefox")
	default:
		src, dst = filepath.Join(tmp, "firefox"), filepath.Join(dir, "firefox")
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("install: %w", err)
	}
	// Remove a half-written target from a previous attempt.
	if _, err := os.Stat(dst); err == nil {
		if err := os.RemoveAll(dst); err != nil {
			return fmt.Errorf("install: %w", err)
		}
	}
	if err := os.Rename(src, dst); err != nil {
		return fmt.Errorf("install: move into the cache: %w", err)
	}
	return nil
}

// findFile looks for name under root (the Windows MSI's folder name varies).
func findFile(root, name string) (string, error) {
	var found string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && d.Name() == name && found == "" {
			found = path
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if found == "" {
		return "", errors.New("the unpacked archive has no " + name)
	}
	return found, nil
}

// writeSource writes the marker discovery reads.
func writeSource(dir string, s Source) error {
	f, err := os.Create(filepath.Join(dir, sourceFile))
	if err != nil {
		return fmt.Errorf("install: %w", err)
	}
	defer func() { _ = f.Close() }()
	if _, err := fmt.Fprintf(f, `{"version":%q,"channel":%q,"url":%q,"installedAt":%q,"verified":%q}`+"\n",
		s.Version, s.Channel, s.URL, s.InstalledAt.Format(time.RFC3339), s.Verified); err != nil {
		return fmt.Errorf("install: %w", err)
	}
	return nil
}

// HumanBytes formats a size for the download notice and the progress line.
func HumanBytes(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.0f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f kB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}
