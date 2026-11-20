//go:build windows

package discovery

import (
	"os"
	"os/exec"
	"strings"
	"syscall"

	"golang.org/x/sys/windows/registry"
)

// regQuery dumps a key and its subkeys in `reg query <key> /s`'s text format,
// which is also what the recorded test fixtures are (parseReg reads both).
// It reads the registry in-process: spawning reg.exe once per key cost about
// 160 ms each on a hosted Windows runner, 4.3 s per discovery (CI run
// 44636485891; the budget is 300 ms). Only string values are written
// (REG_SZ, REG_EXPAND_SZ): discovery reads (Default), DisplayName,
// DisplayVersion and DisplayIcon, all strings.
// API: golang.org/x/sys/windows/registry at the pinned v0.50.0 (OpenKey,
// ReadValueNames, GetStringValue, ReadSubKeyNames).
func regQuery(key string) (string, bool) {
	root, sub, ok := strings.Cut(key, `\`)
	if !ok {
		return "", false
	}
	var base registry.Key
	switch root {
	case "HKEY_LOCAL_MACHINE":
		base = registry.LOCAL_MACHINE
	case "HKEY_CURRENT_USER":
		base = registry.CURRENT_USER
	default:
		return "", false
	}
	k, err := registry.OpenKey(base, sub, registry.READ)
	if err != nil {
		return "", false // the key does not exist: normal for a missing browser
	}
	defer func() { _ = k.Close() }()
	var b strings.Builder
	dumpKey(&b, k, key, 0)
	return b.String(), true
}

// maxRegDepth bounds the walk: App Paths and Uninstall entries are one level
// below the key discovery asks for; reg.exe /s has no bound, but nothing
// discovery reads is deeper.
const maxRegDepth = 2

func dumpKey(b *strings.Builder, k registry.Key, path string, depth int) {
	b.WriteString("\r\n" + path + "\r\n")
	names, _ := k.ReadValueNames(-1)
	for _, name := range names {
		v, typ, err := k.GetStringValue(name)
		if err != nil {
			continue // not a string (DWORD, binary): nothing discovery reads
		}
		label, kind := name, "REG_SZ"
		if label == "" {
			label = "(Default)"
		}
		if typ == registry.EXPAND_SZ {
			kind = "REG_EXPAND_SZ"
		}
		b.WriteString("    " + label + "    " + kind + "    " + v + "\r\n")
	}
	if depth >= maxRegDepth {
		return
	}
	subs, _ := k.ReadSubKeyNames(-1)
	for _, s := range subs {
		c, err := registry.OpenKey(k, s, registry.READ)
		if err != nil {
			continue
		}
		dumpKey(b, c, path+`\`+s, depth+1)
		_ = c.Close()
	}
}

// fileVersion reads a Windows executable's VERSIONINFO. The registry's
// DisplayVersion normally answers first (every Chromium-family browser has
// one); this covers portable builds, where the file's metadata is the only
// source. Go's standard library has no VERSIONINFO reader and Omnisnap takes
// no dependency for it, so PowerShell's FileVersionInfo does the one lookup
// (hidden, no profile, ~300 ms, only when needed).
func fileVersion(path string) string {
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command",
		`(Get-Item -LiteralPath $env:OMNISNAP_FILE).VersionInfo.FileVersion`)
	cmd.Env = append(os.Environ(), "OMNISNAP_FILE="+path)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
