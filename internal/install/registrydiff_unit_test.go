package install

import (
	"strings"
	"testing"
)

// The HKCU\Software\Mozilla part of the snapshots from v0.2.0-rc.1's
// "Install (network)" run 46008712513, attempt 1 (values shortened), plus one
// Uninstall key that both snapshots share.
const rc1Before = `== HKCU\Software\Mozilla
(missing)
== HKLM\Software\Mozilla

HKEY_LOCAL_MACHINE\Software\Mozilla\Firefox

HKEY_LOCAL_MACHINE\Software\Mozilla\Firefox\TaskBarIDs
    C:\Program Files\Mozilla Firefox    REG_SZ    308046B0AF4A39CB

== HKLM\Software\Microsoft\Windows\CurrentVersion\Uninstall

HKEY_LOCAL_MACHINE\Software\Microsoft\Windows\CurrentVersion\Uninstall\Mozilla Firefox
    InstallLocation    REG_SZ    C:\Program Files\Mozilla Firefox
`

const rc1AfterHKCU = `== HKCU\Software\Mozilla

HKEY_CURRENT_USER\Software\Mozilla\Firefox

HKEY_CURRENT_USER\Software\Mozilla\Firefox\Launcher
    C:\Program Files\Mozilla Firefox\firefox.exe|Image    REG_DWORD    0x6afaff77
    C:\Program Files\Mozilla Firefox\firefox.exe|Launcher    REG_QWORD    0xa56f53a6

HKEY_CURRENT_USER\Software\Mozilla\Firefox\PreXULSkeletonUISettings
    C:\Program Files\Mozilla Firefox\firefox.exe|Progress    REG_DWORD    0x1
`

// rc1After is rc1Before with its HKCU section replaced by rc1AfterHKCU.
var rc1After = strings.Replace(rc1Before, "== HKCU\\Software\\Mozilla\n(missing)\n", rc1AfterHKCU, 1)

func TestRegistryChanges(t *testing.T) {
	cases := []struct {
		name          string
		before, after string
		blamed        int
		ignored       int
	}{
		{"unchanged", rc1Before, rc1Before, 0, 0},
		{"the runner's Firefox started (rc.1, attempt 1)", rc1Before, rc1After, 0, 7},
		{"a key naming the cache, under HKCU", rc1Before, strings.Replace(rc1Before,
			"== HKCU\\Software\\Mozilla\n(missing)\n",
			"== HKCU\\Software\\Mozilla\n\nHKEY_CURRENT_USER\\Software\\Mozilla\\Firefox\\Launcher\n"+
				"    C:\\Users\\me\\AppData\\Local\\omnisnap\\browsers\\firefox\\153.8.0esr\\firefox\\firefox.exe|Launcher    REG_QWORD    0x1\n", 1),
			1, 2},
		{"an Uninstall entry appears", rc1Before, rc1Before +
			"\nHKEY_LOCAL_MACHINE\\Software\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\Mozilla Firefox 153.8.0 ESR (x64 en-US)\n", 1, 0},
		{"an HKLM Mozilla value changes", rc1Before, strings.Replace(rc1Before, "308046B0AF4A39CB", "0000000000000000", 1), 2, 0},
		{"Windows line endings", rc1Before, strings.ReplaceAll(rc1Before, "\n", "\r\n"), 0, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			blamed, ignored := registryChanges(c.before, c.after)
			if len(blamed) != c.blamed || len(ignored) != c.ignored {
				t.Errorf("blamed %d, ignored %d; want %d, %d\nblamed:\n%s\nignored:\n%s",
					len(blamed), len(ignored), c.blamed, c.ignored,
					strings.Join(blamed, "\n"), strings.Join(ignored, "\n"))
			}
		})
	}
}
