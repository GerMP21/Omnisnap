module github.com/GerMP21/Omnisnap

go 1.27.4

require (
	github.com/coder/websocket v1.8.15
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.3
	golang.org/x/sys v0.50.0
)

require golang.org/x/text v0.14.0 // indirect

// v0.1.0-rc.1 was withdrawn: its tag was cut from history rewritten afterwards.
// v0.1.0-rc.3 was withdrawn: it was tagged before the report.yml fixes, so a
// consumer's workflow pinned to it fails; v0.1.0-rc.4 replaces it.
retract (
	v0.1.0-rc.1
	v0.1.0-rc.3
)
