package pipeline

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/GerMP21/Omnisnap/internal/schema"
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
