package inject

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/GerMP21/Omnisnap/internal/testutil"
)

// spikeDir holds the originals the scripts were validated as (spike rounds 1–6).
const spikeDir = "../../spikes/layout-diff/inject"

var embedded = map[string]string{
	"stabilize.js": Stabilize,
	"extract.js":   Extract,
	"probes.js":    Probes,
}

func TestEmbeddedScriptsMatchManifest(t *testing.T) {
	manifest := testutil.ReadSHA256Sums(t, "SHA256SUMS")
	if len(manifest) != len(embedded) {
		t.Errorf("manifest lists %d files, want %d: %v", len(manifest), len(embedded), manifest)
	}
	for name, content := range embedded {
		sum := sha256.Sum256([]byte(content))
		if got, want := hex.EncodeToString(sum[:]), manifest[name]; got != want {
			t.Errorf("%s: sha256 %s, manifest %q (changing an injected script needs approval: CONVENTIONS.md)", name, got, want)
		}
	}
}

func TestScriptsAreTheSpikeOriginals(t *testing.T) {
	manifest := testutil.ReadSHA256Sums(t, "SHA256SUMS")
	for name, want := range manifest {
		b, err := os.ReadFile(filepath.Join(spikeDir, name))
		if errors.Is(err, fs.ErrNotExist) {
			t.Skipf("spike removed: %v", err)
		}
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(b)
		if got := hex.EncodeToString(sum[:]); got != want {
			t.Errorf("%s: spike copy sha256 %s, manifest %s: the spike and the embedded script diverged", name, got, want)
		}
	}
}
