package install

import (
	"crypto/sha512"
	"encoding/hex"
	"os"
	"testing"
)

// hostFixture is a fake Firefox 999.0 release in the format this OS's lane
// downloads: the committed tar.xz on Linux, a DMG built with hdiutil on
// macOS, a setup .exe with an embedded 7z archive built with 7-Zip on Windows
// (Task 37, SPEC-install.md "Testing strategy"). The archives the runner builds are signed at test time.
type hostFixture struct {
	rel     string // the archive's path under the version, as SHA512SUMS lists it
	archive []byte
	sums    []byte
	sig     []byte
	key     []byte
}

// committedFixture is the Linux release kept in testdata, signed ahead of time.
func committedFixture(t *testing.T) hostFixture {
	t.Helper()
	archive, err := os.ReadFile("../../testdata/install/fake/firefox-999.0.tar.xz")
	if err != nil {
		t.Fatal(err)
	}
	return hostFixture{
		rel:     "linux-x86_64/en-US/firefox-999.0.tar.xz",
		archive: archive,
		sums:    mustFixture(t, "fake/SHA512SUMS"),
		sig:     mustFixture(t, "fake/SHA512SUMS.asc"),
		key:     testKey(t),
	}
}

// signedFixture lists archive in a fresh SHA512SUMS at this host's path and
// signs it with a key made for the test.
func signedFixture(t *testing.T, archive []byte) hostFixture {
	t.Helper()
	rel, err := archivePath("999.0", HostTarget())
	if err != nil {
		t.Fatal(err)
	}
	sum := sha512.Sum512(archive)
	sums := []byte(hex.EncodeToString(sum[:]) + "  " + rel + "\n")
	s := newTestSigner(t)
	return hostFixture{rel: rel, archive: archive, sums: sums, sig: s.sign(t, sums), key: s.publicKey()}
}
