package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/GerMP21/Omnisnap/internal/schema"
)

// GitHub's comment limit is 65,536 characters; leave room for the truncation
// note and link.
const commentLimit = 65000

// commentMarker identifies Omnisnap's comment, so the Action updates it in
// place instead of posting a second one.
const commentMarker = "<!-- omnisnap-report -->"

// runComment is `omnisnap comment <merged-bundle>` (Task 80): render the one
// PR comment from a merged summary.
func runComment(args []string) int {
	fs, reportURL, runURL, limit := commentFlags()
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, "omnisnap:", err)
		return ExitUsage
	}
	// `comment <dir> --report-url ...` is the order the Action uses, and
	// flag.Parse stops at the first non-flag: parse again with the directory
	// last so both orders work.
	if fs.NArg() != 1 && len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		fs, reportURL, runURL, limit = commentFlags()
		if err := fs.Parse(append(append([]string{}, args[1:]...), args[0])); err != nil {
			fmt.Fprintln(os.Stderr, "omnisnap:", err)
			return ExitUsage
		}
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "omnisnap: comment takes one merged bundle directory")
		return ExitUsage
	}
	dir := fs.Arg(0)
	b, err := os.ReadFile(filepath.Join(dir, "summary.json"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "omnisnap: comment:", err)
		return ExitUsage
	}
	var s schema.Summary
	if err := json.Unmarshal(b, &s); err != nil {
		fmt.Fprintln(os.Stderr, "omnisnap: comment: summary.json:", err)
		return ExitInternal
	}
	fmt.Print(RenderComment(s, CommentLinks{Report: *reportURL, Run: *runURL, Limit: *limit}))
	return ExitOK
}

// commentFlags builds the flag set and the values it fills.
func commentFlags() (*flag.FlagSet, *string, *string, *int) {
	fs := flag.NewFlagSet("comment", flag.ContinueOnError)
	return fs,
		fs.String("report-url", "", "link to the merged report artifact"),
		fs.String("run-url", "", "link to the workflow run"),
		fs.Int("limit", commentLimit, "markdown length limit")
}

// CommentLinks are the URLs the comment points at.
type CommentLinks struct {
	Report string
	Run    string
	Limit  int
}

// Comment layout. The comment is read by someone reviewing a pull request who
// has not seen Omnisnap's vocabulary, so it leads with a verdict, then one row
// per page, then each issue once (with every browser that shows it), and it
// says where on the page to look. The tiers, statuses and kinds of
// summary.json become plain sentences here; the full report keeps the raw
// evidence.
const (
	// maxIssuesPerPage is how many issues a page lists before pointing at the
	// report: past a handful, a comment stops being read.
	maxIssuesPerPage = 5
	// maxAlsoAffected is how many other matching elements an issue names.
	maxAlsoAffected = 3
)

// engineNames are the browser ids of discovery, as people write them.
var engineNames = map[string]string{
	"chrome":      "Chrome",
	"chrome-beta": "Chrome Beta",
	"chromium":    "Chromium",
	"edge":        "Edge",
	"brave":       "Brave",
	"opera":       "Opera",
	"vivaldi":     "Vivaldi",
	"firefox":     "Firefox",
	"firefox-esr": "Firefox ESR",
	"safari":      "Safari",
	"webkitgtk":   "WebKitGTK",
	"wpe":         "WPE WebKit",
}

// osNames are GOOS values, as people write them.
var osNames = map[string]string{
	"linux":   "Linux",
	"darwin":  "macOS",
	"windows": "Windows",
}

// consequenceText says what a visitor sees for each triage consequence
// (internal/triage/consequences.go). Unknown ones are left out rather than
// shown as jargon.
var consequenceText = map[string]string{
	"presence":          "an element is missing in one browser",
	"visibility":        "an element is hidden in one browser",
	"clipped":           "content is cut off",
	"scrollbar":         "an unexpected scrollbar appears",
	"overflow":          "content spills out of its container",
	"protrusion":        "content sticks out past its container",
	"truncation":        "text is truncated",
	"line-count":        "text wraps onto a different number of lines",
	"interactive-moved": "a button, link or form control moves",
	"structural":        "the surrounding layout shifts",
	"intent-broken":     "an alignment or spacing the design relies on breaks",
	"content-moved":     "text moves inside its box",
	"appearance":        "a graphic is drawn differently",
}

func engineName(id string) string {
	if n, ok := engineNames[id]; ok {
		return n
	}
	return id
}

// hostLabels names each host id by its operating system, adding the id only
// when two hosts run the same one (merge --rename-hosts).
func hostLabels(s schema.Summary) map[string]string {
	osOf := map[string]string{}
	for _, h := range s.Hosts {
		osOf[h.ID] = h.OS
	}
	for _, r := range s.Results {
		if _, ok := osOf[r.Host.ID]; !ok {
			osOf[r.Host.ID] = r.Host.OS
		}
	}
	perOS := map[string]int{}
	for _, o := range osOf {
		perOS[o]++
	}
	labels := map[string]string{}
	for id, o := range osOf {
		name, ok := osNames[o]
		if !ok {
			name = o
		}
		switch {
		case name == "":
			labels[id] = id
		case perOS[o] > 1 && id != "":
			labels[id] = name + " (" + id + ")"
		default:
			labels[id] = name
		}
	}
	return labels
}

// issue is one Report-tier cause on one page, gathered across the browsers
// and hosts that show it.
type issue struct {
	title, selector string
	rank            int      // best position in any result's ranked causes
	browsers        []string // the browsers that show it
	deltas          []string // how the element differs, per browser ("" when not measured)
	baseline        string
	why, fix        string
	consequences    []string
	others          []string // further elements with the same cause
	top             *float64 // document y of the element, when known
}

// pageView is what the comment says about one page.
type pageView struct {
	slug, url  string
	compared   []string // browsers compared on this page
	notChecked int
	issues     []*issue
	info       int
	diverged   []string // browsers that loaded a different page state
}

// RenderComment renders the merged summary as one markdown comment.
func RenderComment(s schema.Summary, l CommentLinks) string {
	limit := l.Limit
	if limit <= 0 {
		limit = commentLimit
	}
	hosts := hostLabels(s)
	browser := func(engine, hostID string) string {
		if h := hosts[hostID]; h != "" {
			return engineName(engine) + " on " + h
		}
		return engineName(engine)
	}

	pages, baselines, browsers := gatherPages(s, browser)
	var notChecked [][3]string // browser, page, reason
	for _, sk := range s.Skipped {
		b := engineName(sk.Engine)
		if sk.Host != "" {
			b = browser(sk.Engine, sk.Host)
		}
		notChecked = append(notChecked, [3]string{b, "all pages", "skipped: " + sk.Reason})
	}
	for _, r := range s.Results {
		switch r.Status {
		case schema.StatusFailed:
			notChecked = append(notChecked, [3]string{browser(r.Engine.Engine, r.Host.ID), r.Page.Slug,
				"capture failed"})
		case schema.StatusUnreachable:
			notChecked = append(notChecked, [3]string{browser(r.Engine.Engine, r.Host.ID), r.Page.Slug,
				"page could not be loaded (unreachable, or a bot check)"})
		}
	}

	var b strings.Builder
	b.WriteString(commentMarker + "\n")
	b.WriteString("## Omnisnap: cross-browser layout check\n\n")
	writeVerdict(&b, pages, baselines, browsers, len(notChecked))

	if len(pages) > 0 {
		b.WriteString("| Page | Result |\n|---|---|\n")
		for _, p := range pages {
			fmt.Fprintf(&b, "| %s | %s |\n", pageLink(p), pageResult(p))
		}
		b.WriteString("\n")
	}

	for _, p := range pages {
		if len(p.issues) == 0 && len(p.diverged) == 0 {
			continue
		}
		writePage(&b, p, l.Report)
	}

	if len(notChecked) > 0 {
		b.WriteString("### Not checked\n\n")
		b.WriteString("These browser and page pairs have no result, so the report says nothing about them.\n\n")
		b.WriteString("| Browser | Page | Why |\n|---|---|---|\n")
		for _, n := range notChecked {
			fmt.Fprintf(&b, "| %s | %s | %s |\n", n[0], n[1], strings.ReplaceAll(n[2], "|", "\\|"))
		}
		b.WriteString("\n")
	}

	// The links and the footer are kept whole when the body is truncated, so
	// the comment always points at the full report.
	var f strings.Builder
	var links []string
	if l.Report != "" {
		links = append(links, "**[Open the full report]("+l.Report+")** (screenshots and every difference)")
	}
	if l.Run != "" {
		links = append(links, "[workflow run]("+l.Run+")")
	}
	if len(links) > 0 {
		f.WriteString("---\n\n" + strings.Join(links, " · ") + "\n\n")
	}
	f.WriteString("<sub>Report only: Omnisnap never fails a build. Generated by omnisnap " + s.Tool.Version + ".</sub>\n")

	body, footer := b.String(), f.String()
	if len(body)+len(footer) > limit {
		// GitHub caps comments at 65,536 characters: cut the body at a line
		// boundary and point at the report.
		note := "\n_… comment truncated: the full report" + linkSuffix(l.Report) + " lists every issue._\n\n"
		keep := limit - len(footer) - len(note)
		if keep < len(commentMarker)+1 {
			keep = len(commentMarker) + 1
		}
		cut := body[:min(keep, len(body))]
		if i := strings.LastIndexByte(cut, '\n'); i > 0 {
			cut = cut[:i]
		}
		body = cut + "\n" + note
	}
	return body + footer
}

// gatherPages groups the results by page, in the job's page order, and folds
// the same cause seen in several browsers into one issue. It also returns the
// baseline engines and every browser compared.
func gatherPages(s schema.Summary, browser func(engine, host string) string) ([]*pageView, []string, []string) {
	byslug := map[string]*pageView{}
	var pages []*pageView
	add := func(slug, url string) *pageView {
		if p, ok := byslug[slug]; ok {
			return p
		}
		p := &pageView{slug: slug, url: url}
		byslug[slug] = p
		pages = append(pages, p)
		return p
	}
	for _, pt := range s.Pages {
		add(pt.Slug, pt.URL)
	}
	// Results in a stable order: host, then engine.
	results := append([]schema.Result{}, s.Results...)
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Host.ID != results[j].Host.ID {
			return results[i].Host.ID < results[j].Host.ID
		}
		return results[i].Engine.Engine < results[j].Engine.Engine
	})

	baselineSeen, browserSeen := map[string]bool{}, map[string]bool{}
	var baselines, browsers []string
	issueKey := map[string]*issue{}
	for _, r := range results {
		p := add(r.Page.Slug, r.Page.URL)
		if r.Status == schema.StatusFailed || r.Status == schema.StatusUnreachable {
			p.notChecked++
			continue
		}
		name := browser(r.Engine.Engine, r.Host.ID)
		p.compared = append(p.compared, name)
		if !browserSeen[name] {
			browserSeen[name] = true
			browsers = append(browsers, name)
		}
		if be := engineName(r.Baseline.Engine); be != "" && !baselineSeen[be] {
			baselineSeen[be] = true
			baselines = append(baselines, be)
		}
		if r.Status == schema.StatusDiverged {
			p.diverged = append(p.diverged, name)
		}
		p.info += r.Stats.Info
		rank := 0
		for _, c := range r.Causes {
			if c.Tier != schema.TierReport {
				continue
			}
			key := r.Page.Slug + "\x00" + c.Title + "\x00" + c.Selector
			is, ok := issueKey[key]
			if !ok {
				is = &issue{title: c.Title, selector: c.Selector, rank: rank}
				issueKey[key] = is
				p.issues = append(p.issues, is)
			}
			is.rank = min(is.rank, rank)
			rank++
			if !contains(is.browsers, name) {
				is.browsers = append(is.browsers, name)
				is.deltas = append(is.deltas, describeDelta(c.Delta))
			}
			if is.baseline == "" {
				is.baseline = engineName(r.Baseline.Engine)
			}
			if c.KBDetail != nil && is.why == "" {
				is.why, is.fix = c.KBDetail.Explanation, c.KBDetail.Fix
			}
			for _, k := range c.Consequences {
				if t, ok := consequenceText[k]; ok && !contains(is.consequences, t) {
					is.consequences = append(is.consequences, t)
				}
			}
			for _, in := range c.Instances {
				if in != c.Selector && !contains(is.others, in) {
					is.others = append(is.others, in)
				}
			}
			if is.top == nil {
				if bx := c.Boxes.Baseline; bx != nil {
					y := bx[1]
					is.top = &y
				} else if bx := c.Boxes.Engine; bx != nil {
					y := bx[1]
					is.top = &y
				}
			}
		}
	}
	for _, p := range pages {
		// Strongest first: the best rank any browser gave it, then the issue
		// more browsers show.
		sort.SliceStable(p.issues, func(i, j int) bool {
			if p.issues[i].rank != p.issues[j].rank {
				return p.issues[i].rank < p.issues[j].rank
			}
			return len(p.issues[i].browsers) > len(p.issues[j].browsers)
		})
	}
	return pages, baselines, browsers
}

// writeVerdict is the opening callout: the answer before the details.
func writeVerdict(b *strings.Builder, pages []*pageView, baselines, browsers []string, notChecked int) {
	issues, withIssues, checked := 0, 0, 0
	for _, p := range pages {
		issues += len(p.issues)
		if len(p.issues) > 0 {
			withIssues++
		}
		if len(p.compared) > 0 {
			checked++
		}
	}
	against := "its baseline browser"
	if len(baselines) == 1 {
		against = baselines[0]
	}
	how := fmt.Sprintf("Every browser was compared with %s on the same machine", against)
	switch {
	case checked == 0:
		b.WriteString("> [!CAUTION]\n> **Nothing was compared.** Every capture failed or no browser was available on the runners; ")
		b.WriteString("[Not checked](#not-checked) says why.\n\n")
		return
	case issues == 0:
		fmt.Fprintf(b, "> [!TIP]\n> **No layout issues found** on %s in %s.\n> %s.",
			plural(checked, "page"), plural(len(browsers), "browser"), how)
	default:
		fmt.Fprintf(b, "> [!WARNING]\n> **%s on %d of %s checked.**\n> %s",
			plural(issues, "layout issue"), withIssues, plural(checked, "page"), how)
		b.WriteString("; each issue below is a difference a visitor would notice.")
	}
	if notChecked > 0 {
		fmt.Fprintf(b, "\n> Some browsers or pages could not be checked: see [Not checked](#not-checked).")
	}
	b.WriteString("\n\n")
}

// pageLink is a page's name, linked to its URL.
func pageLink(p *pageView) string {
	if p.url == "" {
		return "`" + p.slug + "`"
	}
	return "[" + p.slug + "](" + p.url + ")"
}

// pageResult is a page's cell in the overview table.
func pageResult(p *pageView) string {
	var out string
	switch {
	case len(p.compared) == 0:
		return "❌ not checked"
	case len(p.issues) > 0:
		var where []string
		for _, is := range p.issues {
			for _, name := range is.browsers {
				if !contains(where, name) {
					where = append(where, name)
				}
			}
		}
		out = fmt.Sprintf("⚠️ %s in %s", plural(len(p.issues), "issue"), strings.Join(where, ", "))
	default:
		out = fmt.Sprintf("✅ no issues in %s", plural(len(p.compared), "browser"))
	}
	if len(p.diverged) > 0 {
		out += " · results may be unreliable"
	}
	if p.notChecked > 0 {
		out += fmt.Sprintf(" · %s not checked", plural(p.notChecked, "browser"))
	}
	return out
}

// writePage writes one page's issues, strongest first.
func writePage(b *strings.Builder, p *pageView, report string) {
	fmt.Fprintf(b, "### %s\n\n", p.slug)
	if p.url != "" {
		fmt.Fprintf(b, "<%s>\n\n", p.url)
	}
	for _, d := range p.diverged {
		fmt.Fprintf(b, "> [!NOTE]\n> %s loaded a different version of this page (a consent banner, an A/B test, a login wall…), so its differences may not be real.\n\n", d)
	}
	for i, is := range p.issues {
		if i >= maxIssuesPerPage {
			fmt.Fprintf(b, "…and %s more on this page, in the full report%s.\n\n",
				plural(len(p.issues)-maxIssuesPerPage, "issue"), linkSuffix(report))
			break
		}
		fmt.Fprintf(b, "**%d. %s** at %s\n\n", i+1, is.title, code(is.selector))
		if is.top != nil {
			fmt.Fprintf(b, "- **Where:** about %s from the top of the page\n", px(*is.top))
		}
		writeSeenIn(b, is)
		if len(is.consequences) > 0 {
			fmt.Fprintf(b, "- **What a visitor sees:** %s\n", strings.Join(is.consequences, "; "))
		}
		if is.why != "" {
			fmt.Fprintf(b, "- **Why:** %s\n", htmlText.Replace(is.why))
		}
		if is.fix != "" {
			fmt.Fprintf(b, "- **Suggested fix:** %s\n", htmlText.Replace(is.fix))
		}
		if n := len(is.others); n > 0 {
			var names []string
			for _, o := range is.others[:min(n, maxAlsoAffected)] {
				names = append(names, code(o))
			}
			more := ""
			if n > maxAlsoAffected {
				more = fmt.Sprintf(" and %d more", n-maxAlsoAffected)
			}
			fmt.Fprintf(b, "- **Same cause also affects:** %s%s\n", strings.Join(names, ", "), more)
		}
		b.WriteString("\n")
	}
	if p.info > 0 {
		fmt.Fprintf(b, "<sub>Omnisnap also found %s on this page that %s normal browser behaviour and %s no action; the full report explains %s.</sub>\n\n",
			plural(p.info, "difference"), pick(p.info, "is", "are"), pick(p.info, "needs", "need"), pick(p.info, "it", "them"))
	}
}

// writeSeenIn lists the browsers that show an issue. Browsers where the
// element differs by the same amount share a line, and the baseline is named
// once.
func writeSeenIn(b *strings.Builder, is *issue) {
	var order []string
	byDelta := map[string][]string{}
	for i, name := range is.browsers {
		d := is.deltas[i]
		if _, ok := byDelta[d]; !ok {
			order = append(order, d)
		}
		byDelta[d] = append(byDelta[d], name)
	}
	if len(order) == 1 && order[0] == "" {
		fmt.Fprintf(b, "- **Seen in:** %s\n", strings.Join(is.browsers, ", "))
		return
	}
	against := ""
	if is.baseline != "" {
		against = " (compared with " + is.baseline + ")"
	}
	fmt.Fprintf(b, "- **Seen in**%s:\n", against)
	for _, d := range order {
		line := strings.Join(byDelta[d], ", ")
		if d != "" {
			line += ": " + d
		}
		b.WriteString("  - " + line + "\n")
	}
}

// describeDelta says how the element differs, engine minus baseline, in the
// units a person checks in devtools.
func describeDelta(d schema.Delta) string {
	var parts []string
	size := func(v float64, more, less string) {
		switch {
		case v >= 0.5:
			parts = append(parts, px(v)+" "+more)
		case v <= -0.5:
			parts = append(parts, px(-v)+" "+less)
		}
	}
	size(d.DW, "wider", "narrower")
	size(d.DH, "taller", "shorter")
	if n := int(math.Round(d.DLn)); n != 0 {
		if n > 0 {
			parts = append(parts, plural(n, "more line"))
		} else {
			parts = append(parts, plural(-n, "fewer line"))
		}
	}
	var move []string
	size2 := func(v float64, pos, neg string) {
		switch {
		case v >= 0.5:
			move = append(move, px(v)+" "+pos)
		case v <= -0.5:
			move = append(move, px(-v)+" "+neg)
		}
	}
	size2(d.DX, "right", "left")
	size2(d.DY, "down", "up")
	if len(move) > 0 {
		parts = append(parts, "moved "+strings.Join(move, " and "))
	}
	return strings.Join(parts, ", ")
}

// px writes a length rounded to a whole pixel, or to a tenth below 10 px.
func px(v float64) string {
	if math.Abs(v) < 10 {
		s := strings.TrimSuffix(fmt.Sprintf("%.1f", v), ".0")
		return s + " px"
	}
	return fmt.Sprintf("%.0f px", v)
}

// htmlText keeps knowledge-base text literal: GitHub would treat "<wbr>" in
// a suggested fix as a tag and drop it.
var htmlText = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

// code puts a selector in inline code, using a longer fence when it holds a
// backtick.
func code(s string) string {
	if strings.Contains(s, "`") {
		return "`` " + s + " ``"
	}
	return "`" + s + "`"
}

// pick chooses the singular or plural form.
func pick(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func plural(n int, word string) string {
	return fmt.Sprintf("%d %s", n, pluralWord(n, word))
}

func pluralWord(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func linkSuffix(url string) string {
	if url == "" {
		return ""
	}
	return " (" + url + ")"
}
