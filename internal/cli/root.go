// Package cli implements the omnisnap command line. Main returns an exit code
// instead of calling os.Exit, so every command can be tested in-process.
package cli

import (
	"fmt"
	"io"
)

// Version is the release version, set at build time with
// -ldflags "-X github.com/GerMP21/Omnisnap/internal/cli.Version=v0.1.0".
var Version = "dev"

const usage = `usage: omnisnap <command> [arguments]

Commands:
  version    print the version
`

// Main runs the command named in args (args[0] is the program name) and
// returns the process exit code.
func Main(args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 {
		fmt.Fprint(stderr, usage)
		return ExitUsage
	}
	cmd, rest := args[1], args[2:]
	switch cmd {
	case "version":
		return runVersion(rest, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "omnisnap: unknown command %q\n\n%s", cmd, usage)
		return ExitUsage
	}
}

func runVersion(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		fmt.Fprintln(stderr, "omnisnap: version takes no arguments")
		return ExitUsage
	}
	fmt.Fprintf(stdout, "omnisnap %s\n", Version)
	return ExitOK
}
