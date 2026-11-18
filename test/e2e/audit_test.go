//go:build e2e

package e2e

import (
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GerMP21/Omnisnap/internal/domaudit"
)

// Task 35: each audit page fires exactly its check in every engine, at the
// right element, and the controls fire nothing.

// auditPages is each page's answer: the check it must fire, and how many
// findings (overlap reports both boxes).
var auditPages = map[string]struct {
	check string
	count int
}{
	"audit-horizontal-scroll":     {"page-horizontal-scroll", 1},
	"audit-clipped-text":          {"clipped-text", 1},
	"audit-truncated-text":        {"truncated-text", 1},
	"audit-overlap-interactive":   {"overlap-interactive", 2},
	"audit-offscreen-interactive": {"offscreen-interactive", 1},
	"audit-zero-size-interactive": {"zero-size-interactive", 1},
	"audit-protrudes-parent":      {"protrudes-parent", 1},
}

var auditControls = []string{"audit-control-clean", "audit-control-nested-link", "audit-control-tooltip"}

// plantedBase serves testdata/planted by slug. The audit pages are the Go
// port's own and are not in expectations.json (which mirrors the spike), so
// groundtruth's allow-list would not serve them.
func plantedBase(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs("../../testdata/planted")
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		slug := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/"), ".html")
		b, err := os.ReadFile(filepath.Join(dir, slug+".html"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(b)
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return "http://" + ln.Addr().String()
}

func TestPlantedAuditPages(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e capture suite")
	}
	base := plantedBase(t)
	engines := engines(t)
	if len(engines) == 0 {
		t.Skip("no engine installed")
	}
	out := t.TempDir()
	for slug, want := range auditPages {
		for _, e := range engines {
			slug, want, e := slug, want, e
			t.Run(slug+"/"+e.golden, func(t *testing.T) {
				c := capturePage(t, e, out, slug, base+"/"+slug, "a")
				findings := domaudit.Audit(c)
				byCheck := map[string][]domaudit.Finding{}
				for _, f := range findings {
					byCheck[f.Check] = append(byCheck[f.Check], f)
				}
				got := byCheck[want.check]
				if len(got) != want.count {
					t.Errorf("%s findings %+v; want %d of %s", slug, findings, want.count, want.check)
				}
				for check, fs := range byCheck {
					if check != want.check {
						t.Errorf("%s fired %s too: %+v", slug, check, fs)
					}
				}
				if want.check != "page-horizontal-scroll" {
					for _, f := range got {
						if !strings.Contains(f.Selector, "#target") && !strings.Contains(f.Selector, "#a") && !strings.Contains(f.Selector, "#b") {
							t.Errorf("%s: finding at %q; want the planted element", slug, f.Selector)
						}
					}
				}
			})
		}
	}
	for _, slug := range auditControls {
		for _, e := range engines {
			slug, e := slug, e
			t.Run(slug+"/"+e.golden, func(t *testing.T) {
				c := capturePage(t, e, out, slug, base+"/"+slug, "a")
				if findings := domaudit.Audit(c); len(findings) != 0 {
					t.Errorf("%s fired %+v; controls must be silent", slug, findings)
				}
			})
		}
	}
}
