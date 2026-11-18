package groundtruth

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// ProbeSites fetches each candidate's homepage and keeps the ones that pass
// the content filters, up to n, filling the rejection reasons. Concurrent, but
// the output is rank-ordered.
func ProbeSites(ctx context.Context, candidates []Site, n, workers int, rejected map[string]int) (*SiteList, error) {
	if workers < 1 {
		workers = 1
	}
	if rejected == nil {
		rejected = map[string]int{}
	}
	list := &SiteList{
		Generated: time.Now().UTC().Format(time.RFC3339),
		Source:    "Tranco top-1M list (https://tranco-list.eu), downloaded for this crawl",
		Criteria:  "not infrastructure or adult content; one site per brand; homepage 200 HTML >= 5 KB with a title; no bot challenge or sign-in redirect",
		Rejected:  rejected,
	}
	client := &http.Client{Timeout: 20 * time.Second}
	var mu sync.Mutex
	var wg sync.WaitGroup
	next := 0
	stop := false
	probed := 0
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				mu.Lock()
				if stop || next >= len(candidates) {
					mu.Unlock()
					return
				}
				c := candidates[next]
				next++
				mu.Unlock()

				r := probeOne(ctx, client, c)
				mu.Lock()
				probed++
				if r.OK && len(list.Sites) < n {
					c.URL, c.Title = r.URL, r.Title
					list.Sites = append(list.Sites, c)
					if len(list.Sites) >= n {
						stop = true
					}
				} else if !r.OK {
					rejected[r.Reason]++
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	list.Probed = probed
	list.Sites = SortedSites(list.Sites)
	names := make([]string, 0, len(rejected))
	for k := range rejected {
		names = append(names, k)
	}
	sort.Strings(names)
	return list, nil
}

var titleRe = regexp.MustCompile(`(?i)<title[^>]*>([^<]{1,200})`)

// probeOne fetches a homepage; network errors are a rejection reason, not a
// crawl failure.
func probeOne(ctx context.Context, client *http.Client, c Site) ProbeResult {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.URL, nil)
	if err != nil {
		return ProbeResult{Reason: "network"}
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/155.0 Safari/537.36")
	req.Header.Set("Accept", "text/html")
	req.Header.Set("Accept-Language", "en-US,en;q=0.8")
	res, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ProbeResult{Reason: "timeout"}
		}
		return ProbeResult{Reason: "network"}
	}
	defer func() { _ = res.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(res.Body, 400<<10))
	if err != nil {
		return ProbeResult{Reason: "network"}
	}
	html := string(body)
	title := ""
	if m := titleRe.FindStringSubmatch(html); m != nil {
		title = strings.TrimSpace(m[1])
	}
	ct := res.Header.Get("Content-Type")
	if ct == "" {
		ct = "text/html"
	}
	r := JudgeProbe(strings.TrimPrefix(strings.TrimPrefix(c.URL, "https://"), "http://"), res.Request.URL.String(), title, html, res.StatusCode, ct)
	if r.OK {
		r.URL = res.Request.URL.String()
	}
	return r
}

var _ = fmt.Sprint
