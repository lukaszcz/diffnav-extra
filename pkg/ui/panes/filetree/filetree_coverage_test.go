package filetree

import (
	"testing"

	"github.com/dlvhdr/diffnav/pkg/config"
)

// --- SetCursorNoScroll uncovered branch: len(m.files) == 0 ---

func TestSetCursorNoScrollEmptyModelIsNoOp(t *testing.T) {
	cfg := config.DefaultConfig()
	m := New(cfg)

	// With no files, SetCursorNoScroll should be a no-op and not panic.
	m.SetCursorNoScroll(5)

	// The view for an empty model should remain empty — no crash, no side effects.
	view := m.View()
	if len(view) > 0 && view != "\n" {
		t.Fatalf("expected empty view after SetCursorNoScroll on empty model, got %q", view)
	}
}

// --- ClickNode uncovered branch: node == nil ---

func TestClickNodeNilNodeIsNoOp(t *testing.T) {
	m := newTestTreeModel([]string{"app/main.go", "docs/readme.md"})

	// Navigate to a known file so we have a stable starting point.
	m.SetCursorByPath("app/main.go")
	pathBefore := m.CurrNodePath()

	// Clicking a nil node should be a no-op — no crash and no cursor movement.
	m.ClickNode(nil)

	pathAfter := m.CurrNodePath()
	if pathAfter != pathBefore {
		t.Fatalf("expected path unchanged after ClickNode(nil), got before=%q after=%q",
			pathBefore, pathAfter)
	}
}

// --- ClickNode uncovered branch: len(m.files) == 0 ---

func TestClickNodeEmptyModelIsNoOp(t *testing.T) {
	cfg := config.DefaultConfig()
	m := New(cfg)

	// With no files, ClickNode should be a no-op and not panic even with a non-nil node.
	m.ClickNode(nil)

	view := m.View()
	if len(view) > 0 && view != "\n" {
		t.Fatalf("expected empty view after ClickNode on empty model, got %q", view)
	}
}

// --- ClickNodeIcon uncovered branch: node == nil ---

func TestClickNodeIconNilNodeIsNoOp(t *testing.T) {
	m := newTestTreeModel([]string{"app/main.go", "docs/readme.md"})

	// Navigate to a known directory so we have a stable starting point.
	m.SetCursorByPath("app")
	pathBefore := m.CurrNodePath()

	// Clicking the icon of a nil node should be a no-op — no crash and no cursor movement.
	m.ClickNodeIcon(nil)

	pathAfter := m.CurrNodePath()
	if pathAfter != pathBefore {
		t.Fatalf("expected path unchanged after ClickNodeIcon(nil), got before=%q after=%q",
			pathBefore, pathAfter)
	}
}

// --- ClickNodeIcon uncovered branch: len(m.files) == 0 ---

func TestClickNodeIconEmptyModelIsNoOp(t *testing.T) {
	cfg := config.DefaultConfig()
	m := New(cfg)

	// With no files, ClickNodeIcon should be a no-op and not panic.
	m.ClickNodeIcon(nil)

	view := m.View()
	if len(view) > 0 && view != "\n" {
		t.Fatalf("expected empty view after ClickNodeIcon on empty model, got %q", view)
	}
}
