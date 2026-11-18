package triage

import (
	"strings"
	"testing"
)

func TestFingerprintStability(t *testing.T) {
	a := Fingerprint([]string{"width"}, "layout", "engine-odd", "div", "text", []string{"spacing-changed", "alignment-broken"}, "firefox")
	b := Fingerprint([]string{"width"}, "layout", "engine-odd", "div", "text", []string{"alignment-broken", "spacing-changed", "alignment-broken"}, "firefox")
	if a != b {
		t.Errorf("fingerprint is order or duplicate sensitive:\n  %s\n  %s", a, b)
	}
	if !strings.Contains(a, "blink~firefox|layout|width|text|alignment-broken+spacing-changed") {
		t.Errorf("fingerprint %q", a)
	}
	// Kind classes and roles.
	c := Fingerprint([]string{"only-in-firefox"}, "", "", "button", "", nil, "firefox")
	if !strings.Contains(c, "|unknown|presence|control|no-events") {
		t.Errorf("fingerprint %q", c)
	}
}

func TestNovelty(t *testing.T) {
	data := []byte(`{"total":100,"fingerprints":{"fp-a":{"n":50,"pages":40},"fp-b":{"n":1,"pages":1}}}`)
	table, err := LoadTable(data)
	if err != nil {
		t.Fatal(err)
	}
	a := table.Surprise("fp-a", "p")
	b := table.Surprise("fp-b", "p")
	missing := table.Surprise("fp-c", "p")
	if a.OtherPages != 39 || b.Bits <= a.Bits || missing.Count != 0 || missing.Bits != b.Bits {
		t.Errorf("novelty a=%+v b=%+v missing=%+v", a, b, missing)
	}
	// No table: absent, not an error.
	if got, err := LoadTable(nil); got != nil || err != nil {
		t.Errorf("LoadTable(nil) = %v, %v", got, err)
	}
	if (*Table)(nil).Surprise("x", "p") != nil {
		t.Error("nil table produced a novelty score")
	}
}

func TestNoveltyBadTable(t *testing.T) {
	if _, err := LoadTable([]byte("{")); err == nil {
		t.Error("a broken table was accepted")
	}
}
