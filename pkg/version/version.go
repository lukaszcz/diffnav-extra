package version

import "runtime/debug"

// Build-time parameters set via -ldflags

var Version = "devel"

// readBuildInfo is the function used to read build info. Exported for testing;
// production code should never reassign it.
var readBuildInfo = debug.ReadBuildInfo

// A user may install diffnav using `go install github.com/dlvhdr/diffnav@latest`.
// without -ldflags, in which case the version above is unset. As a workaround
// we use the embedded build version that *is* set when using `go install` (and
// is only set for `go install` and not for `go build`).

// initBuildVersion sets Version from build info. Exported for testing.
func initBuildVersion() {
	info, ok := readBuildInfo()
	if !ok {
		return
	}
	mainVersion := info.Main.Version
	if mainVersion != "" && mainVersion != "(devel)" {
		Version = mainVersion
	}
}

func init() {
	initBuildVersion()
}
