package bidi

import "context"

// AddPreloadScript runs src before any page script of s on every navigation.
//
// BiDi takes a function declaration (not arbitrary source), so src is wrapped
// in an arrow function body: the statements run when Firefox calls it in each
// new realm, before the page's own scripts.
// Source: https://w3c.github.io/webdriver-bidi/#command-script-addPreloadScript
func (s *Session) AddPreloadScript(ctx context.Context, src string) error {
	params := map[string]any{
		"functionDeclaration": "() => {\n" + src + "\n}",
		"contexts":            []string{s.context},
	}
	return call(ctx, s.c, "script.addPreloadScript", params, nil)
}
