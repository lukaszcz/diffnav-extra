package constants

import "testing"

func TestConstants(t *testing.T) {
	if SearchingFileTreeWidth != 50 {
		t.Errorf("SearchingFileTreeWidth = %d, want 50", SearchingFileTreeWidth)
	}
	if OpenFileTreeWidth != 50 {
		t.Errorf("OpenFileTreeWidth = %d, want 50", OpenFileTreeWidth)
	}
	if RootName != "/" {
		t.Errorf("RootName = %q, want %q", RootName, "/")
	}
}
