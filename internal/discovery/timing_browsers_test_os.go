//go:build browsers

package discovery

import "runtime"

func runtimeGOOS() string { return runtime.GOOS }
