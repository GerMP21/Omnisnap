package groundtruth

import (
	"fmt"
	neturl "net/url"
	"regexp"
	"sort"
	"strings"
)

// Crawl list (Task 72, SPEC-ground-truth.md): walk the public Tranco ranking
// and keep sites a layout comparison can use. Content, infrastructure, brand
// and reachability filters are pure functions so they are tested offline; the
// probing itself is the lab's network job.

// Site is one accepted site.
type Site struct {
	Slug  string `json:"slug"`
	URL   string `json:"url"`
	Rank  int    `json:"rank"`
	Title string `json:"title,omitempty"`
}

// SiteList is the versioned output.
type SiteList struct {
	Generated string         `json:"generated"`
	Source    string         `json:"source"`
	Criteria  string         `json:"criteria"`
	Probed    int            `json:"probed"`
	Rejected  map[string]int `json:"rejected"`
	Sites     []Site         `json:"sites"`
}

var (
	infra      = regexp.MustCompile(`(^|[.-])(cdn|api|apis|static|assets|img|images|edge|dns|ntp|ocsp|crl|pki|ads?|adserver|pixel|metrics|telemetry|tracking|analytics|tags?|push|update|updates|gateway|mx|smtp|mail[0-9]*|login|auth|sso|accounts?)([.-]|$)`)
	infraHosts = regexp.MustCompile(`(googleapis|gstatic|googleusercontent|googlevideo|doubleclick|googlesyndication|googleadservices|google-analytics|googletagmanager|gvt[12]|1e100|akamai|akadns|akamaiedge|edgekey|edgesuite|cloudfront|amazonaws|awsdns|azureedge|azurefd|trafficmanager|msedge|msftncsi|windowsupdate|live\.net|office\.net|fastly|cloudflare-dns|apple-dns|icloud-content|aaplimg|mzstatic|cdninstagram|fbcdn|fbsbx|tiktokcdn|ttwstatic|byteoversea|ibytedtos|scorecardresearch|criteo|rubiconproject|pubmatic|adnxs|casalemedia|openx|taboola|outbrain|demdex|omtrdc|everesttech|bidswitch|digicert|letsencrypt|sectigo|root-servers|nic\.|gtld|ntp\.org|pool\.ntp|xboxlive|playstation\.net|nintendo\.net|spotifycdn|scdn|nflxso|nflxvideo|nflximg|steamstatic|steamcontent|whatsapp\.net|yandex\.net|vk-cdn|userapi|qq\.com\.cn|alicdn|tbcdn|kwai|appsflyer|adjust|branch\.io|onesignal|sentry|newrelic|nr-data|datadoghq|hotjar|mixpanel|segment|amplitude|optimizely|bugsnag)`)
	// Adult content in the languages the crawl meets: English, Japanese,
	// Chinese, Spanish and Russian.
	adultTerms    = regexp.MustCompile(`(porn|xxx|sex|adult|xvideo|xhamster|onlyfans|chaturbate|stripchat|livejasmin|bongacams|cam4|camsoda|nude|naked|hentai|escort|erotic|fetish|brazzers|redtube|youporn|spankbang|xnxx|tube8|beeg|eporner|rule34|nsfw|milf|boobs|hookup|dating18|bokep|jav[a-z]*hd|missav|sexo|porno|imgsrc|polybuzz|crushon|janitorai|エロ|アダルト|セックス|成人|色情|порно|секс|سكس)`)
	gamblingTerms = regexp.MustCompile(`(bet|casino|slot|poker|lottery|lotto|bookmaker|1xbet|stake\.|sportingbet|bwin|parimatch)`)
	challengePage = regexp.MustCompile(`(?i)cf-chl|challenge-platform|cf_chl_opt|captcha|just a moment|attention required|access denied|request unsuccessful|px-captcha|datadome|perimeterx|are you a robot|verify you are human|enable javascript and cookies`)
	signInPath    = regexp.MustCompile(`(?i)/(login|signin|sign-in|sign_in|auth|account/login|consent|sso)(/|\?|$)`)
)

// Brand is the site's brand: example.com and example.co.uk share one.
func Brand(domain string) string {
	parts := strings.Split(strings.ToLower(domain), ".")
	if len(parts) >= 3 && len(parts[len(parts)-2]) <= 3 {
		return parts[len(parts)-3]
	}
	if len(parts) >= 2 {
		return parts[len(parts)-2]
	}
	return domain
}

// Slug turns a domain into a bundle-safe slug: t-<domain with dashes>.
func Slug(domain string) string {
	d := strings.ToLower(domain)
	var b strings.Builder
	lastDash := false
	for _, r := range d {
		ok := r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '.'
		if ok {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return "t-" + strings.Trim(b.String(), "-.")
}

// SiteFilter is the state of the name filters: known hosts, brands already
// taken, and the rejection reasons counted.
type SiteFilter struct {
	Known    map[string]bool
	Brands   map[string]bool
	Rejected map[string]int
}

// NewSiteFilter builds the name filters; known is the existing corpus's host
// set (www. stripped).
func NewSiteFilter(known []string) *SiteFilter {
	f := &SiteFilter{Known: map[string]bool{}, Brands: map[string]bool{}, Rejected: map[string]int{}}
	for _, h := range known {
		f.Known[strings.TrimPrefix(strings.ToLower(h), "www.")] = true
	}
	return f
}

// Accept decides whether a domain enters the candidate list (before probing).
func (f *SiteFilter) Accept(domain string) (string, bool) {
	d := strings.ToLower(strings.TrimPrefix(domain, "www."))
	if infra.MatchString(d) || infraHosts.MatchString(d) {
		f.Rejected["infrastructure"]++
		return "", false
	}
	if adultTerms.MatchString(d) {
		f.Rejected["adult"]++
		return "", false
	}
	if gamblingTerms.MatchString(d) {
		f.Rejected["gambling"]++
		return "", false
	}
	if f.Known[d] {
		f.Rejected["known"]++
		return "", false
	}
	b := Brand(d)
	if f.Brands[b] {
		f.Rejected["brand"]++
		return "", false
	}
	f.Brands[b] = true
	return d, true
}

// ProbeResult is what a homepage probe decided.
type ProbeResult struct {
	OK     bool
	URL    string
	Title  string
	Reason string
}

// JudgeProbe applies the content filters to a fetched homepage (called after
// the HTTP fetch: status, type and size are the caller's checks).
func JudgeProbe(domain, finalURL, title, html string, status int, contentType string) ProbeResult {
	if status != 200 {
		return ProbeResult{Reason: fmt.Sprintf("http %d", status)}
	}
	if !strings.Contains(strings.ToLower(contentType), "text/html") {
		return ProbeResult{Reason: "not html"}
	}
	if len(html) < 5000 {
		return ProbeResult{Reason: "too small"}
	}
	if title == "" {
		return ProbeResult{Reason: "no title"}
	}
	head := html
	if len(head) > 60000 {
		head = head[:60000]
	}
	if challengePage.MatchString(head) {
		return ProbeResult{Reason: "bot challenge"}
	}
	adultHits := len(adultTerms.FindAllString(html, 6))
	if adultTerms.MatchString(title) || adultHits > 5 {
		return ProbeResult{Reason: "adult"}
	}
	if gamblingTerms.MatchString(title) {
		return ProbeResult{Reason: "gambling"}
	}
	finalHost := ""
	if u, err := urlHost(finalURL); err == nil {
		finalHost = u
	}
	if finalHost != "" {
		if signInPath.MatchString(finalURL) || loginHost.MatchString(finalHost) {
			return ProbeResult{Reason: "sign-in redirect"}
		}
		if Brand(finalHost) != Brand(domain) && !strings.HasSuffix(finalHost, domain) {
			return ProbeResult{Reason: "redirects to another site"}
		}
	}
	return ProbeResult{OK: true, URL: finalURL, Title: strings.TrimSpace(title)}
}

var loginHost = regexp.MustCompile(`^(login|accounts?|auth|signin)\.`)

// urlHost is a tiny helper so sites.go does not import net/url everywhere.
func urlHost(raw string) (string, error) {
	u, err := neturl.Parse(raw)
	if err != nil {
		return "", err
	}
	return u.Hostname(), nil
}

// SortedSites orders the accepted sites by rank.
func SortedSites(sites []Site) []Site {
	sort.SliceStable(sites, func(i, j int) bool { return sites[i].Rank < sites[j].Rank })
	return sites
}
