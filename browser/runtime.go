package browser

import (
	"runtime"
)

// Indirections exist so platform-mapping tests can override the reported
// OS/arch without rebuilding.

var runtimeGOOSFn = func() string { return runtime.GOOS }

var runtimeGOARCHFn = func() string { return runtime.GOARCH }

// runtimeGOOS is indirected for tests.
func runtimeGOOS() string { return runtimeGOOSFn() }

// runtimeGOARCH is indirected for tests.
func runtimeGOARCH() string { return runtimeGOARCHFn() }
