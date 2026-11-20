package install

import (
	"bytes"
	"context"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

// fakeRelease serves this OS's fixture archive (hostRelease), its SHA512SUMS
// and signature in Mozilla's layout, with HEAD and Range.
func fakeRelease(t *testing.T) (*httptest.Server, map[string][]byte, *int32, hostFixture) {
	t.Helper()
	return serveRelease(t, hostRelease(t))
}

// fakeTarRelease serves the committed Linux release on every OS, for the
// tests that only need bytes to download.
func fakeTarRelease(t *testing.T) (*httptest.Server, map[string][]byte, *int32, hostFixture) {
	t.Helper()
	return serveRelease(t, committedFixture(t))
}

func serveRelease(t *testing.T, fx hostFixture) (*httptest.Server, map[string][]byte, *int32, hostFixture) {
	t.Helper()
	archive := fx.archive
	files := map[string][]byte{
		"/999.0/" + fx.rel:      archive,
		"/999.0/SHA512SUMS":     fx.sums,
		"/999.0/SHA512SUMS.asc": fx.sig,
	}
	// The low-disk refusal happens before anything is downloaded or unpacked,
	// so it only needs this OS's archive path to answer with a size: serve the
	// same bytes there too (the macOS and Windows names differ from Linux's).
	if rel, err := archivePath("999.0", HostTarget()); err == nil {
		if _, ok := files["/999.0/"+rel]; !ok {
			files["/999.0/"+rel] = archive
		}
	}
	var ranges int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", fmt.Sprint(len(b)))
			return
		}
		if r.Header.Get("Range") != "" {
			atomic.AddInt32(&ranges, 1)
			var start int
			_, _ = fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-", &start)
			if start >= len(b) {
				w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
				return
			}
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, len(b)-1, len(b)))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(b[start:])
			return
		}
		_, _ = w.Write(b)
	}))
	t.Cleanup(srv.Close)
	return srv, files, &ranges, fx
}

func mustFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("../../testdata/install", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func testKey(t *testing.T) []byte { return mustFixture(t, "fake/test-key.asc") }

func TestInstallVerifiesAndUnpacks(t *testing.T) {
	srv, _, _, fx := fakeRelease(t)
	cache := t.TempDir()
	var logged []string
	res, err := Firefox(context.Background(), Options{
		Version: "999.0", Cache: cache, Base: srv.URL + "/", Key: fx.key,
		Log: func(f string, a ...any) { logged = append(logged, fmt.Sprintf(f, a...)) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Already {
		t.Error("fresh install reported as already installed")
	}
	if _, err := os.Stat(res.Executable); err != nil {
		t.Fatalf("executable: %v", err)
	}
	if len(logged) == 0 || !strings.Contains(logged[0], "999.0") {
		t.Errorf("download notice %v; want the version and size", logged)
	}
	src := Installed(cache)
	if len(src) != 1 || src[0].Version != "999.0" || src[0].Verified != "sha512+openpgp" {
		t.Errorf("Installed = %+v", src)
	}
	if _, exe, ok := InstalledExecutable(cache); !ok || exe != res.Executable {
		t.Errorf("InstalledExecutable = %q, %v", exe, ok)
	}
	// The archive is not left in the cache; only the install.
	if entries, _ := os.ReadDir(filepath.Join(cache, "downloads")); len(entries) != 0 {
		t.Errorf("%d leftover downloads", len(entries))
	}
	// Idempotent.
	again, err := Firefox(context.Background(), Options{Version: "999.0", Cache: cache, Base: srv.URL + "/", Key: fx.key})
	if err != nil || !again.Already {
		t.Errorf("second install = %+v, %v; want Already", again, err)
	}
	// Uninstall leaves nothing (criterion 3).
	if err := Uninstall(cache); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"firefox", "downloads"} {
		if _, err := os.Stat(filepath.Join(cache, p)); !os.IsNotExist(err) {
			t.Errorf("%s survived uninstall", p)
		}
	}
}

// A tampered archive is rejected and nothing lands in the cache (criterion 2).
func TestInstallRejectsTamperedArchive(t *testing.T) {
	srv, files, _, fx := fakeRelease(t)
	archive := append([]byte{}, files["/999.0/"+fx.rel]...)
	archive[len(archive)-1] ^= 0xff
	files["/999.0/"+fx.rel] = archive
	cache := t.TempDir()
	_, err := Firefox(context.Background(), Options{Version: "999.0", Cache: cache, Base: srv.URL + "/", Key: fx.key})
	if err == nil || !strings.Contains(err.Error(), "SHA-512 mismatch") {
		t.Fatalf("err = %v; want a SHA-512 mismatch", err)
	}
	if entries, _ := os.ReadDir(filepath.Join(cache, "firefox")); len(entries) != 0 {
		t.Errorf("%d versions landed in the cache", len(entries))
	}
	if entries, _ := os.ReadDir(filepath.Join(cache, "downloads")); len(entries) != 0 {
		t.Errorf("%d downloads left behind", len(entries))
	}
}

// A tampered SHA512SUMS fails the signature check.
func TestInstallRejectsTamperedSums(t *testing.T) {
	srv, files, _, fx := fakeRelease(t)
	sums := append([]byte{}, files["/999.0/SHA512SUMS"]...)
	sums[0] ^= 0x01 // one bit of the first checksum: the signature no longer matches
	files["/999.0/SHA512SUMS"] = sums
	cache := t.TempDir()
	_, err := Firefox(context.Background(), Options{Version: "999.0", Cache: cache, Base: srv.URL + "/", Key: fx.key})
	if err == nil {
		t.Fatal("a tampered SHA512SUMS installed")
	}
	if entries, _ := os.ReadDir(filepath.Join(cache, "firefox")); len(entries) != 0 {
		t.Errorf("%d versions landed in the cache", len(entries))
	}
}

func TestDownloadResumes(t *testing.T) {
	srv, files, ranges, _ := fakeTarRelease(t)
	url := srv.URL + "/999.0/linux-x86_64/en-US/firefox-999.0.tar.xz"
	dest := filepath.Join(t.TempDir(), "partial.tar.xz")
	whole := files["/999.0/linux-x86_64/en-US/firefox-999.0.tar.xz"]
	if err := os.WriteFile(dest, whole[:100], 0o644); err != nil {
		t.Fatal(err)
	}
	if err := download(context.Background(), srv.Client(), url, dest, nil); err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(ranges) != 1 {
		t.Errorf("range requests %d; want 1", atomic.LoadInt32(ranges))
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, whole) {
		t.Errorf("resumed file has %d bytes; want %d identical", len(got), len(whole))
	}
}

func TestArchiveURL(t *testing.T) {
	for _, tc := range []struct {
		t    Target
		want string
	}{
		{Target{"linux", "amd64"}, "linux-x86_64/en-US/firefox-160.0.tar.xz"},
		{Target{"linux", "arm64"}, "linux-aarch64/en-US/firefox-160.0.tar.xz"},
		{Target{"darwin", "arm64"}, "mac/en-US/Firefox%20160.0.dmg"},
		{Target{"windows", "amd64"}, "win64/en-US/Firefox%20Setup%20160.0.msi"},
		{Target{"windows", "386"}, "win32/en-US/Firefox%20Setup%20160.0.msi"},
	} {
		got, err := archiveURL(releaseBase, "160.0", tc.t)
		if err != nil {
			t.Errorf("%+v: %v", tc.t, err)
			continue
		}
		if !strings.HasSuffix(got, tc.want) {
			t.Errorf("%+v: %q; want suffix %q", tc.t, got, tc.want)
		}
	}
	if _, err := archiveURL(releaseBase, "160.0", Target{"plan9", "mips"}); err == nil {
		t.Error("an unsupported platform built a URL")
	}
}

func TestResolveVersion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"LATEST_FIREFOX_VERSION":"160.0.1","FIREFOX_ESR":"153.7.0esr"}`))
	}))
	defer srv.Close()
	old := versionFeed
	versionFeed = srv.URL
	defer func() { versionFeed = old }()
	for c, want := range map[Channel]string{Release: "160.0.1", ESR: "153.7.0esr"} {
		got, err := ResolveVersion(context.Background(), srv.Client(), c)
		if err != nil || got != want {
			t.Errorf("ResolveVersion(%s) = %q, %v; want %q", c, got, err, want)
		}
	}
}

func TestChecksumFor(t *testing.T) {
	sums := []byte("abc  firefox-1.tar.xz\n" + strings.Repeat("a", 128) + "  firefox-2.tar.xz\n")
	if _, err := checksumFor(sums, "firefox-1.tar.xz"); err == nil {
		t.Error("a short checksum accepted")
	}
	h, err := checksumFor(sums, "firefox-2.tar.xz")
	if err != nil || len(h) != 128 {
		t.Errorf("checksumFor = %q, %v", h, err)
	}
	if _, err := checksumFor(sums, "missing.tar.xz"); err == nil {
		t.Error("missing file accepted")
	}
	// The real sums and archive agree: the fixture is self-consistent.
	archive, err := os.ReadFile("../../testdata/install/fake/firefox-999.0.tar.xz")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha512.Sum512(archive)
	want := hex.EncodeToString(sum[:])
	got, err := checksumFor(mustFixture(t, "fake/SHA512SUMS"), "linux-x86_64/en-US/firefox-999.0.tar.xz")
	if err != nil || got != want {
		t.Errorf("fixture checksum = %q, %v; want %q", got, err, want)
	}
}

// A drive with less than twice the archive free is refused before anything is
// written.
func TestInstallRefusesLowDisk(t *testing.T) {
	// The refusal comes before any download or unpack, so it needs no archive
	// in this OS's format: it runs on every OS with the committed one.
	srv, files, _, fx := fakeTarRelease(t)
	cache := t.TempDir()
	old := freeDisk
	freeDisk = func(string) (uint64, error) { return 100, nil } // 100 B: below twice the archive
	defer func() { freeDisk = old }()
	_, err := Firefox(context.Background(), Options{Version: "999.0", Cache: cache, Base: srv.URL + "/", Key: fx.key})
	if err == nil || !strings.Contains(err.Error(), "free") {
		t.Fatalf("err = %v; want a disk-space refusal", err)
	}
	if entries, _ := os.ReadDir(filepath.Join(cache, "downloads")); len(entries) != 0 {
		t.Errorf("%d downloads written before the disk check", len(entries))
	}
	_ = files
}

// The macOS and Windows fixtures are signed at test time (signedFixture);
// this runs that path through the installer on every OS, with the committed
// archive, so a signing bug shows here and not only on those runners.
func TestInstallWithATestTimeSignature(t *testing.T) {
	committed := committedFixture(t)
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("the committed archive is linux-x86_64's")
	}
	srv, _, _, fx := serveRelease(t, signedFixture(t, committed.archive))
	cache := t.TempDir()
	res, err := Firefox(context.Background(), Options{Version: "999.0", Cache: cache, Base: srv.URL + "/", Key: fx.key})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(res.Executable); err != nil {
		t.Fatalf("executable: %v", err)
	}
	// The committed key does not vouch for the test-time signature.
	if _, err := Firefox(context.Background(), Options{Version: "999.0", Cache: t.TempDir(), Base: srv.URL + "/", Key: testKey(t)}); err == nil {
		t.Error("installed with a key that did not sign the sums")
	}
}

// Mozilla's macOS and Windows archive names hold spaces ("Firefox 160.0.dmg",
// "Firefox Setup 160.0.msi"): the name is everything after the hash and its
// separator, not one whitespace-separated field. Lines as published in
// https://ftp.mozilla.org/pub/firefox/releases/160.0/SHA512SUMS.
func TestChecksumForNamesWithSpaces(t *testing.T) {
	h := strings.Repeat("d7", 64)
	sums := []byte(strings.Repeat("e9", 64) + "  linux-x86_64/en-US/firefox-160.0.tar.xz\n" +
		h + "  mac/en-US/Firefox 160.0.dmg\n" +
		strings.Repeat("d1", 64) + " *win64/en-US/Firefox Setup 160.0.msi\n")
	for name, want := range map[string]string{
		"mac/en-US/Firefox 160.0.dmg":         h,
		"win64/en-US/Firefox Setup 160.0.msi": strings.Repeat("d1", 64),
	} {
		got, err := checksumFor(sums, name)
		if err != nil || got != want {
			t.Errorf("checksumFor(%q) = %q, %v; want %q", name, got, err, want)
		}
	}
	if _, err := checksumFor(sums, "mac/en-US/Firefox"); err == nil {
		t.Error("a prefix of a spaced name matched")
	}
}
