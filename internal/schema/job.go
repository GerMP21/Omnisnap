package schema

import (
	"encoding/json"
	"fmt"
	"io"
)

// Job is job.json (schema/job.schema.json): what to capture and compare.
// Optional fields are pointers or omitempty so "absent" (use the default)
// stays distinct from an explicit value; the defaults are the Default*
// constants, which a test checks against the schema's "default" keywords.
type Job struct {
	Schema     string    `json:"$schema,omitempty"`
	Pages      []PageRef `json:"pages"`
	Engines    []string  `json:"engines,omitempty"` // absent: every discovered engine
	Baseline   string    `json:"baseline,omitempty"`
	Viewport   *Viewport `json:"viewport,omitempty"`
	Runs       *int      `json:"runs,omitempty"`
	Probes     *bool     `json:"probes,omitempty"`
	FontParity *bool     `json:"fontParity,omitempty"`
	Neutral    *bool     `json:"neutral,omitempty"`
	Timeouts   *Timeouts `json:"timeouts,omitempty"`
	Output     string    `json:"output,omitempty"`
}

// PageRef is one page to capture.
type PageRef struct {
	URL  string `json:"url"`
	Slug string `json:"slug,omitempty"` // absent: a sanitized URL
}

// Viewport is the CSS viewport every engine is normalized to.
type Viewport struct {
	Width             int     `json:"width"`
	Height            int     `json:"height"`
	DeviceScaleFactor float64 `json:"deviceScaleFactor,omitempty"` // absent: 1
}

// Timeouts bound one navigation and one injected script; 0 means the default.
type Timeouts struct {
	NavigationMS int `json:"navigationMs,omitempty"`
	ScriptMS     int `json:"scriptMs,omitempty"`
}

// Job defaults (schema/job.schema.json "default" keywords).
const (
	DefaultBaseline     = "chrome"
	DefaultRuns         = 2 // run b masks dynamic content
	DefaultProbes       = true
	DefaultFontParity   = true
	DefaultNeutral      = false
	DefaultOutput       = "./omnisnap-out"
	DefaultNavigationMS = 30000
	DefaultScriptMS     = 60000
)

// DefaultViewport is the spike's viewport: every golden capture used it.
var DefaultViewport = Viewport{Width: 1280, Height: 800, DeviceScaleFactor: 1}

// DecodeJob reads a job strictly: an unknown field is an error, so a typo
// ("engine" for "engines") cannot be silently ignored. It checks the JSON
// shape only; validating values against the schema is the caller's job.
func DecodeJob(r io.Reader) (*Job, error) {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	var j Job
	if err := dec.Decode(&j); err != nil {
		return nil, fmt.Errorf("schema: job: %w", err)
	}
	if dec.More() {
		return nil, fmt.Errorf("schema: job: more than one JSON value")
	}
	return &j, nil
}
