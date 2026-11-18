package install

import _ "embed"

// mozillaKey is Mozilla's release signing key, pinned in the binary
// (SPEC-install.md): a key rotation is an Omnisnap release. It is the KEY file
// published next to every release's SHA512SUMS.
//
//go:embed mozilla-key.asc
var mozillaKey []byte
