package groundtruth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/GerMP21/Omnisnap/assets/inject"
	"github.com/GerMP21/Omnisnap/internal/drivers"
	"github.com/GerMP21/Omnisnap/internal/fsutil"
	gtinject "github.com/GerMP21/Omnisnap/internal/groundtruth/inject"
	"github.com/GerMP21/Omnisnap/internal/schema"
)

// Mutation capture (Task 70, SPEC-ground-truth.md): inject ONE realistic
// cross-engine bug into a settled real page, in one engine only, measure, then
// revert. Several mutants share one page load (stabilize.js has frozen the
// timers, so the page state stays comparable). The operator list, the seed
// hash and the class-wide scope are the spike's.

// MutateOps are the operators of inject/mutate.js, in rotation order.
var MutateOps = []string{
	"width-shrink", "padding", "padding-left", "margin-top", "nudge", "hide",
	"invisible", "gap", "wrap-narrow", "font-up", "nowrap", "align-self",
	"overlap", "clip", "order", "letter-spacing", "line-height", "border",
	"ellipsis-clip",
}

// DefaultMutantsPerEngine is the spike's MUT_PER_ENGINE.
const DefaultMutantsPerEngine = 4

// Mutant is one scheduled injection.
type Mutant struct {
	Slug   string // <base>~<engine>~<i>
	Base   string // the clean page's slug
	Engine string
	Index  int
	Seed   uint32
	Ops    []string // rotated: the seeded operator first, the rest as fallbacks
}

// FNV32 is the 32-bit FNV-1a hash the spike seeds mutants with.
func FNV32(s string) uint32 {
	var h uint32 = 2166136261
	for i := range len(s) {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return h
}

// ScheduleMutants builds a page's mutants: one per index, with the operator
// list rotated so a seed starts at a different operator each time.
func ScheduleMutants(baseSlug, engine string, perEngine int) []Mutant {
	if perEngine <= 0 {
		perEngine = DefaultMutantsPerEngine
	}
	out := make([]Mutant, 0, perEngine)
	for i := range perEngine {
		seed := FNV32(fmt.Sprintf("%s|%s|%d", baseSlug, engine, i))
		start := int(seed) % len(MutateOps)
		ops := append(append([]string{}, MutateOps[start:]...), MutateOps[:start]...)
		out = append(out, Mutant{
			Slug: fmt.Sprintf("%s~%s~%d", baseSlug, engine, i), Base: baseSlug,
			Engine: engine, Index: i, Seed: seed, Ops: ops,
		})
	}
	return out
}

// Eligible says whether two engines rendered the same page state: mutating a
// page they disagree about would label state differences as injected bugs
// (the spike requires >= 50 nodes and node counts within 20%).
func Eligible(a, b *schema.Capture) bool {
	if a == nil || b == nil || a.Meta.Nodes < 50 {
		return false
	}
	hi := max(a.Meta.Nodes, b.Meta.Nodes)
	if hi == 0 {
		return false
	}
	return float64(absInt(a.Meta.Nodes-b.Meta.Nodes))/float64(hi) <= 0.2
}

// MutateOptions configure a mutation capture.
type MutateOptions struct {
	Out       string
	BaseSlug  string
	URL       string
	Engine    string
	PerEngine int
	Viewport  schema.Viewport
	Timeouts  schema.Timeouts
	Logf      func(format string, args ...any)

	// RequireEffective retries the operator rotation until the label sees a
	// change and fails a mutant that never moves the page
	// (SPEC-ground-truth-mutant-policy.md). Off by default: the operator is
	// ops[seed % len(ops)], so the same page and seed always inject the same
	// bug and the dataset is reproducible.
	RequireEffective bool
}

// MutantResult is one mutant's capture or failure.
type MutantResult struct {
	Mutant  Mutant
	Capture *schema.Capture
	Err     error
}

// BaselineOptions configure a clean measurement of a settled page.
type BaselineOptions struct {
	URL      string
	Engine   string
	Viewport schema.Viewport
	Timeouts schema.Timeouts
}

// CaptureBaseline settles and measures the page with no mutation. The lab uses
// it as the clean reference the labels compare against; CaptureMutants uses it
// as the oracle for "this operator moved nothing".
func CaptureBaseline(ctx context.Context, s drivers.Session, o BaselineOptions) (*schema.Capture, error) {
	vp := o.Viewport
	if vp.Width == 0 || vp.Height == 0 {
		vp = schema.DefaultViewport
	}
	if vp.DeviceScaleFactor == 0 {
		vp.DeviceScaleFactor = 1
	}
	navTimeout, scriptTimeout := timeouts(o.Timeouts)
	step := func(timeout time.Duration) (context.Context, context.CancelFunc) {
		return context.WithTimeout(ctx, timeout)
	}
	c, cancel := step(scriptTimeout)
	_, err := s.SetViewport(c, vp.Width, vp.Height, vp.DeviceScaleFactor)
	cancel()
	if err != nil {
		return nil, fmt.Errorf("groundtruth: baseline: viewport: %w", err)
	}
	c, cancel = step(navTimeout)
	err = s.Navigate(c, o.URL)
	cancel()
	if err != nil {
		return nil, fmt.Errorf("groundtruth: baseline: navigate: %w", err)
	}
	if s.Capabilities().NetworkIdle {
		c, cancel = step(8 * time.Second)
		_ = s.WaitNetworkIdle(c, 500*time.Millisecond, 8*time.Second)
		cancel()
	}
	c, cancel = step(scriptTimeout)
	_, err = s.Evaluate(c, inject.Stabilize)
	cancel()
	if err != nil {
		return nil, fmt.Errorf("groundtruth: baseline: stabilize: %w", err)
	}
	c, cancel = step(scriptTimeout)
	raw, err := s.Evaluate(c, inject.Extract)
	cancel()
	if err != nil {
		return nil, fmt.Errorf("groundtruth: baseline: extract: %w", err)
	}
	capture := &schema.Capture{}
	if err := json.Unmarshal([]byte(raw), capture); err != nil {
		return nil, fmt.Errorf("groundtruth: baseline: extract: %w", err)
	}
	capture.Meta.Engine = o.Engine
	return capture, nil
}

// CaptureMutants settles the page once, then applies, measures and reverts
// each scheduled mutant, writing <out>/<mutant slug>/<engine>-a.json.
func CaptureMutants(ctx context.Context, s drivers.Session, o MutateOptions) ([]MutantResult, error) {
	if o.Logf == nil {
		o.Logf = func(string, ...any) {}
	}
	// The clean state of this page load is the retry oracle: a mutant that
	// moves nothing in its neighbourhood would teach the model a false
	// negative, so the next operator in the rotation is tried instead.
	baseline, err := CaptureBaseline(ctx, s, BaselineOptions{
		URL: o.URL, Engine: o.Engine, Viewport: o.Viewport, Timeouts: o.Timeouts,
	})
	if err != nil {
		return nil, err
	}
	_, scriptTimeout := timeouts(o.Timeouts)
	c, cancel := context.WithTimeout(ctx, scriptTimeout)
	if _, err := s.Evaluate(c, gtinject.Mutate); err != nil {
		cancel()
		return nil, fmt.Errorf("groundtruth: mutate: install: %w", err)
	}
	cancel()

	mutants := ScheduleMutants(o.BaseSlug, o.Engine, o.PerEngine)
	results := make([]MutantResult, 0, len(mutants))
	for _, m := range mutants {
		if err := ctx.Err(); err != nil {
			return results, err
		}
		capture, mutation, err := captureMutant(ctx, s, m, baseline, scriptTimeout, o.RequireEffective)
		if err != nil {
			writeMutantError(o.Out, m, err)
			results = append(results, MutantResult{Mutant: m, Err: err})
			o.Logf("  %s: %v", m.Slug, err)
			continue
		}
		capture.Meta.Mutation = mutation
		if err := writeMutantCapture(o.Out, m, capture); err != nil {
			return results, err
		}
		results = append(results, MutantResult{Mutant: m, Capture: capture})
	}
	return results, nil
}

// captureMutant applies one mutant and leaves the page as it found it. In the
// default mode it applies exactly the scheduled operator and records the result
// even when nothing moved; with requireEffective it rotates through the
// operators until one moves the target's neighbourhood.
func captureMutant(ctx context.Context, s drivers.Session, m Mutant, baseline *schema.Capture, timeout time.Duration, requireEffective bool) (*schema.Capture, *schema.Mutation, error) {
	tries := 1
	if requireEffective {
		tries = len(m.Ops)
	}
	var last error
	for k := range tries {
		ops, _ := json.Marshal(rotateOps(m.Ops, k))
		c, cancel := context.WithTimeout(ctx, timeout)
		raw, err := s.Evaluate(c, fmt.Sprintf("window.__omnisnapMutate(%s, %d)", ops, m.Seed))
		cancel()
		if err != nil {
			return nil, nil, fmt.Errorf("mutation: apply: %w", err)
		}
		var res mutateResult
		if err := json.Unmarshal([]byte(raw), &res); err != nil {
			return nil, nil, fmt.Errorf("mutation: apply result: %w", err)
		}
		if !res.OK {
			return nil, nil, fmt.Errorf("mutation: no candidate for the %d operators", len(m.Ops))
		}
		c, cancel = context.WithTimeout(ctx, timeout)
		raw, err = s.Evaluate(c, inject.Extract)
		cancel()
		revert(ctx, s, timeout)
		if err != nil {
			return nil, nil, fmt.Errorf("mutation: extract: %w", err)
		}
		capture := &schema.Capture{}
		if err := json.Unmarshal([]byte(raw), capture); err != nil {
			return nil, nil, fmt.Errorf("mutation: extract: %w", err)
		}
		capture.Meta.Engine = m.Engine
		mutation := &schema.Mutation{
			Op: res.Op, Rule: res.Rule, Tag: res.Tag, Scope: res.Scope,
			Candidates: res.Cands, Box: res.Box, Seed: m.Seed, Engine: m.Engine,
		}
		if !requireEffective {
			return capture, mutation, nil
		}
		// Only the opt-in mode judges the mutation; the default mode records
		// it as it is and lets the dataset label it later.
		label, lerr := LabelMutant(baseline, capture, nil)
		if lerr == nil && label.Effective {
			mutation.Attempts = k + 1
			return capture, mutation, nil
		}
		last = fmt.Errorf("the %s mutant moved nothing near its target", res.Op)
	}
	return nil, nil, fmt.Errorf("mutation: no effective mutant in %d attempts (%v)", tries, last)
}

// mutateResult is what window.__omnisnapMutate returns.
type mutateResult struct {
	OK    bool       `json:"ok"`
	Op    string     `json:"op"`
	Rule  string     `json:"rule"`
	Tag   string     `json:"tag"`
	Scope int        `json:"scope"`
	Cands int        `json:"candidates"`
	Box   [4]float64 `json:"box"`
}

// rotateOps starts the operator list at k: a retry begins with the next
// operator instead of repeating the one that moved nothing.
func rotateOps(ops []string, k int) []string {
	if len(ops) == 0 {
		return ops
	}
	k %= len(ops)
	return append(append([]string{}, ops[k:]...), ops[:k]...)
}

func revert(ctx context.Context, s drivers.Session, timeout time.Duration) {
	c, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	_, _ = s.Evaluate(c, "window.__omnisnapMutateRevert()")
}

func writeMutantCapture(out string, m Mutant, c *schema.Capture) error {
	dir := filepath.Join(out, m.Slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("groundtruth: %w", err)
	}
	if err := fsutil.WriteAtomic(filepath.Join(dir, m.Engine+"-a.json"), func(w io.Writer) error {
		return json.NewEncoder(w).Encode(c)
	}); err != nil {
		return err
	}
	page := map[string]any{"slug": m.Slug, "base": m.Base, "url": c.Meta.URL, "group": "mut", "engine": m.Engine, "seed": m.Seed}
	return fsutil.WriteAtomic(filepath.Join(dir, "page.json"), func(w io.Writer) error {
		return json.NewEncoder(w).Encode(page)
	})
}

func writeMutantError(out string, m Mutant, err error) {
	dir := filepath.Join(out, m.Slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	_ = fsutil.WriteAtomic(filepath.Join(dir, m.Engine+"-a.error.txt"), func(w io.Writer) error {
		_, e := io.WriteString(w, err.Error()+"\n")
		return e
	})
}

func timeouts(t schema.Timeouts) (nav, script time.Duration) {
	nav, script = schema.DefaultNavigationMS*time.Millisecond, schema.DefaultScriptMS*time.Millisecond
	if t.NavigationMS > 0 {
		nav = time.Duration(t.NavigationMS) * time.Millisecond
	}
	if t.ScriptMS > 0 {
		script = time.Duration(t.ScriptMS) * time.Millisecond
	}
	return nav, script
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
