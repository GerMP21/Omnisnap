// Package install downloads the official Mozilla Firefox build, verifies it,
// and unpacks it into Omnisnap's cache (SPEC-install.md). Never automatic: only
// `omnisnap install firefox` downloads anything.
package install

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"runtime"
	"strings"
)

// Mozilla's release servers and the version feed.
const (
	releaseBase    = "https://ftp.mozilla.org/pub/firefox/releases/"
	productDetails = "https://product-details.mozilla.org/1.0/firefox_versions.json"
)

// Channel is a Firefox release channel.
type Channel string

// Channels.
const (
	Release Channel = "release"
	ESR     Channel = "esr"
)

// ParseChannel validates a channel name.
func ParseChannel(s string) (Channel, error) {
	switch strings.ToLower(s) {
	case "", "release", "latest":
		return Release, nil
	case "esr":
		return ESR, nil
	}
	return "", fmt.Errorf("install: unknown channel %q: want release or esr", s)
}

// Target is what Mozilla's filenames are keyed by.
type Target struct {
	OS   string // linux, darwin, windows
	Arch string // amd64, arm64, 386
}

// HostTarget is this machine.
func HostTarget() Target { return Target{OS: runtime.GOOS, Arch: runtime.GOARCH} }

// versionFeed is the official version feed; a variable so tests can point it
// at a local server.
var versionFeed = productDetails

// ResolveVersion asks Mozilla's official version feed (LATEST_FIREFOX_VERSION
// or FIREFOX_ESR). An explicit version skips this.
func ResolveVersion(ctx context.Context, client *http.Client, c Channel) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, versionFeed, nil)
	if err != nil {
		return "", err
	}
	res, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("install: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("install: version feed: HTTP %d", res.StatusCode)
	}
	var feed map[string]string
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&feed); err != nil {
		return "", fmt.Errorf("install: version feed: %w", err)
	}
	key := "LATEST_FIREFOX_VERSION"
	if c == ESR {
		key = "FIREFOX_ESR"
	}
	v := feed[key]
	if v == "" {
		return "", fmt.Errorf("install: version feed has no %s", key)
	}
	return v, nil
}

// releaseDir is the per-release directory under base.
func releaseDir(base, version string) string {
	return strings.TrimSuffix(base, "/") + "/" + version + "/"
}

// archiveURL returns the download URL for a version and target.
//
// Layout verified 2026-11-17:
// linux-x86_64/en-US/firefox-<v>.tar.xz, linux-aarch64/...,
// mac/en-US/Firefox <v>.dmg, win64/en-US/Firefox Setup <v>.msi
func archiveURL(base, version string, t Target) (string, error) {
	rel, err := archivePath(version, t)
	if err != nil {
		return "", err
	}
	// Path-escape the segments: the macOS and Windows names contain spaces.
	segs := strings.Split(rel, "/")
	for i, seg := range segs {
		segs[i] = url.PathEscape(seg)
	}
	return releaseDir(base, version) + strings.Join(segs, "/"), nil
}

// archivePath is the archive's path inside the release directory, spelled the
// way SHA512SUMS spells it (spaces are not escaped there).
func archivePath(version string, t Target) (string, error) {
	plat, name := "", ""
	switch t.OS {
	case "linux":
		switch t.Arch {
		case "amd64":
			plat = "linux-x86_64"
		case "arm64":
			plat = "linux-aarch64"
		}
		name = "firefox-" + version + ".tar.xz"
	case "darwin":
		plat = "mac"
		name = "Firefox " + version + ".dmg"
	case "windows":
		switch t.Arch {
		case "amd64":
			plat = "win64"
		case "arm64":
			plat = "win64-aarch64"
		case "386":
			plat = "win32"
		}
		name = "Firefox Setup " + version + ".msi"
	}
	if plat == "" {
		return "", fmt.Errorf("install: no Firefox build for %s/%s", t.OS, t.Arch)
	}
	return plat + "/en-US/" + name, nil
}

// archiveName is the local file name of the download.
func archiveName(version string, t Target) string {
	switch t.OS {
	case "darwin":
		return "firefox-" + version + ".dmg"
	case "windows":
		return "firefox-" + version + ".msi"
	default:
		return "firefox-" + version + ".tar.xz"
	}
}
