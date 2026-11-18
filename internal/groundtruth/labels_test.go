package groundtruth

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/GerMP21/Omnisnap/internal/schema"
)

func TestWriteLabels(t *testing.T) {
	out := t.TempDir()
	m := Mutant{Slug: "page~firefox~0", Base: "page", Engine: "firefox"}
	if err := os.MkdirAll(filepath.Join(out, m.Slug), 0o755); err != nil {
		t.Fatal(err)
	}
	label := MutantLabel{
		Target: "body > main > p", Scope: 2, Effective: true, Changed: 4,
		Positives: []string{"body", "body > main > p"},
		PositiveUIDs: map[int]bool{
			7: true, 2: true, 5: true,
		},
	}
	if err := WriteLabels(out, m, &schema.Mutation{Op: "padding-left"}, label); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(out, m.Slug, "labels.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got LabelFile
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.Op != "padding-left" || !got.Effective || got.Changed != 4 || got.Scope != 2 {
		t.Errorf("labels = %+v", got)
	}
	// The uids are sorted, so two runs of the same label write the same bytes.
	want := []int{2, 5, 7}
	if len(got.PositiveUIDs) != len(want) {
		t.Fatalf("uids = %v, want %v", got.PositiveUIDs, want)
	}
	for i := range want {
		if got.PositiveUIDs[i] != want[i] {
			t.Fatalf("uids = %v, want %v", got.PositiveUIDs, want)
		}
	}
	// The exact identity and the human form are both there.
	if len(got.Positives) != 2 {
		t.Errorf("positives = %v", got.Positives)
	}
	if got.Attempts != 0 {
		t.Errorf("attempts = %d, want it absent by default", got.Attempts)
	}
}

// An ineffective mutant still writes a complete file: the dataset keeps it as
// a hard negative.
func TestWriteLabelsIneffective(t *testing.T) {
	out := t.TempDir()
	m := Mutant{Slug: "page~firefox~1", Base: "page", Engine: "firefox"}
	if err := os.MkdirAll(filepath.Join(out, m.Slug), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := WriteLabels(out, m, &schema.Mutation{Op: "border"}, MutantLabel{}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(out, m.Slug, "labels.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got LabelFile
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.Effective || len(got.Positives) != 0 || len(got.PositiveUIDs) != 0 || got.Op != "border" {
		t.Errorf("labels = %+v", got)
	}
}
