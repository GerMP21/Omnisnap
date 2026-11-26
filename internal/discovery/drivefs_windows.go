//go:build windows

package discovery

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// driveFS is discovery's default file system on Windows. Discovery names
// files by rel() of an absolute slash path, so on Windows a name starts with
// a drive letter ("C:/Program Files/..."), and os.DirFS rejects every name
// that holds a ':'. A name without a drive is read from the current drive's
// root, as os.DirFS("/") does.
type driveFS struct{}

var (
	_ fs.StatFS     = driveFS{}
	_ fs.ReadLinkFS = driveFS{}
)

// local is the Windows path for name, or an error for a name that is not a
// valid fs path or holds a ':' or '\' past the drive (an alternate data
// stream, or a second path syntax).
func (driveFS) local(op, name string) (string, error) {
	drive, rest := "", name
	if len(name) >= 2 && name[1] == ':' && isDriveLetter(name[0]) && (len(name) == 2 || name[2] == '/') {
		drive, rest = name[:2], strings.TrimPrefix(name[2:], "/")
		if rest == "" {
			rest = "."
		}
	}
	if !fs.ValidPath(rest) || strings.ContainsAny(rest, `:\`) {
		return "", &fs.PathError{Op: op, Path: name, Err: fs.ErrInvalid}
	}
	if rest == "." {
		rest = ""
	}
	return drive + `\` + filepath.FromSlash(rest), nil
}

func isDriveLetter(c byte) bool { return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' }

func (d driveFS) Open(name string) (fs.File, error) {
	p, err := d.local("open", name)
	if err != nil {
		return nil, err
	}
	return os.Open(p)
}

func (d driveFS) Stat(name string) (fs.FileInfo, error) {
	p, err := d.local("stat", name)
	if err != nil {
		return nil, err
	}
	return os.Stat(p)
}

func (d driveFS) Lstat(name string) (fs.FileInfo, error) {
	p, err := d.local("lstat", name)
	if err != nil {
		return nil, err
	}
	return os.Lstat(p)
}

// ReadLink returns a drive-letter target as an absolute slash path
// ("/C:/..."), so resolve takes it from the root like any absolute target.
func (d driveFS) ReadLink(name string) (string, error) {
	p, err := d.local("readlink", name)
	if err != nil {
		return "", err
	}
	target, err := os.Readlink(p)
	if err != nil {
		return "", err
	}
	target = filepath.ToSlash(target)
	if len(target) >= 2 && target[1] == ':' {
		target = "/" + target
	}
	return target, nil
}
