//go:build browsers && unix

package viewer

import (
	"context"
	"encoding/json"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GerMP21/Omnisnap/internal/discovery"
	"github.com/GerMP21/Omnisnap/internal/drivers/cdp"
	"github.com/GerMP21/Omnisnap/internal/process"
	"github.com/GerMP21/Omnisnap/internal/schema"
)

// collectErrors runs before any page script: uncaught errors and
// console.error calls end up in window.__errors.
const collectErrors = `window.__errors = [];
addEventListener('error', (e) => window.__errors.push(String(e.message)));
addEventListener('unhandledrejection', (e) => window.__errors.push(String(e.reason)));
const __ce = console.error; console.error = (...a) => { window.__errors.push(a.join(' ')); __ce(...a); };`

func writePNG(t *testing.T, path string, w, h int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := png.Encode(f, image.NewGray(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
}

func TestRenderInChrome(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	found, _ := discovery.Find(ctx, discovery.Options{})
	exe := ""
	for _, b := range found {
		if b.Family == schema.FamilyBlink {
			exe = b.Executable
			break
		}
	}
	if exe == "" {
		t.Skip("no Blink browser installed")
	}

	dir := t.TempDir()
	s := fixture(t)
	// Review finding 4: the schema accepts any URI as a result's page URL;
	// the viewer must never turn a javascript: URL into a link.
	s.Results[1].Page.URL = "javascript:window.__pwned=4"
	// Different heights: the panes must keep the same fraction, not the same px.
	writePNG(t, filepath.Join(dir, "example/chrome-a.png"), 1280, 2000)
	writePNG(t, filepath.Join(dir, "example/firefox-a.png"), 1280, 4000)
	writePNG(t, filepath.Join(dir, "example/safari-a.png"), 1280, 3000)
	if err := Write(dir, s); err != nil {
		t.Fatal(err)
	}

	proc, err := process.StartBlink(ctx, exe, "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = proc.Close(context.Background()) }()
	b, err := cdp.Dial(ctx, proc.WSURL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = b.Close() }()
	sess, err := b.NewSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sess.SetViewport(ctx, 1280, 800, 1); err != nil {
		t.Fatal(err)
	}
	if err := sess.AddPreloadScript(ctx, collectErrors); err != nil {
		t.Fatal(err)
	}
	if err := sess.Navigate(ctx, "file://"+filepath.Join(dir, "report.html")); err != nil {
		t.Fatal(err)
	}

	errs, err := sess.Evaluate(ctx, "JSON.stringify(window.__errors)")
	if err != nil || errs != "[]" {
		t.Errorf("console errors: %s %v", errs, err)
	}
	if pwned, _ := sess.Evaluate(ctx, "String(window.__pwned)"); pwned != "undefined" {
		t.Errorf("XSS probe ran: window.__pwned = %s", pwned)
	}
	if n, _ := sess.Evaluate(ctx, `document.querySelectorAll('a[href^="javascript" i]').length`); n != "0" {
		t.Errorf("%s javascript: links rendered", n)
	}
	if shown, _ := sess.Evaluate(ctx, `document.querySelector('#r1').textContent.includes('javascript:window.__pwned=4')`); shown != "true" {
		t.Errorf("the unsafe URL should still be shown, as text")
	}
	if n, _ := sess.Evaluate(ctx, "document.querySelectorAll('.result').length"); n != "3" {
		t.Errorf("%s result sections; want 3", n)
	}
	// Audit findings render as text, not markup.
	if hasAudit, _ := sess.Evaluate(ctx, `document.querySelector('#r0').textContent.includes('Audit findings')`); hasAudit != "true" {
		t.Errorf("audit findings not rendered for r0")
	}
	if hasCheck, _ := sess.Evaluate(ctx, `document.querySelector('#r0').textContent.includes('overlapping interactive elements')`); hasCheck != "true" {
		t.Errorf("audit check name not rendered for r0")
	}
	// Theme toggle works and is persisted.
	if _, err := sess.Evaluate(ctx, `document.getElementById('theme').click()`); err != nil {
		t.Fatal(err)
	}
	hasTheme, _ := sess.Evaluate(ctx, `document.documentElement.classList.contains('theme-dark') || document.documentElement.classList.contains('theme-light')`)
	if hasTheme != "true" {
		t.Errorf("theme toggle did not apply a theme class")
	}
	// Keyboard navigation moves focus between results.
	if _, err := sess.Evaluate(ctx, `document.querySelector('.result').focus()`); err != nil {
		t.Fatal(err)
	}
	if _, err := sess.Evaluate(ctx, `document.dispatchEvent(new KeyboardEvent('keydown', {key: 'j', bubbles: true}))`); err != nil {
		t.Fatal(err)
	}
	if focused, _ := sess.Evaluate(ctx, `document.activeElement && document.activeElement.id === 'r1'`); focused != "true" {
		t.Errorf("keyboard navigation did not focus r1; activeElement=%s", focused)
	}

	// Scroll the first result's left pane to 30%: the right one follows.
	raw, err := sess.Evaluate(ctx, `(async () => {
  const frames = () => new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r)));
  await Promise.all([...document.images].map((i) => i.decode().catch(() => {})));
  const [a, b] = document.querySelectorAll('#r0 .pane');
  const frac = (p) => p.scrollTop / (p.scrollHeight - p.clientHeight);
  a.scrollTop = 0.3 * (a.scrollHeight - a.clientHeight);
  await frames(); await frames();
  const first = [frac(a), frac(b)];
  b.scrollTop = 0.8 * (b.scrollHeight - b.clientHeight);
  await frames(); await frames();
  return JSON.stringify([...first, frac(a), frac(b)]);
})()`)
	if err != nil {
		t.Fatal(err)
	}
	var f [4]float64
	if err := json.Unmarshal([]byte(raw), &f); err != nil {
		t.Fatal(err)
	}
	if abs(f[0]-0.3) > 0.01 || abs(f[1]-f[0]) > 0.01 || abs(f[3]-0.8) > 0.01 || abs(f[2]-f[3]) > 0.01 {
		t.Errorf("fractions %v; want both panes at 0.3, then both at 0.8", f)
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// Task 60: clicking a cause scrolls both panes to it and outlines the box
// on each pane. The outline must sit within the rendered image bounds.
func TestCauseClickScrollsPanes(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	found, _ := discovery.Find(ctx, discovery.Options{})
	exe := ""
	for _, b := range found {
		if b.Family == schema.FamilyBlink {
			exe = b.Executable
			break
		}
	}
	if exe == "" {
		t.Skip("no Blink browser installed")
	}

	dir := t.TempDir()
	s := fixture(t)
	writePNG(t, filepath.Join(dir, "example/chrome-a.png"), 1280, 2000)
	writePNG(t, filepath.Join(dir, "example/firefox-a.png"), 1280, 4000)
	writePNG(t, filepath.Join(dir, "example/safari-a.png"), 1280, 3000)
	if err := Write(dir, s); err != nil {
		t.Fatal(err)
	}

	proc, err := process.StartBlink(ctx, exe, "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = proc.Close(context.Background()) }()
	b, err := cdp.Dial(ctx, proc.WSURL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = b.Close() }()
	sess, err := b.NewSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sess.SetViewport(ctx, 1280, 800, 1); err != nil {
		t.Fatal(err)
	}
	if err := sess.AddPreloadScript(ctx, collectErrors); err != nil {
		t.Fatal(err)
	}
	if err := sess.Navigate(ctx, "file://"+filepath.Join(dir, "report.html")); err != nil {
		t.Fatal(err)
	}

	raw, err := sess.Evaluate(ctx, `(async () => {
    await Promise.all([...document.images].map((i) => i.decode().catch(() => {})));
    const sec = document.getElementById('r0');
    const causeItems = sec.querySelectorAll('.cause-item');
    const boxes = sec.querySelectorAll('.box');
    if (causeItems.length !== 2) throw new Error('expected 2 cause items, got ' + causeItems.length);
    if (boxes.length !== 3) throw new Error('expected 3 boxes total, got ' + boxes.length);
    const cause = causeItems[0];
    const [a, b] = sec.querySelectorAll('.pane');
    const beforeA = a.scrollTop;
    const beforeB = b.scrollTop;
    cause.click();
    await new Promise((r) => setTimeout(r, 120));
    await new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r)));
    const activeBoxes = sec.querySelectorAll('.box-active');
    const boxBounds = [...activeBoxes].map((box) => {
      const rect = box.getBoundingClientRect();
      const img = box.closest('.shot').querySelector('img');
      const imgRect = img.getBoundingClientRect();
      return {
        left: rect.left - imgRect.left,
        top: rect.top - imgRect.top,
        right: rect.right - imgRect.left,
        bottom: rect.bottom - imgRect.top
      };
    });
    return JSON.stringify({
      before: [beforeA, beforeB],
      after: [a.scrollTop, b.scrollTop],
      activeBoxes: activeBoxes.length,
      boxBounds
    });
  })()`)
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Before      [2]float64 `json:"before"`
		After       [2]float64 `json:"after"`
		ActiveBoxes int        `json:"activeBoxes"`
		BoxBounds   []struct {
			// One tag per field: `json:"left,top,right,bottom"` named all four
			// "left", so none decoded and the bounds check passed vacuously.
			Left   float64 `json:"left"`
			Top    float64 `json:"top"`
			Right  float64 `json:"right"`
			Bottom float64 `json:"bottom"`
		} `json:"boxBounds"`
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatal(err)
	}
	if out.After[0] == out.Before[0] || out.After[1] == out.Before[1] {
		t.Errorf("pane scrollTop did not change: before=%v after=%v", out.Before, out.After)
	}
	if out.ActiveBoxes < 2 {
		t.Errorf("expected active boxes on both panes, got %d", out.ActiveBoxes)
	}
	for _, bb := range out.BoxBounds {
		if bb.Left < -1 || bb.Top < -1 || bb.Right < -1 || bb.Bottom < -1 {
			t.Errorf("active box outside image bounds: %+v", bb)
		}
		if bb.Right <= bb.Left || bb.Bottom <= bb.Top {
			t.Errorf("active box has no area (were the bounds decoded?): %+v", bb)
		}
	}
}

// Task 78: a merged report gets a host selector; filtering to one host hides
// the other hosts' sections, 'All hosts' restores them, and cause boxes still
// align on the panes.
func TestMergedReportHostFilterInChrome(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	found, _ := discovery.Find(ctx, discovery.Options{})
	exe := ""
	for _, b := range found {
		if b.Family == schema.FamilyBlink {
			exe = b.Executable
			break
		}
	}
	if exe == "" {
		t.Skip("no Blink browser installed")
	}

	dir := t.TempDir()
	s := mergedFixture(t)
	// Screenshots referenced by the merged fixture.
	for _, page := range []string{"example", "pricing"} {
		for _, shot := range []string{"chrome-a", "firefox-a", "edge-a", "safari-a"} {
			writePNG(t, filepath.Join(dir, page, shot+".png"), 1280, 1000)
		}
	}
	if err := Write(dir, s); err != nil {
		t.Fatal(err)
	}

	proc, err := process.StartBlink(ctx, exe, "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = proc.Close(context.Background()) }()
	b, err := cdp.Dial(ctx, proc.WSURL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = b.Close() }()
	sess, err := b.NewSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sess.SetViewport(ctx, 1280, 800, 1); err != nil {
		t.Fatal(err)
	}
	if err := sess.AddPreloadScript(ctx, collectErrors); err != nil {
		t.Fatal(err)
	}
	if err := sess.Navigate(ctx, "file://"+filepath.Join(dir, "report.html")); err != nil {
		t.Fatal(err)
	}

	errs, err := sess.Evaluate(ctx, "JSON.stringify(window.__errors)")
	if err != nil || errs != "[]" {
		t.Errorf("console errors: %s %v", errs, err)
	}

	raw, err := sess.Evaluate(ctx, `(() => {
    const select = document.getElementById('host-select');
    const selector = document.getElementById('host-selector');
    return JSON.stringify({
      selectorHidden: selector.hidden,
      selectExists: !!select,
      optionCount: select ? select.options.length : 0,
      initialSections: document.querySelectorAll('.result').length,
      initialHostGroups: document.querySelectorAll('.host-group').length
    });
  })()`)
	if err != nil {
		t.Fatal(err)
	}
	var initOut struct {
		SelectorHidden    bool `json:"selectorHidden"`
		SelectExists      bool `json:"selectExists"`
		OptionCount       int  `json:"optionCount"`
		InitialSections   int  `json:"initialSections"`
		InitialHostGroups int  `json:"initialHostGroups"`
	}
	if err := json.Unmarshal([]byte(raw), &initOut); err != nil {
		t.Fatal(err)
	}
	if initOut.SelectorHidden {
		t.Error("host selector should be visible for a merged bundle")
	}
	if !initOut.SelectExists {
		t.Fatal("host select not found")
	}
	if initOut.OptionCount != 4 { // "All hosts" + 3 hosts
		t.Errorf("select has %d options; want 4", initOut.OptionCount)
	}
	if initOut.InitialSections != len(s.Results) {
		t.Errorf("initial sections %d; want %d", initOut.InitialSections, len(s.Results))
	}
	if initOut.InitialHostGroups != 6 { // example has 3 hosts, pricing has 3 hosts = 6 host groups
		t.Errorf("initial host groups %d; want 6", initOut.InitialHostGroups)
	}

	// Filter to a single host and verify the others hide.
	raw, err = sess.Evaluate(ctx, `(() => {
    const select = document.getElementById('host-select');
    select.value = 'runner-ubuntu';
    select.dispatchEvent(new Event('change', { bubbles: true }));
    return JSON.stringify({
      hiddenHostGroups: [...document.querySelectorAll('.host-group')].filter((g) => g.hidden).length,
      visibleResults: [...document.querySelectorAll('.result')].filter((s) => s.closest('[hidden]') === null).length
    });
  })()`)
	if err != nil {
		t.Fatal(err)
	}
	var filterOut struct {
		HiddenHostGroups int `json:"hiddenHostGroups"`
		VisibleResults   int `json:"visibleResults"`
	}
	if err := json.Unmarshal([]byte(raw), &filterOut); err != nil {
		t.Fatal(err)
	}
	if filterOut.HiddenHostGroups != 4 { // 6 total - 2 ubuntu groups = 4 hidden
		t.Errorf("hidden host groups %d; want 4", filterOut.HiddenHostGroups)
	}
	if filterOut.VisibleResults != 2 { // ubuntu example + ubuntu pricing
		t.Errorf("visible results after filtering %d; want 2", filterOut.VisibleResults)
	}

	// Restore 'All hosts'.
	raw, err = sess.Evaluate(ctx, `(() => {
    const select = document.getElementById('host-select');
    select.value = '';
    select.dispatchEvent(new Event('change', { bubbles: true }));
    return JSON.stringify({
      restoredHostGroups: [...document.querySelectorAll('.host-group')].filter((g) => !g.hidden).length,
      restoredResults: [...document.querySelectorAll('.result')].filter((s) => s.closest('[hidden]') === null).length
    });
  })()`)
	if err != nil {
		t.Fatal(err)
	}
	var restoreOut struct {
		RestoredHostGroups int `json:"restoredHostGroups"`
		RestoredResults    int `json:"restoredResults"`
	}
	if err := json.Unmarshal([]byte(raw), &restoreOut); err != nil {
		t.Fatal(err)
	}
	if restoreOut.RestoredHostGroups != initOut.InitialHostGroups {
		t.Errorf("restored host groups %d; want %d", restoreOut.RestoredHostGroups, initOut.InitialHostGroups)
	}
	if restoreOut.RestoredResults != initOut.InitialSections {
		t.Errorf("restored results %d; want %d", restoreOut.RestoredResults, initOut.InitialSections)
	}

	// Click a cause and check active boxes are within image bounds.
	raw, err = sess.Evaluate(ctx, `(async () => {
    const sec = document.getElementById('r0');
    const cause = sec.querySelector('.cause-item');
    cause.click();
    await new Promise((r) => setTimeout(r, 120));
    await new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r)));
    const activeBoxes = sec.querySelectorAll('.box-active');
    const boxBounds = [...activeBoxes].map((box) => {
      const rect = box.getBoundingClientRect();
      const img = box.closest('.shot').querySelector('img');
      const imgRect = img.getBoundingClientRect();
      return {
        left: rect.left - imgRect.left,
        top: rect.top - imgRect.top,
        right: rect.right - imgRect.left,
        bottom: rect.bottom - imgRect.top
      };
    });
    return JSON.stringify({ activeBoxes: activeBoxes.length, boxBounds });
  })()`)
	if err != nil {
		t.Fatal(err)
	}
	var causeOut struct {
		ActiveBoxes int `json:"activeBoxes"`
		BoxBounds   []struct {
			// One tag per field: `json:"left,top,right,bottom"` named all four
			// "left", so none decoded and the bounds check passed vacuously.
			Left   float64 `json:"left"`
			Top    float64 `json:"top"`
			Right  float64 `json:"right"`
			Bottom float64 `json:"bottom"`
		} `json:"boxBounds"`
	}
	if err := json.Unmarshal([]byte(raw), &causeOut); err != nil {
		t.Fatal(err)
	}
	if causeOut.ActiveBoxes < 2 {
		t.Errorf("expected active boxes on both panes, got %d", causeOut.ActiveBoxes)
	}
	for _, bb := range causeOut.BoxBounds {
		if bb.Left < -1 || bb.Top < -1 || bb.Right < -1 || bb.Bottom < -1 {
			t.Errorf("active box outside image bounds: %+v", bb)
		}
		if bb.Right <= bb.Left || bb.Bottom <= bb.Top {
			t.Errorf("active box has no area (were the bounds decoded?): %+v", bb)
		}
	}
}

// Task 38: a 50-page report is interactive in under 2 s, with no console
// errors from the viewer's own code.
func TestFiftyPageReportInChrome(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	found, _ := discovery.Find(ctx, discovery.Options{})
	exe := ""
	for _, b := range found {
		if b.Family == schema.FamilyBlink {
			exe = b.Executable
			break
		}
	}
	if exe == "" {
		t.Skip("no Blink browser installed")
	}
	dir := t.TempDir()
	if err := Write(dir, largeSummary(50)); err != nil {
		t.Fatal(err)
	}
	proc, err := process.StartBlink(ctx, exe, "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = proc.Close(context.Background()) }()
	b, err := cdp.Dial(ctx, proc.WSURL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = b.Close() }()
	sess, err := b.NewSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = sess.Close(context.Background())
	}()
	if err := sess.AddPreloadScript(ctx, collectErrors); err != nil {
		t.Fatal(err)
	}
	page := "file://" + filepath.Join(dir, "report.html")
	start := time.Now()
	if err := sess.Navigate(ctx, page); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		ready, err := sess.Evaluate(ctx, `document.querySelectorAll('.result').length === 50 && !!document.getElementById('theme')`)
		if err != nil {
			t.Fatal(err)
		}
		if ready == "true" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("50 results not interactive 2 s after navigation")
		}
		time.Sleep(20 * time.Millisecond)
	}
	elapsed := time.Since(start)
	errs, err := sess.Evaluate(ctx, `JSON.stringify(window.__errors || ['no hook'])`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(errs) != "[null]" && strings.TrimSpace(errs) != "[]" {
		// window.__errors holds strings; null means the hook did not run.
		var list []string
		if json.Unmarshal([]byte(errs), &list) == nil && len(list) > 0 {
			t.Errorf("console errors on the 50-page report: %v", list)
		}
	}
	t.Logf("50-page report interactive in %s", elapsed)
}
