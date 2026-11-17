package schema

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// EventType is the "type" of a progress event (NDJSON on stdout with --json).
type EventType string

// Progress event types (schema/progress.schema.json).
const (
	EventJob       EventType = "job"
	EventEngine    EventType = "engine"
	EventPageStart EventType = "page-start"
	EventCapture   EventType = "capture"
	EventPageDone  EventType = "page-done"
	EventDiff      EventType = "diff"
	EventDone      EventType = "done"
	EventError     EventType = "error"
)

// ErrUnknownEvent is returned by DecodeEvent for a type this version does not
// know. Consumers must skip such events: new types are additive.
var ErrUnknownEvent = errors.New("schema: unknown progress event type")

// Header is common to every event.
type Header struct {
	TS   time.Time `json:"ts"`
	Type EventType `json:"type"`
}

// JobEvent is sent when the job starts.
type JobEvent struct {
	Header
	Pages   int      `json:"pages"`
	Engines []string `json:"engines"`
	Output  string   `json:"output"`
}

// EngineStatus is what happened to an engine.
type EngineStatus string

// Engine statuses.
const (
	EngineDiscovered EngineStatus = "discovered"
	EngineLaunched   EngineStatus = "launched"
	EngineFailed     EngineStatus = "failed"
)

// EngineEvent is sent when an engine is discovered, launched, or fails (Error set).
type EngineEvent struct {
	Header
	Engine  string       `json:"engine"`
	Status  EngineStatus `json:"status"`
	Version string       `json:"version,omitempty"`
	Error   string       `json:"error,omitempty"`
}

// PageStartEvent is sent when a page's captures start.
type PageStartEvent struct {
	Header
	Page string `json:"page"`
	URL  string `json:"url"`
}

// CaptureEvent is sent when one capture finishes. Nodes is set when OK, Error when not.
type CaptureEvent struct {
	Header
	Page   string `json:"page"`
	Engine string `json:"engine"`
	Run    string `json:"run"`
	OK     bool   `json:"ok"`
	MS     int64  `json:"ms"`
	Nodes  *int   `json:"nodes,omitempty"`
	Error  string `json:"error,omitempty"`
}

// PageStatus is a page result's status (also summary.json's "status").
type PageStatus string

// Page statuses.
const (
	StatusOK          PageStatus = "ok"
	StatusDiverged    PageStatus = "diverged"    // engines rendered different page states
	StatusFailed      PageStatus = "failed"      // a capture is missing
	StatusUnreachable PageStatus = "unreachable" // baseline failed or a bot wall
)

// PageDoneEvent is sent when a page finishes.
type PageDoneEvent struct {
	Header
	Page   string     `json:"page"`
	Status PageStatus `json:"status"`
}

// DiffEvent is sent when one engine of a page has been compared with the baseline.
type DiffEvent struct {
	Header
	Page   string `json:"page"`
	Engine string `json:"engine"`
	Report int    `json:"report"`
	Info   int    `json:"info"`
}

// DoneEvent is sent when the job finishes: the last event of a successful run.
type DoneEvent struct {
	Header
	Summary string `json:"summary"` // path of summary.json
}

// ErrorEvent is sent when a fatal error stops the run.
type ErrorEvent struct {
	Header
	Message string `json:"message"`
}

func header(ts time.Time, t EventType) Header { return Header{TS: ts.UTC(), Type: t} }

// NewJobEvent returns a job event.
func NewJobEvent(ts time.Time, pages int, engines []string, output string) JobEvent {
	return JobEvent{header(ts, EventJob), pages, engines, output}
}

// NewEngineEvent returns an engine event; errMsg is required for EngineFailed.
func NewEngineEvent(ts time.Time, engine string, status EngineStatus, version, errMsg string) EngineEvent {
	return EngineEvent{header(ts, EventEngine), engine, status, version, errMsg}
}

// NewPageStartEvent returns a page-start event.
func NewPageStartEvent(ts time.Time, page, url string) PageStartEvent {
	return PageStartEvent{header(ts, EventPageStart), page, url}
}

// NewCaptureEvent returns a capture event: OK when errMsg is empty, and then
// nodes must be set.
func NewCaptureEvent(ts time.Time, page, engine, run string, ms int64, nodes *int, errMsg string) CaptureEvent {
	return CaptureEvent{header(ts, EventCapture), page, engine, run, errMsg == "", ms, nodes, errMsg}
}

// NewPageDoneEvent returns a page-done event.
func NewPageDoneEvent(ts time.Time, page string, status PageStatus) PageDoneEvent {
	return PageDoneEvent{header(ts, EventPageDone), page, status}
}

// NewDiffEvent returns a diff event.
func NewDiffEvent(ts time.Time, page, engine string, report, info int) DiffEvent {
	return DiffEvent{header(ts, EventDiff), page, engine, report, info}
}

// NewDoneEvent returns the done event.
func NewDoneEvent(ts time.Time, summary string) DoneEvent {
	return DoneEvent{header(ts, EventDone), summary}
}

// NewErrorEvent returns a fatal error event.
func NewErrorEvent(ts time.Time, message string) ErrorEvent {
	return ErrorEvent{header(ts, EventError), message}
}

// DecodeEvent decodes one NDJSON line into its typed event, as a value
// (e.g. CaptureEvent). It returns ErrUnknownEvent for a type it does not know.
func DecodeEvent(line []byte) (any, error) {
	var h Header
	if err := json.Unmarshal(line, &h); err != nil {
		return nil, fmt.Errorf("schema: progress event: %w", err)
	}
	switch h.Type {
	case EventJob:
		return decodeAs[JobEvent](line)
	case EventEngine:
		return decodeAs[EngineEvent](line)
	case EventPageStart:
		return decodeAs[PageStartEvent](line)
	case EventCapture:
		return decodeAs[CaptureEvent](line)
	case EventPageDone:
		return decodeAs[PageDoneEvent](line)
	case EventDiff:
		return decodeAs[DiffEvent](line)
	case EventDone:
		return decodeAs[DoneEvent](line)
	case EventError:
		return decodeAs[ErrorEvent](line)
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnknownEvent, h.Type)
	}
}

func decodeAs[T any](line []byte) (any, error) {
	var ev T
	if err := json.Unmarshal(line, &ev); err != nil {
		return nil, fmt.Errorf("schema: progress event: %w", err)
	}
	return ev, nil
}
