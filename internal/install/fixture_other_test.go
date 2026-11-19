//go:build !darwin && !windows

package install

import "testing"

func hostRelease(t *testing.T) hostFixture { return committedFixture(t) }
