//go:build windows

package discovery

import (
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"
)

// regQuery reads the registry in-process; reg.exe is the reference for what
// it must produce (the recorded fixtures are reg.exe dumps). For the keys
// discovery reads, both must give the same subkeys (to regQuery's depth) and
// the same string values. Runs on every Windows host (the CI test job).
func TestRegQueryMatchesRegExe(t *testing.T) {
	for _, key := range []string{uninstallKey, appPathsKey + "msedge.exe"} {
		start := time.Now()
		native, ok := regQuery(key)
		took := time.Since(start)
		out, err := exec.Command("reg", "query", key, "/s").Output()
		if !ok || err != nil {
			if ok != (err == nil) {
				t.Errorf("%s: regQuery ok=%v, reg.exe err=%v", key, ok, err)
			}
			continue
		}
		want := regExeStrings(string(out), key)
		got := parseReg(native)
		for k, values := range want {
			gv, present := got[k]
			if !present {
				t.Errorf("%s: key missing from regQuery", k)
				continue
			}
			for name, v := range values {
				if g, _ := regValueOf(gv, name); g != v {
					t.Errorf("%s: %s = %q; reg.exe says %q", k, name, g, v)
				}
			}
		}
		t.Logf("%s: %d keys in %v", key, len(got), took)
	}
}

var regExeValue = regexp.MustCompile(`^\s+(.+?)\s{2,}(REG_\w+)\s{2,}(.*)$`)

// regExeStrings keeps reg.exe's string values (REG_SZ, REG_EXPAND_SZ) for keys
// within regQuery's depth; an unset default ("(value not set)") is not a value.
func regExeStrings(out, root string) map[string]map[string]string {
	keys := map[string]map[string]string{}
	current := ""
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, "HKEY_") {
			current = ""
			if depth := strings.Count(strings.TrimPrefix(line, root), `\`); depth <= maxRegDepth {
				current = line
				keys[current] = map[string]string{}
			}
			continue
		}
		m := regExeValue.FindStringSubmatch(line)
		if current == "" || m == nil || (m[2] != "REG_SZ" && m[2] != "REG_EXPAND_SZ") || m[3] == "(value not set)" {
			continue
		}
		keys[current][m[1]] = strings.TrimSpace(m[3])
	}
	return keys
}
