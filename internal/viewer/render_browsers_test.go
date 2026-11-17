//go:build browsers && unix

package viewer

import (
	"context"
	"encoding/json"
	"image"
	"image/png"
	"os"
	"path/filepath"
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
