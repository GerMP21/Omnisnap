package install

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// syntheticArchive is a stand-in 7z archive: a valid start header, a body and
// a next header. Only the start header is checked by find7z; the bytes after
// it just have to be there.
func syntheticArchive(body, next int) []byte {
	h := make([]byte, startHeaderSize)
	copy(h, sevenZipSignature)
	h[7] = 4 // version 0.4, as Mozilla's
	binary.LittleEndian.PutUint64(h[12:], uint64(body))
	binary.LittleEndian.PutUint64(h[20:], uint64(next))
	binary.LittleEndian.PutUint32(h[8:], crc32.ChecksumIEEE(h[12:32]))
	a := append(h, bytes.Repeat([]byte{0xA5}, body)...)
	return append(a, bytes.Repeat([]byte{0x5A}, next)...)
}

// decoy is the signature with a version but a start-header CRC that does not
// match, as a stub that searches for the signature might carry.
func decoy() []byte {
	d := append([]byte{}, sevenZipSignature...)
	d = append(d, 0, 4, 0xDE, 0xAD, 0xBE, 0xEF)
	return append(d, bytes.Repeat([]byte{1}, 20)...)
}

func writeTemp(t *testing.T, name string, parts ...[]byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, bytes.Join(parts, nil), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestFind7zSkipsADecoyAndCarvesExactly(t *testing.T) {
	stub := append([]byte("MZ"), bytes.Repeat([]byte{0}, 1000)...)
	stub = append(stub, decoy()...)
	stub = append(stub, bytes.Repeat([]byte{0}, 3000)...)
	archive := syntheticArchive(5000, 100)
	tail := bytes.Repeat([]byte{0xEE}, 4096) // stands in for the Authenticode signature
	src := writeTemp(t, "Firefox Setup 999.0.exe", stub, archive, tail)

	f, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	st, _ := f.Stat()
	off, n, err := find7z(f, st.Size())
	if err != nil {
		t.Fatal(err)
	}
	if off != int64(len(stub)) || n != int64(len(archive)) {
		t.Fatalf("find7z = (%d, %d); want (%d, %d)", off, n, len(stub), len(archive))
	}

	dst := filepath.Join(t.TempDir(), "firefox.7z")
	if err := carve7z(src, dst); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, archive) {
		t.Fatalf("carved %d bytes; want exactly the %d-byte archive, no stub, no tail", len(got), len(archive))
	}
}

// A signature that straddles find7z's 1 MiB read boundary is still found.
func TestFind7zAcrossTheChunkBoundary(t *testing.T) {
	stub := bytes.Repeat([]byte{0}, 1<<20-3)
	archive := syntheticArchive(64, 16)
	src := writeTemp(t, "setup.exe", stub, archive)
	f, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	off, n, err := find7z(f, int64(len(stub)+len(archive)))
	if err != nil || off != int64(len(stub)) || n != int64(len(archive)) {
		t.Fatalf("find7z = (%d, %d, %v); want (%d, %d, nil)", off, n, err, len(stub), len(archive))
	}
}

func TestCarve7zRejects(t *testing.T) {
	good := syntheticArchive(500, 50)
	corrupt := append([]byte{}, good...)
	corrupt[9] ^= 0xFF // the start-header CRC no longer matches
	truncated := good[:len(good)-10]
	for name, data := range map[string][]byte{
		"a corrupt start-header CRC": corrupt,
		"a next header past the end": truncated,
		"no signature at all":        bytes.Repeat([]byte("MZ"), 4096),
		"only a decoy":               append([]byte("MZ"), decoy()...),
	} {
		t.Run(name, func(t *testing.T) {
			src := writeTemp(t, "Firefox Setup 999.0.exe", []byte("MZ stub"), data)
			err := carve7z(src, filepath.Join(t.TempDir(), "out.7z"))
			if err == nil {
				t.Fatal("carved an archive that is not valid")
			}
			if !errors.Is(err, errNo7z) || !strings.Contains(err.Error(), "Firefox Setup 999.0.exe") {
				t.Errorf("err = %v; want errNo7z naming the file", err)
			}
		})
	}
}

// TestHelperExtractor is not a test: extract7z's tests run this test binary
// as a fake extractor, with "--", a mode and the destination after the test
// flags. Run normally, without "--", it does nothing.
func TestHelperExtractor(t *testing.T) {
	args := os.Args
	for len(args) > 0 && args[0] != "--" {
		args = args[1:]
	}
	if len(args) < 3 {
		return
	}
	mode, dest := args[1], args[2]
	switch mode {
	case "ok":
		core := filepath.Join(dest, "core")
		_ = os.MkdirAll(core, 0o755)
		_ = os.WriteFile(filepath.Join(core, "firefox.exe"), []byte("fake"), 0o644)
		_ = os.WriteFile(filepath.Join(dest, "setup.exe"), []byte("never run"), 0o644)
		os.Exit(0)
	case "empty": // exits 0 and extracts nothing useful: the MSI path's failure
		_ = os.WriteFile(filepath.Join(dest, "partial.txt"), []byte("partial"), 0o644)
		os.Exit(0)
	default:
		fmt.Fprintln(os.Stderr, "fake extractor: cannot read this archive")
		os.Exit(2)
	}
}

func fakeExtractor(name, mode string) extractor {
	return extractor{
		name: name,
		path: os.Args[0],
		args: func(archive, dest string) []string {
			return []string{"-test.run=^TestHelperExtractor$", "--", mode, dest}
		},
	}
}

func TestExtract7zChain(t *testing.T) {
	cases := []struct {
		name    string
		tools   []extractor
		ok      bool
		inError []string
	}{
		{"the first tool works", []extractor{fakeExtractor("tar.exe", "ok"), fakeExtractor("7-Zip", "fail")}, true, nil},
		{"the first exits 0 without the browser, the second works",
			[]extractor{fakeExtractor("tar.exe", "empty"), fakeExtractor("7-Zip", "ok")}, true, nil},
		{"both fail", []extractor{fakeExtractor("tar.exe", "fail"), fakeExtractor("7-Zip", "empty")}, false,
			[]string{"tar.exe:", "cannot read this archive", "7-Zip: exited 0 but left no core/firefox.exe", "https://www.7-zip.org"}},
		{"7-Zip is not installed", []extractor{fakeExtractor("tar.exe", "fail"), {name: "7-Zip"}}, false,
			[]string{"tar.exe:", "7-Zip: not found", "Install 7-Zip"}},
		{"no extractors at all", nil, false, []string{"no extractor", "Install 7-Zip"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dest := t.TempDir()
			archive := filepath.Join(dest, "firefox.7z")
			if err := os.WriteFile(archive, []byte("7z"), 0o644); err != nil {
				t.Fatal(err)
			}
			err := extract7z(archive, dest, c.tools, "core/firefox.exe")
			entries, _ := os.ReadDir(dest)
			if c.ok {
				if err != nil {
					t.Fatal(err)
				}
				if _, err := os.Stat(filepath.Join(dest, "core", "firefox.exe")); err != nil {
					t.Fatal(err)
				}
				// The first tool's partial tree was cleared before the second ran.
				if _, err := os.Stat(filepath.Join(dest, "partial.txt")); err == nil {
					t.Error("a failed attempt's files survived into the next one")
				}
				return
			}
			if err == nil {
				t.Fatal("extract7z succeeded; want an error")
			}
			for _, s := range c.inError {
				if !strings.Contains(err.Error(), s) {
					t.Errorf("error %q does not mention %q", err, s)
				}
			}
			// Only the archive is left behind after a failure.
			if len(entries) != 1 || entries[0].Name() != "firefox.7z" {
				t.Errorf("left %v in dest; want only the archive", entries)
			}
		})
	}
}
