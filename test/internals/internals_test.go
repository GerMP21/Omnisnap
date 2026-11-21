// Package internals checks the developer wiki in docs/internals/, the pages
// .github/workflows/wiki.yml publishes to the repository's GitHub Wiki.
//
//   - Every page but Home, Glossary, _Sidebar and _Footer ends with a
//     "## Source files" section of repository links.
//   - Every repository link points at main, without a line anchor, and names
//     a file or directory that exists.
//   - Every non-test Go file under cmd/, internal/, tools/ and assets/, and
//     every top-level item of the directories in topLevelRoots, is named in
//     some page's "Source files" section.
//   - Every link between pages, and to a heading on one, resolves.
//   - The sidebar links every page.
//
// Each check lists every offending page and item at once. A page can still
// say something the code no longer does; the rule in CONTRIBUTING.md and
// CLAUDE.md covers that.
package internals

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
	"unicode"
)

const (
	repoURL     = "https://github.com/GerMP21/Omnisnap/"
	sourceFiles = "Source files"
)

// Pages with no "Source files" section.
var exempt = []string{"Home", "Glossary", "_Sidebar", "_Footer"}

// Directories whose non-test Go files must each be named by a page.
var goRoots = []string{"cmd", "internal", "tools", "assets"}

// Directories whose top-level items must each be named by a page, or have
// something under them named.
var topLevelRoots = []string{".github/workflows", "action", "packages", "schema", "scripts", "test", "tools"}

// Items under those roots that need no page: build outputs and caches that
// are not in the repository.
var ignoredItems = []string{"build", "dist", "node_modules", "__pycache__"}

func root(t *testing.T) string {
	t.Helper()
	r, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// pages returns the wiki's pages: page name (file stem) to text.
func pages(t *testing.T) map[string]string {
	t.Helper()
	dir := filepath.Join(root(t), "docs", "internals")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".md" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		out[strings.TrimSuffix(e.Name(), ".md")] = strings.ReplaceAll(string(b), "\r\n", "\n")
	}
	if len(out) == 0 {
		t.Fatal("docs/internals/ has no pages")
	}
	return out
}

func sortedNames(m map[string]string) []string {
	names := make([]string, 0, len(m))
	for n := range m {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// stripCode blanks fenced code blocks and drops inline code spans, so links
// inside code do not count. With keepInline, inline code keeps its text
// without the backticks, as GitHub does when it builds a heading's anchor.
func stripCode(text string, keepInline bool) string {
	var out strings.Builder
	fence := ""
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimLeft(line, " \t")
		if fence != "" {
			if strings.HasPrefix(trimmed, fence) {
				fence = ""
			}
			out.WriteByte('\n')
			continue
		}
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			fence = trimmed[:len(trimmed)-len(strings.TrimLeft(trimmed, "`~"))]
			out.WriteByte('\n')
			continue
		}
		inCode := false
		for _, c := range line {
			if c == '`' {
				inCode = !inCode
			} else if !inCode || keepInline {
				out.WriteRune(c)
			}
		}
		out.WriteByte('\n')
	}
	return out.String()
}

// linkTargets returns every Markdown link target [text](target) in text,
// code excluded.
func linkTargets(text string) []string {
	rest := stripCode(text, false)
	var targets []string
	for {
		i := strings.Index(rest, "](")
		if i < 0 {
			return targets
		}
		after := rest[i+2:]
		end := strings.IndexByte(after, ')')
		if end < 0 {
			return targets
		}
		target := strings.TrimSpace(after[:end])
		if f := strings.Fields(target); len(f) > 0 {
			target = f[0]
		} else {
			target = ""
		}
		targets = append(targets, strings.TrimSuffix(strings.TrimPrefix(target, "<"), ">"))
		rest = after[end+1:]
	}
}

// section returns the body of the "## name" section, up to the next "## "
// heading or the end of the page, and whether the page has one.
func section(text, name string) (string, bool) {
	lines := strings.SplitAfter(text, "\n")
	start := -1
	var body strings.Builder
	for _, line := range lines {
		bare := strings.TrimRight(line, " \t\r\n")
		if start >= 0 {
			if strings.HasPrefix(bare, "## ") {
				return body.String(), true
			}
			body.WriteString(line)
		} else if bare == "## "+name {
			start = 0
		}
	}
	return body.String(), start >= 0
}

// lastSection returns the name of the page's last "## " heading.
func lastSection(text string) string {
	last := ""
	for _, line := range strings.Split(stripCode(text, true), "\n") {
		if strings.HasPrefix(line, "## ") {
			last = strings.TrimSpace(line[3:])
		}
	}
	return last
}

var markdownLink = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)

// slug is GitHub's anchor for a heading's text: links reduced to their
// text, lower case, everything but letters, digits, spaces, '-' and '_'
// removed, spaces turned into '-'.
func slug(heading string) string {
	plain := markdownLink.ReplaceAllString(heading, "$1")
	var out strings.Builder
	for _, c := range strings.ToLower(strings.TrimSpace(plain)) {
		switch {
		case c == ' ':
			out.WriteByte('-')
		case unicode.IsLetter(c), unicode.IsDigit(c), c == '-', c == '_':
			out.WriteRune(c)
		}
	}
	return out.String()
}

// anchors returns every anchor a page's headings produce, with GitHub's -1,
// -2 suffixes on repeats.
func anchors(text string) map[string]bool {
	seen := map[string]int{}
	out := map[string]bool{}
	for _, line := range strings.Split(stripCode(text, true), "\n") {
		hashes := len(line) - len(strings.TrimLeft(line, "#"))
		if hashes < 1 || hashes > 6 || !strings.HasPrefix(line[hashes:], " ") {
			continue
		}
		base := slug(line[hashes+1:])
		anchor := base
		if n := seen[base]; n > 0 {
			anchor = fmt.Sprintf("%s-%d", base, n)
		}
		seen[base]++
		out[anchor] = true
	}
	return out
}

// repoLink splits a link into this repository's tree into its kind ("blob"
// or "tree"), ref, path and fragment. ok is false for any other link.
func repoLink(target string) (kind, ref, path, fragment string, ok bool) {
	rest, found := strings.CutPrefix(target, repoURL)
	if !found {
		return "", "", "", "", false
	}
	kind, rest, found = strings.Cut(rest, "/")
	if !found || (kind != "blob" && kind != "tree") {
		return "", "", "", "", false
	}
	ref, rest, _ = strings.Cut(rest, "/")
	if i := strings.IndexAny(rest, "#?"); i >= 0 {
		if rest[i] == '#' {
			fragment = rest[i+1:]
		}
		rest = rest[:i]
	}
	return kind, ref, strings.TrimSuffix(rest, "/"), fragment, true
}

// namedPaths returns the repository paths named in a page's "Source files".
func namedPaths(text string) []string {
	body, ok := section(text, sourceFiles)
	if !ok {
		return nil
	}
	var out []string
	for _, target := range linkTargets(body) {
		if _, _, path, _, ok := repoLink(target); ok {
			out = append(out, path)
		}
	}
	return out
}

func report(t *testing.T, problems []string) {
	t.Helper()
	if len(problems) > 0 {
		t.Errorf("\n%s", strings.Join(problems, "\n"))
	}
}

func TestEveryPageListsItsSourceFiles(t *testing.T) {
	ps := pages(t)
	var problems []string
	for _, name := range sortedNames(ps) {
		if slices.Contains(exempt, name) {
			continue
		}
		text := ps[name]
		body, ok := section(text, sourceFiles)
		if !ok {
			problems = append(problems, fmt.Sprintf("%s: no `## %s` section", name, sourceFiles))
			continue
		}
		if last := lastSection(text); last != sourceFiles {
			problems = append(problems, fmt.Sprintf("%s: `## %s` comes after `## %s`", name, last, sourceFiles))
		}
		targets := linkTargets(body)
		if len(targets) == 0 {
			problems = append(problems, fmt.Sprintf("%s: `## %s` lists no link", name, sourceFiles))
		}
		for _, target := range targets {
			if _, _, _, _, ok := repoLink(target); !ok {
				problems = append(problems, fmt.Sprintf("%s: `%s` in `## %s` is not a %sblob/main/ or tree/main/ link", name, target, sourceFiles, repoURL))
			}
		}
	}
	report(t, problems)
}

func TestEveryRepositoryLinkExistsOnMain(t *testing.T) {
	ps := pages(t)
	r := root(t)
	var problems []string
	for _, name := range sortedNames(ps) {
		for _, target := range linkTargets(ps[name]) {
			kind, ref, path, fragment, ok := repoLink(target)
			if !ok {
				continue
			}
			if ref != "main" {
				problems = append(problems, fmt.Sprintf("%s: `%s` points at %q, not main", name, target, ref))
				continue
			}
			if strings.HasPrefix(fragment, "L") {
				problems = append(problems, fmt.Sprintf("%s: `%s` has a line anchor, which drifts as main changes; name the function instead", name, target))
			}
			info, err := os.Stat(filepath.Join(r, filepath.FromSlash(path)))
			switch {
			case err != nil:
				problems = append(problems, fmt.Sprintf("%s: no `%s`", name, path))
			case kind == "blob" && info.IsDir():
				problems = append(problems, fmt.Sprintf("%s: `%s` is a directory; link it with tree/main/", name, path))
			case kind == "tree" && !info.IsDir():
				problems = append(problems, fmt.Sprintf("%s: `%s` is a file; link it with blob/main/", name, path))
			}
		}
	}
	report(t, problems)
}

func TestEverySourceFileIsNamed(t *testing.T) {
	ps := pages(t)
	r := root(t)
	named := map[string]bool{}
	for _, text := range ps {
		for _, p := range namedPaths(text) {
			named[p] = true
		}
	}
	underNamed := func(path string) bool {
		for n := range named {
			if n == path || strings.HasPrefix(n, path+"/") {
				return true
			}
		}
		return false
	}
	var problems []string
	for _, dir := range goRoots {
		err := filepath.WalkDir(filepath.Join(r, dir), func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() && (d.Name() == "node_modules" || strings.HasPrefix(d.Name(), ".")) {
				return filepath.SkipDir
			}
			if d.IsDir() || filepath.Ext(p) != ".go" || strings.HasSuffix(p, "_test.go") {
				return nil
			}
			rel, _ := filepath.Rel(r, p)
			rel = filepath.ToSlash(rel)
			if !named[rel] {
				problems = append(problems, rel+": no page names it")
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, top := range topLevelRoots {
		entries, err := os.ReadDir(filepath.Join(r, filepath.FromSlash(top)))
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			item := e.Name()
			if strings.HasPrefix(item, ".") || slices.Contains(ignoredItems, item) || strings.HasSuffix(item, ".egg-info") {
				continue
			}
			if path := top + "/" + item; !underNamed(path) {
				problems = append(problems, path+": no page names it or anything under it")
			}
		}
	}
	sort.Strings(problems)
	report(t, problems)
}

func TestWikiLinksResolve(t *testing.T) {
	ps := pages(t)
	anchorsOf := map[string]map[string]bool{}
	for name, text := range ps {
		anchorsOf[name] = anchors(text)
	}
	var problems []string
	for _, name := range sortedNames(ps) {
		for _, target := range linkTargets(ps[name]) {
			if target == "" || strings.Contains(target, "://") || strings.HasPrefix(target, "/") || strings.HasPrefix(target, "mailto:") {
				continue
			}
			page, fragment, hasFragment := strings.Cut(target, "#")
			if page == "" {
				page = name
			}
			pageAnchors, ok := anchorsOf[page]
			if !ok {
				problems = append(problems, fmt.Sprintf("%s: `%s` names no page", name, target))
				continue
			}
			if hasFragment && !pageAnchors[fragment] {
				problems = append(problems, fmt.Sprintf("%s: `%s`: %s has no heading `#%s`", name, target, page, fragment))
			}
		}
	}
	report(t, problems)
}

func TestSidebarLinksEveryPage(t *testing.T) {
	ps := pages(t)
	sidebar, ok := ps["_Sidebar"]
	if !ok {
		t.Fatal("docs/internals/_Sidebar.md does not exist")
	}
	linked := map[string]bool{}
	for _, target := range linkTargets(sidebar) {
		page, _, _ := strings.Cut(target, "#")
		linked[page] = true
	}
	var missing []string
	for _, name := range sortedNames(ps) {
		if !strings.HasPrefix(name, "_") && !linked[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		t.Errorf("_Sidebar does not link: %v", missing)
	}
}

// Helper tests, on fixed strings.

func TestSlugsFollowGitHub(t *testing.T) {
	for heading, want := range map[string]string{
		"Source files":                     "source-files",
		"One capture: `Page`":              "one-capture-page",
		"Drive levels & the governor":      "drive-levels--the-governor",
		"[Link](Page) text":                "link-text",
		"Where the code differs from spec": "where-the-code-differs-from-spec",
		"1. Choose the version":            "1-choose-the-version",
	} {
		if got := slug(heading); got != want {
			t.Errorf("slug(%q) = %q, want %q", heading, got, want)
		}
	}
	page := "# A\n## Notes\n### Notes\n```\n## Not a heading\n```\n## Sizes: `max_runs`\n"
	got := anchors(page)
	want := map[string]bool{"a": true, "notes": true, "notes-1": true, "sizes-max_runs": true}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("anchors = %v, want %v", got, want)
	}
}

func TestLinksSkipCode(t *testing.T) {
	text := "See [a](Page-A) and `[b](Page-B)`.\n```\n[c](Page-C)\n```\n[d](Page-D#x \"title\")\n"
	if got := linkTargets(text); !slices.Equal(got, []string{"Page-A", "Page-D#x"}) {
		t.Errorf("linkTargets = %v", got)
	}
}

func TestSectionsEndAtTheNextLevelTwoHeading(t *testing.T) {
	text := "# T\n## Source files\n- [x](u)\n### Sub\n- [y](v)\n## After\n- [z](w)\n"
	body, ok := section(text, sourceFiles)
	if !ok || !slices.Equal(linkTargets(body), []string{"u", "v"}) {
		t.Errorf("section = %q, %v", body, ok)
	}
	if _, ok := section(text, "Missing"); ok {
		t.Error("found a missing section")
	}
	if got := lastSection(text); got != "After" {
		t.Errorf("lastSection = %q", got)
	}
}

func TestRepositoryLinksAreParsed(t *testing.T) {
	kind, ref, path, fragment, ok := repoLink(repoURL + "blob/main/internal/merge/merge.go#L10")
	if !ok || kind != "blob" || ref != "main" || path != "internal/merge/merge.go" || fragment != "L10" {
		t.Errorf("blob link: %q %q %q %q %v", kind, ref, path, fragment, ok)
	}
	kind, ref, path, _, ok = repoLink(repoURL + "tree/dev/internal/")
	if !ok || kind != "tree" || ref != "dev" || path != "internal" {
		t.Errorf("tree link: %q %q %q %v", kind, ref, path, ok)
	}
	for _, other := range []string{"https://example.com/x", repoURL + "commit/abc", repoURL + "wiki/Home"} {
		if _, _, _, _, ok := repoLink(other); ok {
			t.Errorf("repoLink(%q) matched", other)
		}
	}
}
