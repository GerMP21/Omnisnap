package install

import (
	"sort"
	"strings"
)

// registryChanges compares two registry snapshots (registrySnapshot: a
// "== <key>" line per key in mozillaKeys, then its `reg query /s` output) and
// sorts each line that appeared or disappeared into blamed, a change the
// install could have made, or ignored, another Firefox's.
//
// Only HKCU\Software\Mozilla can be ignored: Firefox writes it every time it
// starts, under its own executable's path, and the Windows runner image's
// Firefox in C:\Program Files starts on its own (its background-update task;
// v0.2.0-rc.1's "Install (network)" run 46008712513, attempt 1, gained its
// Launcher and PreXULSkeletonUISettings keys mid-test). A line there that
// names Omnisnap's browser cache is still blamed. HKLM and the Uninstall keys
// are written by an installer, never by a running Firefox, so any change
// there is blamed.
func registryChanges(before, after string) (blamed, ignored []string) {
	b, a := registryLines(before), registryLines(after)
	for _, diff := range []struct {
		sign     string
		from, to map[string]string
	}{{"+ ", a, b}, {"- ", b, a}} {
		for line, section := range diff.from {
			if _, ok := diff.to[line]; ok {
				continue
			}
			if strings.EqualFold(section, `HKCU\Software\Mozilla`) &&
				!strings.Contains(strings.ToLower(line), `\omnisnap\browsers\`) {
				ignored = append(ignored, diff.sign+line)
			} else {
				blamed = append(blamed, diff.sign+line)
			}
		}
	}
	sort.Strings(blamed)
	sort.Strings(ignored)
	return blamed, ignored
}

// registryLines maps each line of a snapshot to the mozillaKeys section it is
// in. A value line is prefixed with its key, so the same value under two
// keys is two lines; "(missing)" is prefixed with its section.
func registryLines(snapshot string) map[string]string {
	lines := map[string]string{}
	var section, key string
	for _, l := range strings.Split(strings.ReplaceAll(snapshot, "\r\n", "\n"), "\n") {
		switch {
		case strings.HasPrefix(l, "== "):
			section, key = strings.TrimPrefix(l, "== "), ""
		case strings.TrimSpace(l) == "":
		case strings.HasPrefix(l, "HKEY_"):
			key = strings.TrimSpace(l)
			lines[key] = section
		case strings.TrimSpace(l) == "(missing)":
			lines[section+" (missing)"] = section
		default:
			lines[key+"  "+strings.TrimSpace(l)] = section
		}
	}
	return lines
}
