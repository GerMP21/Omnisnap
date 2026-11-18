package evidence

import (
	"math"
	"strconv"

	"github.com/GerMP21/Omnisnap/internal/layoutdiff"
	"github.com/GerMP21/Omnisnap/internal/schema"
)

// Calibration (Task 55): learn, per page and engine pair, how the engines
// SYSTEMATICALLY differ, with robust statistics (median, MAD) that ignore
// outliers:
//   - text advance: ratio of text widths for identical text in identical fonts
//   - line pitch: per computed line-height, B pitch minus A pitch
//   - form controls: per control type, the typical size difference
//
// A cause the page model explains is expected; outliers are anomalies.
// Ported from lib/calibrate.mjs.

var controlTags = map[string]bool{"input": true, "select": true, "textarea": true, "button": true}

// Advance is the page-wide text advance model.
type Advance struct {
	Ratio float64
	Mad   float64
	N     int
}

// Pitch is one line-height's pitch difference.
type Pitch struct {
	DPitch float64
	Mad    float64
	N      int
}

// Control is one control type's typical size difference.
type Control struct {
	DW, DH float64
	N      int
}

// Calibration is the page's systematic engine model.
type Calibration struct {
	Advance  *Advance
	Pitch    map[string]Pitch
	Controls map[string]Control
}

// Calibrate learns the model from the baseline tree and its match to the other
// engine.
func Calibrate(A *layoutdiff.Tree, M *layoutdiff.Match) *Calibration {
	model := &Calibration{Pitch: map[string]Pitch{}, Controls: map[string]Control{}}
	var ratios []float64
	pitchSamples := map[string][]float64{}
	controlSamples := map[string][2][]float64{}
	for _, a := range A.All {
		b := M.PairOf[a.UID]
		if b == nil || bool(a.N.Hid) || bool(b.N.Hid) {
			continue
		}
		if a.HasText() && b.HasText() && a.TH() != "" && a.TH() == b.TH() && a.LN() == b.LN() &&
			a.N.FF == b.N.FF && a.N.FS == b.N.FS && a.TW() > 20 && b.TW() > 0 {
			ratios = append(ratios, b.TW()/a.TW())
		}
		if a.LN() >= 2 && b.LN() == a.LN() && a.N.LH == b.N.LH {
			pa, pb := ownPitch(a), ownPitch(b)
			if pb > 0 && pa > 0 {
				key := fmtLineHeight(a.N.LH) + "|" + fmtFloat(a.N.FS) + "|" + a.N.FF
				pitchSamples[key] = append(pitchSamples[key], pb-pa)
			}
		}
		if controlTags[a.N.T] {
			s := controlSamples[a.N.T]
			s[0] = append(s[0], b.N.B[2]-a.N.B[2])
			s[1] = append(s[1], b.N.B[3]-a.N.B[3])
			controlSamples[a.N.T] = s
		}
	}
	if len(ratios) > 0 {
		r := layoutdiff.Median(ratios)
		model.Advance = &Advance{Ratio: r, Mad: madOf(ratios, r), N: len(ratios)}
	}
	for k, xs := range pitchSamples {
		m := layoutdiff.Median(xs)
		model.Pitch[k] = Pitch{DPitch: m, Mad: madOf(xs, m), N: len(xs)}
	}
	for k, s := range controlSamples {
		model.Controls[k] = Control{DW: layoutdiff.Median(s[0]), DH: layoutdiff.Median(s[1]), N: len(s[0])}
	}
	if model.Advance == nil && len(model.Pitch) == 0 && len(model.Controls) == 0 {
		return nil
	}
	return model
}

// ownPitch is the median gap between an element's own line tops.
func ownPitch(n *layoutdiff.Node) float64 {
	if n.N.Text == nil || len(n.N.LT) < 2 {
		return 0
	}
	lt := n.N.LT
	gaps := make([]float64, 0, len(lt)-1)
	for i := 1; i < len(lt); i++ {
		gaps = append(gaps, lt[i]-lt[i-1])
	}
	return layoutdiff.Median(gaps)
}

func madOf(xs []float64, med float64) float64 {
	dev := make([]float64, len(xs))
	for i, x := range xs {
		dev[i] = math.Abs(x - med)
	}
	return layoutdiff.Median(dev)
}

// Reason is one model explanation of a cause's measured change.
type Reason struct {
	Model    string  `json:"model"` // advance, pitch, controls
	Expected float64 `json:"expected,omitempty"`
	Tol      float64 `json:"tol,omitempty"`
	DW       float64 `json:"dw,omitempty"`
	DH       float64 `json:"dh,omitempty"`
}

// Explained says whether the page model explains this cause.
type Explained struct {
	Explained bool     `json:"explained"`
	By        []Reason `json:"by"`
}

// ExplainsModel tests a cause against the page model.
func ExplainsModel(c layoutdiff.Cause, model *Calibration) Explained {
	if c.Side != "A" || model == nil || c.D == nil {
		return Explained{By: []Reason{}}
	}
	a, d := c.Node, *c.D
	var out []Reason
	if d.DW != 0 && a.HasText() && model.Advance != nil && model.Advance.N >= 5 {
		expected := (model.Advance.Ratio - 1) * a.TW()
		tol := 1 + 3*model.Advance.Mad*a.TW()
		if math.Abs(d.DW-expected) <= tol {
			out = append(out, Reason{Model: "advance", Expected: round1(expected), Tol: round1(tol)})
		}
	}
	if d.DH != 0 {
		lines, _, ok := layoutdiff.LineInfo(a)
		key := dominantLineHeightKey(a)
		m, has := model.Pitch[key]
		if ok && has && m.N >= 2 && math.Abs(m.DPitch) >= 0.05 {
			expected := float64(lines) * m.DPitch
			tol := 2 + float64(lines)*m.Mad
			if math.Abs(d.DH-expected) <= tol {
				out = append(out, Reason{Model: "pitch", Expected: round1(expected), Tol: round1(tol)})
			}
		}
	}
	if cm, ok := model.Controls[a.N.T]; ok && controlTags[a.N.T] && cm.N >= 2 {
		if math.Abs(d.DW-cm.DW) <= 2 && math.Abs(d.DH-cm.DH) <= 2 {
			out = append(out, Reason{Model: "controls", DW: round1(cm.DW), DH: round1(cm.DH)})
		}
	}
	return Explained{Explained: len(out) > 0, By: out}
}

// dominantLineHeightKey is the typography most of the element's text uses.
func dominantLineHeightKey(n *layoutdiff.Node) string {
	counts := map[string]int{}
	var visit func(x *layoutdiff.Node)
	visit = func(x *layoutdiff.Node) {
		if x.N.Text != nil {
			key := fmtLineHeight(x.N.LH) + "|" + fmtFloat(x.N.FS) + "|" + x.N.FF
			counts[key]++
		}
		for _, k := range x.Kids {
			visit(k)
		}
	}
	visit(n)
	best, bestN := "", 0
	for k, c := range counts {
		if c > bestN {
			best, bestN = k, c
		}
	}
	return best
}

func round1(f float64) float64 { return math.Round(f*10) / 10 }

func fmtLineHeight(lh schema.LineHeight) string {
	if lh.Normal {
		return "normal"
	}
	return strconv.FormatFloat(lh.PX, 'f', -1, 64)
}

func fmtFloat(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }
