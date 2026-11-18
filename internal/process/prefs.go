package process

import "fmt"

// geckoPrefs is the prefs file (user.js) of the spike's Gecko launch
// (SPEC-process.md, "Launch configuration per family"; lib/engines.mjs):
//
//   - overlay scrollbars, so the layout viewport matches Blink's (a classic
//     scrollbar would take 15 px off clientWidth);
//   - devicePixelRatio 1, the same as Blink's --force-device-scale-factor;
//   - no disk cache: a crawl would otherwise write hundreds of MB per session,
//     and WSL's virtual disk never gives that space back;
//   - font parity: generic families resolve to the same fonts as Blink's
//     defaults (fontconfig would otherwise choose different ones on the same
//     machine).
//
// Changing any line changes what is measured: ask first.
func geckoPrefs(fontParity bool) []string {
	prefs := []struct {
		name  string
		value string
	}{
		{"ui.useOverlayScrollbars", "1"},
		{"widget.gtk.overlay-scrollbars.enabled", "true"},
		{"layout.css.devPixelsPerPx", `"1.0"`},
		{"browser.cache.disk.enable", "false"},
		{"browser.cache.disk.capacity", "0"},
	}
	if fontParity {
		// The same fonts as the spike's PARITY_FONTS, in its order.
		fonts := []struct{ generic, font string }{
			{"serif", "Liberation Serif"},
			{"sans-serif", "Liberation Sans"},
			{"monospace", "Noto Sans Mono"},
		}
		for _, lang := range []string{"x-western", "x-unicode"} {
			for _, f := range fonts {
				prefs = append(prefs, struct {
					name  string
					value string
				}{fmt.Sprintf("font.name.%s.%s", f.generic, lang), fmt.Sprintf("%q", f.font)})
			}
		}
	}
	lines := make([]string, len(prefs))
	for i, p := range prefs {
		lines[i] = fmt.Sprintf("user_pref(%q, %s);", p.name, p.value)
	}
	return lines
}

// prefsFile is the content of user.js.
func prefsFile(fontParity bool) []byte {
	out := []byte("// Written by omnisnap: a fresh profile per launch (SPEC-process.md).\n")
	for _, line := range geckoPrefs(fontParity) {
		out = append(out, line...)
		out = append(out, '\n')
	}
	return out
}
