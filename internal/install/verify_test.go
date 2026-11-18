package install

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("../../testdata/install", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// A real release: Mozilla's own SHA512SUMS and its detached signature verify
// against the pinned key (no network, no gpg).
func TestVerifyRealMozillaSignature(t *testing.T) {
	sums := fixture(t, "SHA512SUMS")
	if err := VerifyDetached(sums, fixture(t, "SHA512SUMS.asc"), mozillaKey); err != nil {
		t.Fatalf("Mozilla's signature did not verify: %v", err)
	}
	// The fixture KEY and the pinned key are the same block.
	if err := VerifyDetached(sums, fixture(t, "SHA512SUMS.asc"), fixture(t, "KEY")); err != nil {
		t.Errorf("the published KEY does not verify: %v", err)
	}
}

func TestVerifyRejectsTamperedData(t *testing.T) {
	sums := append([]byte{}, fixture(t, "SHA512SUMS")...)
	sums[len(sums)/2] ^= 0x20 // flip one byte
	if err := VerifyDetached(sums, fixture(t, "SHA512SUMS.asc"), mozillaKey); err == nil {
		t.Error("a tampered SHA512SUMS verified")
	}
}

// A signature packet with one corrupted byte must fail: the verification is
// not a presence check.
func TestVerifyRejectsCorruptSignature(t *testing.T) {
	blocks, err := armor(fixture(t, "SHA512SUMS.asc"))
	if err != nil || len(blocks) == 0 {
		t.Fatal(err)
	}
	raw := append([]byte{}, blocks[0]...)
	raw[len(raw)/2] ^= 0xff
	doc := rearmor("PGP SIGNATURE", raw)
	if err := VerifyDetached(fixture(t, "SHA512SUMS"), doc, mozillaKey); err == nil {
		t.Error("a corrupt signature verified")
	}
}

// rearmor wraps raw binary in an ASCII armor block.
func rearmor(kind string, raw []byte) []byte {
	b64 := base64.StdEncoding.EncodeToString(raw)
	var b strings.Builder
	fmt.Fprintf(&b, "-----BEGIN %s-----\n\n", kind)
	for i := 0; i < len(b64); i += 64 {
		b.WriteString(b64[i:min(i+64, len(b64))])
		b.WriteByte('\n')
	}
	fmt.Fprintf(&b, "-----END %s-----\n", kind)
	return []byte(b.String())
}

func TestVerifyRejectsGarbageKey(t *testing.T) {
	if err := VerifyDetached(fixture(t, "SHA512SUMS"), fixture(t, "SHA512SUMS.asc"), []byte("not a key")); err == nil {
		t.Error("verification without a key block succeeded")
	}
}
