//go:build !linux

package process

import "context"

// visibleSetup: Windows and macOS run the browser on their own desktop; the
// runner has one, and the test cleans the profile up.
func visibleSetup(context.Context) (env, args []string, closeFn func(context.Context) error, err error) {
	return nil, nil, nil, nil
}
