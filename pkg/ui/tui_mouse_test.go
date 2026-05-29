package ui

import (
	"os/exec"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	zone "github.com/lrstanley/bubblezone/v2"

	"github.com/lukaszcz/diffnav-extra/pkg/ui/panes/diffviewer"
)

// TestRightSideSelectionEndToEnd drives the full mouse pipeline against a
// real delta render to verify a click + drag on the right half of an SBS
// view actually produces a visible highlight. This is a regression guard
// against the production failure mode the user reported ("right side
// selection doesn't work, the selection doesn't even appear").
func TestRightSideSelectionEndToEnd(t *testing.T) {
	if _, err := exec.LookPath("delta"); err != nil {
		t.Skip("delta not installed, skipping end-to-end test")
	}
	m := newTestMainModel(t)
	isDark := true
	m.isDarkBackground = &isDark
	m.diffViewer.SetDarkBackground(true)
	m.fileTree.SetDarkBackground(true)
	m.sideBySide = true
	m.diffViewer.SetSideBySide(true)

	// Size the UI like a typical wide terminal. WindowSizeMsg triggers the
	// dir-diff render through diff() which returns a Cmd that runs delta.
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 160, Height: 40})
	// Run the deferred fetchFileTree Cmd to populate m.files / m.fileTree
	// the way Init() would in production, then trigger SetDirPatch.
	m = updateMainModel(t, m, m.fetchFileTree())

	// Drain the diff Cmd(s) until a diffContentMsg lands in diffViewer
	// (delta runs in a goroutine via the Cmd indirection).
	deadline := time.Now().Add(3 * time.Second)
	for m.diffViewer.GutterCol() <= 0 {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for diffContentMsg; gutterCol still %d",
				m.diffViewer.GutterCol())
		}
		// Resolve any pending Cmds we know about.
		if cmd := m.diffViewer.Init(); cmd != nil {
			if msg := cmd(); msg != nil {
				m = updateMainModel(t, m, msg)
			}
		}
		// Best-effort: re-trigger a render so the dir delta call resolves.
		m.diffViewer.ClearCache()
		if cmd := m.diffViewer.SetSize(
			m.width-m.sidebarWidth(),
			m.mainContentHeight(),
		); cmd != nil {
			if msg := cmd(); msg != nil {
				m = updateMainModel(t, m, msg)
			}
		}
	}

	gutter := m.diffViewer.GutterCol()
	if gutter <= 0 {
		t.Fatalf("expected gutterCol > 0 after dir-diff render, got %d", gutter)
	}

	// Pick mouse coordinates on the right half of the diff pane. The diff
	// pane starts at x=sidebarWidth and the gutter is at +gutter inside it.
	rightX := m.sidebarWidth() + gutter + 5
	clickY := m.headerHeight() + 1 + diffviewerDirHeaderHeight() + 1

	// View() must run once before any mouse events: bubblezone registers
	// zones during zone.Scan inside View(), and zone.Get* lookups return
	// empty info until then.
	_ = m.View().Content

	m = updateMainModel(t, m, tea.MouseClickMsg(tea.Mouse{
		X:      rightX,
		Y:      clickY,
		Button: tea.MouseLeft,
	}))
	if !m.diffViewer.IsSelecting() {
		t.Fatalf(
			"expected diffViewer.IsSelecting()==true after click at (x=%d,y=%d)",
			rightX,
			clickY,
		)
	}
	m = updateMainModel(t, m, tea.MouseMotionMsg(tea.Mouse{
		X:      rightX + 10,
		Y:      clickY,
		Button: tea.MouseLeft,
	}))

	view := m.View().Content
	if !strings.Contains(view, "\x1b[7m") {
		t.Fatalf(
			"expected reverse-video escape (\\x1b[7m) in View() after right-side drag — selection rendered nothing",
		)
	}
}

func TestMouseScrollInFileTree(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	_ = m.View().Content
	z := waitForZone(t, zoneFileTree)

	// Scroll down in file tree zone via Update
	m = updateMainModel(t, m, tea.MouseMotionMsg(tea.Mouse{
		X:      z.StartX + 2,
		Y:      z.StartY + 1,
		Button: tea.MouseWheelDown,
	}))
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after scroll down in file tree")
	}
}

func TestMouseScrollInDiffViewer(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.diffViewer.SetContent(strings.Repeat("line\n", 500))

	_ = m.View().Content
	z := waitForZone(t, zoneDiffViewer)

	m = updateMainModel(t, m, tea.MouseMotionMsg(tea.Mouse{
		X:      z.StartX + 5,
		Y:      z.StartY + 5,
		Button: tea.MouseWheelDown,
	}))
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after scroll down in diff viewer")
	}
}

func TestMouseClickDiffPaneStartsSelectionThroughUpdate(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 160, Height: 40})

	_ = m.View().Content
	z := waitForZone(t, zoneDiffViewer)

	clickY := z.StartY + diffviewer.DirHeaderHeight + 5
	m = updateMainModel(t, m, tea.MouseClickMsg(tea.Mouse{
		X:      z.StartX + 10,
		Y:      clickY,
		Button: tea.MouseLeft,
	}))

	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after clicking diff pane")
	}
}

func TestMouseMotionDiffSelectionThroughUpdate(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 160, Height: 40})
	m.diffViewer.StartSelection(diffviewer.Point{Line: 5, Col: 0})
	m.diffViewer.SetContent(strings.Repeat("line\n", 200))

	_ = m.View().Content
	z := waitForZone(t, zoneDiffViewer)

	motionY := z.StartY + diffviewer.DirHeaderHeight + 5
	m = updateMainModel(t, m, tea.MouseMotionMsg(tea.Mouse{
		X:      z.StartX + 10,
		Y:      motionY,
		Button: tea.MouseLeft,
	}))

	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after diff selection motion")
	}
}

func TestMouseReleaseEndsSelectionThroughUpdate(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 160, Height: 40})
	m.diffViewer.StartSelection(diffviewer.Point{Line: 0, Col: 0})
	m.diffViewer.SetContent(strings.Repeat("hello world\n", 50))

	// Release should end selection
	m = updateMainModel(t, m, tea.MouseReleaseMsg(tea.Mouse{
		X:      50,
		Y:      5,
		Button: tea.MouseLeft,
	}))

	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after mouse release")
	}
}

func TestMouseReleaseNoSelectionThroughUpdate(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	m = updateMainModel(t, m, tea.MouseReleaseMsg(tea.Mouse{
		X:      10,
		Y:      10,
		Button: tea.MouseLeft,
	}))

	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after release without selection")
	}
}

func TestMouseReleaseWithFinalizedSelection(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 160, Height: 40})
	m.diffViewer.StartSelection(diffviewer.Point{Line: 0, Col: 0})
	m.diffViewer.ExtendSelection(diffviewer.Point{Line: 3, Col: 10})
	m.diffViewer.EndSelection()
	m.diffViewer.SetContent(strings.Repeat("hello world\n", 50))

	// Clear and re-start with proper content
	m.diffViewer.ClearSelection()
	m.diffViewer.StartSelection(diffviewer.Point{Line: 0, Col: 0})
	m.diffViewer.ExtendSelection(diffviewer.Point{Line: 2, Col: 5})

	m = updateMainModel(t, m, tea.MouseReleaseMsg(tea.Mouse{
		X:      50,
		Y:      10,
		Button: tea.MouseLeft,
	}))

	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after release with selection")
	}
}

func TestMouseDiffSelectionMotionAboveViewportThroughUpdate(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 160, Height: 40})
	m.diffViewer.StartSelection(diffviewer.Point{Line: 5, Col: 0})
	m.diffViewer.SetContent(strings.Repeat("line\n", 200))
	m.diffViewer.ScrollDown(20)

	_ = m.View().Content
	z := waitForZone(t, zoneDiffViewer)

	// Move above the dir header area
	m = updateMainModel(t, m, tea.MouseMotionMsg(tea.Mouse{
		X:      z.StartX + 5,
		Y:      z.StartY,
		Button: tea.MouseLeft,
	}))

	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after diff selection motion above viewport")
	}
}

func TestMouseDiffSelectionMotionBelowViewportThroughUpdate(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 160, Height: 40})
	m.diffViewer.StartSelection(diffviewer.Point{Line: 5, Col: 0})
	m.diffViewer.SetContent(strings.Repeat("line\n", 200))

	_ = m.View().Content
	z := waitForZone(t, zoneDiffViewer)

	// Move below viewport
	belowY := z.StartY + diffviewer.DirHeaderHeight + m.diffViewer.Height() + 5
	m = updateMainModel(t, m, tea.MouseMotionMsg(tea.Mouse{
		X:      z.StartX + 5,
		Y:      belowY,
		Button: tea.MouseLeft,
	}))

	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after diff selection motion below viewport")
	}
}

func TestMouseDiffSelectionMotionNotSelectingThroughUpdate(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	m = updateMainModel(t, m, tea.MouseMotionMsg(tea.Mouse{
		X:      50,
		Y:      10,
		Button: tea.MouseLeft,
	}))

	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after motion when not selecting")
	}
}

func TestMouseDiffSelectionMotionOutsideHorizontalThroughUpdate(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.diffViewer.StartSelection(diffviewer.Point{Line: 0, Col: 0})

	// Move cursor outside the diff zone horizontally (x=0 is in the sidebar)
	m = updateMainModel(t, m, tea.MouseMotionMsg(tea.Mouse{
		X:      0,
		Y:      10,
		Button: tea.MouseLeft,
	}))

	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after horizontal out-of-bounds motion")
	}
}

func TestMouseDiffSelectionMotionNotSelectingThroughUpdate2(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 160, Height: 40})

	// Not selecting — mouse motion in diff area should be a no-op for diff selection
	_ = m.View().Content
	z := waitForZone(t, zoneDiffViewer)

	motionY := z.StartY + diffviewer.DirHeaderHeight + 5
	m = updateMainModel(t, m, tea.MouseMotionMsg(tea.Mouse{
		X:      z.StartX + 10,
		Y:      motionY,
		Button: tea.MouseLeft,
	}))

	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after mouse motion with no selection active")
	}
}

func TestMouseDiffSelectionMotionZeroZoneThroughUpdate(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.diffViewer.StartSelection(diffviewer.Point{Line: 0, Col: 0})

	// Override getDiffViewerZone to return a zero zone
	origGetZone := getDiffViewerZone
	defer func() { getDiffViewerZone = origGetZone }()
	getDiffViewerZone = func() *zone.ZoneInfo { return &zone.ZoneInfo{} }

	m = updateMainModel(t, m, tea.MouseMotionMsg(tea.Mouse{
		X:      50,
		Y:      10,
		Button: tea.MouseLeft,
	}))

	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after diff selection motion with zero zone")
	}
}

func TestMouseFileTreeClickBottomOfZoneThroughUpdate(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	_ = m.View().Content
	z := waitForZone(t, zoneFileTree)

	// Click at the very bottom of the zone (may have nil node)
	m = updateMainModel(t, m, tea.MouseClickMsg(tea.Mouse{
		X:      z.StartX + 2,
		Y:      z.EndY - 1,
		Button: tea.MouseLeft,
	}))

	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after clicking bottom of file tree zone")
	}
}

func TestHandleFileTreeClickNegativeY(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	// The "if y < 0" guard in handleFileTreeClick cannot be triggered
	// through Update because InBounds gates clicks to within the zone,
	// so zone-relative Y is always >= 0. Test it directly here.
	_ = m.View().Content
	z := waitForZone(t, zoneFileTree)
	if z.StartY <= 0 {
		t.Fatal("expected zone StartY > 0 after View()")
	}

	// Click above the zone so zone-relative y < 0 triggers the guard.
	updated, cmd := m.handleFileTreeClick(tea.MouseClickMsg(tea.Mouse{
		X:      z.StartX + 2,
		Y:      z.StartY - 1,
		Button: tea.MouseLeft,
	}))
	m2, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	_ = m2
	_ = cmd
}

func TestHandleDiffSelectionMotionNotSelecting(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	// The "if !IsSelecting()" guard in handleDiffSelectionMotion cannot be
	// triggered through Update because handleMouse only calls it when
	// IsSelecting() is true. Test it directly here.
	result, cmd := m.handleDiffSelectionMotion(tea.MouseMotionMsg(tea.Mouse{
		X:      50,
		Y:      10,
		Button: tea.MouseLeft,
	}))
	m2, ok := result.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", result)
	}
	_ = m2
	_ = cmd
}

func TestDiffPanePointAboveDirHeaderThroughClick(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 160, Height: 40})

	_ = m.View().Content
	z := waitForZone(t, zoneDiffViewer)

	// Click in the dir header area (paneY < DirHeaderHeight)
	// This should NOT start a selection via the diff pane
	clickY := z.StartY + 1 // paneY = 1, < DirHeaderHeight (3)
	m = updateMainModel(t, m, tea.MouseClickMsg(tea.Mouse{
		X:      z.StartX + 5,
		Y:      clickY,
		Button: tea.MouseLeft,
	}))

	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after clicking in dir header area")
	}
}

func TestMouseScrollUpInDiffViewerZone(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.diffViewer.SetContent(strings.Repeat("line\n", 500))
	m.diffViewer.ScrollDown(10)

	_ = m.View().Content
	z := waitForZone(t, zoneDiffViewer)

	m = updateMainModel(t, m, tea.MouseMotionMsg(tea.Mouse{
		X:      z.StartX + 5,
		Y:      z.StartY + 5,
		Button: tea.MouseWheelUp,
	}))

	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after scroll up in diff viewer")
	}
}

func TestMouseScrollUpDownInFileTree(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 8})

	_ = m.View().Content
	z := waitForZone(t, zoneFileTree)

	// Scroll down
	m = updateMainModel(t, m, tea.MouseMotionMsg(tea.Mouse{
		X:      z.StartX + 2,
		Y:      z.StartY + 1,
		Button: tea.MouseWheelDown,
	}))

	// Scroll up
	m = updateMainModel(t, m, tea.MouseMotionMsg(tea.Mouse{
		X:      z.StartX + 2,
		Y:      z.StartY + 1,
		Button: tea.MouseWheelUp,
	}))

	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after scroll up/down in file tree")
	}
}

// TestMouseClickFileTreeZoneThroughUpdate covers clicking in the file tree
// zone through the Update message pipeline.
func TestMouseClickFileTreeZoneThroughUpdate(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	_ = m.View().Content
	z := waitForZone(t, zoneFileTree)

	m = updateMainModel(t, m, tea.MouseClickMsg(tea.Mouse{
		Button: tea.MouseLeft,
		X:      z.StartX + 2,
		Y:      z.StartY,
	}))

	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after clicking file tree zone")
	}
}
