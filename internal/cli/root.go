// Package cli implements the omnisnap command line. Main returns an exit code
// instead of calling os.Exit, so every command can be tested in-process.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/signal"
	"strings"
	"time"

	"github.com/GerMP21/Omnisnap/internal/process"
)

// Version is the release version, set at build time with
// -ldflags "-X github.com/GerMP21/Omnisnap/internal/cli.Version=v0.1.0".
var Version = "dev"

const usage = `usage: omnisnap <url> [<url>...] [flags]
       omnisnap run <job.json> [flags]
       omnisnap report <bundle-dir>
       omnisnap merge <bundle-dir>... -o <merged-dir> [--rename-hosts]
       omnisnap comment <merged-bundle> [--report-url U] [--run-url U]
       omnisnap browsers [--json]
       omnisnap check-fonts
       omnisnap warm <browser>
       omnisnap install firefox [--channel release|esr] [--version v]
       omnisnap uninstall firefox [--channel release|esr]
       omnisnap version

Capture each page in every installed engine and write a report.

Flags (for <url> and run; they override the job file):
  --engines a,b,c     engines to capture (default: every discovered one)
  --baseline id       baseline engine, a Blink browser (default chrome)
  --out dir           output directory (default ./omnisnap-out)
  --runs n            captures per engine and page (default 2)
  --viewport WxH      viewport (default 1280x800)
  --timeout d         navigation timeout (default 30s)
  --resume            skip pages already captured to the end
  --retry-failed      with --resume, retry pages marked unreachable
  --allow-small-pages do not treat a page under 50 elements as a bot wall
  --no-audit          skip the single-engine layout audit
  --neutral           fresh profiles, branded blocking disabled where possible
  --json              NDJSON progress on stdout, nothing else
  --quiet             print only the report path and errors
  --verbose           more detail on errors
`

// Main runs the command named in args (args[0] is the program name) and
// returns the process exit code. Ctrl-C cancels the run; deferred cleanup
// still closes every browser.
func Main(args []string, stdout, stderr io.Writer) int {
	ctx, stop := signal.NotifyContext(context.Background(), stopSignals()...)
	defer stop()
	return main0(ctx, args, stdout, stderr, defaultEnv())
}

func main0(ctx context.Context, args []string, stdout, stderr io.Writer, e env) (code int) {
	defer func() {
		if p := recover(); p != nil {
			fmt.Fprintf(stderr, "omnisnap: internal error: %v\nThis is a bug: please report it with the command you ran (omnisnap %s).\n", p, Version)
			code = ExitInternal
		}
	}()
	if len(args) < 2 {
		fmt.Fprint(stderr, usage)
		return ExitUsage
	}
	cmd, rest := args[1], args[2:]
	// Cleanup layer 3: recover from a previous SIGKILL or power loss before
	// launching anything (SPEC-process.md). Never fatal.
	if cmd != "version" && cmd != "help" && !strings.HasPrefix(cmd, "-") {
		process.SweepLeftovers()
	}
	switch cmd {
	case "version":
		return runVersion(rest, stdout, stderr)
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usage)
		return ExitOK
	case "browsers":
		return runBrowsers(ctx, newReporter(stdout, stderr, options{}), e, rest)
	case "check-fonts":
		return runCheckFonts(ctx, newReporter(stdout, stderr, options{}), e, rest)
	case "report":
		return runReport(ctx, newReporter(stdout, stderr, options{}), rest)
	case "merge":
		return runMerge(ctx, newReporter(stdout, stderr, options{}), rest)
	case "comment":
		return runComment(rest)
	case "warm":
		return runWarm(ctx, newReporter(stdout, stderr, options{}), e, rest)
	case "install":
		return runInstall(ctx, newReporter(stdout, stderr, options{}), e, rest)
	case "uninstall":
		return runUninstall(ctx, newReporter(stdout, stderr, options{}), e, rest)
	case "run":
		o, pos, err := parseArgs(rest)
		if err != nil {
			return usageError(stderr, err)
		}
		if len(pos) != 1 {
			return usageError(stderr, usageErr("run takes one job file"))
		}
		j, err := readJob(pos[0])
		if err != nil {
			return usageError(stderr, err)
		}
		if err := o.apply(j); err != nil {
			return usageError(stderr, err)
		}
		return runJob(ctx, j, o, newReporter(stdout, stderr, o), e)
	}
	if !strings.Contains(cmd, "://") && !strings.HasPrefix(cmd, "-") {
		fmt.Fprintf(stderr, "omnisnap: unknown command %q\n\n%s", cmd, usage)
		return ExitUsage
	}
	o, urls, err := parseArgs(args[1:])
	if err != nil {
		return usageError(stderr, err)
	}
	if len(urls) == 0 {
		return usageError(stderr, usageErr("no URL given"))
	}
	j := jobFromURLs(urls)
	if err := o.apply(j); err != nil {
		return usageError(stderr, err)
	}
	return runJob(ctx, j, o, newReporter(stdout, stderr, o), e)
}

func newReporter(stdout, stderr io.Writer, o options) *reporter {
	return &reporter{out: stdout, err: stderr, json: o.json, quiet: o.quiet, now: time.Now}
}

func usageError(stderr io.Writer, err error) int {
	msg := err.Error()
	if errors.Is(err, errUsage) {
		msg = strings.TrimPrefix(msg, errUsage.Error()+": ")
	}
	fmt.Fprintf(stderr, "omnisnap: %s\n(omnisnap --help for usage)\n", msg)
	return ExitUsage
}

func runVersion(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		fmt.Fprintln(stderr, "omnisnap: version takes no arguments")
		return ExitUsage
	}
	fmt.Fprintf(stdout, "omnisnap %s\n", Version)
	return ExitOK
}
