//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/GerMP21/Omnisnap/internal/capture"
	"github.com/GerMP21/Omnisnap/internal/drivers"
	"github.com/GerMP21/Omnisnap/internal/drivers/bidi"
	"github.com/GerMP21/Omnisnap/internal/drivers/cdp"
	"github.com/GerMP21/Omnisnap/internal/drivers/classic"
	"github.com/GerMP21/Omnisnap/internal/groundtruth"
	"github.com/GerMP21/Omnisnap/internal/process"
	"github.com/GerMP21/Omnisnap/internal/schema"
)

// engine is one installed family, mapped to the golden capture's name.
type engine struct {
	golden string // "chrome", "firefox", "webkit"
	id     string // the browser id in our captures
	start  func(t *testing.T) (drivers.Session, func())
}

func find(t *testing.T, env string, names ...string) string {
	t.Helper()
	if p := os.Getenv(env); p != "" {
		return p
	}
	for _, name := range names {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	return ""
}

// engines returns the families installed here, named as the goldens are.
func engines(t *testing.T) []engine {
	t.Helper()
	var out []engine
	if exe := find(t, "OMNISNAP_CHROME", "chromium", "google-chrome-stable", "google-chrome", "chromium-browser", "chrome"); exe != "" {
		out = append(out, engine{golden: "chrome", id: "chrome", start: func(t *testing.T) (drivers.Session, func()) {
			p, err := process.StartBlink(context.Background(), exe, "")
			if err != nil {
				t.Fatal(err)
			}
			c, err := cdp.Dial(context.Background(), p.WSURL)
			if err != nil {
				t.Fatal(err)
			}
			s, err := c.NewSession(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			return s, func() { _ = c.Close(); _ = p.Close(context.Background()) }
		}})
	}
	if exe := find(t, "OMNISNAP_FIREFOX", "firefox", "firefox-esr"); exe != "" {
		out = append(out, engine{golden: "firefox", id: "firefox", start: func(t *testing.T) (drivers.Session, func()) {
			p, err := process.StartGecko(context.Background(), exe, "")
			if err != nil {
				t.Fatal(err)
			}
			c, err := bidi.Dial(context.Background(), p.WSURL)
			if err != nil {
				t.Fatal(err)
			}
			s, err := c.NewSession(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			return s, func() { _ = c.Close(context.Background()); _ = p.Close(context.Background()) }
		}})
	}
	driver := find(t, "OMNISNAP_WEBKIT_DRIVER", "WebKitWebDriver")
	browser := os.Getenv("OMNISNAP_WEBKIT_BROWSER")
	if browser == "" {
		for _, p := range []string{
			"/usr/libexec/webkitgtk-6.0/MiniBrowser",
			"/usr/libexec/webkit2gtk-4.1/MiniBrowser",
			"/usr/lib/x86_64-linux-gnu/webkit2gtk-4.1/MiniBrowser",
		} {
			if _, err := os.Stat(p); err == nil {
				browser = p
				break
			}
		}
	}
	if driver != "" && browser != "" {
		out = append(out, engine{golden: "webkit", id: "webkit", start: func(t *testing.T) (drivers.Session, func()) {
			p, err := process.StartWebKit(context.Background(), process.WebKitOptions{Driver: driver, Browser: browser})
			if err != nil {
				t.Fatal(err)
			}
			c := classic.New(p.URL)
			c.Capabilities = p.Capabilities
			if err := c.Status(context.Background()); err != nil {
				t.Fatal(err)
			}
			s, err := c.NewSession(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			return s, func() { _ = p.Close(context.Background()) }
		}})
	}
	return out
}

// plantedSlugs lists the golden pages.
func plantedSlugs(t *testing.T) []string {
	t.Helper()
	dir := "../../testdata/golden/planted"
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out
}

func readGolden(t *testing.T, slug, engine string) schema.Capture {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("../../testdata/golden/planted", slug, engine+"-a.json"))
	if err != nil {
		t.Fatal(err)
	}
	var c schema.Capture
	if err := json.Unmarshal(b, &c); err != nil {
		t.Fatal(err)
	}
	return c
}

// maxDelta is Task 33's parity threshold: every box within 0.1 px.
const maxDelta = 0.1

// compareTrees walks the captures in step and reports the first differences.
func compareTrees(t *testing.T, slug string, got, want *schema.Node, path string) {
	t.Helper()
	where := path + "/" + got.T
	if got.ID != "" {
		where += "#" + got.ID
	}
	if got.T != want.T || got.ID != want.ID {
		t.Errorf("%s: %s (%s#%s); want %s#%s", slug, where, got.T, got.ID, want.T, want.ID)
		return
	}
	for i := range 4 {
		if d := math.Abs(got.B[i] - want.B[i]); d > maxDelta {
			t.Errorf("%s: %s box[%d] = %g; golden %g (delta %g)", slug, where, i, got.B[i], want.B[i], d)
		}
	}
	if len(got.K) != len(want.K) {
		t.Errorf("%s: %s has %d children; golden %d", slug, where, len(got.K), len(want.K))
		return
	}
	for i := range got.K {
		compareTrees(t, slug, &got.K[i], &want.K[i], where)
	}
}

// Task 33: the binary's captures of the planted suite match the spike's goldens
// box for box, in every engine installed here, and two runs of a static page
// are identical.
func TestGoldenGeometry(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e capture suite")
	}
	srv, err := groundtruth.Start("../../testdata/planted")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = srv.Close() }()
	engines := engines(t)
	if len(engines) == 0 {
		t.Skip("no engine installed")
	}
	out := t.TempDir()
	for _, slug := range plantedSlugs(t) {
		slug := slug
		t.Run(slug, func(t *testing.T) {
			for _, e := range engines {
				e := e
				t.Run(e.golden, func(t *testing.T) {
					got := capturePage(t, e, out, slug, srv.URL(slug), "a")
					golden := readGolden(t, slug, e.golden)
					compareTrees(t, slug+" "+e.golden, &got.Root, &golden.Root, "")
					if got.Meta.VW != 1280 || got.Meta.VH != 800 {
						t.Errorf("%s: viewport %dx%d; want 1280x800", slug, got.Meta.VW, got.Meta.VH)
					}
					// Run b is identical on a static page: the same geometry,
					// node for node. (Dynamic pages legitimately differ; the
					// spike's own a/b pairs show which.)
					b := capturePage(t, e, out, slug, srv.URL(slug), "b")
					compareTrees(t, slug+" run a vs b", &got.Root, &b.Root, "")
				})
			}
		})
	}
}

// capturePage captures one page in one engine and returns the parsed capture.
func capturePage(t *testing.T, e engine, out, slug, url, letter string) *schema.Capture {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	s, closeAll := e.start(t)
	defer closeAll()
	if _, err := capture.Page(ctx, s, capture.Options{Out: out, Slug: slug, URL: url, Engine: e.id, Run: letter}); err != nil {
		t.Fatalf("capture %s %s: %v", slug, letter, err)
	}
	_ = s.Close(context.WithoutCancel(ctx))
	b, err := os.ReadFile(filepath.Join(out, slug, e.id+"-"+letter+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var c schema.Capture
	if err := json.Unmarshal(b, &c); err != nil {
		t.Fatal(err)
	}
	return &c
}
