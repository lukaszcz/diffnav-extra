package dirnode

import "testing"

func TestDirNode_String(t *testing.T) {
	d := &DirNode{FullPath: "src/pkg", Name: "pkg"}
	got := d.String()
	if got != "pkg" {
		t.Errorf("String() = %q, want %q", got, "pkg")
	}
}

func TestDirNode_Fields(t *testing.T) {
	d := DirNode{FullPath: "a/b/c", Name: "c"}
	if d.FullPath != "a/b/c" {
		t.Errorf("FullPath = %q, want %q", d.FullPath, "a/b/c")
	}
	if d.Name != "c" {
		t.Errorf("Name = %q, want %q", d.Name, "c")
	}
}

func TestDirNode_NilReceiver(t *testing.T) {
	var d *DirNode
	// String() should safely return empty string on nil receiver.
	got := d.String()
	if got != "" {
		t.Errorf("String() on nil *DirNode = %q, want empty string", got)
	}
}
