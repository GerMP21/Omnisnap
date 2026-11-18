package cli

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/GerMP21/Omnisnap/internal/schema"
)

// errUsage marks errors that are the user's (exit 1).
var errUsage = errors.New("usage")

func usageErr(format string, a ...any) error {
	return fmt.Errorf("%w: %s", errUsage, fmt.Sprintf(format, a...))
}

// options are the flags of `omnisnap <url>...` and `omnisnap run`. Flags win
// over the job file.
type options struct {
	engines     string
	baseline    string
	out         string
	runs        int
	viewport    string
	timeout     time.Duration
	resume      bool
	retryFailed bool
	noAudit     bool
	neutral     bool
	json        bool
	quiet       bool
	verbose     bool
	set         map[string]bool // flags given on the command line
}

// parseArgs parses flags anywhere among the positional arguments
// (`omnisnap <url> --out x` and `omnisnap --out x <url>` alike).
func parseArgs(args []string) (options, []string, error) {
	var o options
	fs := flag.NewFlagSet("omnisnap", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&o.engines, "engines", "", "engines to capture, comma separated (default: every discovered one)")
	fs.StringVar(&o.baseline, "baseline", "", "baseline engine, a Blink browser (default chrome)")
	fs.StringVar(&o.out, "out", "", "output directory (default ./omnisnap-out)")
	fs.IntVar(&o.runs, "runs", 0, "captures per engine and page (default 2)")
	fs.StringVar(&o.viewport, "viewport", "", "viewport WIDTHxHEIGHT (default 1280x800)")
	fs.DurationVar(&o.timeout, "timeout", 0, "navigation timeout (default 30s)")
	fs.BoolVar(&o.resume, "resume", false, "skip pages already captured to the end")
	fs.BoolVar(&o.retryFailed, "retry-failed", false, "with --resume, retry pages marked unreachable")
	fs.BoolVar(&o.noAudit, "no-audit", false, "skip the single-engine layout audit")
	fs.BoolVar(&o.neutral, "neutral", false, "fresh profile with branded browsers' blocking disabled where possible")
	fs.BoolVar(&o.json, "json", false, "NDJSON progress on stdout, nothing else")
	fs.BoolVar(&o.quiet, "quiet", false, "print only the report path and errors")
	fs.BoolVar(&o.verbose, "verbose", false, "more detail on errors")
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return o, nil, usageErr("%v", err)
		}
		if fs.NArg() == 0 {
			break
		}
		pos = append(pos, fs.Arg(0))
		args = fs.Args()[1:]
	}
	o.set = map[string]bool{}
	fs.Visit(func(f *flag.Flag) { o.set[f.Name] = true })
	return o, pos, nil
}

// jobFromURLs builds the job of `omnisnap <url>...`.
func jobFromURLs(urls []string) *schema.Job {
	j := &schema.Job{}
	for _, u := range urls {
		j.Pages = append(j.Pages, schema.PageRef{URL: u})
	}
	return j
}

// readJob reads a job file strictly.
func readJob(path string) (*schema.Job, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, usageErr("%v", err)
	}
	j, err := schema.DecodeJob(bytes.NewReader(b))
	if err != nil {
		return nil, usageErr("%s: %v", path, err)
	}
	return j, nil
}

// apply overrides the job with the flags that were given.
func (o options) apply(j *schema.Job) error {
	if o.set["engines"] {
		j.Engines = nil
		for _, e := range strings.Split(o.engines, ",") {
			if e = strings.TrimSpace(e); e != "" {
				j.Engines = append(j.Engines, e)
			}
		}
	}
	if o.set["baseline"] {
		j.Baseline = o.baseline
	}
	if o.set["out"] {
		j.Output = o.out
	}
	if o.set["runs"] {
		j.Runs = &o.runs
	}
	if o.set["viewport"] {
		w, h, ok := strings.Cut(o.viewport, "x")
		width, err1 := strconv.Atoi(w)
		height, err2 := strconv.Atoi(h)
		if !ok || err1 != nil || err2 != nil {
			return usageErr("--viewport %q: want WIDTHxHEIGHT, e.g. 1280x800", o.viewport)
		}
		j.Viewport = &schema.Viewport{Width: width, Height: height, DeviceScaleFactor: 1}
	}
	if o.set["neutral"] {
		v := o.neutral
		j.Neutral = &v
	}
	if o.set["timeout"] {
		if j.Timeouts == nil {
			j.Timeouts = &schema.Timeouts{}
		}
		j.Timeouts.NavigationMS = int(o.timeout.Milliseconds())
	}
	return nil
}

var (
	slugRe   = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,99}$`)
	engineRe = regexp.MustCompile(`^[a-z][a-z0-9-]{0,39}$`)
	unsafeRe = regexp.MustCompile(`[^a-z0-9._-]+`)
)

// resolve fills the defaults and checks the job against the rules of
// schema/job.schema.json (the binary does not ship a schema validator).
func resolve(j *schema.Job) error {
	if len(j.Pages) == 0 {
		return usageErr("no pages: give a URL, or pages in the job file")
	}
	seen := map[string]bool{}
	for i := range j.Pages {
		p := &j.Pages[i]
		u, err := url.Parse(p.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return usageErr("%q: want an http(s) URL", p.URL)
		}
		if p.Slug == "" {
			p.Slug = slugFromURL(p.URL)
		}
		if !slugRe.MatchString(p.Slug) {
			return usageErr("slug %q: lowercase letters, digits, '.', '_' and '-' only", p.Slug)
		}
		base := p.Slug
		for n := 2; seen[p.Slug]; n++ {
			p.Slug = fmt.Sprintf("%s-%d", base, n) // two pages with the same slug would share a directory
		}
		seen[p.Slug] = true
	}
	dup := map[string]bool{}
	for _, e := range j.Engines {
		if !engineRe.MatchString(e) || dup[e] {
			return usageErr("engine %q: unknown form or listed twice", e)
		}
		dup[e] = true
	}
	if j.Baseline != "" && !engineRe.MatchString(j.Baseline) {
		return usageErr("baseline %q", j.Baseline)
	}
	if j.Runs == nil {
		r := schema.DefaultRuns
		j.Runs = &r
	}
	if *j.Runs < 1 || *j.Runs > 10 {
		return usageErr("runs %d: want 1 to 10", *j.Runs)
	}
	if j.Viewport == nil {
		vp := schema.DefaultViewport
		j.Viewport = &vp
	}
	if vp := j.Viewport; vp.Width < 200 || vp.Width > 2560 || vp.Height < 200 || vp.Height > 16000 || vp.DeviceScaleFactor < 0 || vp.DeviceScaleFactor > 4 {
		return usageErr("viewport %dx%d at %v: width 200-2560, height 200-16000, scale up to 4", vp.Width, vp.Height, vp.DeviceScaleFactor)
	}
	if j.Viewport.DeviceScaleFactor == 0 {
		j.Viewport.DeviceScaleFactor = 1
	}
	if j.Timeouts == nil {
		j.Timeouts = &schema.Timeouts{}
	}
	if j.Timeouts.NavigationMS < 0 || j.Timeouts.ScriptMS < 0 {
		return usageErr("timeouts must be positive")
	}
	if j.Timeouts.NavigationMS == 0 {
		j.Timeouts.NavigationMS = schema.DefaultNavigationMS
	}
	if j.Timeouts.ScriptMS == 0 {
		j.Timeouts.ScriptMS = schema.DefaultScriptMS
	}
	for _, b := range []**bool{&j.Probes, &j.FontParity, &j.Neutral} {
		if *b == nil {
			v := b == &j.Probes || b == &j.FontParity // probes and font parity default on, neutral off
			*b = &v
		}
	}
	if j.Output == "" {
		j.Output = schema.DefaultOutput
	}
	j.Schema = schema.ID("job")
	return nil
}

// slugFromURL is a page's directory name: host, port and path, lowercase.
func slugFromURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "page"
	}
	s := strings.ToLower(u.Host + u.Path)
	s = strings.Trim(unsafeRe.ReplaceAllString(strings.ReplaceAll(s, ":", "-"), "-"), "-.")
	if s == "" {
		return "page"
	}
	if len(s) > 100 {
		s = strings.TrimRight(s[:100], "-.")
	}
	return s
}
