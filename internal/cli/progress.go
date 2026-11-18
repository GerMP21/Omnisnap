package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/GerMP21/Omnisnap/internal/schema"
)

// reporter prints progress: NDJSON events (--json, schema/progress.schema.json)
// or one human line per event. With --json nothing else goes to stdout.
type reporter struct {
	out   io.Writer
	err   io.Writer
	json  bool
	quiet bool
	now   func() time.Time
}

// event reports ev (nil: a human line only).
func (r *reporter) event(ev any, human string) {
	if r.json {
		if ev == nil {
			return
		}
		b, _ := json.Marshal(ev) // event structs always marshal
		fmt.Fprintf(r.out, "%s\n", b)
		return
	}
	if !r.quiet && human != "" {
		fmt.Fprintln(r.out, human)
	}
}

// fatal reports an error that ends the run.
func (r *reporter) fatal(err error) {
	if r.json {
		r.event(schema.NewErrorEvent(r.now(), err.Error()), "")
	}
	fmt.Fprintln(r.err, "omnisnap:", err)
}
