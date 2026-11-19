//go:build browsers

package discovery

import (
	"context"
	"os"
	"strings"
	"testing"
)

// Tasks 30 and 32: on a CI runner, the browsers the image ships must be found,
// each with a version. OMNISNAP_EXPECT_BROWSERS lists the discovery ids the
// runner image has (comma-separated); CI sets it per OS from the image readmes
// (github.com/actions/runner-images, "Browsers and Drivers"). Unset, the test
// skips: a developer machine has whatever it has.
func TestExpectedBrowsers(t *testing.T) {
	want := os.Getenv("OMNISNAP_EXPECT_BROWSERS")
	if want == "" {
		t.Skip("OMNISNAP_EXPECT_BROWSERS not set (CI sets it per runner image)")
	}
	found, err := Find(context.Background(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Browser{}
	for _, b := range found {
		byID[b.ID] = b
	}
	for _, id := range strings.Split(want, ",") {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		b, ok := byID[id]
		switch {
		case !ok:
			t.Errorf("%s: not found", id)
		case b.Version == "":
			t.Errorf("%s: found at %s without a version", id, b.Executable)
		default:
			t.Logf("%-8s %-14s %s (%s)", id, b.Version, b.Executable, b.Source)
		}
	}
}
