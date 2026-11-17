// Package inject embeds the scripts injected into every captured page. They
// are the spike's scripts byte for byte (spikes/layout-diff/inject/): the
// precision results were measured with them. SHA256SUMS pins each one;
// changing a script needs approval (CONVENTIONS.md, "Ask first").
package inject

import _ "embed"

// Stabilize waits for the page to settle: fonts, lazy content, animations.
//
//go:embed stabilize.js
var Stabilize string

// Extract records the geometry of every element.
//
//go:embed extract.js
var Extract string

// Probes re-measures the page with suspected causes neutralized
// (fractional line-heights, fonts, form-control defaults).
//
//go:embed probes.js
var Probes string
