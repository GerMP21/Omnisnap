package triage

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/GerMP21/Omnisnap/internal/layoutdiff"
)

// Task 62: the feature vector equals the spike's on every golden finding.

type gBox struct {
	B [4]float64 `json:"b"`
}

type gFinding struct {
	Count     int      `json:"count"`
	Side      string   `json:"side"`
	Tag       string   `json:"tag"`
	Text      *string  `json:"text"`
	Severity  int      `json:"severity"`
	Impact    float64  `json:"impact"`
	RulesTier string   `json:"rulesTier"`
	Tier      string   `json:"tier"`
	Kinds     []string `json:"kinds"`
	Conseq    []string `json:"consequences"`
	A         *gBox    `json:"a"`
	B         *gBox    `json:"b"`
	D         struct {
		DW, DH, DX, DY float64
		DLN            int `json:"dln"`
	} `json:"d"`
	KB *struct {
		ID       string `json:"id"`
		Expected bool   `json:"expected"`
	} `json:"kb"`
	Evidence struct {
		Mechanism *struct {
			Verdict string `json:"verdict"`
		} `json:"mechanism"`
		Consensus *struct {
			Verdict string `json:"verdict"`
		} `json:"consensus"`
		Perception *struct {
			Score  *float64 `json:"score"`
			Events []struct {
				Type string `json:"type"`
			} `json:"events"`
		} `json:"perception"`
		Calibration *struct {
			Explained bool `json:"explained"`
		} `json:"calibration"`
		Ink *struct {
			Agreement float64 `json:"agreement"`
			Ink       int     `json:"ink"`
			Floor     float64 `json:"floor"`
		} `json:"ink"`
		Intent *struct {
			Verdict string `json:"verdict"`
		} `json:"intent"`
	} `json:"evidence"`
	Novelty *struct {
		OtherPages int `json:"otherPages"`
	} `json:"novelty"`
	Features []float64 `json:"features"`
}

func TestFeatureParity(t *testing.T) {
	if testing.Short() {
		t.Skip("golden feature parity")
	}
	entries, err := os.ReadDir(goldenDir)
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		slug := e.Name()
		for _, eng := range []string{"firefox", "webkit"} {
			path := filepath.Join(goldenDir, slug, "findings-"+eng+".json")
			b, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			var doc struct {
				Findings []gFinding `json:"findings"`
			}
			if err := json.Unmarshal(b, &doc); err != nil {
				t.Fatalf("%s: %v", path, err)
			}
			for _, f := range doc.Findings {
				if len(f.Features) != len(FeatureNames) {
					continue // older export without features
				}
				in := FeatureInput{
					Count: f.Count, Side: f.Side, Tag: f.Tag,
					Severity: f.Severity, Impact: f.Impact,
					RulesTier: f.RulesTier, Tier: f.Tier,
					Kinds: f.Kinds, Consequences: f.Conseq, Engine: eng,
					D: layoutdiff.Delta{DW: f.D.DW, DH: f.D.DH, DX: f.D.DX, DY: f.D.DY, DLN: f.D.DLN},
				}
				if f.Text != nil {
					in.Text = *f.Text
				}
				if f.KB != nil {
					in.KBID = f.KB.ID
					in.KBExpected = f.KB.Expected
				}
				if f.Evidence.Mechanism != nil {
					in.Mechanism = f.Evidence.Mechanism.Verdict
				}
				if f.Evidence.Consensus != nil {
					in.Consensus = f.Evidence.Consensus.Verdict
				}
				if f.Evidence.Perception != nil {
					in.Perception = f.Evidence.Perception.Score
					for _, ev := range f.Evidence.Perception.Events {
						in.Events = append(in.Events, ev.Type)
					}
				}
				if f.Evidence.Calibration != nil {
					in.Calibration = f.Evidence.Calibration.Explained
				}
				if f.Evidence.Ink != nil {
					in.Ink = &InkFeature{Agreement: f.Evidence.Ink.Agreement, Floor: f.Evidence.Ink.Floor, Ink: f.Evidence.Ink.Ink}
				}
				if f.Evidence.Intent != nil {
					in.IntentVerdict = f.Evidence.Intent.Verdict
				}
				if f.Novelty != nil {
					op := f.Novelty.OtherPages
					in.NoveltyOthers = &op
				}
				box := f.A
				if box == nil {
					box = f.B
				}
				if box != nil {
					bv := box.B
					in.Box = &bv
				}
				got, err := Features(in)
				if err != nil {
					t.Fatal(err)
				}
				checked++
				for i := range got {
					if math.Abs(got[i]-f.Features[i]) > 1e-9 {
						t.Errorf("%s/%s %s: feature %s = %v; golden %v", slug, eng, f.Side+":"+f.Tag, FeatureNames[i], got[i], f.Features[i])
						break
					}
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no golden findings with features")
	}
	t.Logf("features compared on %d golden findings", checked)
}

var _ = fmt.Sprint
