package groundtruth

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

const plantedDir = "../../testdata/planted"

func TestLoadPlantedMatchesTheSuite(t *testing.T) {
	pages, err := LoadPlanted(plantedDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 31 {
		t.Fatalf("got %d pages, want 31", len(pages))
	}
	// SPEC-ground-truth.md, "Planted suite": 24 planted (6 with intent),
	// 4 control, 1 native, 2 kb.
	kinds := map[Kind]int{}
	intent := 0
	for _, p := range pages {
		kinds[p.Kind]++
		if p.Intent {
			intent++
		}
	}
	want := map[Kind]int{KindPlanted: 24, KindControl: 4, KindNative: 1, KindKB: 2}
	for k, n := range want {
		if kinds[k] != n {
			t.Errorf("%d pages of kind %q, want %d (all: %v)", kinds[k], k, n, kinds)
		}
	}
	if intent != 6 {
		t.Errorf("%d intent pages, want 6", intent)
	}

	bySlug := map[string]Page{}
	for _, p := range pages {
		bySlug[p.Slug] = p
	}
	// An absent maxFindings and an explicit 0 must stay distinct.
	if p := bySlug["09-control-article"]; p.MaxFindings == nil || *p.MaxFindings != 0 {
		t.Errorf("09-control-article MaxFindings = %v, want explicit 0", p.MaxFindings)
	}
	if p := bySlug["30-control-cards"]; p.MaxFindings != nil {
		t.Errorf("30-control-cards MaxFindings = %v, want absent", *p.MaxFindings)
	}
	if p := bySlug["06-order"]; len(p.Expect) != 1 || len(p.Expect[0].AnyOf) != 3 {
		t.Errorf("06-order Expect = %+v, want one group of three ids", p.Expect)
	}
	if p := bySlug["14-kb-line-height"]; p.KB != "K1" {
		t.Errorf("14-kb-line-height KB = %q, want K1", p.KB)
	}
	if p := bySlug["08-forms"]; len(p.OnlyTags) != 5 {
		t.Errorf("08-forms OnlyTags = %v, want 5 tags", p.OnlyTags)
	}
}

func TestLoadPlantedRejectsUnknownFields(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "expectations.json"), `{"pages":[{"slug":"a","kind":"control","surprise":1}]}`)
	writeFile(t, filepath.Join(dir, "a.html"), "<!doctype html>")
	if _, err := LoadPlanted(dir); err == nil {
		t.Fatal("want an error for an unknown field: the spike's answer format changed")
	}
}

func TestLoadPlantedRequiresEveryPageFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "expectations.json"), `{"pages":[{"slug":"a","kind":"control"}]}`)
	if _, err := LoadPlanted(dir); err == nil {
		t.Fatal("want an error when a page's HTML is missing")
	}
}

func TestHandlerServesPlantedPages(t *testing.T) {
	h, err := NewHandler(plantedDir)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	want, err := os.ReadFile(filepath.Join(plantedDir, "01-card-width.html"))
	if err != nil {
		t.Fatal(err)
	}
	// The spike served both /<slug>.html and /<slug>.
	for _, path := range []string{"/01-card-width.html", "/01-card-width"} {
		status, ctype, body := get(t, srv.URL+path)
		if status != http.StatusOK || ctype != "text/html; charset=utf-8" || !bytes.Equal(body, want) {
			t.Errorf("GET %s = %d %q, %d bytes; want 200 text/html with the file's %d bytes", path, status, ctype, len(body), len(want))
		}
	}
	for _, path := range []string{"/", "/expectations.json", "/nope.html", "/..%2Fplanted_test.go"} {
		if status, _, _ := get(t, srv.URL+path); status != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", path, status)
		}
	}
}

func TestStartServesOnLoopback(t *testing.T) {
	srv, err := Start(plantedDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	u := srv.URL("04-missing")
	if want := srv.Base + "/04-missing.html"; u != want {
		t.Errorf("URL = %q, want %q", u, want)
	}
	if status, _, _ := get(t, u); status != http.StatusOK {
		t.Errorf("GET %s = %d, want 200", u, status)
	}
}

func get(t *testing.T, url string) (int, string, []byte) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, resp.Header.Get("Content-Type"), body
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
