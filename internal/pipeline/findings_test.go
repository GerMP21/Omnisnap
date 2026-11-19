package pipeline

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/GerMP21/Omnisnap/internal/schema"
	"github.com/GerMP21/Omnisnap/internal/triage"
)

// The identity fields survive Cause's own MarshalJSON, which is promoted from
// the embedded struct and would otherwise drop them.
func TestFindingsCauseJSON(t *testing.T) {
	node := 14
	b, err := json.Marshal(FindingsCause{
		Cause: schema.Cause{ID: "p|width|table#target", Kinds: []string{"width"}, Selector: "table#target"},
		Node:  &node, Side: "A",
	})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["nodeId"] != float64(14) || m["side"] != "A" {
		t.Errorf("json = %s; want nodeId 14 and side A", b)
	}
	if m["id"] != "p|width|table#target" {
		t.Errorf("the cause's own fields were lost: %s", b)
	}
	// A cause without an identity keeps the old shape.
	b, err = json.Marshal(FindingsCause{Cause: schema.Cause{ID: "x", Kinds: []string{}}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "nodeId") || strings.Contains(string(b), "\"side\"") {
		t.Errorf("json = %s; want no identity fields", b)
	}
}

// The fingerprint reaches the findings file, which is where the crawl's
// frequency table is counted from (Task 73, Task 76).
func TestCausesWithRefsCarriesFingerprint(t *testing.T) {
	causes := []schema.Cause{{ID: "p|width|x", Kinds: []string{"width"}}}
	refs := map[string]triage.CauseRef{
		"p|width|x": {Side: "B", Node: 7, Fingerprint: "blink~firefox|layout|width|text|no-events"},
	}
	out := CausesWithRefs(causes, refs)
	if len(out) != 1 || out[0].Fingerprint != refs["p|width|x"].Fingerprint {
		t.Fatalf("CausesWithRefs = %+v; want the ref's fingerprint", out)
	}
	b, err := json.Marshal(out[0])
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["fingerprint"] != refs["p|width|x"].Fingerprint {
		t.Errorf("json = %s; want the fingerprint", b)
	}
	// A cause with no ref keeps the old shape.
	bare := CausesWithRefs(causes, nil)
	b, err = json.Marshal(bare[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "fingerprint") {
		t.Errorf("json = %s; want no fingerprint field", b)
	}
}
