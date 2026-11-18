package cli

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/GerMP21/Omnisnap/internal/install"
)

// runInstall is `omnisnap install firefox [--channel release|esr] [--version v]`
// (SPEC-install.md). It is the only command that downloads anything.
func runInstall(ctx context.Context, r *reporter, e env, args []string) int {
	channelName, version := "", ""
	var pos []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--channel":
			if i+1 >= len(args) {
				fmt.Fprintln(r.err, "omnisnap: --channel needs a value (release or esr)")
				return ExitUsage
			}
			i++
			channelName = args[i]
		case "--version":
			if i+1 >= len(args) {
				fmt.Fprintln(r.err, "omnisnap: --version needs a value")
				return ExitUsage
			}
			i++
			version = args[i]
		default:
			pos = append(pos, args[i])
		}
	}
	if len(pos) != 1 || strings.ToLower(pos[0]) != "firefox" {
		fmt.Fprintln(r.err, "omnisnap: install takes one browser: firefox")
		return ExitUsage
	}
	channel, err := install.ParseChannel(channelName)
	if err != nil {
		fmt.Fprintln(r.err, "omnisnap:", err)
		return ExitUsage
	}
	cache, err := cacheDir(e)
	if err != nil {
		r.fatal(err)
		return ExitInternal
	}
	progress := func(done, total int64) {
		if r.quiet || r.json {
			return
		}
		if total > 0 {
			fmt.Fprintf(r.out, "\r  %5.1f%%  %s / %s", 100*float64(done)/float64(total), human(done), human(total))
		} else {
			fmt.Fprintf(r.out, "\r  %s", human(done))
		}
	}
	res, err := install.Firefox(ctx, install.Options{
		Channel: channel, Version: version, Cache: cache,
		Client: &http.Client{},
		Log: func(format string, a ...any) {
			if !r.quiet {
				fmt.Fprintf(r.out, "omnisnap: "+format+"\n", a...)
			}
		},
		Progress: progress,
	})
	if !r.quiet && !r.json {
		fmt.Fprintln(r.out) // end the progress line
	}
	if err != nil {
		r.fatal(err)
		return ExitInternal
	}
	if res.Already {
		fmt.Fprintf(r.out, "Firefox %s is already installed: %s\n", res.Version, res.Executable)
		return ExitOK
	}
	fmt.Fprintf(r.out, "installed Firefox %s: %s\n", res.Version, res.Executable)
	fmt.Fprintln(r.out, "it is discovered as \"firefox\" (source omnisnap-cache); remove it with: omnisnap uninstall firefox")
	return ExitOK
}

// runUninstall is `omnisnap uninstall firefox [--channel ...]`.
func runUninstall(ctx context.Context, r *reporter, e env, args []string) int {
	channelName := ""
	var pos []string
	for i := 0; i < len(args); i++ {
		if args[i] == "--channel" && i+1 < len(args) {
			i++
			channelName = args[i]
			continue
		}
		pos = append(pos, args[i])
	}
	if len(pos) != 1 || strings.ToLower(pos[0]) != "firefox" {
		fmt.Fprintln(r.err, "omnisnap: uninstall takes one browser: firefox")
		return ExitUsage
	}
	if _, err := install.ParseChannel(channelName); err != nil {
		fmt.Fprintln(r.err, "omnisnap:", err)
		return ExitUsage
	}
	cache, err := cacheDir(e)
	if err != nil {
		r.fatal(err)
		return ExitInternal
	}
	installed := install.Installed(cache)
	if len(installed) == 0 {
		fmt.Fprintln(r.out, "no Firefox installed by omnisnap")
		return ExitOK
	}
	if err := install.Uninstall(cache); err != nil {
		r.fatal(err)
		return ExitInternal
	}
	fmt.Fprintf(r.out, "removed %d Firefox build(s) from %s\n", len(installed), cache)
	return ExitOK
}

// cacheDir resolves the injected cache (tests) or the real one.
func cacheDir(e env) (string, error) {
	if e.cache == nil {
		return install.CacheDir(nil)
	}
	return e.cache()
}

// human formats a byte count for the progress line.
func human(n int64) string {
	return install.HumanBytes(n)
}
