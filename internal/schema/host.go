package schema

// Family is a browser engine family.
type Family string

// Engine families.
const (
	FamilyBlink  Family = "blink"
	FamilyGecko  Family = "gecko"
	FamilyWebKit Family = "webkit"
)

// Profile is how a browser's profile was prepared (SPEC-branded.md).
type Profile string

// Profile kinds.
const (
	ProfileFresh   Profile = "fresh"   // a new temporary profile
	ProfileWarm    Profile = "warm"    // a copy of a warmed template: blocking lists loaded
	ProfileNeutral Profile = "neutral" // features that block requests turned off
)

// Machine identifies a machine. It is also the per-result host of
// summary.json, so merge can combine bundles from several machines.
type Machine struct {
	OS   string `json:"os"` // GOOS: linux, darwin, windows
	Arch string `json:"arch"`
	ID   string `json:"id,omitempty"` // a label, e.g. runner-ubuntu
}

// Host is host.json (schema/host.schema.json).
type Host struct {
	Schema string `json:"$schema,omitempty"`
	Machine
	OSVersion string            `json:"osVersion,omitempty"`
	Fonts     map[string]string `json:"fonts,omitempty"` // generic family -> resolved family
	Engines   []HostEngine      `json:"engines"`
}

// HostEngine is one browser used on this machine.
type HostEngine struct {
	ID         string  `json:"id"`
	Family     Family  `json:"family"`
	Version    string  `json:"version"` // "" when it could not be read without launching
	Executable string  `json:"executable,omitempty"`
	Profile    Profile `json:"profile"`
}
