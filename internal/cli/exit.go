package cli

// Exit codes are public API for the wrappers and the CI Action (SPEC-cli.md,
// "Exit codes"). Findings never change the code: Omnisnap is report-only.
const (
	ExitOK        = 0 // completed, with or without findings
	ExitUsage     = 1 // bad flag, unknown command, invalid job file
	ExitNoBrowser = 2 // fewer than two engine families, or no baseline
	ExitAllFailed = 3 // every capture of every page failed
	ExitInternal  = 4 // a bug: the message asks for a report
)
