package drivers

import "errors"

// Typed errors, for callers that branch (capture's failure policy). Drivers
// wrap them with the protocol's own message: errors.Is(err, ErrScript).
var (
	ErrTimeout     = errors.New("timeout")
	ErrNavigation  = errors.New("navigation failed")
	ErrScript      = errors.New("script error") // the message carries the page's own error text
	ErrCrashed     = errors.New("browser or page crashed")
	ErrUnsupported = errors.New("not supported by this protocol")
)
