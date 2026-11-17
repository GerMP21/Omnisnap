package cdp

import "context"

// AddPreloadScript runs src before any page script on every later navigation.
func (s *Session) AddPreloadScript(ctx context.Context, src string) error {
	return s.call(ctx, "Page.addScriptToEvaluateOnNewDocument", map[string]any{"source": src}, nil)
}
