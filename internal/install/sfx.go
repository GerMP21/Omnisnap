package install

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Mozilla's Windows download, "Firefox Setup <v>.exe", is a 7-Zip
// self-extracting archive: a PE stub with a 7z archive appended (LZMA + BCJ2,
// the browser under core/) and an Authenticode signature after it (checked
// 2026-11-25 on 161.0 and 153.8.0esr: the archive starts at 0x20251). Mozilla's
// MSI only wraps that same .exe, so there is nothing else to download. The
// archive is found and copied out here, with the standard library only, and
// extracted by a tool the machine already has (unpack_windows.go): nothing
// from the download is ever executed (SPEC-install.md).

// sevenZipSignature opens every 7z archive (7-Zip's 7zFormat.txt,
// "SignatureHeader": 6 signature bytes, 2 version bytes, the start-header
// CRC, then NextHeaderOffset, NextHeaderSize and NextHeaderCRC).
var sevenZipSignature = []byte{'7', 'z', 0xBC, 0xAF, 0x27, 0x1C}

// startHeaderSize is the fixed header every 7z archive opens with.
const startHeaderSize = 32

// errNo7z is find7z's answer when nothing in the file is a 7z archive.
var errNo7z = errors.New("no 7z archive found")

// find7z returns the offset and length of the first 7z archive in r. A
// signature counts only with a valid start header: major version 0, a start
// header CRC that matches, a next header that is not empty, and an archive
// that ends inside the file. The CRC is what makes a stray copy of the
// signature bytes harmless (7-Zip's SFX stubs search for it at run time, so a
// build may carry it as a constant).
func find7z(r io.ReaderAt, size int64) (off, n int64, err error) {
	const chunk = 1 << 20
	overlap := int64(len(sevenZipSignature) - 1)
	buf := make([]byte, chunk+overlap)
	for base := int64(0); base < size; base += chunk {
		m, err := r.ReadAt(buf[:min(int64(len(buf)), size-base)], base)
		if err != nil && err != io.EOF {
			return 0, 0, err
		}
		window := buf[:m]
		for i := 0; ; {
			j := bytes.Index(window[i:], sevenZipSignature)
			if j < 0 {
				break
			}
			at := base + int64(i+j)
			if n, ok := archiveLength(r, at, size); ok {
				return at, n, nil
			}
			i += j + 1
		}
	}
	return 0, 0, errNo7z
}

// archiveLength checks the start header at off and returns the archive's
// total length: the header plus NextHeaderOffset plus NextHeaderSize.
func archiveLength(r io.ReaderAt, off, size int64) (int64, bool) {
	h := make([]byte, startHeaderSize)
	if _, err := r.ReadAt(h, off); err != nil {
		return 0, false
	}
	if h[6] != 0 { // major version
		return 0, false
	}
	if crc32.ChecksumIEEE(h[12:32]) != binary.LittleEndian.Uint32(h[8:12]) {
		return 0, false
	}
	nextOff := binary.LittleEndian.Uint64(h[12:20])
	nextSize := binary.LittleEndian.Uint64(h[20:28])
	rest := uint64(size - off - startHeaderSize)
	if nextSize == 0 || nextOff > rest || nextSize > rest-nextOff {
		return 0, false
	}
	return startHeaderSize + int64(nextOff) + int64(nextSize), true
}

// carve7z copies the 7z archive embedded in src into dst, exactly: the stub
// before it and the signature after it are left out.
func carve7z(src, dst string) error {
	f, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("install: unpack: %w", err)
	}
	defer func() { _ = f.Close() }()
	st, err := f.Stat()
	if err != nil {
		return fmt.Errorf("install: unpack: %w", err)
	}
	off, n, err := find7z(f, st.Size())
	if err != nil {
		return fmt.Errorf("install: unpack: %s: %w", filepath.Base(src), err)
	}
	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("install: unpack: %w", err)
	}
	if _, err := io.Copy(out, io.NewSectionReader(f, off, n)); err != nil {
		_ = out.Close()
		return fmt.Errorf("install: unpack: %w", err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("install: unpack: %w", err)
	}
	return nil
}

// extractor is one tool that can unpack a 7z archive.
type extractor struct {
	name string // in the error: "tar.exe", "7-Zip"
	path string // "" when the tool was not found
	args func(archive, dest string) []string
}

// extract7z unpacks archive into dest with the first tool in tools that
// leaves want (a slash-separated path under dest) behind. An exit status of 0
// is not enough: a tool that "succeeds" without producing the browser is the
// failure that broke the MSI path. dest is emptied, all but the archive,
// before each attempt and after the last failed one, so one tool's partial
// tree never mixes with another's. The error names every attempt.
func extract7z(archive, dest string, tools []extractor, want string) error {
	var tried []string
	for _, x := range tools {
		if x.path == "" {
			tried = append(tried, x.name+": not found")
			continue
		}
		if err := clearExcept(dest, archive); err != nil {
			return fmt.Errorf("install: unpack: %w", err)
		}
		out, err := exec.Command(x.path, x.args(archive, dest)...).CombinedOutput()
		if err != nil {
			tried = append(tried, fmt.Sprintf("%s: %v: %s", x.name, err, lastLine(out)))
			continue
		}
		if _, err := os.Stat(filepath.Join(dest, filepath.FromSlash(want))); err != nil {
			tried = append(tried, fmt.Sprintf("%s: exited 0 but left no %s", x.name, want))
			continue
		}
		return nil
	}
	_ = clearExcept(dest, archive)
	if len(tried) == 0 {
		tried = append(tried, "no extractor to try")
	}
	return fmt.Errorf("could not extract the archive (%s). Install 7-Zip from https://www.7-zip.org and retry",
		strings.Join(tried, "; "))
}

// clearExcept removes everything in dir except keep.
func clearExcept(dir, keep string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		p := filepath.Join(dir, e.Name())
		if p == filepath.Clean(keep) {
			continue
		}
		if err := os.RemoveAll(p); err != nil {
			return err
		}
	}
	return nil
}

// lastLine is the last non-empty line of a tool's output, kept short: enough
// to say why it failed without pasting a listing into the error.
func lastLine(out []byte) string {
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	s := strings.TrimSpace(lines[len(lines)-1])
	if len(s) > 300 {
		s = s[:300] + "…"
	}
	if s == "" {
		return "no output"
	}
	return s
}

// sevenZipPath finds 7-Zip: on PATH (7z, 7zz, 7za), then in its default
// Windows install folder. "" when it is not installed.
func sevenZipPath() string {
	for _, name := range []string{"7z", "7zz", "7za"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	for _, env := range []string{"ProgramFiles", "ProgramW6432"} {
		if pf := os.Getenv(env); pf != "" {
			p := filepath.Join(pf, "7-Zip", "7z.exe")
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	return ""
}

// sevenZipArgs extracts with 7-Zip: overwrite without asking, no progress.
func sevenZipArgs(archive, dest string) []string {
	return []string{"x", "-y", "-bd", "-o" + dest, archive}
}

// tarArgs extracts with bsdtar (Windows' own tar.exe is bsdtar).
func tarArgs(archive, dest string) []string {
	return []string{"-x", "-f", archive, "-C", dest}
}
