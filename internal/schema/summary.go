package schema

import (
	"encoding/json"
	"time"
)

// Summary is summary.json (schema/summary.schema.json): everything the
// viewer, merge, the CI Action and the wrappers read. It holds compact
// evidence only; each result's Findings file has the full evidence.
//
// Lists the schema requires are written as [] when nil (MarshalJSON), so a
// Summary built field by field always validates.
type Summary struct {
	Schema    string      `json:"$schema"`
	Tool      Tool        `json:"tool"`
	Generated time.Time   `json:"generated"`
	Job       Job         `json:"job"`
	Hosts     []Host      `json:"hosts"`
	Skipped   []Skipped   `json:"skipped"`
	Pages     []PageTotal `json:"pages"`
	Results   []Result    `json:"results"`
}

// NewSummary starts a summary written by this binary.
func NewSummary(version string, job Job) Summary {
	return Summary{
		Schema:    ID("summary"),
		Tool:      Tool{Name: "omnisnap", Version: version},
		Generated: time.Now().UTC().Truncate(time.Second),
		Job:       job,
	}
}

// Tool names the program that wrote the summary.
type Tool struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Skipped is an engine the job asked for that did not run.
type Skipped struct {
	Host   string `json:"host,omitempty"` // host id
	Engine string `json:"engine"`
	Reason string `json:"reason"`
}

// PageTotal is one page's cause counts across hosts and engines.
type PageTotal struct {
	Slug   string `json:"slug"`
	URL    string `json:"url"`
	Report int    `json:"report"`
	Info   int    `json:"info"`
}

// Result is one engine compared with the baseline on one page and one host.
type Result struct {
	Host     Machine           `json:"host"`
	Page     ResultPage        `json:"page"`
	Baseline ResultBaseline    `json:"baseline"`
	Engine   ResultEngine      `json:"engine"`
	Status   PageStatus        `json:"status"`
	Warnings []string          `json:"warnings"`
	Stats    Stats             `json:"stats"`
	Shots    *Shots            `json:"shots,omitempty"`    // absent when a capture is missing
	Findings string            `json:"findings,omitempty"` // bundle-relative path of the findings file
	Causes   []Cause           `json:"causes"`
	Intent   []IntentViolation `json:"intent"`
	Blocked  Sides             `json:"blocked"`         // hosts whose requests were blocked
	Dynamic  Sides             `json:"dynamic"`         // selectors of regions that differ between runs
	Audit    *ResultAudit      `json:"audit,omitempty"` // single-engine anomalies (dom-audit)
}

// ResultAudit is the dom-audit of both captures of a result: problems a page
// has within one engine, regardless of what other engines do.
type ResultAudit struct {
	Baseline []AuditFinding `json:"baseline,omitempty"`
	Engine   []AuditFinding `json:"engine,omitempty"`
}

// AuditFinding is one single-engine anomaly.
type AuditFinding struct {
	Check    string     `json:"check"`
	Selector string     `json:"selector"`
	Box      [4]float64 `json:"box"`
	Severity string     `json:"severity"` // report | info
}

// ResultPage identifies the page of a result.
type ResultPage struct {
	Slug string `json:"slug"`
	URL  string `json:"url"`
}

// ResultBaseline is the baseline engine of a result.
type ResultBaseline struct {
	Engine  string `json:"engine"`
	Version string `json:"version"` // "" when unknown
}

// ResultEngine is the engine compared with the baseline.
type ResultEngine struct {
	Engine  string  `json:"engine"`
	Family  Family  `json:"family"`
	Version string  `json:"version"` // "" when unknown
	Profile Profile `json:"profile"`
}

// Stats are a result's counts.
type Stats struct {
	MatchRate float64 `json:"matchRate"` // share of elements matched between the engines
	Report    int     `json:"report"`
	Info      int     `json:"info"`
	MS        int64   `json:"ms"`
}

// Shots are a result's two screenshots.
type Shots struct {
	Baseline ShotRef `json:"baseline"`
	Engine   ShotRef `json:"engine"`
}

// ShotRef is a screenshot in the bundle.
type ShotRef struct {
	Src string `json:"src"` // bundle-relative, / separated
	W   int    `json:"w"`
	H   int    `json:"h"`
}

// Sides holds one list per side of a comparison.
type Sides struct {
	Baseline []string `json:"baseline"`
	Engine   []string `json:"engine"`
}

// Tier is where a cause is shown.
type Tier string

// Tiers.
const (
	TierReport Tier = "report" // worth a developer's time
	TierInfo   Tier = "info"   // expected engine behaviour, explained
)

// Cause is where a difference starts, reported once with its instances.
type Cause struct {
	ID           string   `json:"id"`
	Tier         Tier     `json:"tier"`
	Title        string   `json:"title"`
	Kinds        []string `json:"kinds"`
	Consequences []string `json:"consequences"`
	Selector     string   `json:"selector"`
	Boxes        Boxes    `json:"boxes"`
	Delta        Delta    `json:"delta"`
	KB           *string  `json:"kb"`                  // null when no knowledge-base entry explains it
	KBDetail     *KBEntry `json:"kbDetail,omitempty"`  // the explanation and fix, when kb is set
	BlockedBy    []string `json:"blockedBy,omitempty"` // hosts whose resources the engine blocked
	Evidence     Evidence `json:"evidence"`
	Score        *float64 `json:"score"` // null without a model
	Count        int      `json:"count"`
	Instances    []string `json:"instances"`
}

// KBEntry is the knowledge-base explanation behind a cause (additive: the
// viewer and the PR comment show it without reading findings.json).
type KBEntry struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Explanation string `json:"explanation"`
	Fix         string `json:"fix"`
}

// Boxes are a cause's element on each side; nil on the side it is missing from.
type Boxes struct {
	Baseline *[4]float64 `json:"baseline"`
	Engine   *[4]float64 `json:"engine"`
}

// Delta is engine minus baseline: px, and lines for DLn.
type Delta struct {
	DW  float64 `json:"dw"`
	DH  float64 `json:"dh"`
	DX  float64 `json:"dx"`
	DY  float64 `json:"dy"`
	DLn float64 `json:"dln"`
}

// Consensus says which engine is the odd one out.
type Consensus string

// Consensus verdicts.
const (
	ConsensusBaselineOdd Consensus = "baseline-odd"
	ConsensusEngineOdd   Consensus = "engine-odd"
	ConsensusAllDiffer   Consensus = "all-differ"
)

// Evidence is a cause's compact evidence; nil where a concept had nothing to
// say. Intent nil (no intent evidence) differs from empty.
type Evidence struct {
	Mechanism   *string              `json:"mechanism"`
	Consensus   *Consensus           `json:"consensus"`
	Perception  *float64             `json:"perception"`
	Ink         *float64             `json:"ink"`
	Intent      []string             `json:"intent"`
	Calibration *CalibrationEvidence `json:"calibration,omitempty"`
}

// CalibrationEvidence says whether the page's own systematic-engine model
// explains a cause (Task 55).
type CalibrationEvidence struct {
	Explained bool                `json:"explained"`
	By        []CalibrationReason `json:"by"`
}

// CalibrationReason is one model explanation.
type CalibrationReason struct {
	Model    string  `json:"model"` // advance, pitch, controls
	Expected float64 `json:"expected,omitempty"`
	Tol      float64 `json:"tol,omitempty"`
	DW       float64 `json:"dw,omitempty"`
	DH       float64 `json:"dh,omitempty"`
}

// Side is one side of a comparison.
type Side string

// Sides of a comparison.
const (
	SideBaseline Side = "baseline"
	SideEngine   Side = "engine"
)

// IntentViolation is a design relation one side breaks while the other
// engines keep it.
type IntentViolation struct {
	Engine    Side     `json:"engine"` // the side that breaks it
	Selector  string   `json:"selector"`
	Relations []string `json:"relations"`
	PX        float64  `json:"px"`
	KeptBy    []string `json:"keptBy"`
}

// orEmpty returns s, or an empty slice when s is nil, so it encodes as [].
func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// MarshalJSON writes nil lists as [].
func (s Summary) MarshalJSON() ([]byte, error) {
	type plain Summary
	p := plain(s)
	p.Hosts, p.Skipped, p.Pages, p.Results = orEmpty(p.Hosts), orEmpty(p.Skipped), orEmpty(p.Pages), orEmpty(p.Results)
	return json.Marshal(p)
}

// MarshalJSON writes nil lists as [].
func (r Result) MarshalJSON() ([]byte, error) {
	type plain Result
	p := plain(r)
	p.Warnings, p.Causes, p.Intent = orEmpty(p.Warnings), orEmpty(p.Causes), orEmpty(p.Intent)
	return json.Marshal(p)
}

// MarshalJSON writes nil lists as [].
func (s Sides) MarshalJSON() ([]byte, error) {
	type plain Sides
	p := plain(s)
	p.Baseline, p.Engine = orEmpty(p.Baseline), orEmpty(p.Engine)
	return json.Marshal(p)
}

// MarshalJSON writes nil lists as [] (Evidence.Intent keeps its null).
func (c Cause) MarshalJSON() ([]byte, error) {
	type plain Cause
	p := plain(c)
	p.Kinds, p.Consequences, p.Instances = orEmpty(p.Kinds), orEmpty(p.Consequences), orEmpty(p.Instances)
	return json.Marshal(p)
}

// MarshalJSON writes nil lists as [].
func (v IntentViolation) MarshalJSON() ([]byte, error) {
	type plain IntentViolation
	p := plain(v)
	p.Relations, p.KeptBy = orEmpty(p.Relations), orEmpty(p.KeptBy)
	return json.Marshal(p)
}

// MarshalJSON writes nil lists as [].
func (h Host) MarshalJSON() ([]byte, error) {
	type plain Host
	p := plain(h)
	p.Engines = orEmpty(p.Engines)
	return json.Marshal(p)
}
