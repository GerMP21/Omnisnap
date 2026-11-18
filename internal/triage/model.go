package triage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"sync"

	"github.com/GerMP21/Omnisnap/assets/model"
)

// Model is the trained tree model (Task 63): LightGBM's JSON dump, evaluated
// in Go. The alternative, github.com/dmitryikh/leaves, is not needed: the dump
// has exactly what binary GBDT inference needs (numeric splits, no NaN).
type Model struct {
	Features []string
	Trees    []*treeNode
}

type treeNode struct {
	Split     int // feature index; -1 for a leaf
	Threshold float64
	Left      *treeNode
	Right     *treeNode
	Leaf      float64
	IsLeaf    bool
}

type rawNode struct {
	SplitFeature *int     `json:"split_feature"`
	Threshold    *float64 `json:"threshold"`
	DecisionType string   `json:"decision_type"`
	Left         *rawNode `json:"left_child"`
	Right        *rawNode `json:"right_child"`
	LeafValue    *float64 `json:"leaf_value"`
}

type rawModel struct {
	Objective    string   `json:"objective"`
	FeatureNames []string `json:"feature_names"`
	TreeInfo     []struct {
		TreeStructure rawNode `json:"tree_structure"`
	} `json:"tree_info"`
}

// ParseModel reads a LightGBM dump. Only a binary objective and "<=" splits
// are supported; anything else is refused rather than scored wrong.
func ParseModel(data []byte) (*Model, error) {
	var raw rawModel
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("triage: model: %w", err)
	}
	if raw.Objective != "" && !strings.HasPrefix(raw.Objective, "binary") {
		return nil, fmt.Errorf("triage: model: unsupported objective %q", raw.Objective)
	}
	m := &Model{Features: raw.FeatureNames}
	for i := range raw.TreeInfo {
		node, err := convertNode(&raw.TreeInfo[i].TreeStructure)
		if err != nil {
			return nil, fmt.Errorf("triage: model tree %d: %w", i, err)
		}
		m.Trees = append(m.Trees, node)
	}
	return m, nil
}

func convertNode(n *rawNode) (*treeNode, error) {
	if n.LeafValue != nil {
		return &treeNode{IsLeaf: true, Leaf: *n.LeafValue, Split: -1}, nil
	}
	if n.DecisionType != "<=" {
		return nil, fmt.Errorf("unsupported split %q", n.DecisionType)
	}
	if n.SplitFeature == nil || n.Threshold == nil || n.Left == nil || n.Right == nil {
		return nil, fmt.Errorf("incomplete split node")
	}
	left, err := convertNode(n.Left)
	if err != nil {
		return nil, err
	}
	right, err := convertNode(n.Right)
	if err != nil {
		return nil, err
	}
	return &treeNode{Split: *n.SplitFeature, Threshold: *n.Threshold, Left: left, Right: right}, nil
}

// Compatible reports whether the model can score this binary's vectors: its
// feature names must be exactly the shipped list, in order.
func (m *Model) Compatible() error {
	if len(m.Features) != len(FeatureNames) {
		return fmt.Errorf("triage: model has %d features, this binary %d", len(m.Features), len(FeatureNames))
	}
	for i, f := range m.Features {
		if f != FeatureNames[i] {
			return fmt.Errorf("triage: model feature %d is %q, this binary expects %q (retrain)", i, f, FeatureNames[i])
		}
	}
	return nil
}

// Score computes the vector and the model's probability for one cause.
func (m *Model) Score(in FeatureInput) (float64, error) {
	x, err := Features(in)
	if err != nil {
		return 0, err
	}
	return m.Predict(x)
}

// Predict is P(label 1): sigmoid of the summed leaf values.
func (m *Model) Predict(x []float64) (float64, error) {
	if len(x) != len(m.Features) {
		return 0, fmt.Errorf("triage: model: %d values, %d features", len(x), len(m.Features))
	}
	raw := 0.0
	for _, tree := range m.Trees {
		node := tree
		for !node.IsLeaf {
			if node.Split < 0 || node.Split >= len(x) {
				return 0, fmt.Errorf("triage: model: split feature %d out of range", node.Split)
			}
			if x[node.Split] <= node.Threshold {
				node = node.Left
			} else {
				node = node.Right
			}
		}
		raw += node.Leaf
	}
	return 1 / (1 + math.Exp(-raw)), nil
}

var (
	shippedOnce  sync.Once
	shippedModel *Model
)

// ShippedModel loads assets/model/gbdt.json once. nil when the model cannot be
// read or disagrees with this binary's feature list: the score is then hidden,
// never guessed.
func ShippedModel() *Model {
	shippedOnce.Do(func() {
		m, err := ParseModel(modelassets.GBDT)
		if err != nil || m.Compatible() != nil {
			return
		}
		shippedModel = m
	})
	return shippedModel
}

// inkAgreement is the compact summary's ink evidence (the agreement).
func inkAgreement(i *InkFeature) *float64 {
	if i == nil {
		return nil
	}
	v := i.Agreement
	return &v
}
