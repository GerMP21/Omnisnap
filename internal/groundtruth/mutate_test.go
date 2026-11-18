package groundtruth

import (
	"fmt"
	"strings"
	"testing"

	gtinject "github.com/GerMP21/Omnisnap/internal/groundtruth/inject"
	"github.com/GerMP21/Omnisnap/internal/schema"
)

// Task 70: the scheduling half of mutation capture is pure, so it is tested
// without a browser. The capture half runs in the browsers test file.

func TestFNV32MatchesTheSpike(t *testing.T) {
	cases := map[string]uint32{
		"x":              4245442695,
		"base|chrome|0":  3119831266,
		"base|firefox|3": 817171696,
	}
	for in, want := range cases {
		if got := FNV32(in); got != want {
			t.Errorf("FNV32(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestScheduleMutantsRotatesOperators(t *testing.T) {
	ms := ScheduleMutants("base", "chrome", 4)
	if len(ms) != 4 {
		t.Fatalf("%d mutants, want 4", len(ms))
	}
	seen := map[string]bool{}
	for i, m := range ms {
		id := fmt.Sprintf("base~chrome~%d", i)
		if m.Slug != id {
			t.Errorf("slug %q, want %q", m.Slug, id)
		}
		if m.Base != "base" || m.Engine != "chrome" || m.Index != i {
			t.Errorf("mutant %d = %+v", i, m)
		}
		if want := FNV32(fmt.Sprintf("base|chrome|%d", i)); m.Seed != want {
			t.Errorf("mutant %d seed %d, want %d", i, m.Seed, want)
		}
		// The ops list is the full rotation, starting at the seeded operator:
		// an operator with no candidate falls through to the next.
		if len(m.Ops) != len(MutateOps) {
			t.Fatalf("ops = %d, want %d", len(m.Ops), len(MutateOps))
		}
		start := int(m.Seed) % len(MutateOps)
		for j := range MutateOps {
			if m.Ops[j] != MutateOps[(start+j)%len(MutateOps)] {
				t.Fatalf("mutant %d rotation broken at %d: %v", i, j, m.Ops)
			}
		}
		if seen[m.Slug] {
			t.Errorf("duplicate slug %q", m.Slug)
		}
		seen[m.Slug] = true
	}
	// Different engines get different seeds, so they are independent draws.
	if other := ScheduleMutants("base", "firefox", 1)[0]; other.Seed == ms[0].Seed {
		t.Error("chrome and firefox share a seed")
	}
	if got := ScheduleMutants("base", "chrome", 0); len(got) != DefaultMutantsPerEngine {
		t.Errorf("perEngine 0 gave %d mutants, want the default %d", len(got), DefaultMutantsPerEngine)
	}
}

func TestEligibleRequiresSimilarRenders(t *testing.T) {
	cap := func(nodes int) *schema.Capture {
		return &schema.Capture{Meta: schema.Meta{Nodes: nodes}}
	}
	cases := []struct {
		a, b int
		want bool
	}{
		{100, 100, true},
		{100, 110, true},
		{100, 120, true},  // exactly 20% of the larger
		{100, 130, false}, // 30/130 = 23%
		{100, 40, false},  // the smaller side is below 50 nodes
		{0, 0, false},
		{50, 60, true},
	}
	for _, c := range cases {
		if got := Eligible(cap(c.a), cap(c.b)); got != c.want {
			t.Errorf("Eligible(%d,%d) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
	if Eligible(nil, cap(100)) || Eligible(cap(100), nil) {
		t.Error("a missing capture is eligible")
	}
}

func TestMutateOpsCoverTheScript(t *testing.T) {
	// Every rotated operator must exist in the embedded script, or a mutant
	// would silently fall through to the next one.
	for _, op := range MutateOps {
		if !strings.Contains(gtinject.Mutate, op+": [") && !strings.Contains(gtinject.Mutate, op+"': [") {
			t.Errorf("%q is not an operator in mutate.js", op)
		}
	}
}

func TestRotateOps(t *testing.T) {
	ops := []string{"a", "b", "c"}
	cases := map[int][]string{0: {"a", "b", "c"}, 1: {"b", "c", "a"}, 3: {"a", "b", "c"}, 4: {"b", "c", "a"}}
	for k, want := range cases {
		got := rotateOps(ops, k)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("rotateOps(%d) = %v, want %v", k, got, want)
		}
	}
	// The input is not aliased: a rotated retry must not corrupt the schedule.
	if rotateOps(ops, 1)[0] == "a" {
		t.Error("rotateOps did not rotate")
	}
	if ops[0] != "a" {
		t.Error("rotateOps reordered its input")
	}
}
