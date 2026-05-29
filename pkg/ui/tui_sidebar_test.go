package ui

import (
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	zone "github.com/lrstanley/bubblezone/v2"

	"github.com/dlvhdr/diffnav/pkg/config"
)

// sidebarWidthFromView extracts the sidebar width from the rendered View
// by measuring the visible character width before the "┬" junction in the
// separator line. Returns 0 if the sidebar is hidden (no "┬" found).
func sidebarWidthFromView(m mainModel) int {
	view := m.View().Content
	idx := strings.Index(view, "┬")
	if idx < 0 {
		return 0
	}
	lineStart := strings.LastIndex(view[:idx], "\n") + 1
	prefix := view[lineStart:idx]
	return lipgloss.Width(prefix)
}

// sidebarVisibleInView checks whether the sidebar is visible in the rendered
// View. When the sidebar is visible, the separator line contains a "┬"
// junction; when hidden, it is all "─".
func sidebarVisibleInView(m mainModel) bool {
	return strings.Contains(m.View().Content, "┬")
}

func TestHiddenTreeSearchClickNearLeftEdgeDoesNotShowFileTree(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	// Hide file tree via toggle key.
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "e", Code: 'e'}))
	// Enter search mode via search key.
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "t", Code: 't'}))

	// Click near the left edge (X=1), which is inside the search box, not
	// on the hidden grab line (X=0).
	m = updateMainModel(t, m, tea.MouseClickMsg(tea.Mouse{X: 1, Y: 1, Button: tea.MouseLeft}))

	// Exit search mode so we can observe whether the file tree is visible.
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))

	if sidebarVisibleInView(m) {
		t.Fatal("expected left-edge click during hidden-tree search to leave the file tree hidden")
	}
}

func TestHiddenSidebarGrabStillShowsFileTreeWhenNotSearching(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	// Hide file tree via toggle key.
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "e", Code: 'e'}))

	// Click on the hidden grab line at X=0 (the "│" column when sidebar is hidden).
	m = updateMainModel(t, m, tea.MouseClickMsg(tea.Mouse{X: 0, Y: 1, Button: tea.MouseLeft}))

	if !sidebarVisibleInView(m) {
		t.Fatal("expected left-edge click on the hidden sidebar grab line to show the file tree")
	}
}

// Regression: clicking the first column of the diff viewer (one column right
// of the hidden sidebar's grab line) must not reopen the file tree — the
// sidebar grab zone must not extend into diff-viewer columns or selection
// starting just past the divider becomes impossible.
func TestHiddenSidebarGrabDoesNotConsumeDiffViewerClicks(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	// Hide file tree via toggle key.
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "e", Code: 'e'}))

	// Click one column right of the hidden grab line.
	m = updateMainModel(t, m, tea.MouseClickMsg(tea.Mouse{X: 1, Y: 1, Button: tea.MouseLeft}))

	// File tree should remain hidden.
	if sidebarVisibleInView(m) {
		t.Fatal(
			"expected click one column right of the hidden grab line to fall through to the diff viewer",
		)
	}

	// Dragging should not have started; verify by sending a motion event
	// and checking the sidebar remains hidden.
	m = updateMainModel(t, m, tea.MouseMotionMsg(tea.Mouse{
		X: 40, Y: 1, Button: tea.MouseLeft,
	}))
	if sidebarVisibleInView(m) {
		t.Fatal(
			"expected click one column right of the hidden grab line to not start a sidebar drag",
		)
	}
}

// Regression: clicking one or two columns to the right of the "│" divider
// between the file tree and the diff viewer must start a diff selection
// rather than initiating a sidebar resize drag.
func TestDiffViewerClickJustRightOfDividerDoesNotStartDragging(t *testing.T) {
	for _, offset := range []int{1, 2} {
		m := newTestMainModel(t)
		m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 160, Height: 40})

		sidebarWidth := sidebarWidthFromView(m)
		x := sidebarWidth + offset

		m = updateMainModel(t, m, tea.MouseClickMsg(tea.Mouse{
			X: x, Y: 1, Button: tea.MouseLeft,
		}))

		// Dragging should not have started; verify by sending a motion event
		// and checking the sidebar width does not change.
		widthBeforeMotion := sidebarWidthFromView(m)
		m = updateMainModel(t, m, tea.MouseMotionMsg(tea.Mouse{
			X: 50, Y: 1, Button: tea.MouseLeft,
		}))
		widthAfterMotion := sidebarWidthFromView(m)
		if widthBeforeMotion != widthAfterMotion {
			t.Fatalf(
				"offset=%d: expected click %d col(s) right of divider to not start a sidebar drag, but sidebar width changed from %d to %d",
				offset,
				offset,
				widthBeforeMotion,
				widthAfterMotion,
			)
		}
	}
}

func TestSearchSidebarBorderClickDoesNotStartDragging(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	// Enter search mode via search key.
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "t", Code: 't'}))

	// Click at the sidebar border position for the search-mode sidebar.
	sidebarWidth := sidebarWidthFromView(m)
	m = updateMainModel(t, m, tea.MouseClickMsg(tea.Mouse{
		X:      sidebarWidth,
		Y:      1,
		Button: tea.MouseLeft,
	}))

	// Dragging should not have started; verify by sending a motion event
	// and checking the sidebar width does not change.
	widthBeforeMotion := sidebarWidthFromView(m)
	m = updateMainModel(t, m, tea.MouseMotionMsg(tea.Mouse{
		X:      60,
		Y:      1,
		Button: tea.MouseLeft,
	}))
	widthAfterMotion := sidebarWidthFromView(m)
	if widthBeforeMotion != widthAfterMotion {
		t.Fatal("expected sidebar dragging to stay disabled while searching")
	}
}

func TestSearchSidebarDragMotionIsIgnored(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	// Start sidebar drag by clicking the visible sidebar border.
	sidebarWidth := sidebarWidthFromView(m)
	m = updateMainModel(t, m, tea.MouseClickMsg(tea.Mouse{
		X:      sidebarWidth,
		Y:      5,
		Button: tea.MouseLeft,
	}))

	// Enter search mode while dragging. This exercises a state
	// (draggingSidebar=true + searching=true) that is reachable through the
	// public API: the user clicks the sidebar border, then presses the
	// search key while still holding the mouse button.
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "t", Code: 't'}))

	// Send a drag motion — the sidebar drag handler must clear the drag
	// state and ignore the resize because we are in search mode.
	widthBeforeMotion := sidebarWidthFromView(m)
	m = updateMainModel(t, m, tea.MouseMotionMsg(tea.Mouse{
		X:      40,
		Y:      1,
		Button: tea.MouseLeft,
	}))
	widthAfterMotion := sidebarWidthFromView(m)
	if widthBeforeMotion != widthAfterMotion {
		t.Fatalf(
			"expected file tree width to remain %d, got %d",
			widthBeforeMotion,
			widthAfterMotion,
		)
	}
}

func TestInitialActivePanelWhenFileTreeIsHidden(t *testing.T) {
	zone.NewGlobal()

	cfg := config.DefaultConfig()
	cfg.UI.ShowFileTree = false

	data, err := os.ReadFile("../../examples/multiple_files.txt")
	if err != nil {
		t.Fatal(err)
	}

	m := New(string(data), cfg)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	// When showFileTree is false, the sidebar should be hidden.
	if sidebarVisibleInView(m) {
		t.Fatal("expected file tree to be hidden when showFileTree is false")
	}

	// activePanel cannot be reliably observed through the public API
	// when the file tree is hidden: Tab does not switch panels when
	// isShowingFileTree is false, and up/down keys scroll the diff viewer
	// regardless of which panel would be active. The panel value only
	// becomes observable once the file tree is toggled visible.
	// Directly read activePanel as this state is not testable through View.
	if m.activePanel != DiffViewerPanel {
		t.Fatalf(
			"expected activePanel to be DiffViewerPanel when showFileTree is false, got %v",
			m.activePanel,
		)
	}
}

func TestToggleFileTreeKeyboard(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})

	// Initially the file tree should be visible
	viewBefore := m.View().Content
	if viewBefore == "" {
		t.Fatal("expected non-empty initial view")
	}

	// Press "e" to hide file tree
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "e", Code: 'e'}))
	viewHidden := m.View().Content
	if viewHidden == "" {
		t.Fatal("expected non-empty view after hiding file tree")
	}

	// Press "e" again to show file tree
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "e", Code: 'e'}))
	viewShown := m.View().Content
	if viewShown == "" {
		t.Fatal("expected non-empty view after showing file tree")
	}
}

func TestUpdateSwitchPanel(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	// Default activePanel is FileTreePanel; no need to set it.

	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	// After tab, diff viewer should be active
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after switching panel")
	}
}

func TestUpdateSwitchPanelWhenTreeHidden(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	// Hide file tree via toggle key — this also sets activePanel to DiffViewerPanel.
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "e", Code: 'e'}))

	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after tab with tree hidden")
	}
}

func TestUpdateSwitchPanelFromDiffToTree(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	// File tree is visible by default. Switch to DiffViewerPanel via Tab.
	m = switchToPanel(t, m, DiffViewerPanel)

	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after tab from diff viewer to file tree")
	}
}

func TestMouseClickSidebarBorderThroughUpdate(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 160, Height: 40})

	sidebarWidth := sidebarWidthFromView(m)
	m = updateMainModel(t, m, tea.MouseClickMsg(tea.Mouse{
		X:      sidebarWidth,
		Y:      5,
		Button: tea.MouseLeft,
	}))

	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after clicking sidebar border")
	}
}

func TestMouseClickHiddenSidebarGrabLineThroughUpdate(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	// Hide file tree via toggle key.
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "e", Code: 'e'}))

	m = updateMainModel(t, m, tea.MouseClickMsg(tea.Mouse{
		X:      0,
		Y:      5,
		Button: tea.MouseLeft,
	}))

	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after clicking hidden sidebar grab line")
	}
}

func TestMouseMotionSidebarDragThroughUpdate(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 160, Height: 40})

	// Start sidebar drag by clicking the visible sidebar border.
	sidebarWidth := sidebarWidthFromView(m)
	m = updateMainModel(t, m, tea.MouseClickMsg(tea.Mouse{
		X:      sidebarWidth,
		Y:      5,
		Button: tea.MouseLeft,
	}))

	// Drag to a new position (wide enough to trigger resize)
	newX := 50
	m = updateMainModel(t, m, tea.MouseMotionMsg(tea.Mouse{
		X:      newX,
		Y:      5,
		Button: tea.MouseLeft,
	}))

	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after sidebar drag motion")
	}
}

func TestMouseSidebarDragHidesBelowThreshold(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	// Start sidebar drag by clicking the visible sidebar border.
	sidebarWidth := sidebarWidthFromView(m)
	m = updateMainModel(t, m, tea.MouseClickMsg(tea.Mouse{
		X:      sidebarWidth,
		Y:      5,
		Button: tea.MouseLeft,
	}))

	// Drag to very small width
	m = updateMainModel(t, m, tea.MouseMotionMsg(tea.Mouse{
		X:      2,
		Y:      5,
		Button: tea.MouseLeft,
	}))

	// Sidebar should be hidden after dragging below threshold.
	if sidebarVisibleInView(m) {
		t.Fatal("expected sidebar to be hidden after dragging below hide threshold")
	}
}

func TestMouseSidebarDragDuringSearch(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	// Start sidebar drag by clicking the visible sidebar border.
	sidebarWidth := sidebarWidthFromView(m)
	m = updateMainModel(t, m, tea.MouseClickMsg(tea.Mouse{
		X:      sidebarWidth,
		Y:      5,
		Button: tea.MouseLeft,
	}))

	// Enter search mode while dragging — the drag should be cleared.
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "t", Code: 't'}))

	m = updateMainModel(t, m, tea.MouseMotionMsg(tea.Mouse{
		X:      40,
		Y:      5,
		Button: tea.MouseLeft,
	}))

	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after sidebar drag during search")
	}
}

func TestMouseSidebarDragSameWidth(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	// Start sidebar drag by clicking the visible sidebar border.
	currentWidth := sidebarWidthFromView(m)
	m = updateMainModel(t, m, tea.MouseClickMsg(tea.Mouse{
		X:      currentWidth,
		Y:      5,
		Button: tea.MouseLeft,
	}))

	// Drag to same position as current sidebar width (no resize should happen).
	m = updateMainModel(t, m, tea.MouseMotionMsg(tea.Mouse{
		X:      currentWidth,
		Y:      5,
		Button: tea.MouseLeft,
	}))

	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after sidebar drag with same width")
	}
}

func TestMouseReleaseStopsSidebarDrag(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	// Start sidebar drag by clicking the visible sidebar border.
	sidebarWidth := sidebarWidthFromView(m)
	m = updateMainModel(t, m, tea.MouseClickMsg(tea.Mouse{
		X:      sidebarWidth,
		Y:      5,
		Button: tea.MouseLeft,
	}))

	m = updateMainModel(t, m, tea.MouseReleaseMsg(tea.Mouse{
		X:      10,
		Y:      10,
		Button: tea.MouseLeft,
	}))

	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after mouse release stops sidebar drag")
	}

	// After release, a motion event should not resize the sidebar.
	widthBeforeMotion := sidebarWidthFromView(m)
	m = updateMainModel(t, m, tea.MouseMotionMsg(tea.Mouse{
		X:      50,
		Y:      5,
		Button: tea.MouseLeft,
	}))
	widthAfterMotion := sidebarWidthFromView(m)
	if widthBeforeMotion != widthAfterMotion {
		t.Fatal("expected sidebar width to remain unchanged after release stops drag")
	}
}

func TestMainContentHeightTable(t *testing.T) {
	cases := []struct {
		name        string
		hideHeader  bool
		hideFooter  bool
		totalHeight int
		want        int
	}{
		{"with_header", false, false, 40, 37},
		{"without_header", true, false, 40, 39},
		{"without_footer", false, true, 40, 38},
		{"without_both", true, true, 40, 40},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			zone.NewGlobal()
			cfg := config.DefaultConfig()
			cfg.UI.HideHeader = tc.hideHeader
			cfg.UI.HideFooter = tc.hideFooter

			data, err := os.ReadFile("../../examples/multiple_files.txt")
			if err != nil {
				t.Fatal(err)
			}

			m := New(string(data), cfg)
			m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: tc.totalHeight})

			// Derive mainContentHeight from the rendered View.
			// The View layout is: header (1 line if shown) + separator (1 line)
			// + mainContent + footer (1 line if shown).
			viewHeight := lipgloss.Height(m.View().Content)
			headerLines := 0
			if !tc.hideHeader {
				headerLines = 1
			}
			footerLines := 0
			if !tc.hideFooter {
				footerLines = 1
			}
			got := viewHeight - headerLines - 1 - footerLines
			if got != tc.want {
				t.Fatalf(
					"expected mainContentHeight %d, got %d (viewHeight=%d)",
					tc.want,
					got,
					viewHeight,
				)
			}
		})
	}
}
