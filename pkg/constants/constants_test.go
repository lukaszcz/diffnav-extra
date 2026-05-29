package constants

import "testing"

func TestConstants(t *testing.T) {
	if OpenFileTreeWidth <= 0 {
		t.Errorf("OpenFileTreeWidth = %d, want positive", OpenFileTreeWidth)
	}
	if RootName != "/" {
		t.Errorf("RootName = %q, want %q", RootName, "/")
	}
}
