package inject

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/GerMP21/Omnisnap/internal/testutil"
)

const spikeMutate = "../../../spikes/layout-diff/inject/mutate.js"

func TestMutateMatchesManifestAndSpike(t *testing.T) {
	manifest := testutil.ReadSHA256Sums(t, "SHA256SUMS")
	want, ok := manifest["mutate.js"]
	if !ok || len(manifest) != 1 {
		t.Fatalf("manifest = %v, want exactly mutate.js", manifest)
	}
	sum := sha256.Sum256([]byte(Mutate))
	if got := hex.EncodeToString(sum[:]); got != want {
		t.Errorf("embedded mutate.js sha256 %s, manifest %s", got, want)
	}

	b, err := os.ReadFile(spikeMutate)
	if errors.Is(err, fs.ErrNotExist) {
		t.Skipf("spike removed: %v", err)
	}
	if err != nil {
		t.Fatal(err)
	}
	sum = sha256.Sum256(b)
	if got := hex.EncodeToString(sum[:]); got != want {
		t.Errorf("spike mutate.js sha256 %s, manifest %s: the spike and the embedded script diverged", got, want)
	}
}

// mutate.js injects bugs into pages: it is ground-truth tooling and must
// never ship (plan: "mutate.js never ships to users"). Checked on what the
// binary embeds, so a copy elsewhere is caught too.
func TestMutateIsNotInTheShippedBinary(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go list")
	}
	out, err := exec.Command("go", "list", "-deps",
		"-f", `{{.ImportPath}}{{range .EmbedFiles}} {{.}}{{end}}`,
		"github.com/GerMP21/Omnisnap/cmd/omnisnap").CombinedOutput()
	if err != nil {
		t.Fatalf("go list: %v\n%s", err, out)
	}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if fields[0] == "github.com/GerMP21/Omnisnap/internal/groundtruth/inject" {
			t.Errorf("cmd/omnisnap imports %s", fields[0])
		}
		for _, f := range fields[1:] {
			if strings.HasSuffix(f, "mutate.js") {
				t.Errorf("%s embeds %s into cmd/omnisnap", fields[0], f)
			}
		}
	}
}
