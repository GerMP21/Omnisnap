package schema

import (
	"encoding/json"
	"fmt"
)

// Capture is one engine's geometry of one page in one run
// (schema/capture.schema.json): assets/inject/extract.js's output plus what
// the capture pipeline adds to Meta. Field names are extract.js's, kept short
// because a large page has up to 20,000 nodes.
type Capture struct {
	Meta Meta `json:"meta"`
	Root Node `json:"root"`
}

// Meta describes the page and how it was captured.
type Meta struct {
	URL       string          `json:"url"`
	Title     string          `json:"title"`
	UA        string          `json:"ua"`
	VW        int             `json:"vw"`
	VH        int             `json:"vh"`
	CW        int             `json:"cw"` // viewport minus a classic scrollbar
	DPR       float64         `json:"dpr"`
	DocW      int             `json:"docW"`
	DocH      int             `json:"docH"`
	Nodes     int             `json:"nodes"`
	Truncated bool            `json:"truncated"` // the 20,000-node limit was reached
	Fonts     Fonts           `json:"fonts"`
	Post      Post            `json:"post"`
	Probes    ProbesMeta      `json:"probes"`
	Shot      Shot            `json:"shot"`
	ShotNote  string          `json:"shotNote,omitempty"`
	Stabilize Stabilize       `json:"stabilize"`
	Failed    []FailedRequest `json:"failed"`
	Blocked   int             `json:"blocked"`
	MS        int64           `json:"ms"`
	Engine    string          `json:"engine,omitempty"`
	Version   string          `json:"version,omitempty"`
	// Mutation is the lab's marker of what it injected (lab tooling only).
	Mutation *Mutation `json:"mutation,omitempty"`
}

// Mutation describes one injected bug (SPEC-ground-truth.md): the operator,
// the CSS rule it wrote, the element it hit and the class scope it applied to.
type Mutation struct {
	Op         string     `json:"op"`
	Rule       string     `json:"rule,omitempty"`
	Tag        string     `json:"tag,omitempty"`
	Scope      int        `json:"scope,omitempty"`
	Candidates int        `json:"candidates,omitempty"`
	Box        [4]float64 `json:"box,omitempty"`
	Seed       uint32     `json:"seed"`
	Attempts   int        `json:"attempts,omitempty"` // operator rotations tried before one moved the page
	Engine     string     `json:"engine,omitempty"`
}

// Fonts lists the font families the page loaded and failed to load.
type Fonts struct {
	Loaded []string `json:"loaded"`
	Failed []string `json:"failed"`
}

// Post is the page re-measured after the screenshot.
type Post struct {
	DocH  int `json:"docH"`
	Nodes int `json:"nodes"`
}

// ProbesMeta records which probes ran and how long they took.
type ProbesMeta struct {
	IDs []string `json:"ids"`
	MS  int64    `json:"ms"`
}

// Shot is the screenshot's size in pixels.
type Shot struct {
	W int `json:"w"`
	H int `json:"h"`
}

// Stabilize holds stabilize.js's statistics.
type Stabilize struct {
	MS            int64 `json:"ms"`
	Finished      int   `json:"finished"`
	Frozen        int   `json:"frozen"`
	TimersCleared int   `json:"timersCleared"`
	ScrolledTo    int   `json:"scrolledTo"`
	PendingImages int   `json:"pendingImages"`
}

// FailedRequest is a request that failed while the page loaded.
type FailedRequest struct {
	URL string `json:"url"`
	Err string `json:"err"`
}

// Node is one rendered element.
type Node struct {
	T    string `json:"t"`
	ID   string `json:"id,omitempty"`
	TID  string `json:"tid,omitempty"`
	Role string `json:"role,omitempty"`
	C    string `json:"c,omitempty"`
	// NT is the 1-based position among the parent's same-tag element
	// children in the DOM, display: none ones included. Set only when the tag
	// repeats; captures from before it have none (see layoutdiff.Selector).
	NT int `json:"nt,omitempty"`
	// SR marks a child of an open shadow root. document.querySelector cannot
	// reach into a shadow tree, so a selector names its host instead
	// (package selector).
	SR Flag   `json:"sr,omitempty"`
	N  string `json:"n,omitempty"`
	U  string `json:"u,omitempty"`
	// Text is set for elements with own text. Its zero values are meaningful
	// (ti 0, ln 0 with an empty lt), so they live in a struct that is present
	// or absent as a whole.
	*Text
	IA   Flag        `json:"ia,omitempty"`
	Mut  Flag        `json:"mut,omitempty"`
	MutP Flag        `json:"mutp,omitempty"`
	TR   Flag        `json:"tr,omitempty"`
	B    [4]float64  `json:"b"` // border box x, y, w, h in document px
	D    string      `json:"d"`
	P    string      `json:"p,omitempty"`
	Hid  Flag        `json:"hid,omitempty"`
	O    *[2]float64 `json:"o,omitempty"` // px of content beyond the box
	OF   string      `json:"of,omitempty"`
	K    []Node      `json:"k,omitempty"`
}

// Text is an element's own text and its typography.
type Text struct {
	TX string     `json:"tx"` // first 60 characters
	TH string     `json:"th"` // hash of the full text
	LN int        `json:"ln"`
	TW float64    `json:"tw"`
	TI *float64   `json:"ti,omitempty"`
	LT []float64  `json:"lt"`
	LH LineHeight `json:"lh"`
	FS float64    `json:"fs"`
	FF string     `json:"ff"`
	FW string     `json:"fw"`
	WS string     `json:"ws,omitempty"`
}

// Flag is a boolean written as 1 when set; unset flags are omitted.
type Flag bool

// MarshalJSON writes 1 or 0 (0 only when the field has no omitempty).
func (f Flag) MarshalJSON() ([]byte, error) {
	if f {
		return []byte("1"), nil
	}
	return []byte("0"), nil
}

// UnmarshalJSON accepts 1/0 and true/false.
func (f *Flag) UnmarshalJSON(b []byte) error {
	switch string(b) {
	case "1", "true":
		*f = true
	case "0", "false", "null":
		*f = false
	default:
		return fmt.Errorf("schema: flag: %s", b)
	}
	return nil
}

// LineHeight is a computed line-height: px, or "normal".
type LineHeight struct {
	Normal bool
	PX     float64
}

// MarshalJSON writes "normal" or the px number.
func (lh LineHeight) MarshalJSON() ([]byte, error) {
	if lh.Normal {
		return []byte(`"normal"`), nil
	}
	return json.Marshal(lh.PX)
}

// UnmarshalJSON accepts "normal" or a number.
func (lh *LineHeight) UnmarshalJSON(b []byte) error {
	if string(b) == `"normal"` {
		*lh = LineHeight{Normal: true}
		return nil
	}
	var px float64
	if err := json.Unmarshal(b, &px); err != nil {
		return fmt.Errorf("schema: line-height: %s", b)
	}
	*lh = LineHeight{PX: px}
	return nil
}
