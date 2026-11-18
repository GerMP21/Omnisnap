// Package branded explains branded Blink browsers (SPEC-branded.md): the
// requests they block and whether their warmed template is still fresh. The
// capture itself is Blink's; this package only attributes.
package branded

import (
	"net/url"
	"path"
	"strings"

	"github.com/GerMP21/Omnisnap/internal/schema"
)

// BlockedHosts lists the distinct hosts whose requests the browser itself
// blocked (ERR_BLOCKED_BY_CLIENT and its equivalents).
func BlockedHosts(failed []schema.FailedRequest) []string {
	var out []string
	seen := map[string]bool{}
	for _, f := range failed {
		if !strings.Contains(strings.ToUpper(f.Err), "BLOCKED_BY_CLIENT") {
			continue
		}
		host := HostOf(f.URL)
		if host == "" || seen[host] {
			continue
		}
		seen[host] = true
		out = append(out, host)
	}
	return out
}

// HostOf extracts the host of a URL or element resource key. A key may be a
// relative path (no host) or a bare host.
func HostOf(resource string) string {
	if u, err := url.Parse(resource); err == nil && u.Host != "" {
		return u.Hostname()
	}
	s := strings.TrimSpace(resource)
	if s == "" || strings.ContainsAny(s, " /\\") {
		return ""
	}
	// A bare file name ("image.png") is a relative key, not a host.
	switch strings.ToLower(path.Ext(s)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".avif", ".svg", ".ico", ".css", ".js", ".mjs", ".woff", ".woff2", ".ttf", ".otf", ".html", ".json":
		return ""
	}
	if strings.Count(s, ".") >= 1 && !strings.HasPrefix(s, ".") {
		return strings.TrimPrefix(strings.ToLower(s), "www.")
	}
	return ""
}

// Matches reports whether a cause's resource host is one the browser blocked.
// Subdomains match their parent's blocked host.
func Matches(resourceHost string, blocked []string) (string, bool) {
	h := strings.TrimPrefix(strings.ToLower(resourceHost), "www.")
	for _, b := range blocked {
		b = strings.TrimPrefix(strings.ToLower(b), "www.")
		if h == b || strings.HasSuffix(h, "."+b) {
			return b, true
		}
	}
	return "", false
}
