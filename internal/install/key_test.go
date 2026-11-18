package install

import "testing"

// The pinned key is Mozilla's release key (SPEC-install.md: a rotation is a
// deliberate Omnisnap release). These ids catch an accidental replacement:
// the primary key and the signing subkey Mozilla currently signs SHA512SUMS
// with.
func TestMozillaKeyIsTheReleaseKey(t *testing.T) {
	keys, err := parseKeyring(mozillaKey)
	if err != nil {
		t.Fatalf("pinned key: %v", err)
	}
	for _, want := range []struct {
		id   uint64
		what string
	}{
		{0x61B7B526D98F0353, "primary key 14F26682D0916CDD81E37B6D61B7B526D98F0353"},
		{0x678E455D76767AA3, "signing subkey 827E658608679618CD349F93678E455D76767AA3"},
	} {
		if _, ok := keys[want.id]; !ok {
			t.Errorf("pinned key lacks %s", want.what)
		}
	}
}
