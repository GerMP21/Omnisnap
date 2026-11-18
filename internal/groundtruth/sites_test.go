package groundtruth

import (
	"strings"
	"testing"
)

func TestSiteFilterInfrastructure(t *testing.T) {
	f := NewSiteFilter(nil)
	for _, d := range []string{"cdn.example.com", "api.foo.com", "static.site.io", "img.news.com", "googleapis.com", "gstatic.com", "doubleclick.net", "amazonaws.com", "login.example.com"} {
		if _, ok := f.Accept(d); ok {
			t.Errorf("%s accepted: it is infrastructure", d)
		}
	}
	if d, ok := f.Accept("example.com"); !ok || d != "example.com" {
		t.Errorf("example.com rejected")
	}
	if f.Rejected["infrastructure"] == 0 {
		t.Error("no infrastructure rejections counted")
	}
}

func TestSiteFilterAdultMultilingual(t *testing.T) {
	// Adult sites are fogged or geo-blocked and are not research material.
	for _, d := range []string{"pornhub.com", "xvideos.com", "onlyfans.com", "xhamster.com", "sex.com", "bokep.tv", "missav.com", "imgsrc.ru", "色情.com", "порно.рф", "sexo.com"} {
		f := NewSiteFilter(nil)
		if _, ok := f.Accept(d); ok {
			t.Errorf("%s accepted: adult content", d)
		}
	}
}

func TestSiteFilterGambling(t *testing.T) {
	for _, d := range []string{"bet365.com", "casino.example", "1xbet.com", "pokerstars.com", "lottery.com"} {
		f := NewSiteFilter(nil)
		if _, ok := f.Accept(d); ok {
			t.Errorf("%s accepted: gambling", d)
		}
	}
}

func TestSiteFilterBrandDedupe(t *testing.T) {
	f := NewSiteFilter(nil)
	if _, ok := f.Accept("google.com"); !ok {
		t.Fatal("google.com rejected")
	}
	for _, d := range []string{"google.de", "google.co.uk"} {
		if _, ok := f.Accept(d); ok {
			t.Errorf("%s accepted: same brand as google.com", d)
		}
	}
	if Brand("example.co.uk") != Brand("example.com") {
		t.Error("brand extraction differs for the same brand")
	}
	if _, ok := f.Accept("example.co.uk"); !ok {
		t.Error("a second brand was rejected")
	}
	// Known corpus hosts are skipped.
	g := NewSiteFilter([]string{"www.known.com"})
	if _, ok := g.Accept("known.com"); ok {
		t.Error("a corpus host was accepted")
	}
}

func TestSlug(t *testing.T) {
	for in, want := range map[string]string{
		"example.com": "t-example.com", "news.bbc.co.uk": "t-news.bbc.co.uk", "weird_host!.io": "t-weird-host-.io",
	} {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q; want %q", in, got, want)
		}
	}
}

func TestJudgeProbe(t *testing.T) {
	html := "<title>Example</title>" + strings.Repeat("<p>content</p>", 500)
	if r := JudgeProbe("example.com", "https://example.com/", "Example", html, 200, "text/html; charset=utf-8"); !r.OK || r.Title != "Example" {
		t.Errorf("a good homepage was rejected: %+v", r)
	}
	for name, r := range map[string]ProbeResult{
		"status":         JudgeProbe("e.com", "https://e.com/", "E", html, 403, "text/html"),
		"type":           JudgeProbe("e.com", "https://e.com/", "E", html, 200, "application/json"),
		"small":          JudgeProbe("e.com", "https://e.com/", "E", "<title>x</title>", 200, "text/html"),
		"no title":       JudgeProbe("e.com", "https://e.com/", "", html, 200, "text/html"),
		"challenge":      JudgeProbe("e.com", "https://e.com/", "E", "<title>Just a moment...</title>"+html, 200, "text/html"),
		"adult title":    JudgeProbe("e.com", "https://e.com/", "Hot porn", html, 200, "text/html"),
		"sign-in":        JudgeProbe("e.com", "https://e.com/login?next=/", "E", html, 200, "text/html"),
		"login host":     JudgeProbe("e.com", "https://accounts.e.com/", "E", html, 200, "text/html"),
		"other redirect": JudgeProbe("e.com", "https://elsewhere.net/", "E", html, 200, "text/html"),
	} {
		if r.OK {
			t.Errorf("%s accepted: %+v", name, r)
		}
		if r.Reason == "" {
			t.Errorf("%s: no reason", name)
		}
	}
	// A redirect inside the site is fine.
	if r := JudgeProbe("example.com", "https://www.example.com/home", "Example", html, 200, "text/html"); !r.OK {
		t.Errorf("same-site redirect rejected: %+v", r)
	}
}
