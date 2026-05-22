package version

import (
	"runtime/debug"
	"testing"
)

func TestVersionNotEmpty(t *testing.T) {
	// init() runs before tests; under `go test` debug.ReadBuildInfo() returns
	// ok=true, so Version will at minimum be "devel". When built with a
	// module version it gets overridden. Either way it must be non-empty.
	if Version == "" {
		t.Fatalf("Version must not be empty, got %q", Version)
	}
}

func TestDebugReadBuildInfoWorks(t *testing.T) {
	// Verify that debug.ReadBuildInfo() returns ok=true when running under
	// `go test`, which confirms the happy path of init() is exercised.
	info, ok := debug.ReadBuildInfo()
	if !ok {
		t.Fatal("debug.ReadBuildInfo() returned ok=false; expected ok=true under go test")
	}
	if info == nil {
		t.Fatal("debug.ReadBuildInfo() returned nil info even though ok=true")
	}
}

func TestInitSetsVersionFromBuildInfo(t *testing.T) {
	// Under `go test`, info.Main.Version is typically "" or "(devel)",
	// so Version stays as "devel". Verify that the init() logic correctly
	// leaves Version at its default when there's no real version.
	info, ok := debug.ReadBuildInfo()
	if !ok {
		t.Skip("debug.ReadBuildInfo() not available; cannot verify init path")
	}
	mainVersion := info.Main.Version
	if mainVersion == "" || mainVersion == "(devel)" {
		// init() should have left Version as "devel"
		if Version != "devel" {
			t.Fatalf(
				"expected Version=%q when Main.Version=%q, got %q",
				"devel",
				mainVersion,
				Version,
			)
		}
	} else {
		// If a real version was embedded (e.g. via go install), init()
		// should have set Version to that value.
		if Version != mainVersion {
			t.Fatalf("expected Version=%q to match Main.Version, got %q", mainVersion, Version)
		}
	}
}

func TestInitBuildVersionReadBuildInfoFails(t *testing.T) {
	// Mock readBuildInfo to return (nil, false), hitting the !ok branch.
	orig := readBuildInfo
	readBuildInfo = func() (*debug.BuildInfo, bool) { return nil, false }
	defer func() { readBuildInfo = orig }()

	Version = "devel" // reset before calling
	initBuildVersion()

	if Version != "devel" {
		t.Fatalf("expected Version to remain %q when ReadBuildInfo fails, got %q", "devel", Version)
	}
}

func TestInitBuildVersionEmptyMainVersion(t *testing.T) {
	// Mock readBuildInfo to return ok=true but with an empty Main.Version.
	orig := readBuildInfo
	readBuildInfo = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{}, true
	}
	defer func() { readBuildInfo = orig }()

	Version = "devel"
	initBuildVersion()

	if Version != "devel" {
		t.Fatalf(
			"expected Version to remain %q when Main.Version is empty, got %q",
			"devel",
			Version,
		)
	}
}

func TestInitBuildVersionDevelMainVersion(t *testing.T) {
	// Mock readBuildInfo to return ok=true with Main.Version="(devel)".
	orig := readBuildInfo
	readBuildInfo = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}, true
	}
	defer func() { readBuildInfo = orig }()

	Version = "devel"
	initBuildVersion()

	if Version != "devel" {
		t.Fatalf(
			"expected Version to remain %q when Main.Version is (devel), got %q",
			"devel",
			Version,
		)
	}
}

func TestInitBuildVersionRealVersion(t *testing.T) {
	// Mock readBuildInfo to return ok=true with a real semver version.
	const fakeVersion = "v1.2.3"
	orig := readBuildInfo
	readBuildInfo = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{Main: debug.Module{Version: fakeVersion}}, true
	}
	defer func() { readBuildInfo = orig }()

	Version = "devel"
	initBuildVersion()

	if Version != fakeVersion {
		t.Fatalf("expected Version=%q, got %q", fakeVersion, Version)
	}
}
