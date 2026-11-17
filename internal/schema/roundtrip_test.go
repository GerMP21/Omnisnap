package schema

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func fixtures(t *testing.T, name string) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(fixtureDir, name, "valid", "*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no valid %s fixtures: %v", name, err)
	}
	return files
}

// sameJSON reports whether a and b encode the same JSON value (key order and
// whitespace aside).
func sameJSON(t *testing.T, a, b []byte) bool {
	t.Helper()
	var va, vb any
	if err := json.Unmarshal(a, &va); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &vb); err != nil {
		t.Fatal(err)
	}
	return reflect.DeepEqual(va, vb)
}

// The job is input: decoding is strict, and every field survives the round trip.
func TestJobRoundTrip(t *testing.T) {
	for _, f := range fixtures(t, "job") {
		t.Run(filepath.Base(f), func(t *testing.T) {
			in, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			job, err := DecodeJob(bytes.NewReader(in))
			if err != nil {
				t.Fatal(err)
			}
			out, err := json.Marshal(job)
			if err != nil {
				t.Fatal(err)
			}
			if !sameJSON(t, in, out) {
				t.Errorf("round trip changed the job:\n in: %s\nout: %s", in, out)
			}
			validate(t, "job", job)
		})
	}
}

func TestDecodeJobRejectsUnknownFields(t *testing.T) {
	_, err := DecodeJob(strings.NewReader(`{"pages":[{"url":"https://example.com/"}],"engine":["chrome"]}`))
	if err == nil || !strings.Contains(err.Error(), "engine") {
		t.Fatalf("err = %v, want an unknown-field error naming it", err)
	}
}

func TestJobDefaultsMatchSchema(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(schemaDir, "job.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var s struct {
		Properties map[string]struct {
			Default    any `json:"default"`
			Properties map[string]struct {
				Default any `json:"default"`
			} `json:"properties"`
		} `json:"properties"`
		Defs map[string]struct {
			Properties map[string]struct {
				Default any `json:"default"`
			} `json:"properties"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	vp := s.Defs["viewport"].Properties
	checks := []struct {
		name          string
		schema, goVal any
	}{
		{"baseline", s.Properties["baseline"].Default, DefaultBaseline},
		{"runs", s.Properties["runs"].Default, float64(DefaultRuns)},
		{"probes", s.Properties["probes"].Default, DefaultProbes},
		{"fontParity", s.Properties["fontParity"].Default, DefaultFontParity},
		{"neutral", s.Properties["neutral"].Default, DefaultNeutral},
		{"output", s.Properties["output"].Default, DefaultOutput},
		{"timeouts.navigationMs", s.Properties["timeouts"].Properties["navigationMs"].Default, float64(DefaultNavigationMS)},
		{"timeouts.scriptMs", s.Properties["timeouts"].Properties["scriptMs"].Default, float64(DefaultScriptMS)},
		{"viewport.width", vp["width"].Default, float64(DefaultViewport.Width)},
		{"viewport.height", vp["height"].Default, float64(DefaultViewport.Height)},
		{"viewport.deviceScaleFactor", vp["deviceScaleFactor"].Default, DefaultViewport.DeviceScaleFactor},
	}
	for _, c := range checks {
		if c.schema != c.goVal {
			t.Errorf("%s: schema default %v, Go default %v", c.name, c.schema, c.goVal)
		}
	}
}

// host.json is output: consumers ignore unknown fields, so the round trip is
// checked on the Go value (decode, encode, validate, decode, compare).
func TestHostRoundTrip(t *testing.T) {
	for _, f := range fixtures(t, "host") {
		t.Run(filepath.Base(f), func(t *testing.T) {
			in, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			var h Host
			if err := json.Unmarshal(in, &h); err != nil {
				t.Fatal(err)
			}
			out, err := json.Marshal(h)
			if err != nil {
				t.Fatal(err)
			}
			validate(t, "host", h)
			var again Host
			if err := json.Unmarshal(out, &again); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(h, again) {
				t.Errorf("round trip changed the host:\n%+v\n%+v", h, again)
			}
		})
	}
}

// An unknown engine version is "" and must stay in the JSON (SPEC-bundle-schema.md, "Code style").
func TestHostKeepsEmptyVersion(t *testing.T) {
	h := Host{Machine: Machine{OS: "linux", Arch: "amd64"}, Engines: []HostEngine{{ID: "webkit", Family: FamilyWebKit, Profile: ProfileFresh}}}
	b, err := json.Marshal(h)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(b, []byte(`"version":""`)) {
		t.Errorf("%s: want an explicit empty version", b)
	}
	validate(t, "host", h)
}

func TestProgressFixturesRoundTrip(t *testing.T) {
	for _, f := range fixtures(t, "progress") {
		t.Run(filepath.Base(f), func(t *testing.T) {
			in, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			ev, err := DecodeEvent(in)
			if strings.HasPrefix(filepath.Base(f), "unknown-") {
				if !errors.Is(err, ErrUnknownEvent) {
					t.Fatalf("err = %v, want ErrUnknownEvent so consumers can skip it", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			out, err := json.Marshal(ev)
			if err != nil {
				t.Fatal(err)
			}
			if !sameJSON(t, in, out) {
				t.Errorf("round trip changed the event:\n in: %s\nout: %s", in, out)
			}
		})
	}
}

// Zero counts and false flags are meaningful: none may be dropped by omitempty.
func TestConstructedEventsValidate(t *testing.T) {
	ts := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	zero := 0
	events := []any{
		NewJobEvent(ts, 1, []string{"chrome"}, "out"),
		NewEngineEvent(ts, "chrome", EngineDiscovered, "", ""),
		NewEngineEvent(ts, "webkit", EngineFailed, "", "no display"),
		NewPageStartEvent(ts, "example", "https://example.com/"),
		NewCaptureEvent(ts, "example", "chrome", "a", 0, &zero, ""),
		NewCaptureEvent(ts, "example", "chrome", "b", 30000, nil, "navigation timeout"),
		NewPageDoneEvent(ts, "example", StatusOK),
		NewDiffEvent(ts, "example", "firefox", 0, 0),
		NewDoneEvent(ts, "out/summary.json"),
		NewErrorEvent(ts, "no usable browsers"),
	}
	for _, ev := range events {
		validate(t, "progress", ev)
	}
}
