package triage

import (
	"encoding/json"
	"math"
	"os"
	"testing"

	"github.com/GerMP21/Omnisnap/assets/model"
)

// Task 63: Go inference matches LightGBM on the parity rows exported by the
// spike's own evaluator (lib/gbdt.mjs); a mismatched feature list is refused.

type parityDoc struct {
	Features []string `json:"features"`
	Rows     []struct {
		Slug   string    `json:"slug"`
		Engine string    `json:"engine"`
		ID     string    `json:"id"`
		P      float64   `json:"p"`
		X      []float64 `json:"x"`
	} `json:"rows"`
}

func loadParity(t *testing.T) parityDoc {
	t.Helper()
	b, err := os.ReadFile("../../testdata/model/parity.json")
	if err != nil {
		t.Fatal(err)
	}
	var d parityDoc
	if err := json.Unmarshal(b, &d); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestModelParity(t *testing.T) {
	m, err := ParseModel(modelassets.GBDT)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Compatible(); err != nil {
		t.Fatalf("shipped model and feature list disagree: %v", err)
	}
	d := loadParity(t)
	if len(d.Rows) == 0 {
		t.Fatal("no parity rows")
	}
	if len(d.Features) != len(FeatureNames) {
		t.Fatalf("parity features %d, names %d", len(d.Features), len(FeatureNames))
	}
	for i := range d.Features {
		if d.Features[i] != FeatureNames[i] {
			t.Fatalf("feature %d: %q vs %q", i, d.Features[i], FeatureNames[i])
		}
	}
	maxDiff := 0.0
	for _, row := range d.Rows {
		got, err := m.Predict(row.X)
		if err != nil {
			t.Fatalf("%s: %v", row.ID, err)
		}
		if diff := math.Abs(got - row.P); diff > maxDiff {
			maxDiff = diff
		}
		if diff := math.Abs(got - row.P); diff > 1e-9 {
			t.Errorf("%s/%s %s: P = %.15f, LightGBM %.15f (diff %g)", row.Slug, row.Engine, row.ID, got, row.P, diff)
		}
	}
	t.Logf("%d rows, max |ΔP| = %g", len(d.Rows), maxDiff)
}

func TestModelMetaMatchesFeatureNames(t *testing.T) {
	var meta struct {
		Features []string `json:"features"`
		Tau      float64  `json:"tau"`
	}
	if err := json.Unmarshal(modelassets.Meta, &meta); err != nil {
		t.Fatal(err)
	}
	if len(meta.Features) != len(FeatureNames) {
		t.Fatalf("meta features %d, names %d", len(meta.Features), len(FeatureNames))
	}
	for i := range meta.Features {
		if meta.Features[i] != FeatureNames[i] {
			t.Errorf("meta feature %d: %q vs %q", i, meta.Features[i], FeatureNames[i])
		}
	}
	if meta.Tau <= 0 || meta.Tau >= 1 {
		t.Errorf("tau = %v; want a probability", meta.Tau)
	}
}

// A model trained on a different feature list is refused, not scored wrong.
func TestModelRefusesDifferentFeatures(t *testing.T) {
	m, err := ParseModel(modelassets.GBDT)
	if err != nil {
		t.Fatal(err)
	}
	m.Features[0] = "something_else"
	if err := m.Compatible(); err == nil {
		t.Error("a model with different feature names was accepted")
	}
}

// Features() must produce the vector the model was trained on.
func TestModelScoreNeedsAFullVector(t *testing.T) {
	m, err := ParseModel(modelassets.GBDT)
	if err != nil {
		t.Fatal(err)
	}
	x, err := Features(FeatureInput{Tag: "div", Side: "A", Count: 1})
	if err != nil || len(x) != 82 {
		t.Fatalf("features: %d values, %v", len(x), err)
	}
	p, err := m.Predict(x)
	if err != nil || p < 0 || p > 1 {
		t.Errorf("predict = %v, %v", p, err)
	}
}
