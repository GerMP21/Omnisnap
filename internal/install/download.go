package install

import (
	"context"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Progress reports a download's bytes; total is -1 when the server does not
// say (no Content-Length).
type Progress func(done, total int64)

// download fetches url into dest, resuming a partial file with a Range request
// (SPEC-install.md, "resumable download").
func download(ctx context.Context, client *http.Client, url, dest string, progress Progress) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("install: download: %w", err)
	}
	var offset int64
	if fi, err := os.Stat(dest); err == nil {
		offset = fi.Size()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	if offset > 0 {
		req.Header.Set("Range", "bytes="+strconv.FormatInt(offset, 10)+"-")
	}
	res, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("install: download %s: %w", url, err)
	}
	defer func() { _ = res.Body.Close() }()
	switch res.StatusCode {
	case http.StatusOK:
		offset = 0 // the server ignored the range: start over
	case http.StatusPartialContent:
	case http.StatusRequestedRangeNotSatisfiable:
		return nil // already complete
	default:
		return fmt.Errorf("install: download %s: HTTP %d", url, res.StatusCode)
	}
	total := int64(-1)
	if res.ContentLength >= 0 {
		total = offset + res.ContentLength
	}
	f, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("install: download: %w", err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return fmt.Errorf("install: download: %w", err)
	}
	if offset == 0 {
		if err := f.Truncate(0); err != nil {
			return fmt.Errorf("install: download: %w", err)
		}
	}
	done := offset
	last := time.Now()
	buf := make([]byte, 256<<10)
	for {
		n, rerr := res.Body.Read(buf)
		if n > 0 {
			if _, err := f.Write(buf[:n]); err != nil {
				return fmt.Errorf("install: download: %w", err)
			}
			done += int64(n)
			if progress != nil && time.Since(last) > 200*time.Millisecond {
				progress(done, total)
				last = time.Now()
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return fmt.Errorf("install: download: %w", rerr)
		}
	}
	if progress != nil {
		progress(done, max(total, done))
	}
	return nil
}

// fetch fetches a small file (SHA512SUMS and its signature).
func fetch(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("install: %s: %w", url, err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("install: %s: HTTP %d", url, res.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, 16<<20))
	if err != nil {
		return nil, fmt.Errorf("install: %s: %w", url, err)
	}
	return b, nil
}

// checksumFor returns the SHA-512 recorded for name in a SHA512SUMS file.
func checksumFor(sums []byte, name string) (string, error) {
	for _, line := range strings.Split(string(sums), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// "<hash>  <name>" (or " *<name>"): the name is the rest of the line,
		// spaces included — Mozilla's macOS and Windows archives are
		// "Firefox 160.0.dmg" and "Firefox Setup 160.0.msi".
		hash, rest, ok := strings.Cut(line, " ")
		if !ok || strings.TrimPrefix(strings.TrimLeft(rest, " "), "*") != name {
			continue
		}
		h := strings.ToLower(hash)
		if len(h) != 128 {
			return "", fmt.Errorf("install: %s: checksum is not SHA-512", name)
		}
		if _, err := hex.DecodeString(h); err != nil {
			return "", fmt.Errorf("install: %s: checksum: %w", name, err)
		}
		return h, nil
	}
	return "", fmt.Errorf("install: %s is not in SHA512SUMS", name)
}

// verifyArchive checks the downloaded file's SHA-512 against SHA512SUMS.
func verifyArchive(path string, sums []byte, name string) error {
	want, err := checksumFor(sums, name)
	if err != nil {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("install: verify: %w", err)
	}
	defer func() { _ = f.Close() }()
	h := sha512.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("install: verify: %w", err)
	}
	got := hex.EncodeToString(h.Sum(nil))
	if got != want {
		return fmt.Errorf("install: %s: SHA-512 mismatch: got %s, want %s", filepath.Base(path), got, want)
	}
	return nil
}
