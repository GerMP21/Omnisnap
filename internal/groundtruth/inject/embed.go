// Package inject embeds mutate.js, which injects one realistic bug into a
// settled page for mutation testing (SPEC-ground-truth.md). It is lab-only:
// it lives here, not in assets/inject, because go:embed can only reach the
// package's own directory, and a test checks that cmd/omnisnap never embeds it.
package inject

import _ "embed"

// Mutate is the spike's mutate.js, byte for byte (pinned in SHA256SUMS).
//
//go:embed mutate.js
var Mutate string
