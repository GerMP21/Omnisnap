// Package selector builds the CSS selector Omnisnap shows for an element
// (specs/SPEC-layout-diff.md, "Selectors"): causes, intent violations,
// warnings and DOM audit findings all name an element with it, so the same
// element carries the same string everywhere.
//
// A selector is up to five segments joined with " > ", from the nearest
// ancestor with an id (or body, or html) down to the element. A segment is the
// tag with its #id, or with up to two .classes, plus :nth-of-type when the
// parent has other children with that tag. Ids and classes are escaped as
// CSS.escape() does, so the selector is valid CSS: anchored at an id, body or
// html, document.querySelector returns the element; cut at five segments, the
// element is among its matches.
//
// The position comes from the capture's nt (the DOM position, hidden siblings
// included). Captures from before nt have none, and then the captured same-tag
// siblings are counted, as before; a new capture without nt means the tag is
// unique among the DOM children, so the fallback writes no position either.
package selector

import (
	"strconv"
	"strings"

	"github.com/GerMP21/Omnisnap/internal/schema"
)

// MaxSegments is the most segments a selector has.
const MaxSegments = 5

// maxClasses is the most classes a segment names.
const maxClasses = 2

// Path is the selector of n. up returns an element's parent (false at the
// root), and node the captured node of an element, so callers keep their own
// tree types: layoutdiff walks *layoutdiff.Node, domaudit entry indices.
func Path[T any](n T, up func(T) (T, bool), node func(T) *schema.Node) string {
	var parts []string
	for cur, ok := n, true; ok && len(parts) < MaxSegments; cur, ok = up(cur) {
		el := node(cur)
		var siblings []schema.Node
		if p, has := up(cur); has {
			siblings = node(p).K
		}
		parts = append(parts, Segment(el, siblings))
		if anchor(el) {
			break
		}
	}
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	return strings.Join(parts, " > ")
}

// anchor reports whether a selector starts at n: an id is meant to be unique,
// and body and html are.
func anchor(n *schema.Node) bool {
	return n.ID != "" || n.T == "body" || n.T == "html"
}

// Segment is n's part of a selector. siblings are its parent's captured
// children (n among them, nil at the root); they are read only when n has no
// nt.
func Segment(n *schema.Node, siblings []schema.Node) string {
	if n.ID != "" {
		return n.T + "#" + Ident(n.ID)
	}
	var b strings.Builder
	b.WriteString(n.T)
	classes := strings.Fields(n.C)
	for _, c := range classes[:min(maxClasses, len(classes))] {
		b.WriteByte('.')
		b.WriteString(Ident(c))
	}
	if pos := position(n, siblings); pos > 0 {
		b.WriteString(":nth-of-type(")
		b.WriteString(strconv.Itoa(pos))
		b.WriteByte(')')
	}
	return b.String()
}

// position is n's :nth-of-type index, or 0 when it needs none.
func position(n *schema.Node, siblings []schema.Node) int {
	if n.NT > 0 {
		return n.NT
	}
	same, pos := 0, 0
	for i := range siblings {
		if siblings[i].T != n.T {
			continue
		}
		same++
		if &siblings[i] == n {
			pos = same
		}
	}
	if same < 2 {
		return 0
	}
	return pos
}

// Ident serializes s as a CSS identifier, the way CSS.escape() does.
// Source: CSSOM, "Serializing Identifiers",
// https://drafts.csswg.org/cssom/#serialize-an-identifier
func Ident(s string) string {
	rs := []rune(s)
	var b strings.Builder
	for i, r := range rs {
		switch {
		case r == 0:
			b.WriteRune('�')
		case r >= 0x1 && r <= 0x1F, r == 0x7F,
			i == 0 && isDigit(r),
			i == 1 && isDigit(r) && rs[0] == '-':
			b.WriteByte('\\')
			b.WriteString(strconv.FormatInt(int64(r), 16))
			b.WriteByte(' ')
		case i == 0 && r == '-' && len(rs) == 1:
			b.WriteString(`\-`)
		case r >= 0x80, r == '-', r == '_', isDigit(r),
			r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z':
			b.WriteRune(r)
		default:
			b.WriteByte('\\')
			b.WriteRune(r)
		}
	}
	return b.String()
}

func isDigit(r rune) bool { return r >= '0' && r <= '9' }
