package layoutdiff

import "strings"

// Severity ranks a cause for display (1..3): presence, visibility and
// horizontal overflow always matter; wrapping and vertical overflow matter;
// otherwise size or movement of at least 10 px.
func Severity(kinds []string, d *Delta) int {
	for _, k := range kinds {
		if strings.HasPrefix(k, "only-in-") || strings.HasPrefix(k, "hidden-in-") || k == "overflow-x" {
			return 3
		}
	}
	for _, k := range kinds {
		if k == "wrap" || k == "overflow-y" {
			return 2
		}
	}
	if d != nil {
		m := 0.0
		for _, v := range []float64{d.DW, d.DH, d.DX, d.DY} {
			if a := absFloat(v); a > m {
				m = a
			}
		}
		if m >= 10 {
			return 2
		}
	}
	return 1
}
