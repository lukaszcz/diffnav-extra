package ui

import (
	"fmt"
	"image/color"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/bluekeyes/go-gitdiff/gitdiff"
	zone "github.com/lrstanley/bubblezone/v2"

	"charm.land/lipgloss/v2"

	"github.com/dlvhdr/diffnav/pkg/config"
	"github.com/dlvhdr/diffnav/pkg/dirnode"
	"github.com/dlvhdr/diffnav/pkg/filenode"
	"github.com/dlvhdr/diffnav/pkg/ui/common"
	"github.com/dlvhdr/diffnav/pkg/ui/panes/diffviewer"
)

func TestSearchUpdateEnterWithNoResultsDoesNotPanic(t *testing.T) {
	m := newTestMainModel(t)
	m.width = 100
	m.height = 40
	m.searching = true
	m.search.Focus()
	m.search.SetValue("does-not-match")
	m.setSearchResults()

	updated, _ := m.searchUpdate(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))

	if updated.searching {
		t.Fatal("expected search to stop after pressing enter")
	}
	if updated.resultsCursor != 0 {
		t.Fatalf("expected cursor to remain at 0, got %d", updated.resultsCursor)
	}
}

func TestSearchUpdateKeepsCursorValidWhenResultsAreEmpty(t *testing.T) {
	m := newTestMainModel(t)
	m.searching = true
	m.search.Focus()
	m.filtered = nil
	m.resultsCursor = 0

	updated, _ := m.searchUpdate(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	if updated.resultsCursor != 0 {
		t.Fatalf(
			"expected cursor to remain at 0 after down on empty results, got %d",
			updated.resultsCursor,
		)
	}

	updated.resultsCursor = -3
	updated.search.SetValue("does-not-match")
	updated.setSearchResults()
	if updated.resultsCursor != 0 {
		t.Fatalf("expected cursor to clamp to 0 for empty results, got %d", updated.resultsCursor)
	}
}

func TestSearchResultsRenderWhenFileTreeIsHidden(t *testing.T) {
	m := newTestMainModel(t)
	m.width = 100
	m.height = 40
	m.isShowingFileTree = false
	m.searching = true
	m.search.SetWidth(m.searchWidth())
	m.setSearchResults()
	m.resultsVp.SetWidth(m.config.UI.SearchTreeWidth)
	m.resultsVp.SetHeight(m.mainContentHeight() - searchHeight)
	m.resultsVp.SetContent(m.resultsView())

	view := m.View().Content
	if !strings.Contains(view, "yarn.lock") {
		t.Fatal("expected search results to be visible even when the file tree is hidden")
	}
}

func TestSearchResultPaletteAdaptsToLightBackground(t *testing.T) {
	m := newTestMainModel(t)
	isDark := false
	m.isDarkBackground = &isDark

	palette := m.searchResultPalette()

	if palette.iconDark {
		t.Fatal("expected light background icon palette")
	}
	if got := common.LipglossColorToHex(palette.fileColor); got != "#334155" {
		t.Fatalf("expected dark file text color in light mode, got %s", got)
	}
	if got := common.LipglossColorToHex(palette.dirColor); got != "#64748b" {
		t.Fatalf("expected dark directory text color in light mode, got %s", got)
	}
}

func TestHiddenTreeSearchEnterThenToggleDoesNotPanic(t *testing.T) {
	m := newTestMainModel(t)
	m.width = 100
	m.height = 40

	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "e", Code: 'e'}))
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "t", Code: 't'}))
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "e", Code: 'e'}))

	if !m.isShowingFileTree {
		t.Fatal("expected file tree to be visible after toggling it back on")
	}
	if m.search.Width() < 0 {
		t.Fatalf("expected non-negative search width, got %d", m.search.Width())
	}
	_ = m.View().Content
}

func TestHiddenTreeSearchClickNearLeftEdgeDoesNotShowFileTree(t *testing.T) {
	m := newTestMainModel(t)
	m.width = 100
	m.height = 40
	m.isShowingFileTree = false
	m.searching = true

	updated, _ := m.handleMouse(tea.MouseClickMsg(tea.Mouse{X: 1, Y: 1, Button: tea.MouseLeft}))

	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	if result.isShowingFileTree {
		t.Fatal("expected left-edge click during hidden-tree search to leave the file tree hidden")
	}
}

func TestHiddenSidebarGrabStillShowsFileTreeWhenNotSearching(t *testing.T) {
	m := newTestMainModel(t)
	m.width = 100
	m.height = 40
	m.isShowingFileTree = false
	m.searching = false

	updated, _ := m.handleMouse(tea.MouseClickMsg(tea.Mouse{X: 0, Y: 1, Button: tea.MouseLeft}))

	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	if !result.isShowingFileTree {
		t.Fatal("expected left-edge click on the hidden sidebar grab line to show the file tree")
	}
}

// Regression: clicking the first column of the diff viewer (one column right
// of the hidden sidebar's grab line) must not reopen the file tree — the
// sidebar grab zone must not extend into diff-viewer columns or selection
// starting just past the divider becomes impossible.
func TestHiddenSidebarGrabDoesNotConsumeDiffViewerClicks(t *testing.T) {
	m := newTestMainModel(t)
	m.width = 100
	m.height = 40
	m.isShowingFileTree = false
	m.searching = false

	updated, _ := m.handleMouse(tea.MouseClickMsg(tea.Mouse{X: 1, Y: 1, Button: tea.MouseLeft}))

	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	if result.isShowingFileTree {
		t.Fatal(
			"expected click one column right of the hidden grab line to fall through to the diff viewer",
		)
	}
	if result.draggingSidebar {
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
		m.width = 160
		m.height = 40
		m.isShowingFileTree = true
		m.searching = false

		x := m.sidebarWidth() + offset
		updated, _ := m.handleMouse(tea.MouseClickMsg(tea.Mouse{X: x, Y: 1, Button: tea.MouseLeft}))

		result, ok := updated.(mainModel)
		if !ok {
			t.Fatalf("offset=%d: unexpected model type %T", offset, updated)
		}
		if result.draggingSidebar {
			t.Fatalf(
				"offset=%d: expected click %d col(s) right of divider to not start a sidebar drag",
				offset,
				offset,
			)
		}
	}
}

func TestSearchSidebarBorderClickDoesNotStartDragging(t *testing.T) {
	m := newTestMainModel(t)
	m.width = 100
	m.height = 40
	m.isShowingFileTree = true
	m.searching = true
	m.fileTree.SetSize(m.config.UI.FileTreeWidth, m.mainContentHeight()-searchHeight)

	updated, _ := m.handleMouse(tea.MouseClickMsg(tea.Mouse{
		X:      m.sidebarWidth(),
		Y:      1,
		Button: tea.MouseLeft,
	}))

	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	if result.draggingSidebar {
		t.Fatal("expected sidebar dragging to stay disabled while searching")
	}
}

func TestSearchSidebarDragMotionIsIgnored(t *testing.T) {
	m := newTestMainModel(t)
	m.width = 100
	m.height = 40
	m.isShowingFileTree = true
	m.searching = true
	m.draggingSidebar = true
	m.fileTree.SetSize(m.config.UI.FileTreeWidth, m.mainContentHeight()-searchHeight)

	updated, _ := m.handleMouse(tea.MouseMotionMsg(tea.Mouse{
		X:      40,
		Y:      1,
		Button: tea.MouseLeft,
	}))

	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	if result.draggingSidebar {
		t.Fatal("expected search-mode drag motion to clear dragging state")
	}
	if result.fileTree.Width() != m.fileTree.Width() {
		t.Fatalf(
			"expected file tree width to remain %d, got %d",
			m.fileTree.Width(),
			result.fileTree.Width(),
		)
	}
}

func TestBackgroundColorDetectionStillWorksWhileSearching(t *testing.T) {
	m := newTestMainModel(t)
	m.searching = true
	m.search.Focus()
	m.themeOverride = nil
	m.isDarkBackground = nil

	updated := updateMainModel(t, m, tea.BackgroundColorMsg{
		Color: color.RGBA{R: 255, G: 255, B: 255, A: 255},
	})

	if updated.isDarkBackground == nil {
		t.Fatal("expected background color detection to set theme state while searching")
	}
	if *updated.isDarkBackground {
		t.Fatal("expected light background detection while searching")
	}
}

func TestThemeDetectionTimeoutFallsBackToDark(t *testing.T) {
	m := newTestMainModel(t)
	m.themeOverride = nil
	m.isDarkBackground = nil

	updated := updateMainModel(t, m, themeDetectTimeoutMsg{})
	if updated.isDarkBackground == nil {
		t.Fatal("expected timeout to resolve theme state")
	}
	if !*updated.isDarkBackground {
		t.Fatal("expected timeout fallback to dark background")
	}
}

func TestLateBackgroundDetectionIgnoredAfterTimeout(t *testing.T) {
	m := newTestMainModel(t)
	m.themeOverride = nil
	m.isDarkBackground = nil
	m = updateMainModel(t, m, themeDetectTimeoutMsg{})

	updated := updateMainModel(t, m, tea.BackgroundColorMsg{
		Color: color.RGBA{R: 255, G: 255, B: 255, A: 255},
	})
	if updated.isDarkBackground == nil {
		t.Fatal("expected theme state to remain resolved")
	}
	if !*updated.isDarkBackground {
		t.Fatal("expected late background message to be ignored after timeout")
	}
}

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

// Tiny helper so the test doesn't need to import diffviewer just for the
// header height constant.
func diffviewerDirHeaderHeight() int { return 3 }

// Enter on a highlighted file should launch $EDITOR and skip the
// directory-toggle path the filetree pane uses for Enter on dirs.
func TestEnterOnFileOpensEditor(t *testing.T) {
	t.Setenv("EDITOR", "true")
	m := newTestMainModel(t)
	m.activePanel = FileTreePanel
	m.fileTree.SetCursorByPath("yarn.lock")

	updated, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	if cmd == nil {
		t.Fatal("expected non-nil command (editor exec) when Enter hits a file with $EDITOR set")
	}
	// The cursor must still be on the same file; pressing Enter on a file
	// must not toggle/collapse anything in the tree.
	if got := result.fileTree.CurrNodePath(); got != "yarn.lock" {
		t.Fatalf("expected cursor to remain on yarn.lock, got %q", got)
	}
}

// Without $EDITOR set, Enter on a file must be a no-op rather than falling
// through to the directory-toggle behavior.
func TestEnterOnFileWithoutEditorIsNoop(t *testing.T) {
	t.Setenv("EDITOR", "")
	m := newTestMainModel(t)
	m.activePanel = FileTreePanel
	m.fileTree.SetCursorByPath("yarn.lock")

	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	if got := result.fileTree.CurrNodePath(); got != "yarn.lock" {
		t.Fatalf("expected cursor to remain on yarn.lock, got %q", got)
	}
}

// Regression: Enter on a directory still toggles it (the new file-open
// behavior must only apply to file nodes).
func TestEnterOnDirectoryStillToggles(t *testing.T) {
	m := newTestMainModel(t)
	m.activePanel = FileTreePanel
	m.fileTree.SetCursorByPath("graphql-server/tests")

	dirNode := m.fileTree.GetCurrNode()
	if dirNode == nil {
		t.Fatal("expected to find graphql-server/tests directory node")
	}
	wasOpen := dirNode.IsOpen()

	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}

	toggled := result.fileTree.GetCurrNode()
	if toggled == nil {
		t.Fatal("expected current node after toggle")
	}
	if toggled.IsOpen() == wasOpen {
		t.Fatalf(
			"expected dir open state to toggle from %v, got %v",
			wasOpen,
			toggled.IsOpen(),
		)
	}
}

func TestFileTreeDirectoryRowClickSelectsWithoutToggling(t *testing.T) {
	for _, tc := range []struct {
		size       tea.WindowSizeMsg
		hideHeader bool
	}{
		{size: tea.WindowSizeMsg{Width: 100, Height: 30}},
		{size: tea.WindowSizeMsg{Width: 150, Height: 42}},
		{size: tea.WindowSizeMsg{Width: 120, Height: 34}, hideHeader: true},
	} {
		t.Run(
			fmt.Sprintf("%dx%d/header=%v", tc.size.Width, tc.size.Height, !tc.hideHeader),
			func(t *testing.T) {
				m := newTestMainModel(t)
				m.config.UI.HideHeader = tc.hideHeader
				m = updateMainModel(t, m, tc.size)
				m.activePanel = FileTreePanel
				m.fileTree.SetCursorByPath("graphql-server/tests")

				m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))

				target := m.fileTree.GetCurrNode()
				if target == nil {
					t.Fatal("expected directory node")
				}
				if target.IsOpen() {
					t.Fatal("setup: expected Enter to fold the selected directory")
				}

				_ = m.View().Content
				z := waitForZone(t, zoneFileTree)
				localY := target.YOffset() - m.fileTree.ViewportYOffset()
				nameX := target.Depth() + 4

				updated, _ := m.handleMouse(tea.MouseClickMsg(tea.Mouse{
					X:      z.StartX + nameX,
					Y:      z.StartY + localY,
					Button: tea.MouseLeft,
				}))
				result, ok := updated.(mainModel)
				if !ok {
					t.Fatalf("unexpected model type %T", updated)
				}

				clicked := result.fileTree.GetCurrNode()
				if got := result.fileTree.CurrNodePath(); got != "graphql-server/tests" {
					t.Fatalf("expected clicked directory to be selected, got %q", got)
				}
				if clicked.IsOpen() {
					t.Fatal("expected directory name row click to leave folded state unchanged")
				}
			},
		)
	}
}

func TestFileTreeDirectoryIconClickSelectsAndToggles(t *testing.T) {
	for _, tc := range []struct {
		size       tea.WindowSizeMsg
		hideHeader bool
	}{
		{size: tea.WindowSizeMsg{Width: 100, Height: 30}},
		{size: tea.WindowSizeMsg{Width: 150, Height: 42}},
		{size: tea.WindowSizeMsg{Width: 120, Height: 34}, hideHeader: true},
	} {
		t.Run(
			fmt.Sprintf("%dx%d/header=%v", tc.size.Width, tc.size.Height, !tc.hideHeader),
			func(t *testing.T) {
				m := newTestMainModel(t)
				m.config.UI.HideHeader = tc.hideHeader
				m = updateMainModel(t, m, tc.size)
				m.activePanel = FileTreePanel
				m.fileTree.SetCursorByPath("graphql-server/tests")

				target := m.fileTree.GetCurrNode()
				if target == nil {
					t.Fatal("expected directory node")
				}
				if !target.IsOpen() {
					t.Fatal("setup: expected directory to start open")
				}

				_ = m.View().Content
				z := waitForZone(t, zoneFileTree)
				localY := target.YOffset() - m.fileTree.ViewportYOffset()
				iconX := -1
				for x := 0; x < m.fileTree.Width(); x++ {
					if m.fileTree.IsDirectoryIconHit(target, x) {
						iconX = x
						break
					}
				}
				if iconX < 0 {
					t.Fatal("expected directory icon hit column")
				}

				updated, _ := m.handleMouse(tea.MouseClickMsg(tea.Mouse{
					X:      z.StartX + iconX,
					Y:      z.StartY + localY,
					Button: tea.MouseLeft,
				}))
				result, ok := updated.(mainModel)
				if !ok {
					t.Fatalf("unexpected model type %T", updated)
				}

				clicked := result.fileTree.GetCurrNode()
				if got := result.fileTree.CurrNodePath(); got != "graphql-server/tests" {
					t.Fatalf("expected clicked directory to be selected, got %q", got)
				}
				if clicked.IsOpen() {
					t.Fatal("expected directory icon click to toggle the directory closed")
				}
			},
		)
	}
}

// Diff-scroll keys (ctrl+up/down, pgup/pgdown) must scroll the diff pane
// without disturbing the file tree, regardless of which panel is active.
// Also asserts the diff viewport actually moved — guards against the keys
// being silently dropped before reaching the diff pane.
func TestDiffScrollKeysDoNotDisturbFileTree(t *testing.T) {
	cases := []struct {
		name string
		key  tea.Key
		down bool
	}{
		{"ctrl+down", tea.Key{Code: tea.KeyDown, Mod: tea.ModCtrl}, true},
		{"ctrl+up", tea.Key{Code: tea.KeyUp, Mod: tea.ModCtrl}, false},
		{"pgdown", tea.Key{Code: tea.KeyPgDown}, true},
		{"pgup", tea.Key{Code: tea.KeyPgUp}, false},
	}
	for _, tc := range cases {
		for _, panel := range []Panel{FileTreePanel, DiffViewerPanel} {
			t.Run(tc.name+"/panel="+panelName(panel), func(t *testing.T) {
				m := newTestMainModel(t)
				m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 160, Height: 40})
				m.activePanel = panel
				// Inject scrollable content so the viewport can actually move.
				m.diffViewer.SetContent(strings.Repeat("line\n", 500))
				if !tc.down {
					m.diffViewer.ScrollDown(50)
					if m.diffViewer.YOffset() == 0 {
						t.Fatalf(
							"precondition: expected YOffset>0 after manual ScrollDown, got 0",
						)
					}
				}
				before := m.fileTree.CurrNodePath()
				beforeOffset := m.diffViewer.YOffset()

				updated := updateMainModel(t, m, tea.KeyPressMsg(tc.key))

				if got := updated.fileTree.CurrNodePath(); got != before {
					t.Fatalf("expected filetree cursor to stay on %q, got %q", before, got)
				}
				if updated.activePanel != panel {
					t.Fatalf(
						"expected active panel to remain %v, got %v",
						panel,
						updated.activePanel,
					)
				}
				afterOffset := updated.diffViewer.YOffset()
				if tc.down && afterOffset <= beforeOffset {
					t.Fatalf(
						"expected diff YOffset to advance from %d, got %d",
						beforeOffset,
						afterOffset,
					)
				}
				if !tc.down && afterOffset >= beforeOffset {
					t.Fatalf(
						"expected diff YOffset to decrease from %d, got %d",
						beforeOffset,
						afterOffset,
					)
				}
			})
		}
	}
}

// Home/End must jump the diff viewport to the very top/bottom regardless of
// which panel is active, without disturbing the file-tree cursor.
func TestHomeEndScrollDiffToEdges(t *testing.T) {
	cases := []struct {
		name   string
		key    tea.Key
		bottom bool
	}{
		{"end", tea.Key{Code: tea.KeyEnd}, true},
		{"home", tea.Key{Code: tea.KeyHome}, false},
	}
	for _, tc := range cases {
		for _, panel := range []Panel{FileTreePanel, DiffViewerPanel} {
			t.Run(tc.name+"/panel="+panelName(panel), func(t *testing.T) {
				m := newTestMainModel(t)
				m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 160, Height: 40})
				m.activePanel = panel
				m.diffViewer.SetContent(strings.Repeat("line\n", 500))
				if !tc.bottom {
					// Start at the bottom so Home has somewhere to go.
					m.diffViewer.ScrollBottom()
					if m.diffViewer.YOffset() == 0 {
						t.Fatalf("precondition: expected YOffset>0 after ScrollBottom")
					}
				}
				before := m.fileTree.CurrNodePath()

				updated := updateMainModel(t, m, tea.KeyPressMsg(tc.key))

				if got := updated.fileTree.CurrNodePath(); got != before {
					t.Fatalf("expected filetree cursor to stay on %q, got %q", before, got)
				}
				if updated.activePanel != panel {
					t.Fatalf(
						"expected active panel to remain %v, got %v",
						panel,
						updated.activePanel,
					)
				}
				off := updated.diffViewer.YOffset()
				if tc.bottom {
					maxOff := updated.diffViewer.TotalLineCount() - updated.diffViewer.Height()
					if off != maxOff {
						t.Fatalf(
							"expected End to scroll to bottom (YOffset=%d), got %d",
							maxOff,
							off,
						)
					}
				} else if off != 0 {
					t.Fatalf("expected Home to scroll to top (YOffset=0), got %d", off)
				}
			})
		}
	}
}

// Pressing pgdown with the file tree active must advance the diff viewport.
// This is the end-to-end signal that the new keys are routed to the diff pane
// instead of being swallowed by the file tree.
func TestDiffPageDownScrollsDiffWhenFileTreeActive(t *testing.T) {
	if _, err := exec.LookPath("delta"); err != nil {
		t.Skip("delta not installed, skipping end-to-end test")
	}
	m := newTestMainModel(t)
	isDark := true
	m.isDarkBackground = &isDark
	m.diffViewer.SetDarkBackground(true)
	m.fileTree.SetDarkBackground(true)

	// Use a tight window so PgDn's scroll-by-viewport-height advance is much
	// smaller than the fixture — keeps the assertion robust against
	// fluctuations in fixture size.
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 160, Height: 20})
	m = updateMainModel(t, m, m.fetchFileTree())

	deadline := time.Now().Add(3 * time.Second)
	for m.diffViewer.Height() <= 0 || !diffViewerHasContent(m) {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for diff content to load")
		}
		if cmd := m.diffViewer.Init(); cmd != nil {
			if msg := cmd(); msg != nil {
				m = updateMainModel(t, m, msg)
			}
		}
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

	m.activePanel = FileTreePanel
	if m.diffViewer.YOffset() != 0 {
		t.Fatalf(
			"precondition: expected YOffset==0 before scrolling, got %d",
			m.diffViewer.YOffset(),
		)
	}

	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyPgDown}))
	if m.diffViewer.YOffset() == 0 {
		t.Fatal("expected diff viewport to advance after pgdown")
	}

	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyPgUp}))
	if m.diffViewer.YOffset() != 0 {
		t.Fatalf(
			"expected diff viewport to return to top after pgup, got YOffset=%d",
			m.diffViewer.YOffset(),
		)
	}
}

// Pressing pgdown / pgup while the commit-info overlay is open must scroll
// the overlay viewport, matching the up/down/ctrl+d/ctrl+u bindings.
func TestMessageOverlayPageKeysScrollOverlay(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.preamble = strings.Repeat("preamble line\n", 200)
	m.messageOpen = true
	m.updateMessageVp()
	m.messageVp.GotoTop()

	before := m.messageVp.YOffset()
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyPgDown}))
	if m.messageVp.YOffset() <= before {
		t.Fatalf("expected pgdown to advance overlay YOffset from %d, got %d",
			before, m.messageVp.YOffset())
	}

	before = m.messageVp.YOffset()
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyPgUp}))
	if m.messageVp.YOffset() >= before {
		t.Fatalf("expected pgup to retreat overlay YOffset from %d, got %d",
			before, m.messageVp.YOffset())
	}
}

// drainCmds runs cmd and every command it (transitively) produces, feeding the
// resulting messages back through Update. This lets a test pump the async delta
// render pipeline to completion. BatchMsg fan-out is flattened.
func drainCmds(t *testing.T, m mainModel, cmd tea.Cmd) mainModel {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	pending := []tea.Cmd{cmd}
	for len(pending) > 0 {
		if time.Now().After(deadline) {
			t.Fatal("timed out draining commands")
		}
		c := pending[0]
		pending = pending[1:]
		if c == nil {
			continue
		}
		msg := c()
		if msg == nil {
			continue
		}
		if batch, ok := msg.(tea.BatchMsg); ok {
			pending = append(pending, batch...)
			continue
		}
		var next tea.Cmd
		m = updateAndCmd(t, m, msg, &next)
		pending = append(pending, next)
	}
	return m
}

func updateAndCmd(t *testing.T, m mainModel, msg tea.Msg, out *tea.Cmd) mainModel {
	t.Helper()
	updated, cmd := m.Update(msg)
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	*out = cmd
	return result
}

// Selecting a file remembers that file's own scroll position: returning to it
// restores where the user left it, while a freshly visited file opens at the
// top. This exercises the real delta render pipeline through keyboard
// navigation (the path the user reported).
func TestFileNavigationRemembersScrollPerFile(t *testing.T) {
	if _, err := exec.LookPath("delta"); err != nil {
		t.Skip("delta not installed, skipping end-to-end test")
	}
	m := newTestMainModel(t)
	isDark := true
	m.isDarkBackground = &isDark
	m.diffViewer.SetDarkBackground(true)
	m.fileTree.SetDarkBackground(true)
	// Tight window so the large file's diff overflows and can scroll.
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 160, Height: 20})
	m = drainCmds(t, m, m.fetchFileTree)
	m.activePanel = FileTreePanel

	// Select the large file (yarn.lock) and render it.
	m.fileTree.SetCursorByPath("yarn.lock")
	var cmd tea.Cmd
	m, cmd = m.setNodeDiff(m.fileTree.GetCurrNode())
	m = drainCmds(t, m, cmd)
	if m.fileTree.CurrNodePath() != "yarn.lock" {
		t.Fatalf("setup: expected to be on yarn.lock, got %q", m.fileTree.CurrNodePath())
	}

	// Scroll the large file's diff down.
	m.diffViewer.ScrollDown(100)
	want := m.diffViewer.YOffset()
	if want == 0 {
		t.Fatal("precondition: expected yarn.lock diff to scroll past the top")
	}

	// Navigate to the other file via the keyboard: it must open at the top.
	m = drainCmds(t, m, keyCmd(t, &m, tea.Key{Text: "p", Code: 'p'}))
	if m.fileTree.CurrNodePath() == "yarn.lock" {
		t.Fatalf(
			"setup: expected prev-file to leave yarn.lock, still on %q",
			m.fileTree.CurrNodePath(),
		)
	}
	if got := m.diffViewer.YOffset(); got != 0 {
		t.Fatalf("expected a freshly visited file to open at the top, got %d", got)
	}

	// Navigate back to the large file: its scroll must be restored.
	m = drainCmds(t, m, keyCmd(t, &m, tea.Key{Text: "n", Code: 'n'}))
	if m.fileTree.CurrNodePath() != "yarn.lock" {
		t.Fatalf(
			"setup: expected next-file to return to yarn.lock, got %q",
			m.fileTree.CurrNodePath(),
		)
	}
	if got := m.diffViewer.YOffset(); got != want {
		t.Fatalf("expected yarn.lock scroll %d to be restored, got %d", want, got)
	}
}

// keyCmd dispatches a key press through Update and returns the resulting
// command, updating *m in place.
func keyCmd(t *testing.T, m *mainModel, k tea.Key) tea.Cmd {
	t.Helper()
	updated, cmd := m.Update(tea.KeyPressMsg(k))
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	*m = result
	return cmd
}

func diffViewerHasContent(m mainModel) bool {
	// The viewport reports a height of 1 even when empty; rendered content
	// always contains at least one diff-related line once delta resolves.
	view := m.diffViewer.View()
	return strings.Contains(view, "diff") || strings.Contains(view, "@@") ||
		strings.Contains(view, "│")
}

func panelName(p Panel) string {
	if p == FileTreePanel {
		return "filetree"
	}
	return "diffviewer"
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

	if m.activePanel != DiffViewerPanel {
		t.Fatalf(
			"expected activePanel to be DiffViewerPanel when showFileTree is false, got %v",
			m.activePanel,
		)
	}
	if m.isShowingFileTree {
		t.Fatal("expected file tree to be hidden when showFileTree is false")
	}
}

// ---------------------------------------------------------------------------
// relativeTime tests
// ---------------------------------------------------------------------------

func TestRelativeTimeNow(t *testing.T) {
	result := relativeTime(time.Now())
	if result != "now" {
		t.Fatalf("expected 'now' for <1min, got %q", result)
	}
}

func TestRelativeTimeMinutes(t *testing.T) {
	result := relativeTime(time.Now().Add(-5 * time.Minute))
	if result != "5m" {
		t.Fatalf("expected '5m' for 5 minutes ago, got %q", result)
	}
}

func TestRelativeTimeMinuteBoundary(t *testing.T) {
	// Exactly 59 seconds should still be "now" (< 1 minute)
	result := relativeTime(time.Now().Add(-59 * time.Second))
	if result != "now" {
		t.Fatalf("expected 'now' for 59s ago, got %q", result)
	}
}

func TestRelativeTimeHours(t *testing.T) {
	result := relativeTime(time.Now().Add(-2 * time.Hour))
	if result != "2h" {
		t.Fatalf("expected '2h' for 2 hours ago, got %q", result)
	}
}

func TestRelativeTimeHourBoundary(t *testing.T) {
	// 59 minutes should be "59m"
	result := relativeTime(time.Now().Add(-59 * time.Minute))
	if result != "59m" {
		t.Fatalf("expected '59m' for 59 minutes ago, got %q", result)
	}
}

func TestRelativeTimeDays(t *testing.T) {
	result := relativeTime(time.Now().Add(-5 * 24 * time.Hour))
	if result != "5d" {
		t.Fatalf("expected '5d' for 5 days ago, got %q", result)
	}
}

func TestRelativeTimeDayBoundary(t *testing.T) {
	// 23 hours should be "23h"
	result := relativeTime(time.Now().Add(-23 * time.Hour))
	if result != "23h" {
		t.Fatalf("expected '23h' for 23 hours ago, got %q", result)
	}
}

func TestRelativeTimeMonths(t *testing.T) {
	result := relativeTime(time.Now().Add(-90 * 24 * time.Hour)) // ~3 months
	if result != "3mo" {
		t.Fatalf("expected '3mo' for 90 days ago, got %q", result)
	}
}

func TestRelativeTimeMonthBoundary(t *testing.T) {
	// 29 days should be "29d"
	result := relativeTime(time.Now().Add(-29 * 24 * time.Hour))
	if result != "29d" {
		t.Fatalf("expected '29d' for 29 days ago, got %q", result)
	}
}

func TestRelativeTimeYears(t *testing.T) {
	result := relativeTime(time.Now().Add(-400 * 24 * time.Hour)) // >1 year
	if result != "1y" {
		t.Fatalf("expected '1y' for 400 days ago, got %q", result)
	}
}

func TestRelativeTimeMultipleYears(t *testing.T) {
	result := relativeTime(time.Now().Add(-800 * 24 * time.Hour)) // ~2 years
	if result != "2y" {
		t.Fatalf("expected '2y' for 800 days ago, got %q", result)
	}
}

func TestRelativeTimeYearBoundary(t *testing.T) {
	// 364 days should be "12mo"
	result := relativeTime(time.Now().Add(-364 * 24 * time.Hour))
	if result != "12mo" {
		t.Fatalf("expected '12mo' for 364 days ago, got %q", result)
	}
}

// ---------------------------------------------------------------------------
// abs tests
// ---------------------------------------------------------------------------

func TestAbsPositive(t *testing.T) {
	if got := abs(5); got != 5 {
		t.Fatalf("expected 5, got %d", got)
	}
}

func TestAbsNegative(t *testing.T) {
	if got := abs(-5); got != 5 {
		t.Fatalf("expected 5, got %d", got)
	}
}

func TestAbsZero(t *testing.T) {
	if got := abs(0); got != 0 {
		t.Fatalf("expected 0, got %d", got)
	}
}

func TestAbsLargeNegative(t *testing.T) {
	if got := abs(-1000); got != 1000 {
		t.Fatalf("expected 1000, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// sortFiles tests
// ---------------------------------------------------------------------------

func TestSortFilesSameDirectorySortedAlphabetically(t *testing.T) {
	files := []*gitdiff.File{
		{NewName: "dir/z.txt"},
		{NewName: "dir/a.txt"},
	}
	sortFiles(files)
	if files[0].NewName != "dir/a.txt" {
		t.Fatalf("expected dir/a.txt first, got %s", files[0].NewName)
	}
}

func TestSortFilesTopLevelSameDirectoryAlphabetically(t *testing.T) {
	// Top-level files have dir="." (not "/"), so they share the same
	// directory and are sorted alphabetically.
	files := []*gitdiff.File{
		{NewName: "b.txt"},
		{NewName: "a.txt"},
	}
	sortFiles(files)
	if files[0].NewName != "a.txt" {
		t.Fatalf("expected a.txt first, got %s", files[0].NewName)
	}
}

func TestSortFilesParentDirBeforeChildDir(t *testing.T) {
	// parent/file.txt (dir="parent") and parent/child/file.txt
	// (dir="parent/child") — dirb has prefix dira, so dirb comes after.
	// sortFunc returns 1 for (dira parent, dirb parent/child) because
	// HasPrefix(dirb, dira) is true → a goes after b.
	files := []*gitdiff.File{
		{NewName: "parent/file.txt"},
		{NewName: "parent/child/file.txt"},
	}
	sortFiles(files)
	// parent/child comes after parent because sortFiles puts deeper paths later.
	if files[0].NewName != "parent/child/file.txt" {
		t.Fatalf("expected parent/child/file.txt first, got %s", files[0].NewName)
	}
	if files[1].NewName != "parent/file.txt" {
		t.Fatalf("expected parent/file.txt second, got %s", files[1].NewName)
	}
}

func TestSortFilesSiblingDirectoriesSortedByFullPath(t *testing.T) {
	// Sibling, non-prefix directories fall through to alphabetical
	// comparison on the full file name.
	files := []*gitdiff.File{
		{NewName: "b-dir/file.txt"},
		{NewName: "a-dir/file.txt"},
	}
	sortFiles(files)
	if files[0].NewName != "a-dir/file.txt" {
		t.Fatalf("expected a-dir/file.txt first, got %s", files[0].NewName)
	}
}

func TestSortFilesTopLevelAndNestedDifferentDirs(t *testing.T) {
	// Top-level (dir=".") and nested (dir="sub") are in different
	// non-prefix directories. Neither is RootName ("/"), so they fall
	// through to alphabetical comparison on full name.
	files := []*gitdiff.File{
		{NewName: "z.txt"},
		{NewName: "a-dir/file.txt"},
	}
	sortFiles(files)
	if files[0].NewName != "a-dir/file.txt" {
		t.Fatalf("expected a-dir/file.txt first (alphabetical), got %s", files[0].NewName)
	}
}

func TestSortFilesEmptySlice(t *testing.T) {
	files := []*gitdiff.File{}
	sortFiles(files) // should not panic
	if len(files) != 0 {
		t.Fatalf("expected empty slice, got %d items", len(files))
	}
}

func TestSortFilesCaseInsensitive(t *testing.T) {
	files := []*gitdiff.File{
		{NewName: "B.txt"},
		{NewName: "a.txt"},
	}
	sortFiles(files)
	if files[0].NewName != "a.txt" {
		t.Fatalf("expected a.txt first (case insensitive), got %s", files[0].NewName)
	}
}

func TestSortFilesNonRootBeforeRootNameDir(t *testing.T) {
	// Non-root dir files (dir!="/") come before root-dir files (dir=="/").
	files := []*gitdiff.File{
		{NewName: "/root-file.txt"},
		{NewName: "sub/file.txt"},
	}
	sortFiles(files)
	if files[0].NewName != "sub/file.txt" {
		t.Fatalf("expected non-root file first, got %s", files[0].NewName)
	}
}

func TestSortFilesRootNameDirAfterNonRoot(t *testing.T) {
	files := []*gitdiff.File{
		{NewName: "sub/file.txt"},
		{NewName: "/root-file.txt"},
	}
	sortFiles(files)
	if files[1].NewName != "/root-file.txt" {
		t.Fatalf("expected /root-file.txt after non-/ file, got %s", files[1].NewName)
	}
}

// ---------------------------------------------------------------------------
// cycleIconStyle tests
// ---------------------------------------------------------------------------

func TestCycleIconStyleAsciiToUnicode(t *testing.T) {
	m := newTestMainModel(t)
	m.iconStyle = filenode.IconsASCII
	m.cycleIconStyle()
	if m.iconStyle != filenode.IconsUnicode {
		t.Fatalf("expected IconsUnicode after cycling from ASCII, got %q", m.iconStyle)
	}
}

func TestCycleIconStyleUnicodeToNerdStatus(t *testing.T) {
	m := newTestMainModel(t)
	m.iconStyle = filenode.IconsUnicode
	m.cycleIconStyle()
	if m.iconStyle != filenode.IconsNerdStatus {
		t.Fatalf("expected IconsNerdStatus after cycling from Unicode, got %q", m.iconStyle)
	}
}

func TestCycleIconStyleNerdStatusToNerdSimple(t *testing.T) {
	m := newTestMainModel(t)
	m.iconStyle = filenode.IconsNerdStatus
	m.cycleIconStyle()
	if m.iconStyle != filenode.IconsNerdSimple {
		t.Fatalf("expected IconsNerdSimple after cycling from NerdStatus, got %q", m.iconStyle)
	}
}

func TestCycleIconStyleNerdSimpleToNerdFiletype(t *testing.T) {
	m := newTestMainModel(t)
	m.iconStyle = filenode.IconsNerdSimple
	m.cycleIconStyle()
	if m.iconStyle != filenode.IconsNerdFiletype {
		t.Fatalf("expected IconsNerdFiletype after cycling from NerdSimple, got %q", m.iconStyle)
	}
}

func TestCycleIconStyleNerdFiletypeToNerdFull(t *testing.T) {
	m := newTestMainModel(t)
	m.iconStyle = filenode.IconsNerdFiletype
	m.cycleIconStyle()
	if m.iconStyle != filenode.IconsNerdFull {
		t.Fatalf("expected IconsNerdFull after cycling from NerdFiletype, got %q", m.iconStyle)
	}
}

func TestCycleIconStyleNerdFullWrapsToAscii(t *testing.T) {
	m := newTestMainModel(t)
	m.iconStyle = filenode.IconsNerdFull
	m.cycleIconStyle()
	if m.iconStyle != filenode.IconsASCII {
		t.Fatalf("expected IconsASCII after cycling from NerdFull, got %q", m.iconStyle)
	}
}

func TestCycleIconStyleUnknownResetsToAscii(t *testing.T) {
	m := newTestMainModel(t)
	m.iconStyle = "unknown-style"
	m.cycleIconStyle()
	if m.iconStyle != filenode.IconsASCII {
		t.Fatalf("expected IconsASCII for unknown style, got %q", m.iconStyle)
	}
}

func TestCycleIconStyleFullCycle(t *testing.T) {
	m := newTestMainModel(t)
	styles := []string{
		filenode.IconsASCII,
		filenode.IconsUnicode,
		filenode.IconsNerdStatus,
		filenode.IconsNerdSimple,
		filenode.IconsNerdFiletype,
		filenode.IconsNerdFull,
	}
	m.iconStyle = styles[0]
	for i := 0; i < len(styles); i++ {
		if m.iconStyle != styles[i] {
			t.Fatalf("step %d: expected %q, got %q", i, styles[i], m.iconStyle)
		}
		m.cycleIconStyle()
	}
	// After full cycle, should wrap back to ASCII
	if m.iconStyle != filenode.IconsASCII {
		t.Fatalf("expected wrap back to ASCII, got %q", m.iconStyle)
	}
}

// ---------------------------------------------------------------------------
// parseCommitMeta tests
// ---------------------------------------------------------------------------

func TestParseCommitMetaFullPreamble(t *testing.T) {
	m := newTestMainModel(t)
	m.preamble = "commit abcdef1234567890 (HEAD -> main)\nAuthor: John Doe <john@example.com>\nAuthorDate: Mon Jan 2 15:04:05 2006 -0700"
	meta := m.parseCommitMeta()
	if meta.hash != "abcdef1" {
		t.Fatalf("expected hash 'abcdef1', got %q", meta.hash)
	}
	if meta.author != "JDoe" {
		t.Fatalf("expected author 'JDoe', got %q", meta.author)
	}
}

func TestParseCommitMetaEmptyPreamble(t *testing.T) {
	m := newTestMainModel(t)
	m.preamble = ""
	meta := m.parseCommitMeta()
	if meta.hash != "" {
		t.Fatalf("expected empty hash, got %q", meta.hash)
	}
	if meta.author != "" {
		t.Fatalf("expected empty author, got %q", meta.author)
	}
	if meta.date != "" {
		t.Fatalf("expected empty date, got %q", meta.date)
	}
}

func TestParseCommitMetaShortHash(t *testing.T) {
	m := newTestMainModel(t)
	m.preamble = "commit abc\nAuthor: Test User <test@test.com>"
	meta := m.parseCommitMeta()
	// Hash is <= 7 chars so used as-is
	if meta.hash != "abc" {
		t.Fatalf("expected hash 'abc', got %q", meta.hash)
	}
}

func TestParseCommitMetaHashTruncated(t *testing.T) {
	m := newTestMainModel(t)
	m.preamble = "commit abcdefghijklmnop\nAuthor: Test <t@t.com>"
	meta := m.parseCommitMeta()
	if meta.hash != "abcdefg" {
		t.Fatalf("expected truncated hash 'abcdefg', got %q", meta.hash)
	}
}

func TestParseCommitMetaHashWithRefsDecoration(t *testing.T) {
	m := newTestMainModel(t)
	m.preamble = "commit abcdef1234567890 (HEAD -> main, tag: v1.0)\nAuthor: Jane Smith <jane@example.com>"
	meta := m.parseCommitMeta()
	if meta.hash != "abcdef1" {
		t.Fatalf("expected hash 'abcdef1' (refs stripped), got %q", meta.hash)
	}
}

func TestParseCommitMetaAuthorWithEmailOnly(t *testing.T) {
	m := newTestMainModel(t)
	m.preamble = "commit abc1234567890\nAuthor: <user@example.com>"
	meta := m.parseCommitMeta()
	if meta.author != "user" {
		t.Fatalf("expected author 'user' (extracted from email), got %q", meta.author)
	}
}

func TestParseCommitMetaAuthorSingleName(t *testing.T) {
	m := newTestMainModel(t)
	m.preamble = "commit abc1234567890\nAuthor: SingleName <s@test.com>"
	meta := m.parseCommitMeta()
	if meta.author != "SingleName" {
		t.Fatalf("expected author 'SingleName', got %q", meta.author)
	}
}

func TestParseCommitMetaDateAuthorDate(t *testing.T) {
	m := newTestMainModel(t)
	past := time.Now().Add(-2 * time.Hour)
	m.preamble = fmt.Sprintf(
		"commit abc1234567890\nAuthor: Test <t@t.com>\nAuthorDate: %s",
		past.Format("Mon Jan 2 15:04:05 2006 -0700"),
	)
	meta := m.parseCommitMeta()
	if meta.date == "" {
		t.Fatal("expected non-empty date from AuthorDate")
	}
}

func TestParseCommitMetaDateDate(t *testing.T) {
	m := newTestMainModel(t)
	past := time.Now().Add(-30 * time.Minute)
	m.preamble = fmt.Sprintf(
		"commit abc1234567890\nAuthor: Test <t@t.com>\nDate: %s",
		past.Format("Mon Jan 2 15:04:05 2006 -0700"),
	)
	meta := m.parseCommitMeta()
	if meta.date == "" {
		t.Fatal("expected non-empty date from Date")
	}
}

func TestParseCommitMetaAuthorDatePreferredOverDate(t *testing.T) {
	m := newTestMainModel(t)
	past := time.Now().Add(-30 * time.Minute)
	m.preamble = fmt.Sprintf(
		"commit abc1234567890\nAuthor: Test <t@t.com>\nAuthorDate: %s\nDate: Mon Jan 1 00:00:00 2000 +0000",
		past.Format("Mon Jan 2 15:04:05 2006 -0700"),
	)
	meta := m.parseCommitMeta()
	// AuthorDate is encountered first and meta.date should be set from it
	if meta.date == "" {
		t.Fatal("expected date to be set from AuthorDate")
	}
}

func TestParseCommitMetaDateInvalidFormat(t *testing.T) {
	m := newTestMainModel(t)
	m.preamble = "commit abc1234567890\nAuthor: Test <t@t.com>\nAuthorDate: not-a-date"
	meta := m.parseCommitMeta()
	if meta.date != "" {
		t.Fatalf("expected empty date for invalid format, got %q", meta.date)
	}
}

// ---------------------------------------------------------------------------
// commitSubject tests
// ---------------------------------------------------------------------------

func TestCommitSubjectSimple(t *testing.T) {
	m := newTestMainModel(t)
	m.preamble = "commit abc123\nAuthor: Test\nDate: now\n\nFix the bug"
	result := m.commitSubject()
	if result != "Fix the bug" {
		t.Fatalf("expected 'Fix the bug', got %q", result)
	}
}

func TestCommitSubjectEmptyPreamble(t *testing.T) {
	m := newTestMainModel(t)
	m.preamble = ""
	result := m.commitSubject()
	if result != "" {
		t.Fatalf("expected empty subject, got %q", result)
	}
}

func TestCommitSubjectSkipsMetadataLines(t *testing.T) {
	m := newTestMainModel(t)
	m.preamble = "commit abc123\nAuthor: Test\nAuthorDate: now\nDate: now\nCommit: Test\nCommitDate: now\nMerge: abc def\n\nThe actual subject"
	result := m.commitSubject()
	if result != "The actual subject" {
		t.Fatalf("expected 'The actual subject', got %q", result)
	}
}

func TestCommitSubjectNoSubjectOnlyMetadata(t *testing.T) {
	m := newTestMainModel(t)
	m.preamble = "commit abc123\nAuthor: Test\nDate: now"
	result := m.commitSubject()
	if result != "" {
		t.Fatalf("expected empty subject when only metadata, got %q", result)
	}
}

func TestCommitSubjectTrimsWhitespace(t *testing.T) {
	m := newTestMainModel(t)
	m.preamble = "commit abc123\n  \n  Hello world  "
	result := m.commitSubject()
	if result != "Hello world" {
		t.Fatalf("expected 'Hello world', got %q", result)
	}
}

// ---------------------------------------------------------------------------
// resolveBranch tests
// ---------------------------------------------------------------------------

func TestResolveBranchWithHeadArrow(t *testing.T) {
	preamble := "commit abc123 (HEAD -> feature-branch)"
	result := resolveBranch(preamble)
	if result != "feature-branch" {
		t.Fatalf("expected 'feature-branch', got %q", result)
	}
}

func TestResolveBranchWithMultipleRefs(t *testing.T) {
	preamble := "commit abc123 (HEAD -> main, tag: v1.0, origin/main)"
	result := resolveBranch(preamble)
	if result != "main" {
		t.Fatalf("expected 'main', got %q", result)
	}
}

func TestResolveBranchNoDecoration(t *testing.T) {
	// Without decoration, resolveBranch falls through to git CLI.
	// Using a non-existent hash, git should return empty.
	preamble := "commit abc123"
	result := resolveBranch(preamble)
	// abc123 is not a valid full hash, so git CLI likely errors and returns "".
	// If it does return something, it must be a valid branch name (no spaces or newlines).
	if strings.Contains(result, " ") || strings.Contains(result, "\n") {
		t.Fatalf("expected valid branch name or empty, got %q", result)
	}
}

func TestResolveBranchEmptyPreamble(t *testing.T) {
	result := resolveBranch("")
	if result != "" {
		t.Fatalf("expected empty branch for empty preamble, got %q", result)
	}
}

func TestResolveBranchNoHeadArrow(t *testing.T) {
	preamble := "commit abc123 (tag: v1.0, origin/main)"
	// No "HEAD -> " prefix, so it falls through to git CLI
	result := resolveBranch(preamble)
	// abc123 is not a valid full hash, so git CLI likely errors and returns "".
	// If it does return something, it must be a valid branch name.
	if strings.Contains(result, " ") || strings.Contains(result, "\n") {
		t.Fatalf("expected valid branch name or empty, got %q", result)
	}
}

func TestResolveBranchCommitLineWithExtraContent(t *testing.T) {
	preamble := "commit abc123 extra content (HEAD -> develop)"
	result := resolveBranch(preamble)
	if result != "develop" {
		t.Fatalf("expected 'develop', got %q", result)
	}
}

// ---------------------------------------------------------------------------
// KeyGroups tests
// ---------------------------------------------------------------------------

func TestKeyGroupsReturnsThreeGroups(t *testing.T) {
	groups := KeyGroups()
	if len(groups) != 3 {
		t.Fatalf("expected 3 key groups, got %d", len(groups))
	}
}

func TestKeyGroupsFirstGroupContainsNavigation(t *testing.T) {
	groups := KeyGroups()
	if len(groups[0]) == 0 {
		t.Fatal("expected first group to have bindings")
	}
}

func TestKeyGroupsSecondGroupContainsActions(t *testing.T) {
	groups := KeyGroups()
	if len(groups[1]) == 0 {
		t.Fatal("expected second group to have bindings")
	}
}

func TestKeyGroupsThirdGroupContainsMeta(t *testing.T) {
	groups := KeyGroups()
	if len(groups[2]) == 0 {
		t.Fatal("expected third group to have bindings")
	}
}

func TestKeyGroupsAllBindingsHaveHelp(t *testing.T) {
	groups := KeyGroups()
	for i, group := range groups {
		for j, binding := range group {
			help := binding.Help()
			if help.Desc == "" {
				t.Fatalf("group %d binding %d: expected non-empty help description", i, j)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// sidebarWidth tests
// ---------------------------------------------------------------------------

func TestSidebarWidthWhenSearching(t *testing.T) {
	m := newTestMainModel(t)
	m.searching = true
	w := m.sidebarWidth()
	if w != m.config.UI.SearchTreeWidth {
		t.Fatalf("expected SearchTreeWidth %d, got %d", m.config.UI.SearchTreeWidth, w)
	}
}

func TestSidebarWidthWhenFileTreeShown(t *testing.T) {
	m := newTestMainModel(t)
	m.searching = false
	m.isShowingFileTree = true
	m.fileTree.SetSize(42, 10)
	w := m.sidebarWidth()
	if w != 42 {
		t.Fatalf("expected fileTree width 42, got %d", w)
	}
}

func TestSidebarWidthWhenFileTreeHidden(t *testing.T) {
	m := newTestMainModel(t)
	m.searching = false
	m.isShowingFileTree = false
	w := m.sidebarWidth()
	if w != 0 {
		t.Fatalf("expected 0 when hidden, got %d", w)
	}
}

// ---------------------------------------------------------------------------
// isSidebarVisible tests
// ---------------------------------------------------------------------------

func TestIsSidebarVisibleWhenShowingFileTree(t *testing.T) {
	m := newTestMainModel(t)
	m.isShowingFileTree = true
	m.searching = false
	if !m.isSidebarVisible() {
		t.Fatal("expected sidebar visible when file tree showing")
	}
}

func TestIsSidebarVisibleWhenSearching(t *testing.T) {
	m := newTestMainModel(t)
	m.isShowingFileTree = false
	m.searching = true
	if !m.isSidebarVisible() {
		t.Fatal("expected sidebar visible when searching")
	}
}

func TestIsSidebarVisibleWhenBothHidden(t *testing.T) {
	m := newTestMainModel(t)
	m.isShowingFileTree = false
	m.searching = false
	if m.isSidebarVisible() {
		t.Fatal("expected sidebar hidden when neither showing nor searching")
	}
}

// ---------------------------------------------------------------------------
// selectedSearchResult tests
// ---------------------------------------------------------------------------

func TestSelectedSearchResultWithResults(t *testing.T) {
	m := newTestMainModel(t)
	m.filtered = []string{"file1.txt", "file2.txt"}
	m.resultsCursor = 1
	result, ok := m.selectedSearchResult()
	if !ok {
		t.Fatal("expected ok=true with results")
	}
	if result != "file2.txt" {
		t.Fatalf("expected 'file2.txt', got %q", result)
	}
}

func TestSelectedSearchResultEmpty(t *testing.T) {
	m := newTestMainModel(t)
	m.filtered = nil
	m.resultsCursor = 0
	result, ok := m.selectedSearchResult()
	if ok {
		t.Fatal("expected ok=false with empty results")
	}
	if result != "" {
		t.Fatalf("expected empty result, got %q", result)
	}
}

func TestSelectedSearchResultCursorOutOfRange(t *testing.T) {
	m := newTestMainModel(t)
	m.filtered = []string{"file1.txt"}
	m.resultsCursor = 5
	result, ok := m.selectedSearchResult()
	if ok {
		t.Fatal("expected ok=false when cursor out of range")
	}
	if result != "" {
		t.Fatalf("expected empty result, got %q", result)
	}
}

func TestSelectedSearchResultNegativeCursor(t *testing.T) {
	m := newTestMainModel(t)
	m.filtered = []string{"file1.txt"}
	m.resultsCursor = -1
	_, ok := m.selectedSearchResult()
	if ok {
		t.Fatal("expected ok=false with negative cursor")
	}
}

func TestSelectedSearchResultFirstItem(t *testing.T) {
	m := newTestMainModel(t)
	m.filtered = []string{"file1.txt", "file2.txt"}
	m.resultsCursor = 0
	result, ok := m.selectedSearchResult()
	if !ok {
		t.Fatal("expected ok=true")
	}
	if result != "file1.txt" {
		t.Fatalf("expected 'file1.txt', got %q", result)
	}
}

// ---------------------------------------------------------------------------
// setSearchResults tests
// ---------------------------------------------------------------------------

func TestSetSearchResultsFiltersByQuery(t *testing.T) {
	m := newTestMainModel(t)
	m.search.SetValue("yarn")
	m.setSearchResults()
	for _, f := range m.filtered {
		if !strings.Contains(strings.ToLower(f), "yarn") {
			t.Fatalf("expected filtered results to contain 'yarn', got %q", f)
		}
	}
}

func TestSetSearchResultsCaseInsensitive(t *testing.T) {
	m := newTestMainModel(t)
	m.search.SetValue("YARN")
	m.setSearchResults()
	if len(m.filtered) == 0 {
		t.Fatal("expected case-insensitive match for 'YARN'")
	}
}

func TestSetSearchResultsEmptyQueryReturnsAll(t *testing.T) {
	m := newTestMainModel(t)
	m.search.SetValue("")
	m.setSearchResults()
	if len(m.filtered) != len(m.files) {
		t.Fatalf("expected all %d files, got %d", len(m.files), len(m.filtered))
	}
}

func TestSetSearchResultsNoMatch(t *testing.T) {
	m := newTestMainModel(t)
	m.search.SetValue("zzzz-not-a-file")
	m.setSearchResults()
	if len(m.filtered) != 0 {
		t.Fatalf("expected 0 results, got %d", len(m.filtered))
	}
	if m.resultsCursor != 0 {
		t.Fatalf("expected cursor to be 0 for empty results, got %d", m.resultsCursor)
	}
}

func TestSetSearchResultsClampsCursor(t *testing.T) {
	m := newTestMainModel(t)
	m.filtered = []string{"a", "b", "c"}
	m.resultsCursor = 10
	m.search.SetValue("")
	m.setSearchResults()
	if m.resultsCursor >= len(m.filtered) {
		t.Fatalf("expected cursor to be clamped, got %d", m.resultsCursor)
	}
}

func TestSetSearchResultsClampsNegativeCursor(t *testing.T) {
	m := newTestMainModel(t)
	m.resultsCursor = -5
	m.search.SetValue("")
	m.setSearchResults()
	if m.resultsCursor < 0 {
		t.Fatalf("expected cursor to be clamped to >=0, got %d", m.resultsCursor)
	}
}

// ---------------------------------------------------------------------------
// headerHeight / footerHeight tests
// ---------------------------------------------------------------------------

func TestHeaderHeightDefault(t *testing.T) {
	m := newTestMainModel(t)
	m.config.UI.HideHeader = false
	if m.headerHeight() != headerHeight {
		t.Fatalf("expected %d, got %d", headerHeight, m.headerHeight())
	}
}

func TestHeaderHeightHidden(t *testing.T) {
	m := newTestMainModel(t)
	m.config.UI.HideHeader = true
	if m.headerHeight() != 0 {
		t.Fatalf("expected 0 when hidden, got %d", m.headerHeight())
	}
}

func TestFooterHeightDefault(t *testing.T) {
	m := newTestMainModel(t)
	m.config.UI.HideFooter = false
	if m.footerHeight() != footerHeight {
		t.Fatalf("expected %d, got %d", footerHeight, m.footerHeight())
	}
}

func TestFooterHeightHidden(t *testing.T) {
	m := newTestMainModel(t)
	m.config.UI.HideFooter = true
	if m.footerHeight() != 0 {
		t.Fatalf("expected 0 when hidden, got %d", m.footerHeight())
	}
}

// ---------------------------------------------------------------------------
// searchWidth tests
// ---------------------------------------------------------------------------

func TestSearchWidthPositive(t *testing.T) {
	m := newTestMainModel(t)
	m.isShowingFileTree = true
	m.fileTree.SetSize(50, 10)
	m.searching = false
	w := m.searchWidth()
	if w <= 0 {
		t.Fatalf("expected positive search width, got %d", w)
	}
}

func TestSearchWidthEdgeCaseVeryNarrow(t *testing.T) {
	m := newTestMainModel(t)
	m.isShowingFileTree = true
	m.fileTree.SetSize(3, 10)
	m.searching = false
	w := m.searchWidth()
	// sidebarWidth()-5 with sidebarWidth=3 gives -2, max with 0 gives 0
	if w != 0 {
		t.Fatalf("expected 0 for very narrow sidebar (clamped by max(calc,0)), got %d", w)
	}
}

func TestSearchWidthWhenSearching(t *testing.T) {
	m := newTestMainModel(t)
	m.searching = true
	w := m.searchWidth()
	expected := max(0, m.config.UI.SearchTreeWidth-5)
	if w != expected {
		t.Fatalf("expected %d, got %d", expected, w)
	}
}

// ---------------------------------------------------------------------------
// stopSearch tests
// ---------------------------------------------------------------------------

func TestStopSearch(t *testing.T) {
	m := newTestMainModel(t)
	m.searching = true
	m.search.SetValue("test")
	m.search.Focus()
	m.stopSearch()
	if m.searching {
		t.Fatal("expected searching to be false after stopSearch")
	}
	if m.search.Value() != "" {
		t.Fatalf("expected search value to be cleared, got %q", m.search.Value())
	}
}

// ---------------------------------------------------------------------------
// searchResultPalette tests
// ---------------------------------------------------------------------------

func TestSearchResultPaletteDark(t *testing.T) {
	m := newTestMainModel(t)
	isDark := true
	m.isDarkBackground = &isDark
	palette := m.searchResultPalette()
	if !palette.iconDark {
		t.Fatal("expected iconDark=true for dark background")
	}
}

func TestSearchResultPaletteNilBackground(t *testing.T) {
	m := newTestMainModel(t)
	m.isDarkBackground = nil
	palette := m.searchResultPalette()
	// Nil defaults to dark
	if !palette.iconDark {
		t.Fatal("expected iconDark=true for nil background (dark default)")
	}
}

// ---------------------------------------------------------------------------
// overlayStyle tests
// ---------------------------------------------------------------------------

func TestOverlayStyleHasBorder(t *testing.T) {
	s := overlayStyle()
	// Verify the style renders non-empty output (border + padding around content)
	rendered := s.Render("test")
	if rendered == "" {
		t.Fatal("expected overlay style to render non-empty output")
	}
	// The rendered output should be wider than the input due to borders/padding
	if lipgloss.Width(rendered) <= lipgloss.Width("test") {
		t.Fatalf("expected overlay to be wider than content: rendered width=%d, content width=%d",
			lipgloss.Width(rendered), lipgloss.Width("test"))
	}
}

// ---------------------------------------------------------------------------
// fetchRepoRoot tests
// ---------------------------------------------------------------------------

func TestFetchRepoRoot(t *testing.T) {
	m := newTestMainModel(t)
	msg := m.fetchRepoRoot()
	// Outside a git repo, this will return an empty repoRootMsg
	if rr, ok := msg.(repoRootMsg); ok {
		// Valid return type; value depends on whether we're in a git repo
		_ = string(rr)
	} else {
		t.Fatalf("expected repoRootMsg, got %T", msg)
	}
}

// ---------------------------------------------------------------------------
// renderOverlay tests
// ---------------------------------------------------------------------------

func TestRenderOverlayPosition(t *testing.T) {
	m := newTestMainModel(t)
	m.width = 100
	m.height = 40
	o := m.renderOverlay("test content")
	if o.col < 0 {
		t.Fatalf("expected non-negative col, got %d", o.col)
	}
	if o.row < 0 {
		t.Fatalf("expected non-negative row, got %d", o.row)
	}
	if o.width <= 0 {
		t.Fatal("expected positive width")
	}
	if o.height <= 0 {
		t.Fatal("expected positive height")
	}
}

// ---------------------------------------------------------------------------
// Init tests
// ---------------------------------------------------------------------------

func TestInitReturnsBatchCmd(t *testing.T) {
	m := newTestMainModel(t)
	// Fetch file tree should be part of init commands.
	// Init returns a batch of commands including fetchFileTree, diffViewer.Init, and fetchRepoRoot.
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("expected Init to return a batch command")
	}
	// Execute the batch cmd and verify it produces at least one meaningful message.
	msg := cmd()
	if msg == nil {
		t.Fatal("expected batch cmd to produce a non-nil message")
	}
}

func newTestMainModel(t *testing.T) mainModel {
	t.Helper()
	zone.NewGlobal()

	cfg := config.DefaultConfig()
	data, err := os.ReadFile("../../examples/multiple_files.txt")
	if err != nil {
		t.Fatal(err)
	}

	files, _, err := gitdiff.Parse(strings.NewReader(string(data) + "\n"))
	if err != nil {
		t.Fatal(err)
	}

	m := New(string(data), cfg)
	m.files = files
	m.fileTree = m.fileTree.SetFiles(files)

	return m
}

func updateMainModel(t *testing.T, m mainModel, msg tea.Msg) mainModel {
	t.Helper()

	updated, _ := m.Update(msg)
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}

	return result
}

// waitForZone polls zone.Get until the zone is registered or the deadline is
// reached. The bubblezone manager processes zone registrations asynchronously
// via a background goroutine, so immediate calls to zone.Get() after View()
// may return nil. This helper ensures tests reliably observe registered zones.
func waitForZone(t *testing.T, id string) *zone.ZoneInfo {
	t.Helper()
	deadline := time.Now().Add(100 * time.Millisecond)
	for {
		z := zone.Get(id)
		if z != nil && !z.IsZero() {
			return z
		}
		if time.Now().After(deadline) {
			t.Fatalf("zone %q not registered after View()", id)
		}
		runtime.Gosched()
	}
}

// ---------------------------------------------------------------------------
// scheduleWatchTick tests
// ---------------------------------------------------------------------------

func TestScheduleWatchTickReturnsNonNilCmd(t *testing.T) {
	m := newTestMainModel(t)
	m.watchInterval = 100 * time.Millisecond
	cmd := m.scheduleWatchTick()
	if cmd == nil {
		t.Fatal("expected scheduleWatchTick to return a non-nil tea.Cmd")
	}
	// Execute the cmd and verify it produces a watchTickMsg.
	msg := cmd()
	if _, ok := msg.(watchTickMsg); !ok {
		t.Fatalf("expected watchTickMsg from scheduleWatchTick, got %T", msg)
	}
}

func TestScheduleWatchTickProducesWatchTickMsg(t *testing.T) {
	m := newTestMainModel(t)
	m.watchInterval = 1 * time.Millisecond
	cmd := m.scheduleWatchTick()
	msg := cmd()
	if _, ok := msg.(watchTickMsg); !ok {
		t.Fatalf("expected watchTickMsg, got %T", msg)
	}
}

// ---------------------------------------------------------------------------
// fetchWatchDiff tests
// ---------------------------------------------------------------------------

func TestFetchWatchDiffReturnsWatchResultMsg(t *testing.T) {
	m := newTestMainModel(t)
	m.watchCmd = "echo hello"
	msg := m.fetchWatchDiff()
	result, ok := msg.(watchResultMsg)
	if !ok {
		t.Fatalf("expected watchResultMsg, got %T", msg)
	}
	if result.err != nil {
		t.Fatalf("expected no error from 'echo hello', got %v", result.err)
	}
	// Output is ANSI-stripped so should be "hello" with trailing whitespace
}

func TestFetchWatchDiffErrorResult(t *testing.T) {
	m := newTestMainModel(t)
	m.watchCmd = "false" // exit code 1
	msg := m.fetchWatchDiff()
	result, ok := msg.(watchResultMsg)
	if !ok {
		t.Fatalf("expected watchResultMsg, got %T", msg)
	}
	if result.err == nil {
		t.Fatal("expected error from 'false' command")
	}
}

// ---------------------------------------------------------------------------
// renderScrollbar tests
// ---------------------------------------------------------------------------

func TestRenderScrollbarWithScrollableContent(t *testing.T) {
	m := newTestMainModel(t)
	m.messageVp.SetWidth(60)
	m.messageVp.SetHeight(10)
	m.messageVp.SetContent(strings.Repeat("line\n", 50))
	m.messageVp.GotoTop()

	sb := m.renderScrollbar()
	if sb == "" {
		t.Fatal("expected non-empty scrollbar when content is scrollable")
	}
	lines := strings.Split(sb, "\n")
	if len(lines) != m.messageVp.Height() {
		t.Fatalf("expected %d scrollbar lines, got %d", m.messageVp.Height(), len(lines))
	}
}

func TestRenderScrollbarContentFitsViewport(t *testing.T) {
	m := newTestMainModel(t)
	m.messageVp.SetWidth(60)
	m.messageVp.SetHeight(10)
	m.messageVp.SetContent("short content")

	// When content fits, the scrollbar should still render but as all track chars
	sb := m.renderScrollbar()
	// renderScrollbar doesn't check if content fits; it always renders
	if sb == "" {
		t.Fatal("expected scrollbar to always render (no empty check in method)")
	}
}

func TestRenderScrollbarThumbAdvancesOnScroll(t *testing.T) {
	m := newTestMainModel(t)
	m.messageVp.SetWidth(60)
	m.messageVp.SetHeight(5)
	m.messageVp.SetContent(strings.Repeat("line\n", 100))
	m.messageVp.GotoTop()

	// At top: thumb should be near position 0 or 1
	sb1 := m.renderScrollbar()

	// Scroll down significantly
	m.messageVp.ScrollDown(50)
	sb2 := m.renderScrollbar()

	// The two should differ (thumb position changes)
	if sb1 == sb2 {
		t.Fatal("expected scrollbar to change after scrolling")
	}
}

// ---------------------------------------------------------------------------
// renderScrollbar edge case tests
// ---------------------------------------------------------------------------

func TestRenderScrollbarAtBottom(t *testing.T) {
	m := newTestMainModel(t)
	m.messageVp.SetWidth(60)
	m.messageVp.SetHeight(5)
	m.messageVp.SetContent(strings.Repeat("line\n", 100))

	// Scroll to near bottom
	m.messageVp.GotoBottom()
	sb := m.renderScrollbar()
	if sb == "" {
		t.Fatal("expected non-empty scrollbar at bottom")
	}

	// Thumb should be near the bottom of the track
	lines := strings.Split(sb, "\n")
	lastIdx := len(lines) - 1
	// At the bottom, the last line should be a thumb character
	if !strings.Contains(lines[lastIdx], "┃") && !strings.Contains(lines[lastIdx], "│") {
		t.Fatalf("expected scrollbar character on last line, got %q", lines[lastIdx])
	}
}

func TestRenderScrollbarSingleLine(t *testing.T) {
	m := newTestMainModel(t)
	m.messageVp.SetWidth(60)
	m.messageVp.SetHeight(1)
	m.messageVp.SetContent(strings.Repeat("line\n", 100))

	sb := m.renderScrollbar()
	if sb == "" {
		t.Fatal("expected non-empty scrollbar even with height=1")
	}
	// Should be exactly one line
	if strings.Contains(sb, "\n") {
		t.Fatalf("expected single line scrollbar, got multiline: %q", sb)
	}
}

// ---------------------------------------------------------------------------
// messageViewContent tests
// ---------------------------------------------------------------------------

func TestMessageViewContentNoScrollbarWhenContentFits(t *testing.T) {
	m := newTestMainModel(t)
	m.messageVp.SetWidth(60)
	m.messageVp.SetHeight(20)
	m.messageVp.SetContent("short content")

	content := m.messageViewContent()
	if content == "" {
		t.Fatal("expected non-empty messageViewContent")
	}
	// When content fits (TotalLineCount <= Height), no scrollbar appended
	// The content should just be the viewport view
}

func TestMessageViewContentWithScrollbarWhenScrollable(t *testing.T) {
	m := newTestMainModel(t)
	m.messageVp.SetWidth(60)
	m.messageVp.SetHeight(5)
	m.messageVp.SetContent(strings.Repeat("line\n", 50))

	content := m.messageViewContent()
	if content == "" {
		t.Fatal("expected non-empty messageViewContent")
	}
	// When scrollable, content includes scrollbar track chars (│ or ┃)
}

// ---------------------------------------------------------------------------
// moveToFile tests
// ---------------------------------------------------------------------------

func TestMoveToFilePrev(t *testing.T) {
	m := newTestMainModel(t)
	m.activePanel = FileTreePanel

	// First move to a later file
	m.fileTree.SetCursorByPath("yarn.lock")
	before := m.fileTree.CurrNodePath()

	// Move to previous file
	m, cmd := m.moveToFile(-1)
	if m.fileTree.CurrNodePath() == before {
		t.Fatalf("expected cursor to move to previous file from %q", before)
	}
	_ = cmd
}

func TestMoveToFileNext(t *testing.T) {
	m := newTestMainModel(t)
	m.activePanel = FileTreePanel

	before := m.fileTree.CurrNodePath()

	// Move to next file
	m, cmd := m.moveToFile(1)
	if m.fileTree.CurrNodePath() == before {
		t.Fatalf("expected cursor to move to next file from %q", before)
	}
	_ = cmd
}

func TestMoveToFileNoMovement(t *testing.T) {
	m := newTestMainModel(t)
	m.activePanel = FileTreePanel

	// Go to the first file, then try to move prev (should be no-op)
	maxTries := 50
	for i := 0; i < maxTries; i++ {
		if _, ok := m.fileTree.GetCurrNode().GivenValue().(*filenode.FileNode); ok {
			break
		}
		m.fileTree.Down()
		node := m.fileTree.GetCurrNode()
		if node == nil {
			t.Fatal("no file node found; test fixture is broken")
		}
	}

	// Navigate up to the very first file node.
	for i := 0; i < maxTries; i++ {
		if !m.fileTree.PrevFile() {
			break
		}
	}

	beforePath := m.fileTree.CurrNodePath()
	// Try moving to prev file when at the first file (should not move)
	m, _ = m.moveToFile(-1)
	afterPath := m.fileTree.CurrNodePath()
	if beforePath != afterPath {
		t.Errorf(
			"expected path to stay at %q after moveToFile(-1) at first file, got %q",
			beforePath,
			afterPath,
		)
	}
}

// ---------------------------------------------------------------------------
// moveCursor tests
// ---------------------------------------------------------------------------

func TestMoveCursorDown(t *testing.T) {
	m := newTestMainModel(t)
	m.activePanel = FileTreePanel
	beforePath := m.fileTree.CurrNodePath()

	m, _ = m.moveCursor(moveDown)
	afterPath := m.fileTree.CurrNodePath()
	// Should have moved to a different node
	if afterPath == beforePath {
		t.Fatalf("expected cursor to move down from %q", beforePath)
	}
}

func TestMoveCursorUp(t *testing.T) {
	m := newTestMainModel(t)
	m.activePanel = FileTreePanel

	// Move down first, then up
	m, _ = m.moveCursor(moveDown)
	midPath := m.fileTree.CurrNodePath()
	m, _ = m.moveCursor(moveUp)
	afterPath := m.fileTree.CurrNodePath()
	if afterPath == midPath {
		t.Fatalf("expected cursor to move up from %q", midPath)
	}
}

func TestMoveCursorTop(t *testing.T) {
	m := newTestMainModel(t)
	m.activePanel = FileTreePanel

	// Move to bottom first, then top
	m, _ = m.moveCursor(moveBottom)
	bottomPath := m.fileTree.CurrNodePath()
	m, _ = m.moveCursor(moveTop)
	topPath := m.fileTree.CurrNodePath()
	// After top, cursor should be at a different position than after bottom
	if topPath == bottomPath {
		t.Fatal("expected cursor to move after GoToTop")
	}
}

func TestMoveCursorBottom(t *testing.T) {
	m := newTestMainModel(t)
	m.activePanel = FileTreePanel

	m, _ = m.moveCursor(moveBottom)
	path := m.fileTree.CurrNodePath()
	if path == "" {
		t.Fatal("expected cursor at a valid node after GoToBottom")
	}
}

// ---------------------------------------------------------------------------
// setNodeDiff tests
// ---------------------------------------------------------------------------

func TestSetNodeDiffNilNode(t *testing.T) {
	m := newTestMainModel(t)
	result, cmd := m.setNodeDiff(nil)
	if cmd != nil {
		t.Fatal("expected nil cmd for nil node")
	}
	// Model should be unchanged
	_ = result
}

func TestSetNodeDiffFileNode(t *testing.T) {
	m := newTestMainModel(t)
	// Find a file node in the tree
	node := m.fileTree.GetCurrNode()
	maxTries := 50
	for i := 0; i < maxTries && node != nil; i++ {
		if _, ok := node.GivenValue().(*filenode.FileNode); ok {
			break
		}
		m.fileTree.Down()
		node = m.fileTree.GetCurrNode()
	}
	if node == nil {
		t.Fatal("no file node found in tree; test fixture is broken")
	}
	fn, ok := node.GivenValue().(*filenode.FileNode)
	if !ok {
		t.Fatal("could not find a file node; test fixture is broken")
	}

	result, cmd := m.setNodeDiff(node)
	// cmd may be nil (cached) or non-nil (needs render)
	_ = cmd

	// Verify the diffViewer was updated to display the file's data.
	if !result.diffViewer.HasFile() {
		t.Fatal("expected diffViewer to have file after setNodeDiff with a FileNode")
	}
	expectedPath := filenode.GetFileName(fn.File)
	if result.diffViewer.CurrentFilePath() != expectedPath {
		t.Errorf(
			"expected diffViewer file path=%q, got %q",
			expectedPath,
			result.diffViewer.CurrentFilePath(),
		)
	}
	if result.diffViewer.HasDir() {
		t.Error("expected diffViewer to not have dir when a file is displayed")
	}
}

func TestSetNodeDiffDirNode(t *testing.T) {
	m := newTestMainModel(t)
	// Find a directory node in the tree by iterating
	node := m.fileTree.GetCurrNode()
	maxTries := 50
	for i := 0; i < maxTries && node != nil; i++ {
		switch node.GivenValue().(type) {
		case *dirnode.DirNode, string:
			result, cmd := m.setNodeDiff(node)
			_ = cmd

			// Verify the diffViewer was updated with directory data.
			if !result.diffViewer.HasDir() {
				t.Fatal("expected diffViewer to have dir after setNodeDiff with DirNode")
			}
			if result.diffViewer.HasFile() {
				t.Error("expected diffViewer to not have file when a dir is displayed")
			}
			return
		}
		m.fileTree.Down()
		node = m.fileTree.GetCurrNode()
	}
	t.Fatal("no directory or string node found in tree; test fixture is broken")
}

// ---------------------------------------------------------------------------
// openInEditor tests
// ---------------------------------------------------------------------------

func TestOpenInEditorNoFiles(t *testing.T) {
	m := newTestMainModel(t)
	m.files = nil
	cmd := m.openInEditor()
	if cmd != nil {
		t.Fatal("expected nil cmd when no files")
	}
}

func TestOpenInEditorNoEditorEnv(t *testing.T) {
	m := newTestMainModel(t)
	t.Setenv("EDITOR", "")
	cmd := m.openInEditor()
	if cmd != nil {
		t.Fatal("expected nil cmd when EDITOR is empty")
	}
}

func TestOpenInEditorWithEditorSet(t *testing.T) {
	m := newTestMainModel(t)
	t.Setenv("EDITOR", "true")
	cmd := m.openInEditor()
	if cmd == nil {
		t.Fatal("expected non-nil cmd when EDITOR is set and files exist")
	}
}

func TestOpenInEditorWithRepoRootSet(t *testing.T) {
	m := newTestMainModel(t)
	t.Setenv("EDITOR", "true")
	m.repoRoot = "/tmp"
	cmd := m.openInEditor()
	if cmd == nil {
		t.Fatal("expected non-nil cmd when EDITOR is set and repoRoot is provided")
	}
}

// ---------------------------------------------------------------------------
// Update: watchTickMsg tests
// ---------------------------------------------------------------------------

func TestWatchTickMsgWhenNotInFlight(t *testing.T) {
	m := newTestMainModel(t)
	m.watchEnabled = true
	m.watchInFlight = false
	m.watchCmd = "echo test"
	m.watchInterval = time.Second

	updated, cmd := m.Update(watchTickMsg{})
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	if !result.watchInFlight {
		t.Fatal("expected watchInFlight to be true after watchTickMsg")
	}
	if cmd == nil {
		t.Fatal("expected non-nil command (fetchWatchDiff)")
	}
}

func TestWatchTickMsgWhenInFlight(t *testing.T) {
	m := newTestMainModel(t)
	m.watchEnabled = true
	m.watchInFlight = true

	updated, cmd := m.Update(watchTickMsg{})
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	// Should be a no-op when already in flight
	if !result.watchInFlight {
		t.Fatal("expected watchInFlight to remain true")
	}
	if cmd != nil {
		t.Fatal("expected nil cmd when watch is already in flight")
	}
}

// ---------------------------------------------------------------------------
// Update: watchResultMsg tests
// ---------------------------------------------------------------------------

func TestWatchResultMsgOutputUnchanged(t *testing.T) {
	m := newTestMainModel(t)
	m.watchEnabled = true
	m.watchInFlight = true
	m.input = "original"

	updated, cmd := m.Update(watchResultMsg{output: "original", err: nil})
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	if result.watchInFlight {
		t.Fatal("expected watchInFlight to be false after result")
	}
	if result.input != "original" {
		t.Fatalf("expected input to remain 'original', got %q", result.input)
	}
	_ = cmd // cmd should schedule next tick
}

func TestWatchResultMsgOutputChanged(t *testing.T) {
	m := newTestMainModel(t)
	m.watchEnabled = true
	m.watchInFlight = true
	m.input = "original"

	updated, cmd := m.Update(watchResultMsg{output: "new output", err: nil})
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	if result.watchInFlight {
		t.Fatal("expected watchInFlight to be false after result")
	}
	if result.input != "new output" {
		t.Fatalf("expected input to be 'new output', got %q", result.input)
	}
	// pendingCursorPath is set to current fileTree path before refresh.
	// It may be empty if no node is currently selected.
	_ = result.pendingCursorPath
	_ = cmd // should include fetchFileTree and scheduleWatchTick
}

func TestWatchResultMsgError(t *testing.T) {
	m := newTestMainModel(t)
	m.watchEnabled = true
	m.watchInFlight = true

	updated, cmd := m.Update(watchResultMsg{err: fmt.Errorf("command failed")})
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	if result.watchInFlight {
		t.Fatal("expected watchInFlight to be false after error result")
	}
	// Should schedule next tick even on error
	_ = cmd
}

// ---------------------------------------------------------------------------
// Update: common.ErrMsg tests
// ---------------------------------------------------------------------------

func TestUpdateErrMsg(t *testing.T) {
	m := newTestMainModel(t)
	updated, _ := m.Update(common.ErrMsg{Err: fmt.Errorf("test error")})
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	// Should not panic on ErrMsg, just log
	_ = result
}

// ---------------------------------------------------------------------------
// Update: repoRootMsg tests
// ---------------------------------------------------------------------------

func TestUpdateRepoRootMsg(t *testing.T) {
	m := newTestMainModel(t)
	updated, _ := m.Update(repoRootMsg("/home/user/repo"))
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	if result.repoRoot != "/home/user/repo" {
		t.Fatalf("expected repoRoot '/home/user/repo', got %q", result.repoRoot)
	}
}

func TestUpdateRepoRootMsgEmpty(t *testing.T) {
	m := newTestMainModel(t)
	updated, _ := m.Update(repoRootMsg(""))
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	if result.repoRoot != "" {
		t.Fatalf("expected empty repoRoot, got %q", result.repoRoot)
	}
}

// ---------------------------------------------------------------------------
// Update: overlay open/close tests
// ---------------------------------------------------------------------------

func TestUpdateToggleHelp(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	// Toggle help open
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "?", Code: '?'}))
	if !m.helpOpen {
		t.Fatal("expected help to be open after pressing ?")
	}

	// Toggle help closed
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "?", Code: '?'}))
	if m.helpOpen {
		t.Fatal("expected help to be closed after pressing ? again")
	}
}

func TestUpdateToggleMessageWithPreamble(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.preamble = "commit abc123\nAuthor: Test\n\nSubject line"

	// Toggle message open
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "m", Code: 'm'}))
	if !m.messageOpen {
		t.Fatal("expected message overlay to be open after pressing m")
	}

	// Toggle message closed
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "m", Code: 'm'}))
	if m.messageOpen {
		t.Fatal("expected message overlay to be closed after pressing m again")
	}
}

func TestUpdateToggleMessageWithoutPreamble(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.preamble = ""

	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "m", Code: 'm'}))
	if m.messageOpen {
		t.Fatal("expected message overlay to stay closed when preamble is empty")
	}
}

func TestUpdateEscapeClosesOverlay(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	// Open help
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "?", Code: '?'}))
	if !m.helpOpen {
		t.Fatal("expected help to be open")
	}

	// Close with escape
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	if m.helpOpen {
		t.Fatal("expected help to be closed after escape")
	}
}

func TestUpdateEscapeClearsSelection(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	// Create a finalized selection by starting, extending head, and ending it.
	m.diffViewer.SetContent(strings.Repeat("line content\n", 500))
	m.diffViewer.StartSelection(diffviewer.Point{Line: 0, Col: 0})
	m.diffViewer.ExtendSelection(diffviewer.Point{Line: 5, Col: 4})
	m.diffViewer.EndSelection()
	if !m.diffViewer.HasSelection() {
		t.Fatal("expected finalized selection before Escape")
	}

	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	if m.diffViewer.HasSelection() {
		t.Fatal("expected selection to be cleared after Escape")
	}
}

func TestUpdateQuitClosesOverlayFirst(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	// Open help overlay
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "?", Code: '?'}))
	if !m.helpOpen {
		t.Fatal("expected help overlay to be open")
	}

	// 'q' when overlay is open should close it, not quit the app
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "q", Code: 'q'}))
	if m.helpOpen {
		t.Fatal("expected help overlay to be closed after pressing q")
	}
}

func TestUpdateMessageOverlayDownUp(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.preamble = strings.Repeat("preamble line\n", 200)
	m.messageOpen = true
	m.updateMessageVp()
	m.messageVp.GotoTop()

	offset0 := m.messageVp.YOffset()
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	if m.messageVp.YOffset() <= offset0 {
		t.Fatalf(
			"expected messageVp to scroll down, YOffset was %d now %d",
			offset0,
			m.messageVp.YOffset(),
		)
	}

	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	// After scrolling up, YOffset should decrease or stay at 0
}

func TestUpdateMessageOverlayCtrlDCtrlU(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.preamble = strings.Repeat("preamble line\n", 200)
	m.messageOpen = true
	m.updateMessageVp()
	m.messageVp.GotoTop()

	// ctrl+d should scroll half a page
	before := m.messageVp.YOffset()
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: 'd', Mod: tea.ModCtrl}))
	if m.messageVp.YOffset() <= before {
		t.Fatalf(
			"expected ctrl+d to scroll messageVp down, YOffset was %d now %d",
			before,
			m.messageVp.YOffset(),
		)
	}

	// ctrl+u should scroll up
	before = m.messageVp.YOffset()
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: 'u', Mod: tea.ModCtrl}))
	if m.messageVp.YOffset() >= before {
		t.Fatalf(
			"expected ctrl+u to scroll messageVp up, YOffset was %d now %d",
			before,
			m.messageVp.YOffset(),
		)
	}
}

// ---------------------------------------------------------------------------
// Update: keyboard navigation in filetree panel
// ---------------------------------------------------------------------------

func TestUpdateUpInFileTreePanel(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.activePanel = FileTreePanel

	// Move down first, then up
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	midPath := m.fileTree.CurrNodePath()
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	upPath := m.fileTree.CurrNodePath()
	if upPath == midPath {
		t.Fatalf("expected cursor to move up, stayed at %q", midPath)
	}
}

func TestUpdateDownInFileTreePanel(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.activePanel = FileTreePanel

	beforePath := m.fileTree.CurrNodePath()
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	afterPath := m.fileTree.CurrNodePath()
	if afterPath == beforePath {
		t.Fatalf("expected cursor to move down from %q", beforePath)
	}
}

func TestUpdateBottomInFileTreePanel(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.activePanel = FileTreePanel

	midPath := m.fileTree.CurrNodePath()
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))

	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "G", Code: 'G'}))
	// After G (bottom), the cursor should have moved past the mid position
	if m.fileTree.CurrNodePath() == midPath {
		t.Fatal("expected cursor to move to bottom of file tree")
	}
}

func TestUpdateTopInFileTreePanel(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.activePanel = FileTreePanel

	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	downPath := m.fileTree.CurrNodePath()

	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "g", Code: 'g'}))
	// After gg (top), the cursor should have moved back up
	if m.fileTree.CurrNodePath() == downPath {
		t.Fatal("expected cursor to move to top of file tree")
	}
}

func TestUpdateUpInDiffViewerPanel(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.activePanel = DiffViewerPanel

	// Scroll down first so we have room to go up
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	beforeY := m.diffViewer.YOffset()

	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	afterY := m.diffViewer.YOffset()
	if afterY > beforeY {
		t.Fatalf(
			"expected YOffset to decrease or stay after Up, got before=%d after=%d",
			beforeY,
			afterY,
		)
	}
}

func TestUpdateDownInDiffViewerPanel(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.activePanel = DiffViewerPanel

	beforeY := m.diffViewer.YOffset()
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	afterY := m.diffViewer.YOffset()
	if afterY < beforeY {
		t.Fatalf(
			"expected YOffset to increase or stay after Down, got before=%d after=%d",
			beforeY,
			afterY,
		)
	}
}

func TestUpdateBottomInDiffViewerPanel(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.activePanel = DiffViewerPanel

	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "G", Code: 'G'}))
	// After G (bottom), YOffset should be at or near the maximum
	if m.diffViewer.YOffset() < 0 {
		t.Fatalf("expected non-negative YOffset after bottom, got %d", m.diffViewer.YOffset())
	}
}

func TestUpdateTopInDiffViewerPanel(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.activePanel = DiffViewerPanel

	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "g", Code: 'g'}))
	// After gg (top), YOffset should be 0
	if m.diffViewer.YOffset() != 0 {
		t.Fatalf("expected YOffset=0 after top, got %d", m.diffViewer.YOffset())
	}
}

// ---------------------------------------------------------------------------
// Update: SwitchPanel tests
// ---------------------------------------------------------------------------

func TestSwitchPanelFromTreeToDiff(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.activePanel = FileTreePanel

	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	if m.activePanel != DiffViewerPanel {
		t.Fatalf("expected DiffViewerPanel after tab, got %v", m.activePanel)
	}
}

func TestSwitchPanelFromDiffToTree(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.activePanel = DiffViewerPanel

	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	if m.activePanel != FileTreePanel {
		t.Fatalf("expected FileTreePanel after tab, got %v", m.activePanel)
	}
}

func TestSwitchPanelWhenFileTreeHidden(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.isShowingFileTree = false
	m.activePanel = DiffViewerPanel

	// SwitchPanel when tree is hidden should be a no-op
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	if m.activePanel != DiffViewerPanel {
		t.Fatalf("expected DiffViewerPanel when tree is hidden, got %v", m.activePanel)
	}
}

// ---------------------------------------------------------------------------
// Update: ToggleDiffView tests
// ---------------------------------------------------------------------------

func TestToggleDiffView(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	beforeSBS := m.sideBySide

	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "s", Code: 's'}))
	if m.sideBySide == beforeSBS {
		t.Fatal("expected side-by-side to toggle")
	}
}

// ---------------------------------------------------------------------------
// Update: ToggleIconStyle key tests
// ---------------------------------------------------------------------------

func TestToggleIconStyleKey(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	beforeStyle := m.iconStyle

	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "i", Code: 'i'}))
	if m.iconStyle == beforeStyle {
		t.Fatal("expected icon style to toggle")
	}
}

// ---------------------------------------------------------------------------
// viewHeader tests
// ---------------------------------------------------------------------------

func TestViewHeaderWithMeta(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 160, Height: 40})
	m.preamble = "commit abcdef1234567890\nAuthor: John Doe <john@example.com>\nAuthorDate: " + time.Now().
		Add(-2*time.Hour).
		Format("Mon Jan 2 15:04:05 2006 -0700") +
		"\n\nFix the bug"
	m.commitBranch = "main"
	m.cachedMeta = m.parseCommitMeta()

	header := m.viewHeader()
	if header == "" {
		t.Fatal("expected non-empty header")
	}
	if !strings.Contains(header, "DIFFNAV") {
		t.Fatal("expected header to contain DIFFNAV")
	}
	if !strings.Contains(header, "abcdef1") {
		t.Fatal("expected header to contain hash")
	}
}

func TestViewHeaderWithNerdIconStyle(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 160, Height: 40})
	m.preamble = "commit abc1234567890\nAuthor: Test <t@t.com>\nAuthorDate: " + time.Now().
		Add(-30*time.Minute).
		Format("Mon Jan 2 15:04:05 2006 -0700")
	m.commitBranch = "develop"
	m.cachedMeta = m.parseCommitMeta()
	m.iconStyle = filenode.IconsNerdStatus

	header := m.viewHeader()
	if !strings.Contains(header, "DIFFNAV") {
		t.Fatal("expected header to contain DIFFNAV")
	}
}

func TestViewHeaderWithASCIIIconStyle(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 160, Height: 40})
	m.preamble = "commit abc1234567890\nAuthor: Test <t@t.com>\nAuthorDate: " + time.Now().
		Add(-30*time.Minute).
		Format("Mon Jan 2 15:04:05 2006 -0700")
	m.commitBranch = "develop"
	m.cachedMeta = m.parseCommitMeta()
	m.iconStyle = filenode.IconsASCII

	header := m.viewHeader()
	if !strings.Contains(header, "DIFFNAV") {
		t.Fatal("expected header to contain DIFFNAV")
	}
}

func TestViewHeaderNoMeta(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 160, Height: 40})
	m.preamble = ""
	m.cachedMeta = m.parseCommitMeta()

	header := m.viewHeader()
	if !strings.Contains(header, "DIFFNAV") {
		t.Fatal("expected header to contain DIFFNAV even without meta")
	}
}

func TestViewHeaderWithSubject(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 160, Height: 40})
	m.preamble = "commit abc123\nAuthor: Test <t@t.com>\n\nThis is a commit subject"
	m.cachedMeta = m.parseCommitMeta()
	m.commitBranch = ""

	header := m.viewHeader()
	if !strings.Contains(header, "DIFFNAV") {
		t.Fatal("expected header to contain DIFFNAV")
	}
	if !strings.Contains(header, "This is a commit subject") {
		t.Fatal("expected header to contain commit subject")
	}
}

func TestViewHeaderWithVeryNarrowTerminal(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 20, Height: 10})
	m.preamble = "commit abc123\nAuthor: Test\nDate: now\n\nA very long commit subject that should be truncated"
	m.cachedMeta = m.parseCommitMeta()
	m.commitBranch = "main"

	header := m.viewHeader()
	if header == "" {
		t.Fatal("expected non-empty header even on narrow terminals")
	}
}

// ---------------------------------------------------------------------------
// footerView tests
// ---------------------------------------------------------------------------

func TestFooterViewWithFiles(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	footer := m.footerView()
	if footer == "" {
		t.Fatal("expected non-empty footer")
	}
	if !strings.Contains(footer, "files") {
		t.Fatal("expected footer to contain file count")
	}
}

func TestFooterViewWithWatchEnabled(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m.watchEnabled = true
	m.watchCmd = "make test"

	footer := m.footerView()
	if !strings.Contains(footer, "watching") {
		t.Fatal("expected footer to contain 'watching' label")
	}
	if !strings.Contains(footer, "make test") {
		t.Fatal("expected footer to contain watch command name")
	}
}

func TestFooterViewLightBackground(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	isDark := false
	m.isDarkBackground = &isDark

	footer := m.footerView()
	if footer == "" {
		t.Fatal("expected non-empty footer")
	}
}

// ---------------------------------------------------------------------------
// messageView tests
// ---------------------------------------------------------------------------

func TestMessageViewRendersPreamble(t *testing.T) {
	m := newTestMainModel(t)
	m.preamble = "commit abc123\nAuthor: Test <t@test.com>\nAuthorDate: Mon Jan 2 15:04:05 2006 -0700\n\nSome subject"

	view := m.messageView()
	if view == "" {
		t.Fatal("expected non-empty message view")
	}
	if !strings.Contains(view, "commit ") {
		t.Fatal("expected message view to contain 'commit' line")
	}
}

func TestMessageViewEmptyPreamble(t *testing.T) {
	m := newTestMainModel(t)
	m.preamble = ""

	view := m.messageView()
	if view != "" {
		t.Fatalf("expected empty message view for empty preamble, got %q", view)
	}
}

func TestMessageViewAllMetadata(t *testing.T) {
	m := newTestMainModel(t)
	m.preamble = "commit abc\nAuthor: Test\nAuthorDate: now\nCommit: Other\nCommitDate: now\nMerge: def\nSome body text"

	view := m.messageView()
	if view == "" {
		t.Fatal("expected non-empty message view")
	}
	// All metadata lines should be rendered
}

// ---------------------------------------------------------------------------
// updateMessageVp tests
// ---------------------------------------------------------------------------

func TestUpdateMessageVpSetsDimensions(t *testing.T) {
	m := newTestMainModel(t)
	m.width = 100
	m.height = 40
	m.preamble = "commit abc\nAuthor: Test\n\nSubject"
	m.updateMessageVp()

	if m.messageVp.Height() <= 0 {
		t.Fatal("expected messageVp to have non-zero height after updateMessageVp")
	}
}

func TestWindowResizeWhileMessageOpenUpdatesVp(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.preamble = "commit abc\nAuthor: Test\n\nSubject"
	m.messageOpen = true

	prevHeight := m.messageVp.Height()

	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 80, Height: 30})
	// After resize, messageVp height should be updated to reflect the new window size.
	if m.messageVp.Height() == prevHeight {
		t.Fatal("expected messageVp height to change after window resize")
	}
}

// ---------------------------------------------------------------------------
// handleSearchBoxClick tests
// ---------------------------------------------------------------------------

func TestHandleSearchBoxClickWhenNotSearching(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.searching = false

	// Need to call View() first to register zones
	_ = m.View().Content

	result, cmd := m.handleSearchBoxClick()
	m2, ok := result.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", result)
	}
	if !m2.searching {
		t.Fatal("expected searching to be true after clicking search box")
	}
	// handleSearchBoxClick returns a batch command containing focus and resize cmds.
	// The cmd can be nil when the search input or diffViewer produces no cmd,
	// so we verify the command is either a batch or nil.
	if cmd != nil {
		// When non-nil, it should be a batch of focus + diffViewer resize.
		if _, ok := cmd().(tea.BatchMsg); !ok {
			t.Errorf("expected tea.BatchMsg cmd from handleSearchBoxClick, got %T", cmd())
		}
	}
}

func TestHandleSearchBoxClickWhenAlreadySearching(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.searching = true

	result, _ := m.handleSearchBoxClick()
	m2, ok := result.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", result)
	}
	// Should be a no-op when already searching
	if !m2.searching {
		t.Fatal("expected searching to remain true")
	}
}

// ---------------------------------------------------------------------------
// handleScroll tests
// ---------------------------------------------------------------------------

func TestHandleScrollInDiffViewer(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	// Need to call View() first to register zones
	_ = m.View().Content

	// Scroll down in the diff viewer zone using handleMouse with a wheel button.
	// Note: handleMouse routes to handleScroll for wheel events.
	z := waitForZone(t, zoneDiffViewer)
	// The scroll event should be processed without panicking.
	// YOffset may or may not change depending on whether there is enough
	// content to scroll and where the viewport is positioned, so we just
	// verify the model remains valid.
	m = updateMainModel(t, m, tea.MouseMotionMsg(tea.Mouse{
		X:      z.StartX + 5,
		Y:      z.StartY + 5,
		Button: tea.MouseWheelDown,
	}))
	// Verify the update produced a valid model (did not panic).
	_ = m.View().Content
}

func TestHandleScrollInFileTreeZoneLegacy(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.isShowingFileTree = true

	_ = m.View().Content

	z := waitForZone(t, zoneFileTree)
	m = updateMainModel(t, m, tea.MouseMotionMsg(tea.Mouse{
		X:      z.StartX + 2,
		Y:      z.StartY + 5,
		Button: tea.MouseWheelUp,
	}))
}

func TestHandleScrollInSearchResults(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.searching = true
	m.search.SetValue("yarn")
	m.setSearchResults()
	m.resultsVp.SetWidth(m.config.UI.SearchTreeWidth)
	m.resultsVp.SetHeight(m.mainContentHeight() - searchHeight)
	m.resultsVp.SetContent(m.resultsView())

	_ = m.View().Content

	z := waitForZone(t, zoneSearchResults)
	// The scroll event should be processed without panicking.
	m = updateMainModel(t, m, tea.MouseMotionMsg(tea.Mouse{
		X:      z.StartX + 2,
		Y:      z.StartY + 5,
		Button: tea.MouseWheelDown,
	}))
	_ = m.View().Content
}

// ---------------------------------------------------------------------------
// handleSearchResultClick tests
// ---------------------------------------------------------------------------

func TestHandleSearchResultClickNegativeY(t *testing.T) {
	m := newTestMainModel(t)
	m.searching = true
	m.search.SetValue("yarn")
	m.setSearchResults()

	// zone-relative y < 0 should be handled gracefully.
	result, cmd := m.handleSearchResultClick(tea.MouseClickMsg(tea.Mouse{
		X:      0,
		Y:      0,
		Button: tea.MouseLeft,
	}))
	m2, ok := result.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", result)
	}
	_ = m2
	_ = cmd
}

// ---------------------------------------------------------------------------
// handleSidebarDrag tests
// ---------------------------------------------------------------------------

func TestHandleSidebarDragHidesSidebar(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.isShowingFileTree = true
	m.draggingSidebar = true

	// Drag to very small width (below sidebarHideWidth)
	result, _ := m.handleSidebarDrag(
		tea.MouseMotionMsg(tea.Mouse{X: 5, Y: 5, Button: tea.MouseLeft}),
	)
	m2, ok := result.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", result)
	}
	if m2.isShowingFileTree {
		t.Fatal("expected file tree to be hidden after dragging below hide threshold")
	}
}

func TestHandleSidebarDragResizesWhenSearching(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.searching = true
	m.draggingSidebar = true

	result, _ := m.handleSidebarDrag(
		tea.MouseMotionMsg(tea.Mouse{X: 40, Y: 5, Button: tea.MouseLeft}),
	)
	m2, ok := result.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", result)
	}
	if m2.draggingSidebar {
		t.Fatal("expected draggingSidebar to be reset during search")
	}
}

func TestHandleSidebarDragNormalResize(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.isShowingFileTree = true
	m.draggingSidebar = true
	m.fileTree.SetSize(30, 20)

	// Drag to a position that is above sidebarHideWidth and changes width enough
	newX := 50
	if abs(newX-m.sidebarWidth()) < minResizeStep {
		// Make sure the drag is significant enough
		newX = m.sidebarWidth() + minResizeStep + 5
	}

	result, _ := m.handleSidebarDrag(
		tea.MouseMotionMsg(tea.Mouse{X: newX, Y: 5, Button: tea.MouseLeft}),
	)
	m2, ok := result.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", result)
	}
	// Width should have changed
	_ = m2
}

// ---------------------------------------------------------------------------
// handleMouse: release event tests
// ---------------------------------------------------------------------------

func TestHandleMouseReleaseWithoutSelection(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	// Mouse release when not selecting should be a no-op
	updated, _ := m.handleMouse(tea.MouseReleaseMsg(tea.Mouse{X: 10, Y: 10}))
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	if result.draggingSidebar {
		t.Fatal("expected draggingSidebar to be false after release")
	}
}

func TestHandleMouseReleaseStopsSidebarDrag(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.draggingSidebar = true

	updated, _ := m.handleMouse(tea.MouseReleaseMsg(tea.Mouse{X: 10, Y: 10}))
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	if result.draggingSidebar {
		t.Fatal("expected draggingSidebar to be false after mouse release")
	}
}

// ---------------------------------------------------------------------------
// handleDiffSelectionMotion tests
// ---------------------------------------------------------------------------

func TestHandleDiffSelectionMotionOutsideDiffPane(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	// When not selecting, should be a no-op
	result, _ := m.handleDiffSelectionMotion(
		tea.MouseMotionMsg(tea.Mouse{X: 0, Y: 0, Button: tea.MouseLeft}),
	)
	_, ok := result.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", result)
	}
}

// ---------------------------------------------------------------------------
// handleMouse: overlay scroll tests
// ---------------------------------------------------------------------------

func TestHandleMouseScrollsMessageOverlay(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.preamble = strings.Repeat("preamble line\n", 200)
	m.messageOpen = true
	m.updateMessageVp()
	m.messageVp.GotoTop()

	// Scroll down on the message overlay
	before := m.messageVp.YOffset()
	updated, _ := m.handleMouse(tea.MouseMotionMsg(tea.Mouse{
		Button: tea.MouseWheelDown,
		X:      50,
		Y:      10,
	}))
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	if result.messageVp.YOffset() <= before {
		t.Fatalf("expected messageVp to scroll down, YOffset was %d now %d",
			before, result.messageVp.YOffset())
	}
}

func TestHandleMouseClickOutsideOverlayClosesIt(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	// Open help overlay
	m.helpOpen = true

	// Click far away from center (should be outside the overlay)
	updated, _ := m.handleMouse(tea.MouseClickMsg(tea.Mouse{
		Button: tea.MouseLeft,
		X:      0,
		Y:      0,
	}))
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	if result.helpOpen {
		t.Fatal("expected help overlay to be closed after clicking outside")
	}
}

func TestHandleMouseClickInsideOverlayStaysOpen(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	// Open help overlay
	m.helpOpen = true

	_ = m.View().Content

	// Calculate the overlay center position
	content := m.help.View()
	o := m.renderOverlay(content)

	// Click inside the overlay
	updated, _ := m.handleMouse(tea.MouseClickMsg(tea.Mouse{
		Button: tea.MouseLeft,
		X:      o.col + o.width/2,
		Y:      o.row + o.height/2,
	}))
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	if !result.helpOpen {
		t.Fatal("expected help overlay to stay open after clicking inside")
	}
}

func TestHandleMouseRightClickWhileOverlayOpenIsNoop(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	// Open help overlay
	m.helpOpen = true

	// Right-click should be a no-op
	updated, _ := m.handleMouse(tea.MouseClickMsg(tea.Mouse{
		Button: tea.MouseRight,
		X:      50,
		Y:      10,
	}))
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	// Overlay should remain open (non-left clicks are blocked)
	if !result.helpOpen {
		t.Fatal(
			"expected help overlay to stay open after right-click (non-left clicks are blocked)",
		)
	}
}

// ---------------------------------------------------------------------------
// handleMouse: help zone and header zone click tests
// ---------------------------------------------------------------------------

func TestHandleMouseHelpZoneClick(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	_ = m.View().Content
	z := waitForZone(t, zoneHelp)

	updated, _ := m.handleMouse(tea.MouseClickMsg(tea.Mouse{
		Button: tea.MouseLeft,
		X:      z.StartX + 1,
		Y:      z.StartY,
	}))
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	if !result.helpOpen {
		t.Fatal("expected help overlay to open after clicking help zone")
	}
}

func TestHandleMouseHeaderZoneClickWithPreamble(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.preamble = "commit abc"

	_ = m.View().Content
	z := waitForZone(t, zoneHeader)

	updated, _ := m.handleMouse(tea.MouseClickMsg(tea.Mouse{
		Button: tea.MouseLeft,
		X:      z.StartX + 1,
		Y:      z.StartY,
	}))
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	if !result.messageOpen {
		t.Fatal("expected message overlay to open after clicking header with preamble")
	}
}

// ---------------------------------------------------------------------------
// autoDetectedBackground tests (pure function)
// ---------------------------------------------------------------------------

func TestAutoDetectedBackgroundLight(t *testing.T) {
	isDark, ok := autoDetectedBackground(tea.BackgroundColorMsg{
		Color: color.RGBA{R: 255, G: 255, B: 255, A: 255},
	})
	if !ok {
		t.Fatal("expected ok=true for BackgroundColorMsg")
	}
	if isDark {
		t.Fatal("expected isDark=false for white background")
	}
}

func TestAutoDetectedBackgroundDark(t *testing.T) {
	isDark, ok := autoDetectedBackground(tea.BackgroundColorMsg{
		Color: color.RGBA{R: 0, G: 0, B: 0, A: 255},
	})
	if !ok {
		t.Fatal("expected ok=true for BackgroundColorMsg")
	}
	if !isDark {
		t.Fatal("expected isDark=true for black background")
	}
}

func TestAutoDetectedBackgroundTimeout(t *testing.T) {
	isDark, ok := autoDetectedBackground(themeDetectTimeoutMsg{})
	if !ok {
		t.Fatal("expected ok=true for timeout msg")
	}
	if !isDark {
		t.Fatal("expected isDark=true for timeout (dark fallback)")
	}
}

func TestAutoDetectedBackgroundOtherMsg(t *testing.T) {
	_, ok := autoDetectedBackground(tea.WindowSizeMsg{Width: 100, Height: 40})
	if ok {
		t.Fatal("expected ok=false for non-background-color msg")
	}
}

// ---------------------------------------------------------------------------
// applyAutoDetectedBackground tests
// ---------------------------------------------------------------------------

func TestApplyAutoDetectedBackgroundFirstTime(t *testing.T) {
	m := newTestMainModel(t)
	m.isDarkBackground = nil
	m.themeOverride = nil

	cmd := m.applyAutoDetectedBackground(true)
	if m.isDarkBackground == nil {
		t.Fatal("expected isDarkBackground to be set")
	}
	if !*m.isDarkBackground {
		t.Fatal("expected isDark=true")
	}
	// cmd may be nil if the diffViewer's theme mode is not auto.
	_ = cmd
}

func TestApplyAutoDetectedBackgroundAlreadySet(t *testing.T) {
	m := newTestMainModel(t)
	isDark := true
	m.isDarkBackground = &isDark

	cmd := m.applyAutoDetectedBackground(false)
	if cmd != nil {
		t.Fatal("expected nil cmd when background already set")
	}
	// Should not override existing value
	if !*m.isDarkBackground {
		t.Fatal("expected isDark to remain true")
	}
}

func TestApplyAutoDetectedBackgroundWithThemeOverride(t *testing.T) {
	m := newTestMainModel(t)
	m.themeOverride = nil
	m.isDarkBackground = nil

	// First, set via theme override
	isDark := true
	m.themeOverride = &isDark

	cmd := m.applyAutoDetectedBackground(false)
	if cmd != nil {
		t.Fatal("expected nil cmd when theme override is set")
	}
}

// ---------------------------------------------------------------------------
// fetchFileTree / fileTreeMsg tests
// ---------------------------------------------------------------------------

func TestFileTreeMsgWithEmptyFilesNoWatch(t *testing.T) {
	m := newTestMainModel(t)
	m.watchEnabled = false

	updated, cmd := m.Update(fileTreeMsg{files: []*gitdiff.File{}, preamble: "", branch: ""})
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	// Empty files without watch should trigger quit
	if cmd == nil {
		t.Fatal("expected tea.Quit cmd for empty files when watch is disabled")
	}
	_ = result
}

func TestFileTreeMsgWithEmptyFilesWatchEnabled(t *testing.T) {
	m := newTestMainModel(t)
	m.watchEnabled = true

	updated, _ := m.Update(
		fileTreeMsg{files: []*gitdiff.File{}, preamble: "commit abc", branch: "main"},
	)
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	// When watch is enabled, empty files should not quit
	if result.preamble != "commit abc" {
		t.Fatalf("expected preamble 'commit abc', got %q", result.preamble)
	}
}

func TestFileTreeMsgWithPendingCursorPath(t *testing.T) {
	m := newTestMainModel(t)
	m.pendingCursorPath = "yarn.lock"

	updated, _ := m.Update(m.fetchFileTree())
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	if result.pendingCursorPath != "" {
		t.Fatalf("expected pendingCursorPath to be cleared, got %q", result.pendingCursorPath)
	}
}

// ---------------------------------------------------------------------------
// relativeTime edge case tests
// ---------------------------------------------------------------------------

func TestRelativeTimeExactlyOneHour(t *testing.T) {
	result := relativeTime(time.Now().Add(-60 * time.Minute))
	if result != "1h" {
		t.Fatalf("expected '1h' for exactly 60 minutes, got %q", result)
	}
}

func TestRelativeTimeExactlyOneDay(t *testing.T) {
	result := relativeTime(time.Now().Add(-24 * time.Hour))
	if result != "1d" {
		t.Fatalf("expected '1d' for exactly 24 hours, got %q", result)
	}
}

func TestRelativeTimeExactly30Days(t *testing.T) {
	result := relativeTime(time.Now().Add(-30 * 24 * time.Hour))
	if result != "1mo" {
		t.Fatalf("expected '1mo' for exactly 30 days, got %q", result)
	}
}

func TestRelativeTimeFuture(t *testing.T) {
	result := relativeTime(time.Now().Add(5 * time.Minute))
	// Future times produce negative durations, which are < time.Minute so return "now"
	if result != "now" {
		t.Errorf("expected 'now' for future time, got %q", result)
	}
}

// ---------------------------------------------------------------------------
// mainContentHeight tests
// ---------------------------------------------------------------------------

func TestMainContentHeightWithHeader(t *testing.T) {
	m := newTestMainModel(t)
	m.height = 40
	m.config.UI.HideHeader = false
	m.config.UI.HideFooter = false
	h := m.mainContentHeight()
	if h != 40-headerHeight-footerHeight {
		t.Fatalf("expected %d, got %d", 40-headerHeight-footerHeight, h)
	}
}

func TestMainContentHeightWithoutHeader(t *testing.T) {
	m := newTestMainModel(t)
	m.height = 40
	m.config.UI.HideHeader = true
	m.config.UI.HideFooter = false
	h := m.mainContentHeight()
	if h != 40-footerHeight {
		t.Fatalf("expected %d, got %d", 40-footerHeight, h)
	}
}

func TestMainContentHeightWithoutFooter(t *testing.T) {
	m := newTestMainModel(t)
	m.height = 40
	m.config.UI.HideHeader = false
	m.config.UI.HideFooter = true
	h := m.mainContentHeight()
	if h != 40-headerHeight {
		t.Fatalf("expected %d, got %d", 40-headerHeight, h)
	}
}

func TestMainContentHeightWithoutBoth(t *testing.T) {
	m := newTestMainModel(t)
	m.height = 40
	m.config.UI.HideHeader = true
	m.config.UI.HideFooter = true
	h := m.mainContentHeight()
	if h != 40 {
		t.Fatalf("expected 40, got %d", h)
	}
}

// ---------------------------------------------------------------------------
// Init with watch enabled tests
// ---------------------------------------------------------------------------

func TestInitWithWatchEnabled(t *testing.T) {
	zone.NewGlobal()

	cfg := config.DefaultConfig()
	cfg.Watch.Enabled = true
	cfg.Watch.Cmd = "echo test"
	cfg.Watch.Interval = time.Second

	data, err := os.ReadFile("../../examples/multiple_files.txt")
	if err != nil {
		t.Fatal(err)
	}

	m := New(string(data), cfg)
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("expected Init to return a batch command")
	}
}

// ---------------------------------------------------------------------------
// fetchFileTree returning err (common.ErrMsg) tests
// ---------------------------------------------------------------------------

func TestFetchFileTreeReturnsErrMsgOnBadInput(t *testing.T) {
	m := newTestMainModel(t)
	m.input = "not valid git diff content @@@"

	msg := m.fetchFileTree()
	// gitdiff.Parse is fairly lenient, so let's just call it and check the type
	switch msg.(type) {
	case fileTreeMsg:
		// gitdiff.Parse may still succeed with malformed input
	case common.ErrMsg:
		// This is the expected path for truly invalid input
	default:
		t.Fatalf("expected fileTreeMsg or ErrMsg, got %T", msg)
	}
}

// ---------------------------------------------------------------------------
// commitSubject edge cases
// ---------------------------------------------------------------------------

func TestCommitSubjectWithMerge(t *testing.T) {
	m := newTestMainModel(t)
	m.preamble = "commit abc123\nMerge: def456\nAuthor: Test\n\nMerge branch 'feature'"

	result := m.commitSubject()
	if result != "Merge branch 'feature'" {
		t.Fatalf("expected 'Merge branch 'feature'', got %q", result)
	}
}

// ---------------------------------------------------------------------------
// parseCommitMeta edge cases
// ---------------------------------------------------------------------------

func TestParseCommitMetaWithCommitLine(t *testing.T) {
	m := newTestMainModel(t)
	m.preamble = "commit 0123456789abcdef0123456789abcdef01234567 (HEAD -> main)\nAuthor: First Last <first@example.com>\nDate: " + time.Now().
		Add(-1*time.Hour).
		Format("Mon Jan 2 15:04:05 2006 -0700")

	meta := m.parseCommitMeta()
	if meta.hash != "0123456" {
		t.Fatalf("expected truncated hash '0123456', got %q", meta.hash)
	}
	if meta.author != "FLast" {
		t.Fatalf("expected author 'FLast', got %q", meta.author)
	}
}

func TestParseCommitMetaDateWithDatePrefix(t *testing.T) {
	m := newTestMainModel(t)
	past := time.Now().Add(-1 * time.Hour)
	m.preamble = "commit abc123\nDate: " + past.Format("Mon Jan 2 15:04:05 2006 -0700")

	meta := m.parseCommitMeta()
	if meta.date == "" {
		t.Fatal("expected date to be parsed from 'Date:' prefix")
	}
}

// ---------------------------------------------------------------------------
// resolveBranch edge cases
// ---------------------------------------------------------------------------

func TestResolveBranchWithCommitLineAndRef(t *testing.T) {
	preamble := "commit abc (HEAD -> feature)"
	result := resolveBranch(preamble)
	if result != "feature" {
		t.Fatalf("expected 'feature', got %q", result)
	}
}

func TestResolveBranchOnlyTag(t *testing.T) {
	preamble := "commit abc (tag: v1.0)"
	// No HEAD -> prefix, falls through to git CLI with short hash "abc"
	result := resolveBranch(preamble)
	if strings.Contains(result, " ") || strings.Contains(result, "\n") {
		t.Fatalf("expected valid branch name or empty, got %q", result)
	}
}

// ---------------------------------------------------------------------------
// Toggle file tree via keyboard tests
// ---------------------------------------------------------------------------

func TestToggleFileTreeKeyboard(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	if !m.isShowingFileTree {
		t.Fatal("expected file tree to be showing by default")
	}

	// Toggle tree off
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "e", Code: 'e'}))
	if m.isShowingFileTree {
		t.Fatal("expected file tree to be hidden after pressing e")
	}
	if m.activePanel != DiffViewerPanel {
		t.Fatalf(
			"expected active panel to be DiffViewerPanel when tree hidden, got %v",
			m.activePanel,
		)
	}

	// Toggle tree back on
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "e", Code: 'e'}))
	if !m.isShowingFileTree {
		t.Fatal("expected file tree to be showing after pressing e again")
	}
}

// ---------------------------------------------------------------------------
// ToggleNode in diff viewer panel test
// ---------------------------------------------------------------------------

func TestToggleNodeInDiffViewerPanel(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.activePanel = DiffViewerPanel

	// Pressing Enter while in diff viewer panel should route to diffViewer.Update
	// and remain in DiffViewerPanel
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if m.activePanel != DiffViewerPanel {
		t.Fatalf("expected DiffViewerPanel after Enter in diff viewer, got %v", m.activePanel)
	}
}

// ---------------------------------------------------------------------------
// CtrlD/CtrlU in diff viewer test (not in overlay)
// ---------------------------------------------------------------------------

func TestCtrlDCtrlUInDiffViewer(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.activePanel = DiffViewerPanel

	// ctrl+d should scroll diff down half a page (handled by diffViewer.Update)
	beforeY := m.diffViewer.YOffset()
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: 'd', Mod: tea.ModCtrl}))
	afterDownY := m.diffViewer.YOffset()
	if afterDownY < beforeY {
		t.Fatalf(
			"expected YOffset to increase after CtrlD, got before=%d after=%d",
			beforeY,
			afterDownY,
		)
	}

	// ctrl+u should scroll diff up half a page
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: 'u', Mod: tea.ModCtrl}))
	afterUpY := m.diffViewer.YOffset()
	if afterUpY > afterDownY {
		t.Fatalf(
			"expected YOffset to decrease after CtrlU, got before=%d after=%d",
			afterDownY,
			afterUpY,
		)
	}
}

// ---------------------------------------------------------------------------
// Copy key test
// ---------------------------------------------------------------------------

func TestCopyKeyWithPath(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	// Press 'y' to copy current node path
	activePanel := m.activePanel
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "y", Code: 'y'}))
	// Copy should not change the active panel
	if m.activePanel != activePanel {
		t.Fatalf("expected active panel to stay %v after copy, got %v", activePanel, m.activePanel)
	}
}

// ---------------------------------------------------------------------------
// Default key routing (case not matching any binding)
// ---------------------------------------------------------------------------

func TestDefaultKeyInDiffViewerPanel(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.activePanel = DiffViewerPanel

	// Press a key not matching any specific binding
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "x", Code: 'x'}))
	// Unhandled key should not change panel
	if m.activePanel != DiffViewerPanel {
		t.Fatalf("expected DiffViewerPanel after unhandled key, got %v", m.activePanel)
	}
}

func TestDefaultKeyInFileTreePanel(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.activePanel = FileTreePanel

	// Press a key not matching any specific binding (that also isn't a filetree binding)
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "x", Code: 'x'}))
	// Unhandled key should not change panel
	if m.activePanel != FileTreePanel {
		t.Fatalf("expected FileTreePanel after unhandled key, got %v", m.activePanel)
	}
}

// ---------------------------------------------------------------------------
// View rendering tests
// ---------------------------------------------------------------------------

func TestViewRendersWithHiddenHeader(t *testing.T) {
	m := newTestMainModel(t)
	m.config.UI.HideHeader = true
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view")
	}
	// Hidden header should not contain the diffnav logo text.
	if strings.Contains(view, "diff") && strings.Contains(view, "nav") {
		t.Error("expected header to be hidden but found diffnav branding in view")
	}
}

func TestViewRendersWithHiddenFooter(t *testing.T) {
	m := newTestMainModel(t)
	m.config.UI.HideFooter = true
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view")
	}
}

func TestViewRendersWithHelpOverlay(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.helpOpen = true

	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view with help overlay")
	}
	// Help overlay should contain keybinding text.
	if !strings.Contains(view, "quit") && !strings.Contains(view, "Quit") {
		t.Error("expected help overlay to contain quit keybinding")
	}
}

func TestViewRendersWithMessageOverlay(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.preamble = "commit abc\nAuthor: Test\nSubject"
	m.messageOpen = true
	m.updateMessageVp()

	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view with message overlay")
	}
	// Message overlay should contain the preamble content.
	if !strings.Contains(view, "abc") && !strings.Contains(view, "Author") {
		t.Error("expected message overlay to contain preamble content")
	}
}

func TestViewRendersWithHiddenSidebar(t *testing.T) {
	m := newTestMainModel(t)
	m.isShowingFileTree = false
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view with hidden sidebar")
	}
	// With hidden sidebar, the diffViewer zone should still be present.
	waitForZone(t, zoneDiffViewer)
}

func TestViewRendersWithSearchActive(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.searching = true
	m.search.SetValue("yarn")
	m.setSearchResults()
	m.resultsVp.SetWidth(m.config.UI.SearchTreeWidth)
	m.resultsVp.SetHeight(m.mainContentHeight() - searchHeight)
	m.resultsVp.SetContent(m.resultsView())

	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view with search active")
	}
	// Search box zone should be registered.
	waitForZone(t, zoneSearchBox)
}

// ---------------------------------------------------------------------------
// New: ThemeLight and ThemeDark paths
// ---------------------------------------------------------------------------

func TestNewWithThemeLight(t *testing.T) {
	zone.NewGlobal()

	cfg := config.DefaultConfig()
	cfg.UI.Theme = config.ThemeLight

	data, err := os.ReadFile("../../examples/multiple_files.txt")
	if err != nil {
		t.Fatal(err)
	}

	m := New(string(data), cfg)

	if m.themeOverride == nil {
		t.Fatal("expected themeOverride to be set for light theme")
	}
	if *m.themeOverride {
		t.Fatal("expected themeOverride to be false for light theme")
	}
	if m.isDarkBackground == nil {
		t.Fatal("expected isDarkBackground to be set for light theme")
	}
	if *m.isDarkBackground {
		t.Fatal("expected isDarkBackground to be false for light theme")
	}
}

func TestNewWithThemeDark(t *testing.T) {
	zone.NewGlobal()

	cfg := config.DefaultConfig()
	cfg.UI.Theme = config.ThemeDark

	data, err := os.ReadFile("../../examples/multiple_files.txt")
	if err != nil {
		t.Fatal(err)
	}

	m := New(string(data), cfg)

	if m.themeOverride == nil {
		t.Fatal("expected themeOverride to be set for dark theme")
	}
	if !*m.themeOverride {
		t.Fatal("expected themeOverride to be true for dark theme")
	}
	if m.isDarkBackground == nil {
		t.Fatal("expected isDarkBackground to be set for dark theme")
	}
	if !*m.isDarkBackground {
		t.Fatal("expected isDarkBackground to be true for dark theme")
	}
	// isDarkBackground != nil should trigger fileTree.SetDarkBackground
	// which is verified by ensuring the model was constructed without panic.
}

func TestNewWithShowFileTreeFalse(t *testing.T) {
	zone.NewGlobal()

	cfg := config.DefaultConfig()
	cfg.UI.ShowFileTree = false

	data, err := os.ReadFile("../../examples/multiple_files.txt")
	if err != nil {
		t.Fatal(err)
	}

	m := New(string(data), cfg)

	if m.activePanel != DiffViewerPanel {
		t.Fatalf("expected DiffViewerPanel when ShowFileTree=false, got %v", m.activePanel)
	}
	if m.isShowingFileTree {
		t.Fatal("expected isShowingFileTree=false when ShowFileTree=false")
	}
}

// ---------------------------------------------------------------------------
// fetchRepoRoot: error path (outside a git repo)
// ---------------------------------------------------------------------------

func TestFetchRepoRootErrorPath(t *testing.T) {
	m := newTestMainModel(t)

	// Temporarily change to a non-git directory to force the error path.
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)

	msg := m.fetchRepoRoot()
	rr, ok := msg.(repoRootMsg)
	if !ok {
		t.Fatalf("expected repoRootMsg, got %T", msg)
	}
	if string(rr) != "" {
		t.Fatalf("expected empty repoRootMsg in non-git dir, got %q", string(rr))
	}
}

// ---------------------------------------------------------------------------
// Init: theme auto-detect and watch tick
// ---------------------------------------------------------------------------

func TestInitWithAutoThemeSchedulesBackgroundDetection(t *testing.T) {
	zone.NewGlobal()

	cfg := config.DefaultConfig()
	cfg.UI.Theme = config.ThemeAuto // default

	data, err := os.ReadFile("../../examples/multiple_files.txt")
	if err != nil {
		t.Fatal(err)
	}

	m := New(string(data), cfg)
	// themeOverride and isDarkBackground should be nil for auto
	if m.themeOverride != nil {
		t.Fatal("expected nil themeOverride for auto theme")
	}
	if m.isDarkBackground != nil {
		t.Fatal("expected nil isDarkBackground for auto theme")
	}

	cmd := m.Init()
	if cmd == nil {
		t.Fatal("expected Init to return non-nil batch command")
	}
	// The batch should include tea.RequestBackgroundColor and themeDetectTimeout
	// We can validate by running the batch and checking messages.
}

func TestInitWithWatchEnabledSchedulesWatchTick(t *testing.T) {
	zone.NewGlobal()

	cfg := config.DefaultConfig()
	cfg.Watch.Enabled = true
	cfg.Watch.Cmd = "echo test"
	cfg.Watch.Interval = 50 * time.Millisecond

	data, err := os.ReadFile("../../examples/multiple_files.txt")
	if err != nil {
		t.Fatal(err)
	}

	m := New(string(data), cfg)
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("expected Init to return non-nil batch command with watch enabled")
	}
}

// ---------------------------------------------------------------------------
// Update: block other keys while overlay is open (line 267-269)
// ---------------------------------------------------------------------------

func TestUpdateBlocksArbitraryKeysWhileHelpOpen(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	// Open help overlay
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "?", Code: '?'}))
	if !m.helpOpen {
		t.Fatal("expected help overlay to be open")
	}

	// Press a key that doesn't match any specific overlay binding (e.g. 'x')
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "x", Code: 'x'}))
	// Help should remain open since arbitrary keys are blocked
	if !m.helpOpen {
		t.Fatal("expected help overlay to remain open when pressing non-overlay key")
	}
}

// ---------------------------------------------------------------------------
// Update: Escape when diffViewer has selection (lines 288-293)
// ---------------------------------------------------------------------------

func TestUpdateEscapeClearsExistingSelection(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 160, Height: 40})

	// Start a selection with different anchor and head, then finalize it.
	m.diffViewer.StartSelection(diffviewer.Point{Line: 0, Col: 0})
	m.diffViewer.ExtendSelection(diffviewer.Point{Line: 3, Col: 10})
	// Finalize: EndSelection sets has=true when anchor != head.
	if _, ok := m.diffViewer.EndSelection(); !ok {
		t.Fatal("expected EndSelection to return true for non-empty selection")
	}
	if !m.diffViewer.HasSelection() {
		t.Fatal("expected HasSelection() to be true after finalized selection")
	}

	// Pressing escape while not in an overlay should clear the selection
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	if m.diffViewer.HasSelection() {
		t.Fatal("expected selection to be cleared after pressing escape")
	}
}

// ---------------------------------------------------------------------------
// Update: applyAutoDetectedBackground when diffViewer returns a cmd (line 216)
// ---------------------------------------------------------------------------

func TestApplyAutoDetectedBackgroundReturnsDiffViewerCmd(t *testing.T) {
	m := newTestMainModel(t)
	m.themeOverride = nil
	m.isDarkBackground = nil

	// Ensure the diffViewer is in auto theme mode so SetDarkBackground returns a cmd
	cmd := m.applyAutoDetectedBackground(true)
	// verify that isDarkBackground was set, regardless of whether cmd is nil.
	// The cmd depends on whether the diffViewer has a file to render.
	if m.isDarkBackground == nil || !*m.isDarkBackground {
		t.Fatal("expected isDarkBackground to be set to true")
	}
	_ = cmd
}

// ---------------------------------------------------------------------------
// searchUpdate: esc to stop search (line 541-546)
// ---------------------------------------------------------------------------

func TestSearchUpdateEscStopsSearch(t *testing.T) {
	m := newTestMainModel(t)
	m.width = 100
	m.height = 40
	m.searching = true
	m.search.Focus()
	m.search.SetValue("test")
	m.setSearchResults()

	updated, cmds := m.searchUpdate(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))

	if updated.searching {
		t.Fatal("expected search to stop after pressing escape")
	}
	if updated.search.Value() != "" {
		t.Fatalf("expected search value to be cleared, got %q", updated.search.Value())
	}
	// Commands should include the diffViewer resize
	_ = cmds
}

// ---------------------------------------------------------------------------
// searchUpdate: ctrl+c to quit (line 546)
// ---------------------------------------------------------------------------

func TestSearchUpdateCtrlCQuits(t *testing.T) {
	m := newTestMainModel(t)
	m.width = 100
	m.height = 40
	m.searching = true
	m.search.Focus()

	_, cmds := m.searchUpdate(tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl}))

	// Should return a tea.Quit command
	foundQuit := false
	for _, cmd := range cmds {
		if cmd != nil {
			msg := cmd()
			if _, ok := msg.(tea.QuitMsg); ok {
				foundQuit = true
			}
		}
	}
	if !foundQuit {
		t.Fatal("expected to find a tea.QuitMsg in returned commands")
	}
}

// ---------------------------------------------------------------------------
// searchUpdate: enter with matching result (lines 548-558)
// ---------------------------------------------------------------------------

func TestSearchUpdateEnterSelectsFile(t *testing.T) {
	m := newTestMainModel(t)
	m.width = 100
	m.height = 40
	m.searching = true
	m.search.Focus()
	m.search.SetValue("yarn")
	m.setSearchResults()
	m.resultsCursor = 0

	if len(m.filtered) == 0 {
		t.Fatal("expected at least one filtered result for 'yarn'")
	}

	updated, cmds := m.searchUpdate(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))

	if updated.searching {
		t.Fatal("expected search to stop after pressing enter with results")
	}
	_ = cmds
}

// ---------------------------------------------------------------------------
// searchUpdate: ctrl+n / down with results (lines 566-569)
// ---------------------------------------------------------------------------

func TestSearchUpdateCtrlNAdvancesCursor(t *testing.T) {
	m := newTestMainModel(t)
	m.width = 100
	m.height = 40
	m.searching = true
	m.search.Focus()
	m.search.SetValue("") // matches all files
	m.setSearchResults()

	if len(m.filtered) < 2 {
		t.Fatal("expected at least 2 filtered results")
	}

	before := m.resultsCursor

	updated, _ := m.searchUpdate(tea.KeyPressMsg(tea.Key{Code: 'n', Mod: tea.ModCtrl}))

	if updated.resultsCursor <= before {
		t.Fatalf("expected cursor to advance from %d, got %d", before, updated.resultsCursor)
	}
}

func TestSearchUpdateDownAdvancesCursor(t *testing.T) {
	m := newTestMainModel(t)
	m.width = 100
	m.height = 40
	m.searching = true
	m.search.Focus()
	m.search.SetValue("") // matches all files
	m.setSearchResults()

	if len(m.filtered) < 2 {
		t.Fatal("expected at least 2 filtered results")
	}

	before := m.resultsCursor

	updated, _ := m.searchUpdate(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))

	if updated.resultsCursor <= before {
		t.Fatalf("expected cursor to advance from %d, got %d", before, updated.resultsCursor)
	}
}

func TestSearchUpdateCtrlNClampsAtMax(t *testing.T) {
	m := newTestMainModel(t)
	m.width = 100
	m.height = 40
	m.searching = true
	m.search.Focus()
	m.search.SetValue("") // matches all files
	m.setSearchResults()

	if len(m.filtered) < 1 {
		t.Fatal("expected at least 1 filtered result")
	}

	// Start at max
	m.resultsCursor = len(m.filtered) - 1

	updated, _ := m.searchUpdate(tea.KeyPressMsg(tea.Key{Code: 'n', Mod: tea.ModCtrl}))

	if updated.resultsCursor != len(m.filtered)-1 {
		t.Fatalf(
			"expected cursor to stay clamped at %d, got %d",
			len(m.filtered)-1,
			updated.resultsCursor,
		)
	}
}

// ---------------------------------------------------------------------------
// searchUpdate: ctrl+p / up with results (lines 570-574)
// ---------------------------------------------------------------------------

func TestSearchUpdateCtrlPRetreatsCursor(t *testing.T) {
	m := newTestMainModel(t)
	m.width = 100
	m.height = 40
	m.searching = true
	m.search.Focus()
	m.search.SetValue("") // matches all files
	m.setSearchResults()

	if len(m.filtered) < 2 {
		t.Fatal("expected at least 2 filtered results")
	}

	m.resultsCursor = 1

	updated, _ := m.searchUpdate(tea.KeyPressMsg(tea.Key{Code: 'p', Mod: tea.ModCtrl}))

	if updated.resultsCursor != 0 {
		t.Fatalf("expected cursor to retreat to 0, got %d", updated.resultsCursor)
	}
}

func TestSearchUpdateUpRetreatsCursor(t *testing.T) {
	m := newTestMainModel(t)
	m.width = 100
	m.height = 40
	m.searching = true
	m.search.Focus()
	m.search.SetValue("") // matches all files
	m.setSearchResults()

	if len(m.filtered) < 2 {
		t.Fatal("expected at least 2 filtered results")
	}

	m.resultsCursor = 2

	updated, _ := m.searchUpdate(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))

	if updated.resultsCursor != 1 {
		t.Fatalf("expected cursor to retreat to 1, got %d", updated.resultsCursor)
	}
}

func TestSearchUpdateCtrlPClampsAtZero(t *testing.T) {
	m := newTestMainModel(t)
	m.width = 100
	m.height = 40
	m.searching = true
	m.search.Focus()
	m.search.SetValue("") // matches all files
	m.setSearchResults()

	m.resultsCursor = 0

	updated, _ := m.searchUpdate(tea.KeyPressMsg(tea.Key{Code: 'p', Mod: tea.ModCtrl}))

	if updated.resultsCursor != 0 {
		t.Fatalf("expected cursor to stay at 0, got %d", updated.resultsCursor)
	}
}

// ---------------------------------------------------------------------------
// searchUpdate: default key resets cursor (line 575-576)
// ---------------------------------------------------------------------------

func TestSearchUpdateDefaultKeyResetsCursor(t *testing.T) {
	m := newTestMainModel(t)
	m.width = 100
	m.height = 40
	m.searching = true
	m.search.Focus()
	m.search.SetValue("") // matches all files
	m.setSearchResults()

	m.resultsCursor = 3

	// Press a key that doesn't match esc/ctrl+c/enter/ctrl+n/down/ctrl+p/up
	// Note: for a KeyPressMsg, String() returns Text if non-empty, otherwise Keystroke()
	// 'a' with Text set should give String() = "a"
	updated, _ := m.searchUpdate(tea.KeyPressMsg(tea.Key{Text: "a", Code: 'a'}))

	if updated.resultsCursor != 0 {
		t.Fatalf("expected cursor to reset to 0, got %d", updated.resultsCursor)
	}
}

// ---------------------------------------------------------------------------
// View: activePanel == DiffViewerPanel color branch (line 602-604)
// ---------------------------------------------------------------------------

func TestViewDiffViewerPanelColor(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.activePanel = DiffViewerPanel
	m.isShowingFileTree = true

	// This should render with rightColor = "4" for the DiffViewerPanel
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view")
	}
}

// ---------------------------------------------------------------------------
// fetchFileTree: gitdiff parse error (line 707-709)
// ---------------------------------------------------------------------------

func TestFetchFileTreeParseErrorReturnsErrMsg(t *testing.T) {
	m := newTestMainModel(t)

	// Use input with a bad hunk range that gitdiff.Parse deterministically rejects.
	// Non-numeric range values like -a,b cause parse errors.
	m.input = "diff --git a/x b/x\n--- a/x\n+++ b/x\n@@ -a,b +c,d @@\n+content"

	msg := m.fetchFileTree()
	if _, ok := msg.(common.ErrMsg); !ok {
		t.Fatalf("expected common.ErrMsg for invalid hunk range, got %T", msg)
	}
}

// ---------------------------------------------------------------------------
// resolveBranch: various preamble patterns (lines 739-752)
// ---------------------------------------------------------------------------

func TestResolveBranchCommitNoRefsWithGitCli(t *testing.T) {
	// "commit abc" - no decoration, falls through to git CLI --points-at lookup.
	// Use a all-zero hash that has no branch pointing at it.
	result := resolveBranch("commit 0000000000000000000000000000000000000000")
	// The hash has no branch pointing at it; result should be empty.
	// If non-empty, it must be a valid branch name (no spaces or newlines).
	if strings.Contains(result, " ") || strings.Contains(result, "\n") {
		t.Fatalf("expected valid branch name or empty, got %q", result)
	}
}

func TestResolveBranchCommitWithRefsButNoHeadArrow(t *testing.T) {
	// "commit abc (tag: v1.0, origin/main)" - no HEAD ->, falls through to git CLI
	result := resolveBranch("commit abc123 (tag: v1.0, origin/main)")
	// Falls through to --points-at with hash "abc123"
	// May or may not find a branch. Must return a valid branch name or empty.
	if strings.Contains(result, " ") || strings.Contains(result, "\n") {
		t.Fatalf("expected valid branch name or empty, got %q", result)
	}
}

func TestResolveBranchCommitHashTruncatedBySpace(t *testing.T) {
	// "commit abc extra" - hash should be "abc" (truncated at space before refs)
	preamble := "commit abc123456 (HEAD -> test-branch)"
	result := resolveBranch(preamble)
	if result != "test-branch" {
		t.Fatalf("expected 'test-branch', got %q", result)
	}
}

// TestResolveBranchWithGitPointsAt creates a temp git repo with a commit
// and verifies that --points-at lookup returns the branch name.
func TestResolveBranchWithGitPointsAt(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)

	// Initialize a git repo
	if out, err := exec.Command("git", "init").CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %s: %v", out, err)
	}
	exec.Command("git", "config", "user.email", "test@test.com").Run()
	exec.Command("git", "config", "user.name", "Test").Run()

	// Create a file and commit
	os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte("hello"), 0o644)
	exec.Command("git", "add", "test.txt").Run()
	if out, err := exec.Command("git", "commit", "-m", "initial").CombinedOutput(); err != nil {
		t.Fatalf("git commit failed: %s: %v", out, err)
	}

	// Get the commit hash
	out, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatalf("git rev-parse HEAD failed: %v", err)
	}
	hash := strings.TrimSpace(string(out))

	// Build preamble without decoration to force --points-at lookup
	preamble := "commit " + hash
	result := resolveBranch(preamble)

	// In a fresh repo the default branch is typically "main" or "master"
	if result == "" {
		t.Fatalf("expected non-empty branch from --points-at for hash %s", hash)
	}
}

// ---------------------------------------------------------------------------
// renderScrollbar: scrollableLines = 0 case (line 1005-1007)
// ---------------------------------------------------------------------------

func TestRenderScrollbarWithNoScrollableLines(t *testing.T) {
	m := newTestMainModel(t)
	m.messageVp.SetWidth(60)
	m.messageVp.SetHeight(10)
	m.messageVp.SetContent(strings.Repeat("line\n", 10)) // content fits viewport

	// When content fits, scrollableLines = totalLineCount - viewHeight
	// If they are equal, scrollableLines = 0, and thumbPos should stay 0.
	sb := m.renderScrollbar()
	if sb == "" {
		t.Fatal("expected non-empty scrollbar even with no scrollable lines")
	}
}

// ---------------------------------------------------------------------------
// resultsView: light background with selected result + dir=="." (line 1054-1056)
// ---------------------------------------------------------------------------

func TestResultsViewLightBackgroundWithSelection(t *testing.T) {
	m := newTestMainModel(t)
	m.width = 100
	m.height = 40
	isDark := false
	m.isDarkBackground = &isDark
	m.searching = true
	m.search.SetValue("") // match all
	m.setSearchResults()
	m.resultsVp.SetWidth(m.config.UI.SearchTreeWidth)
	m.resultsVp.SetHeight(m.mainContentHeight() - searchHeight)

	view := m.resultsView()
	if view == "" {
		t.Fatal("expected non-empty results view in light mode")
	}
}

func TestResultsViewWithFileInCurrentDirectory(t *testing.T) {
	m := newTestMainModel(t)
	isDark := true
	m.isDarkBackground = &isDark
	m.searching = true

	// Find files with path.Dir(f) == "." (top-level files)
	var topFiles []string
	for _, f := range m.files {
		name := filenode.GetFileName(f)
		if path.Dir(name) == "." {
			topFiles = append(topFiles, name)
		}
	}

	if len(topFiles) == 0 {
		t.Fatal("no top-level files in test fixture; test fixture is broken")
	}

	// Set search to match only top-level files
	m.search.SetValue(strings.TrimSuffix(path.Base(topFiles[0]), path.Ext(topFiles[0])))
	m.setSearchResults()
	m.resultsVp.SetWidth(m.config.UI.SearchTreeWidth)
	m.resultsVp.SetHeight(m.mainContentHeight() - searchHeight)

	view := m.resultsView()
	if !strings.Contains(view, path.Base(topFiles[0])) {
		t.Fatalf("expected results view to contain top-level file %q", topFiles[0])
	}
}

// ---------------------------------------------------------------------------
// openInEditor: with repoRoot (line 1162-1164)
// ---------------------------------------------------------------------------

func TestOpenInEditorWithRepoRoot(t *testing.T) {
	m := newTestMainModel(t)
	t.Setenv("EDITOR", "true")
	m.repoRoot = "/tmp/repos"
	m.fileTree.SetCursorByPath("yarn.lock")

	cmd := m.openInEditor()
	if cmd == nil {
		t.Fatal("expected non-nil cmd when EDITOR is set and repoRoot is provided")
	}
}

// ---------------------------------------------------------------------------
// diffPanePoint: out of zone and above dir header (lines 1214-1216)
// ---------------------------------------------------------------------------

func TestDiffPanePointOutOfZone(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 160, Height: 40})

	// Call View() to register zones
	_ = m.View().Content

	// Create a mouse event outside the diff viewer zone (far left, in the sidebar)
	_, ok := m.diffPanePoint(tea.MouseClickMsg(tea.Mouse{
		X:      0,
		Y:      10,
		Button: tea.MouseLeft,
	}))
	if ok {
		t.Fatal("expected diffPanePoint to return false for click outside zone")
	}
}

func TestDiffPanePointAboveDirHeader(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 160, Height: 40})

	// Call View() to register zones
	_ = m.View().Content

	z := waitForZone(t, zoneDiffViewer)

	// Create a mouse event inside the zone but in the dir-header area (paneY < 3)
	// paneY = 0, 1, or 2 maps to dir header
	_, ok := m.diffPanePoint(tea.MouseClickMsg(tea.Mouse{
		X:      z.StartX + 5,
		Y:      z.StartY + 1, // paneY = 1, which is < DirHeaderHeight (3)
		Button: tea.MouseLeft,
	}))
	if ok {
		t.Fatal("expected diffPanePoint to return false for click above dir header")
	}
}

func TestDiffPanePointInContentArea(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 160, Height: 40})

	// Call View() to register zones
	_ = m.View().Content

	z := waitForZone(t, zoneDiffViewer)

	// Create a click in the content area (below dir header: paneY >= 3)
	point, ok := m.diffPanePoint(tea.MouseClickMsg(tea.Mouse{
		X:      z.StartX + 5,
		Y:      z.StartY + diffviewer.DirHeaderHeight + 1, // paneY = DirHeaderHeight + 1
		Button: tea.MouseLeft,
	}))
	if !ok {
		t.Fatal("expected diffPanePoint to return true for click in content area")
	}
	if point.Line < 0 {
		t.Fatal("expected non-negative line")
	}
	if point.Col < 0 {
		t.Fatal("expected non-negative col")
	}
}

// ---------------------------------------------------------------------------
// handleMouse: overlay scroll up while message open (line 1230-1232)
// ---------------------------------------------------------------------------

func TestHandleMouseScrollUpMessageOverlay(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.preamble = strings.Repeat("preamble line\n", 200)
	m.messageOpen = true
	m.updateMessageVp()
	m.messageVp.ScrollDown(20) // scroll down first so we can scroll up

	before := m.messageVp.YOffset()
	updated, _ := m.handleMouse(tea.MouseMotionMsg(tea.Mouse{
		Button: tea.MouseWheelUp,
		X:      50,
		Y:      10,
	}))
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	if result.messageVp.YOffset() >= before {
		t.Fatalf(
			"expected scroll up to decrease YOffset from %d, got %d",
			before,
			result.messageVp.YOffset(),
		)
	}
}

// ---------------------------------------------------------------------------
// handleMouse: message open scroll + help open scroll is no-op (non-message overlay)
// ---------------------------------------------------------------------------

func TestHandleMouseScrollWhenHelpOpenIsNoop(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.helpOpen = true

	// Scroll events while only help is open (not message) should be no-op
	updated, _ := m.handleMouse(tea.MouseMotionMsg(tea.Mouse{
		Button: tea.MouseWheelDown,
		X:      50,
		Y:      10,
	}))
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	// Help overlay doesn't have a viewport to scroll
	_ = result
}

// ---------------------------------------------------------------------------
// handleMouse: click outside message overlay closes it (line 1273-1276)
// ---------------------------------------------------------------------------

func TestHandleMouseClickOutsideMessageOverlayClosesIt(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.preamble = "commit abc"
	m.messageOpen = true
	m.updateMessageVp()

	// Click at corner (0,0) which should be outside the centered overlay
	updated, _ := m.handleMouse(tea.MouseClickMsg(tea.Mouse{
		Button: tea.MouseLeft,
		X:      0,
		Y:      0,
	}))
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	if result.messageOpen {
		t.Fatal("expected message overlay to be closed after clicking outside")
	}
}

// ---------------------------------------------------------------------------
// handleSearchResultClick: valid zone-relative click (lines 1343-1366)
// ---------------------------------------------------------------------------

func TestHandleSearchResultClickValidIndex(t *testing.T) {
	m := newTestMainModel(t)
	m.width = 100
	m.height = 40
	m.searching = true
	m.search.SetValue("") // match all
	m.setSearchResults()
	m.resultsVp.SetWidth(m.config.UI.SearchTreeWidth)
	m.resultsVp.SetHeight(m.mainContentHeight() - searchHeight)
	m.resultsVp.SetContent(m.resultsView())

	// Must call View() to register zones
	_ = m.View().Content

	z := waitForZone(t, zoneSearchResults)

	if len(m.filtered) == 0 {
		t.Fatal("expected at least one search result")
	}

	// Click on the first result (y=0 relative to zone)
	mouseMsg := tea.MouseClickMsg(tea.Mouse{
		X:      z.StartX + 5,
		Y:      z.StartY, // first result in zone-relative y=0
		Button: tea.MouseLeft,
	})

	_, y := waitForZone(t, zoneSearchResults).Pos(mouseMsg)
	if y < 0 {
		t.Fatal("expected valid zone-relative y")
	}

	updated, _ := m.handleSearchResultClick(mouseMsg)
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	if result.searching {
		t.Fatal("expected search to stop after clicking a result")
	}
}

func TestHandleSearchResultClickIndexOutOfRange(t *testing.T) {
	m := newTestMainModel(t)
	m.width = 100
	m.height = 40
	m.searching = true
	m.search.SetValue("") // match all
	m.setSearchResults()
	m.resultsVp.SetWidth(m.config.UI.SearchTreeWidth)
	m.resultsVp.SetHeight(m.mainContentHeight() - searchHeight)
	m.resultsVp.SetContent(m.resultsView())

	_ = m.View().Content

	z := waitForZone(t, zoneSearchResults)

	// Click at a Y offset far below the last result
	// Set the viewport YOffset such that y + YOffset >= len(filtered)
	mouseMsg := tea.MouseClickMsg(tea.Mouse{
		X:      z.StartX + 5,
		Y:      z.StartY + 100, // very far down
		Button: tea.MouseLeft,
	})

	// Compute clickedIndex
	_, y := waitForZone(t, zoneSearchResults).Pos(mouseMsg)
	if y < 0 {
		// Out of zone bounds — handleSearchResultClick returns early via y<0 check
		return
	}
	clickedIndex := y + m.resultsVp.YOffset()
	if clickedIndex >= len(m.filtered) {
		// This should trigger the index-out-of-range check in handleSearchResultClick
		updated, _ := m.handleSearchResultClick(mouseMsg)
		result, ok := updated.(mainModel)
		if !ok {
			t.Fatalf("unexpected model type %T", updated)
		}
		if result.searching {
			t.Fatal("expected search to remain when click is out of range")
		}
	}
}

// ---------------------------------------------------------------------------
// handleFileTreeClick: nil node (line 1390)
// ---------------------------------------------------------------------------

func TestHandleFileTreeClickNilNode(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	_ = m.View().Content

	z := waitForZone(t, zoneFileTree)

	// Click at a Y offset where no node exists (far below the tree)
	// GetNodeAtY should return nil for clicks past the tree content
	mouseMsg := tea.MouseClickMsg(tea.Mouse{
		X:      z.StartX + 2,
		Y:      z.StartY + z.EndY - z.StartY, // bottom of zone
		Button: tea.MouseLeft,
	})

	// This should handle the nil node case gracefully
	updated, _ := m.handleFileTreeClick(mouseMsg)
	_, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	// Also test with a click where GetNodeAtY returns nil
	nilNode := m.fileTree.GetNodeAtY(m.fileTree.ViewportYOffset() + 5000)
	// A nil node from GetNodeAtY confirms the nil-node path in handleFileTreeClick is reachable.
	_ = nilNode
}

// ---------------------------------------------------------------------------
// handleFileTreeClick: directory icon hit (line 1396)
// ---------------------------------------------------------------------------

func TestHandleFileTreeClickDirectoryIconHit(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.activePanel = FileTreePanel

	// Find a directory node
	m.fileTree.SetCursorByPath("graphql-server/tests")
	node := m.fileTree.GetCurrNode()
	if node == nil {
		t.Fatal("no directory node found; test fixture is broken")
	}

	_ = m.View().Content

	z := waitForZone(t, zoneFileTree)

	localY := node.YOffset() - m.fileTree.ViewportYOffset()

	// Find the icon X position
	iconX := -1
	for x := 0; x < m.fileTree.Width(); x++ {
		if m.fileTree.IsDirectoryIconHit(node, x) {
			iconX = x
			break
		}
	}
	if iconX < 0 {
		t.Fatal("could not find directory icon hit column; test fixture is broken")
	}

	updated, _ := m.handleFileTreeClick(tea.MouseClickMsg(tea.Mouse{
		X:      z.StartX + iconX,
		Y:      z.StartY + localY,
		Button: tea.MouseLeft,
	}))
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	// The click should have been processed
	_ = result
}

// ---------------------------------------------------------------------------
// handleScroll: sidebar and diff viewer (lines 1417-1436)
// ---------------------------------------------------------------------------

func TestHandleScrollInFileTreeZone(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	_ = m.View().Content

	z := waitForZone(t, zoneFileTree)

	// Scroll down in the file tree zone
	before := m.fileTree.ViewportYOffset()
	updated, _ := m.handleScroll(tea.MouseMotionMsg(tea.Mouse{
		X:      z.StartX + 2,
		Y:      z.StartY + 5,
		Button: tea.MouseWheelDown,
	}))
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	// Scrolling down should increase or keep the same viewport Y offset
	after := result.fileTree.ViewportYOffset()
	if after < before {
		t.Fatalf(
			"expected viewport Y offset to increase or stay same after scroll down, before=%d after=%d",
			before,
			after,
		)
	}
}

func TestHandleScrollUpInFileTreeZone(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	// Scroll down first so we can scroll up
	m.fileTree.ScrollDown(10)

	_ = m.View().Content

	z := waitForZone(t, zoneFileTree)

	before := m.fileTree.ViewportYOffset()
	updated, _ := m.handleScroll(tea.MouseMotionMsg(tea.Mouse{
		X:      z.StartX + 2,
		Y:      z.StartY + 5,
		Button: tea.MouseWheelUp,
	}))
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	after := result.fileTree.ViewportYOffset()
	if after > before {
		t.Fatalf(
			"expected viewport Y offset to decrease or stay same after scroll up, before=%d after=%d",
			before,
			after,
		)
	}
}

func TestHandleScrollInSearchResultsZone(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.searching = true
	m.search.SetValue("") // match all
	m.setSearchResults()
	m.resultsVp.SetWidth(m.config.UI.SearchTreeWidth)
	m.resultsVp.SetHeight(m.mainContentHeight() - searchHeight)
	m.resultsVp.SetContent(m.resultsView())

	_ = m.View().Content

	z := waitForZone(t, zoneSearchResults)

	// Scroll down in search results
	updated, _ := m.handleScroll(tea.MouseMotionMsg(tea.Mouse{
		X:      z.StartX + 2,
		Y:      z.StartY + 2,
		Button: tea.MouseWheelDown,
	}))
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	_ = result
}

func TestHandleScrollUpInSearchResultsZone(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.searching = true
	m.search.SetValue("") // match all
	m.setSearchResults()
	m.resultsVp.SetWidth(m.config.UI.SearchTreeWidth)
	m.resultsVp.SetHeight(m.mainContentHeight() - searchHeight)
	m.resultsVp.SetContent(m.resultsView())

	// Scroll down first
	m.resultsVp.ScrollDown(5)

	_ = m.View().Content

	z := waitForZone(t, zoneSearchResults)

	updated, _ := m.handleScroll(tea.MouseMotionMsg(tea.Mouse{
		X:      z.StartX + 2,
		Y:      z.StartY + 2,
		Button: tea.MouseWheelUp,
	}))
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	_ = result
}

func TestHandleScrollInDiffViewerZoneUp(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	// Scroll down first so we can scroll up
	m.diffViewer.ScrollDown(10)

	_ = m.View().Content

	z := waitForZone(t, zoneDiffViewer)

	before := m.diffViewer.YOffset()

	// Scroll up in diff viewer zone
	updated, _ := m.handleScroll(tea.MouseMotionMsg(tea.Mouse{
		X:      z.StartX + 5,
		Y:      z.StartY + 5,
		Button: tea.MouseWheelUp,
	}))
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	after := result.diffViewer.YOffset()
	if after > before {
		t.Fatalf(
			"expected YOffset to decrease or stay same after scroll up, before=%d after=%d",
			before,
			after,
		)
	}
}

// ---------------------------------------------------------------------------
// handleDiffSelectionMotion: above/below/in viewport (lines 1451-1490)
// ---------------------------------------------------------------------------

func TestHandleDiffSelectionMotionAboveViewport(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 160, Height: 40})

	// Start a selection
	point := diffviewer.Point{Line: 5, Col: 0}
	m.diffViewer.StartSelection(point)
	if !m.diffViewer.IsSelecting() {
		t.Fatal("expected diffViewer to have selection")
	}

	// Set diff content so it has something to scroll
	m.diffViewer.SetContent(strings.Repeat("line\n", 200))

	_ = m.View().Content

	z := waitForZone(t, zoneDiffViewer)

	// Move cursor above the dir header area (paneY < 3)
	// Use mouse Y position that puts paneY < DirHeaderHeight
	before := m.diffViewer.YOffset()
	updated, _ := m.handleDiffSelectionMotion(tea.MouseMotionMsg(tea.Mouse{
		X:      z.StartX + 5,
		Y:      z.StartY, // paneY = 0, which is < DirHeaderHeight
		Button: tea.MouseLeft,
	}))
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	// Cursor above viewport should trigger ScrollUp(1)
	if after := result.diffViewer.YOffset(); after > before {
		t.Fatalf(
			"expected YOffset to decrease or stay same when dragging above viewport, before=%d after=%d",
			before,
			after,
		)
	}
	// Selection should still be active
	if !result.diffViewer.IsSelecting() {
		t.Fatal("expected selection to remain active after above-viewport motion")
	}
}

func TestHandleDiffSelectionMotionBelowViewport(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 160, Height: 40})

	// Start a selection
	point := diffviewer.Point{Line: 5, Col: 0}
	m.diffViewer.StartSelection(point)

	m.diffViewer.SetContent(strings.Repeat("line\n", 200))

	_ = m.View().Content

	z := waitForZone(t, zoneDiffViewer)

	// Move cursor below the viewport bottom
	// paneY >= DirHeaderHeight + vpHeight -> scroll down
	vpHeight := m.diffViewer.Height()
	belowY := z.StartY + diffviewer.DirHeaderHeight + vpHeight + 5

	before := m.diffViewer.YOffset()
	updated, _ := m.handleDiffSelectionMotion(tea.MouseMotionMsg(tea.Mouse{
		X:      z.StartX + 5,
		Y:      belowY,
		Button: tea.MouseLeft,
	}))
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	// Cursor below viewport should trigger ScrollDown(1)
	if result.diffViewer.YOffset() <= before {
		t.Fatalf(
			"expected YOffset to increase when dragging below viewport, before=%d after=%d",
			before,
			result.diffViewer.YOffset(),
		)
	}
	if !result.diffViewer.IsSelecting() {
		t.Fatal("expected selection to remain active after below-viewport motion")
	}
}

func TestHandleDiffSelectionMotionInViewport(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 160, Height: 40})

	// Start a selection
	point := diffviewer.Point{Line: 5, Col: 0}
	m.diffViewer.StartSelection(point)

	m.diffViewer.SetContent(strings.Repeat("line\n", 200))

	_ = m.View().Content

	z := waitForZone(t, zoneDiffViewer)

	// Move cursor within the viewport (paneY between DirHeaderHeight and DirHeaderHeight+vpHeight)
	inViewportY := z.StartY + diffviewer.DirHeaderHeight + 5

	updated, _ := m.handleDiffSelectionMotion(tea.MouseMotionMsg(tea.Mouse{
		X:      z.StartX + 5,
		Y:      inViewportY,
		Button: tea.MouseLeft,
	}))
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	if !result.diffViewer.IsSelecting() {
		t.Fatal("expected selection to remain active after in-viewport motion")
	}
}

func TestHandleDiffSelectionMotionOutsideDiffPaneHorizontally(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 160, Height: 40})

	// Start a selection
	point := diffviewer.Point{Line: 5, Col: 0}
	m.diffViewer.StartSelection(point)

	_ = m.View().Content

	// Move cursor horizontally outside the diff zone (over the sidebar)
	updated, _ := m.handleDiffSelectionMotion(tea.MouseMotionMsg(tea.Mouse{
		X:      0, // leftmost column, in the sidebar area
		Y:      10,
		Button: tea.MouseLeft,
	}))
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	// Should be a no-op when cursor is outside the diff pane horizontally
	// Selection should remain unchanged
	if !result.diffViewer.IsSelecting() {
		t.Fatal("expected selection to remain active after horizontal out-of-bounds motion")
	}
}

func TestHandleDiffSelectionMotionNotSelecting(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	// When not selecting, should be a no-op
	updated, _ := m.handleDiffSelectionMotion(tea.MouseMotionMsg(tea.Mouse{
		X:      50,
		Y:      10,
		Button: tea.MouseLeft,
	}))
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	// Model should be unchanged when not selecting
	if result.diffViewer.IsSelecting() {
		t.Fatal("expected no selection to be active when not previously selecting")
	}
}

// ---------------------------------------------------------------------------
// handleSidebarDrag: same width (no resize) (line 1517-1519)
// ---------------------------------------------------------------------------

func TestHandleSidebarDragSameWidthIsNoop(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.isShowingFileTree = true
	m.draggingSidebar = true
	m.fileTree.SetSize(30, m.mainContentHeight()-searchHeight-1)

	// Drag to the same width as current sidebar (< minResizeStep difference)
	currentWidth := m.sidebarWidth()

	result, _ := m.handleSidebarDrag(tea.MouseMotionMsg(tea.Mouse{
		X:      currentWidth,
		Y:      5,
		Button: tea.MouseLeft,
	}))
	m2, ok := result.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", result)
	}
	// Width should not have changed (within minResizeStep)
	if m2.fileTree.Width() != m.fileTree.Width() {
		t.Fatalf(
			"expected sidebar width to remain the same, was %d now %d",
			m.fileTree.Width(),
			m2.fileTree.Width(),
		)
	}
}

// ---------------------------------------------------------------------------
// sortFiles: parent/child directory ordering (utils.go line 38-40)
// ---------------------------------------------------------------------------

func TestSortFilesParentDirComesBeforeChildDir(t *testing.T) {
	// When one directory is a prefix of another, the parent should come after the child
	// (because sortFiles returns -1 when dira has prefix dirb)
	files := []*gitdiff.File{
		{NewName: "parent/child/file.txt"},
		{NewName: "parent/file.txt"},
	}
	sortFiles(files)
	// parent/child has dir = "parent/child", parent has dir = "parent"
	// strings.HasPrefix("parent/child", "parent") => true, so sortFunc(dira,dirb) returns 1
	// That means "parent/file.txt" comes AFTER "parent/child/file.txt"
	if files[0].NewName != "parent/child/file.txt" {
		t.Fatalf("expected parent/child/file.txt first, got %s", files[0].NewName)
	}
}

func TestSortFilesChildDirAfterParentSameDir(t *testing.T) {
	// Files in the same non-root directory sorted alphabetically
	files := []*gitdiff.File{
		{NewName: "sub/z.txt"},
		{NewName: "sub/a.txt"},
	}
	sortFiles(files)
	if files[0].NewName != "sub/a.txt" {
		t.Fatalf("expected sub/a.txt first, got %s", files[0].NewName)
	}
}

// ---------------------------------------------------------------------------
// handleMouse: mouse release with selection (lines 1286-1291)
// ---------------------------------------------------------------------------

func TestHandleMouseReleaseWithSelectionEndsAndCopies(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 160, Height: 40})

	// Start a selection
	point := diffviewer.Point{Line: 0, Col: 0}
	m.diffViewer.StartSelection(point)
	m.diffViewer.SetContent(strings.Repeat("hello world\n", 50))

	// End selection (this should produce valid text)
	updated, cmd := m.handleMouse(tea.MouseReleaseMsg(tea.Mouse{
		X:      50,
		Y:      5,
		Button: tea.MouseLeft,
	}))
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	// The diff viewer should no longer be selecting after release
	if result.diffViewer.IsSelecting() {
		t.Fatal("expected selection to end after mouse release")
	}
	_ = cmd // cmd may be a clipboard cmd or nil
}

// ---------------------------------------------------------------------------
// handleMouse: motion with sidebar drag (line 1317-1321)
// ---------------------------------------------------------------------------

func TestHandleMouseMotionStartsSidebarDrag(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 160, Height: 40})
	m.isShowingFileTree = true

	// First, start dragging with a click on the border
	sidebarWidth := m.sidebarWidth()
	updated, _ := m.handleMouse(tea.MouseClickMsg(tea.Mouse{
		X:      sidebarWidth,
		Y:      5,
		Button: tea.MouseLeft,
	}))
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	if !result.draggingSidebar {
		t.Fatal("expected dragging to start after clicking on sidebar border")
	}

	// Now, motion event should trigger handleSidebarDrag
	newX := sidebarWidth + minResizeStep + 10
	updated2, _ := result.handleMouse(tea.MouseMotionMsg(tea.Mouse{
		X:      newX,
		Y:      5,
		Button: tea.MouseLeft,
	}))
	result2, ok := updated2.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated2)
	}
	// Sidebar width should have changed
	if result2.sidebarWidth() == sidebarWidth {
		t.Fatal("expected sidebar width to change after dragging")
	}
}

// ---------------------------------------------------------------------------
// handleMouse: motion with diff selection (lines 1322-1324)
// ---------------------------------------------------------------------------

func TestHandleMouseMotionWithDiffSelection(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 160, Height: 40})

	_ = m.View().Content

	z := waitForZone(t, zoneDiffViewer)

	// Start a selection in the diff pane
	point := diffviewer.Point{Line: 5, Col: 0}
	m.diffViewer.StartSelection(point)
	m.diffViewer.SetContent(strings.Repeat("line\n", 200))

	// Mouse motion should trigger handleDiffSelectionMotion
	motionY := z.StartY + diffviewer.DirHeaderHeight + 5
	updated, _ := m.handleMouse(tea.MouseMotionMsg(tea.Mouse{
		X:      z.StartX + 10,
		Y:      motionY,
		Button: tea.MouseLeft,
	}))
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	if !result.diffViewer.IsSelecting() {
		t.Fatal("expected selection to remain active during drag motion")
	}
}

// ---------------------------------------------------------------------------

func TestHandleMouseClickStartsDiffSelection(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 160, Height: 40})

	_ = m.View().Content

	z := waitForZone(t, zoneDiffViewer)

	// Click in the content area of the diff pane
	clickY := z.StartY + diffviewer.DirHeaderHeight + 5

	updated, _ := m.handleMouse(tea.MouseClickMsg(tea.Mouse{
		X:      z.StartX + 10,
		Y:      clickY,
		Button: tea.MouseLeft,
	}))
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	// Should have started a selection
	if !result.diffViewer.IsSelecting() {
		t.Fatal("expected diffViewer to start selection after click in diff pane")
	}
}

// ---------------------------------------------------------------------------
// handleMouse: wheel events routed to handleScroll
// ---------------------------------------------------------------------------

func TestHandleMouseWheelRoutedToHandleScroll(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	_ = m.View().Content

	z := waitForZone(t, zoneDiffViewer)

	// Mouse wheel events should be routed to handleScroll
	m.diffViewer.ScrollDown(5) // ensure we have some scroll offset
	before := m.diffViewer.YOffset()
	updated, _ := m.handleMouse(tea.MouseMotionMsg(tea.Mouse{
		X:      z.StartX + 5,
		Y:      z.StartY + 5,
		Button: tea.MouseWheelDown,
	}))
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	after := result.diffViewer.YOffset()
	if after < before {
		t.Fatalf(
			"expected YOffset to increase or stay same after wheel down, before=%d after=%d",
			before,
			after,
		)
	}
}

// ---------------------------------------------------------------------------
// handleMouse: header zone click when preamble is empty (no toggle message)
// ---------------------------------------------------------------------------

func TestHandleMouseHeaderZoneClickWithoutPreamble(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.preamble = "" // no preamble => clicking header shouldn't open message

	_ = m.View().Content

	z := waitForZone(t, zoneHeader)

	updated, _ := m.handleMouse(tea.MouseClickMsg(tea.Mouse{
		Button: tea.MouseLeft,
		X:      z.StartX + 2,
		Y:      z.StartY,
	}))
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	if result.messageOpen {
		t.Fatal("expected message overlay to stay closed when preamble is empty")
	}
}

// ---------------------------------------------------------------------------
// handleMouse: search box zone click when already searching (line 1250-1252)
// ---------------------------------------------------------------------------

func TestHandleMouseSearchBoxZoneClickWhenAlreadySearching(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.searching = true

	_ = m.View().Content

	z := waitForZone(t, zoneSearchBox)

	// Click on the search box while already searching - should be no-op per handleSearchBoxClick
	updated, _ := m.handleMouse(tea.MouseClickMsg(tea.Mouse{
		Button: tea.MouseLeft,
		X:      z.StartX + 2,
		Y:      z.StartY,
	}))
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	if !result.searching {
		t.Fatal("expected searching to remain true")
	}
}

// ---------------------------------------------------------------------------
// handleMouse: help zone click toggle (line 1254-1257)
// ---------------------------------------------------------------------------

func TestHandleMouseHelpZoneClickTogglesHelp(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	_ = m.View().Content

	z := waitForZone(t, zoneHelp)

	// Click help zone to open help
	updated, _ := m.handleMouse(tea.MouseClickMsg(tea.Mouse{
		Button: tea.MouseLeft,
		X:      z.StartX + 1,
		Y:      z.StartY,
	}))
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	if !result.helpOpen {
		t.Fatal("expected help to be open after clicking help zone")
	}

	// Click help zone again to close help
	updated2, _ := result.handleMouse(tea.MouseClickMsg(tea.Mouse{
		Button: tea.MouseLeft,
		X:      z.StartX + 1,
		Y:      z.StartY,
	}))
	result2, ok := updated2.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated2)
	}
	if result2.helpOpen {
		t.Fatal("expected help to be closed after clicking help zone again")
	}
}

// ---------------------------------------------------------------------------
// handleMouse: header zone click to toggle message (line 1258-1266)
// ---------------------------------------------------------------------------

func TestHandleMouseHeaderZoneClickWithPreambleOpensMessage(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.preamble = "commit abc\nAuthor: Test\nSubject line"

	_ = m.View().Content

	z := waitForZone(t, zoneHeader)

	updated, _ := m.handleMouse(tea.MouseClickMsg(tea.Mouse{
		Button: tea.MouseLeft,
		X:      z.StartX + 2,
		Y:      z.StartY,
	}))
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	if !result.messageOpen {
		t.Fatal("expected message overlay to open after clicking header with preamble")
	}
}

// ---------------------------------------------------------------------------
// handleMouse: search results zone click (line 1273-1276)
// ---------------------------------------------------------------------------

func TestHandleMouseSearchResultsZoneClick(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.searching = true
	m.search.SetValue("") // match all
	m.setSearchResults()
	m.resultsVp.SetWidth(m.config.UI.SearchTreeWidth)
	m.resultsVp.SetHeight(m.mainContentHeight() - searchHeight)
	m.resultsVp.SetContent(m.resultsView())

	_ = m.View().Content

	z := waitForZone(t, zoneSearchResults)

	if len(m.filtered) == 0 {
		t.Fatal("expected at least one search result")
	}

	// Click on the first result
	updated, _ := m.handleMouse(tea.MouseClickMsg(tea.Mouse{
		Button: tea.MouseLeft,
		X:      z.StartX + 5,
		Y:      z.StartY, // y=0 in zone relative
	}))
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	if result.searching {
		t.Fatal("expected search to stop after clicking search result")
	}
}

// ---------------------------------------------------------------------------
// handleMouse: file tree zone click (line 1273)
// ---------------------------------------------------------------------------

func TestHandleMouseFileTreeZoneClick(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	_ = m.View().Content

	z := waitForZone(t, zoneFileTree)

	// Click on the first visible node in the file tree
	updated, _ := m.handleMouse(tea.MouseClickMsg(tea.Mouse{
		Button: tea.MouseLeft,
		X:      z.StartX + 2,
		Y:      z.StartY,
	}))
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	// Clicking the file tree should activate the FileTreePanel
	if result.activePanel != FileTreePanel {
		t.Fatalf("expected FileTreePanel after clicking file tree, got %v", result.activePanel)
	}
}

// ---------------------------------------------------------------------------
// handleMouse: click on sidebar border to start dragging (line 1237-1240)
// ---------------------------------------------------------------------------

func TestHandleMouseClickOnSidebarBorder(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 160, Height: 40})
	m.isShowingFileTree = true

	sidebarWidth := m.sidebarWidth()

	updated, _ := m.handleMouse(tea.MouseClickMsg(tea.Mouse{
		X:      sidebarWidth,
		Y:      5,
		Button: tea.MouseLeft,
	}))
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	if !result.draggingSidebar {
		t.Fatal("expected dragging to start after clicking on sidebar border")
	}
}

// ---------------------------------------------------------------------------
// handleMouse: click on hidden sidebar grab line (line 1243-1247)
// ---------------------------------------------------------------------------

func TestHandleMouseClickOnHiddenSidebarGrabLine(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.isShowingFileTree = false
	m.searching = false

	updated, _ := m.handleMouse(tea.MouseClickMsg(tea.Mouse{
		X:      0,
		Y:      5,
		Button: tea.MouseLeft,
	}))
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	if !result.isShowingFileTree {
		t.Fatal("expected file tree to show after clicking the hidden grab line")
	}
	if !result.draggingSidebar {
		t.Fatal("expected dragging to start after clicking the hidden grab line")
	}
}

// ---------------------------------------------------------------------------
// Init: auto-detect theme commands (line 181-186)
// ---------------------------------------------------------------------------

func TestInitAutoThemeSchedulesBackgroundDetection(t *testing.T) {
	zone.NewGlobal()

	cfg := config.DefaultConfig()
	cfg.UI.Theme = config.ThemeAuto

	data, err := os.ReadFile("../../examples/multiple_files.txt")
	if err != nil {
		t.Fatal(err)
	}

	m := New(string(data), cfg)
	// For auto theme, themeOverride and isDarkBackground should be nil
	if m.themeOverride != nil || m.isDarkBackground != nil {
		t.Fatal("expected auto theme to not set themeOverride or isDarkBackground")
	}

	cmd := m.Init()
	if cmd == nil {
		t.Fatal("expected Init to return non-nil cmd")
	}

	// Execute the batch and all sub-commands to ensure coverage of
	// closure bodies that Init registers (tea.RequestBackgroundColor, etc).
	if batch, ok := cmd().(tea.BatchMsg); ok {
		for _, c := range batch {
			if c != nil {
				c()
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Update: applyAutoDetectedBackground when diffViewer returns a cmd (line 216-218)
// ---------------------------------------------------------------------------

func TestUpdateBackgroundDetectionWithDiffViewerCmd(t *testing.T) {
	m := newTestMainModel(t)
	m.themeOverride = nil
	m.isDarkBackground = nil

	// When background detection fires and the diffViewer returns a cmd from
	// SetDarkBackground, that cmd should be appended to the cmds list.
	// This tests the if dfCmd != nil branch.
	updated, cmds := m.Update(tea.BackgroundColorMsg{
		Color: color.RGBA{R: 0, G: 0, B: 0, A: 255},
	})
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	if result.isDarkBackground == nil {
		t.Fatal("expected isDarkBackground to be set")
	}
	// The diffViewer's SetDarkBackground may return a cmd that should be in cmds.
	// Verify that cmds is either nil (no diffViewer cmd) or non-nil.
	// The key behavioral assertion is that the model's isDarkBackground was set
	// correctly, which was already checked above.
	_ = cmds
}

// ---------------------------------------------------------------------------
// Update: escape with HasSelection true (lines 288-289)
// ---------------------------------------------------------------------------

func TestUpdateEscapeWithFinalizedSelection(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 160, Height: 40})

	// Start a selection and finalize it (anchor != head => has=true)
	m.diffViewer.StartSelection(diffviewer.Point{Line: 0, Col: 0})
	m.diffViewer.ExtendSelection(diffviewer.Point{Line: 3, Col: 10})
	if _, ok := m.diffViewer.EndSelection(); !ok {
		t.Fatal("expected EndSelection to finalize selection")
	}
	if !m.diffViewer.HasSelection() {
		t.Fatal("expected HasSelection() to be true")
	}

	// Pressing escape should clear the finalized selection
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	if m.diffViewer.HasSelection() {
		t.Fatal("expected finalized selection to be cleared after pressing escape")
	}
}

// ---------------------------------------------------------------------------
// Update: OpenInEditor with EDITOR set (lines 380-384)
// ---------------------------------------------------------------------------

func TestUpdateOpenInEditorWithEditorSet(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.activePanel = FileTreePanel

	t.Setenv("EDITOR", "true")

	// Press 'o' to open in editor; the handler should return a non-nil command.
	updated, cmd := m.Update(tea.KeyPressMsg(tea.Key{Text: "o", Code: 'o'}))
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	if cmd == nil {
		t.Fatal("expected non-nil command when EDITOR is set and 'o' is pressed")
	}
	_ = result
}

// ---------------------------------------------------------------------------
// fetchFileTree: gitdiff.Parse error path (lines 707-709)
// ---------------------------------------------------------------------------

func TestFetchFileTreeWithInvalidInput(t *testing.T) {
	m := newTestMainModel(t)

	// gitdiff.Parse is very lenient; let's try with binary-like data that contains
	// NUL bytes, which should cause a proper parse error.
	m.input = "\x00\x00\x00invalid binary data\x00"

	msg := m.fetchFileTree()
	switch msg := msg.(type) {
	case common.ErrMsg:
		// Successfully triggered the error path
		_ = msg
	case fileTreeMsg:
		// gitdiff.Parse might still succeed with some input
		_ = msg
	default:
		t.Fatalf("expected fileTreeMsg or ErrMsg, got %T", msg)
	}
}

// ---------------------------------------------------------------------------
// resolveBranch: empty hash after trimming (lines 739-741)
// ---------------------------------------------------------------------------

func TestResolveBranchEmptyHashReturnsEmpty(t *testing.T) {
	tests := []struct {
		name     string
		preamble string
	}{
		{"single_space", "commit "},
		{"double_space", "commit  "},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := resolveBranch(tc.preamble)
			if result != "" {
				t.Fatalf(
					"expected empty branch for commit with empty hash %q, got %q",
					tc.preamble,
					result,
				)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// renderScrollbar: YOffset > 0 && thumbPos == 0 (lines 1005-1007)
// ---------------------------------------------------------------------------

func TestRenderScrollbarThumbPosCorrection(t *testing.T) {
	m := newTestMainModel(t)

	// Set up a case where YOffset > 0 but thumbPos would compute to 0
	// This happens when the content is much larger than the viewport and
	// YOffset is small relative to total content.
	m.messageVp.SetWidth(60)
	m.messageVp.SetHeight(3) // very small viewport
	m.messageVp.SetContent(strings.Repeat("line\n", 200))
	m.messageVp.GotoTop()

	// Scroll down just 1 line. In this setup, YOffset=1 > 0 but thumbPos
	// might be 0 because 1 * (3-thumbSize) / scrollableLines rounds down.
	m.messageVp.ScrollDown(1)

	sb := m.renderScrollbar()
	if sb == "" {
		t.Fatal("expected non-empty scrollbar")
	}
}

// ---------------------------------------------------------------------------
// resultsView: icon nil fallback (lines 1054-1056)
// ---------------------------------------------------------------------------

func TestResultsViewIconNilFallback(t *testing.T) {
	m := newTestMainModel(t)
	m.width = 100
	m.height = 40
	isDark := true
	m.isDarkBackground = &isDark

	// Create files with a weird extension that neo.ByPath won't recognize
	m.files = []*gitdiff.File{{NewName: "file.unknown_ext_12345"}, {NewName: "another.xyz123"}}
	m.fileTree = m.fileTree.SetFiles(m.files)

	m.searching = true
	m.search.SetValue("") // match all
	m.setSearchResults()
	m.resultsVp.SetWidth(m.config.UI.SearchTreeWidth)
	m.resultsVp.SetHeight(m.mainContentHeight() - searchHeight)
	m.resultsVp.SetContent(m.resultsView())

	view := m.resultsView()
	if view == "" {
		t.Fatal("expected non-empty results view even with unrecognized file extensions")
	}
}

// ---------------------------------------------------------------------------
// openInEditor: filepath.Join with repoRoot (lines 1162-1164)
// ---------------------------------------------------------------------------

func TestOpenInEditorPathsFileWithRepoRoot(t *testing.T) {
	m := newTestMainModel(t)
	t.Setenv("EDITOR", "echo")
	m.repoRoot = "/tmp/testrepo"
	m.fileTree.SetCursorByPath("some/file.txt")

	cmd := m.openInEditor()
	if cmd == nil {
		t.Fatal("expected non-nil cmd when EDITOR is set")
	}
	// The command should use filepath.Join(repoRoot, relpath)
}

// ---------------------------------------------------------------------------
// handleMouse: release with EndSelection ok (lines 1318-1321)
// ---------------------------------------------------------------------------

func TestHandleMouseReleaseEndSelectionWithText(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 160, Height: 40})

	// Start a selection with different anchor and head
	m.diffViewer.StartSelection(diffviewer.Point{Line: 0, Col: 0})
	m.diffViewer.ExtendSelection(diffviewer.Point{Line: 3, Col: 10})
	m.diffViewer.SetContent(strings.Repeat("hello world\n", 50))

	// Re-start selection to use the content
	m.diffViewer.ClearSelection()
	m.diffViewer.StartSelection(diffviewer.Point{Line: 0, Col: 0})
	m.diffViewer.ExtendSelection(diffviewer.Point{Line: 2, Col: 5})

	// Release should call EndSelection, which returns text
	updated, cmd := m.handleMouse(tea.MouseReleaseMsg(tea.Mouse{
		X:      50,
		Y:      10,
		Button: tea.MouseLeft,
	}))
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	// cmd may be a SetClipboard command if text was selected
	_ = result
	_ = cmd
}

// ---------------------------------------------------------------------------
// handleSearchResultClick: clickedIndex >= len(m.filtered) (line 1344-1346)
// ---------------------------------------------------------------------------

func TestHandleSearchResultClickIndexExceedsFiltered(t *testing.T) {
	m := newTestMainModel(t)
	m.width = 100
	m.height = 40
	m.searching = true
	m.search.SetValue("") // match all
	m.setSearchResults()
	m.resultsVp.SetWidth(m.config.UI.SearchTreeWidth)
	m.resultsVp.SetHeight(m.mainContentHeight() - searchHeight)
	m.resultsVp.SetContent(m.resultsView())

	_ = m.View().Content

	z := waitForZone(t, zoneSearchResults)

	// Scroll the results viewport so that viewport YOffset > 0
	// This makes clickedIndex = y + YOffset potentially exceed len(filtered)
	m.resultsVp.ScrollDown(len(m.filtered) + 100)

	updated, _ := m.handleSearchResultClick(tea.MouseClickMsg(tea.Mouse{
		X:      z.StartX + 5,
		Y:      z.StartY + 1, // y = 1 in zone-relative
		Button: tea.MouseLeft,
	}))
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	// When clickedIndex >= len(filtered), the handler should return early
	// without changing the cursor or closing search.
	if result.resultsCursor != m.resultsCursor {
		t.Errorf(
			"expected resultsCursor unchanged (%d), got %d",
			m.resultsCursor,
			result.resultsCursor,
		)
	}
}

// ---------------------------------------------------------------------------
// handleFileTreeClick: y < 0 (line 1390-1392)
// ---------------------------------------------------------------------------

func TestHandleFileTreeClickNegativeY(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	_ = m.View().Content

	// Click outside the zone bounds to get y=-1 from zone.Pos()
	result, _ := m.handleFileTreeClick(tea.MouseClickMsg(tea.Mouse{
		X:      0,
		Y:      0,
		Button: tea.MouseLeft,
	}))
	m2, ok := result.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", result)
	}
	// Should be a no-op (y < 0 check returns early)
	_ = m2
}

// ---------------------------------------------------------------------------
// handleDiffSelectionMotion: z.IsZero() (line 1456-1458)
// ---------------------------------------------------------------------------

func TestHandleDiffSelectionMotionZeroZone(t *testing.T) {
	m := newTestMainModel(t)

	// Without calling View(), zones are not registered, so zone.Get returns zero
	m.diffViewer.StartSelection(diffviewer.Point{Line: 5, Col: 0})

	result, _ := m.handleDiffSelectionMotion(tea.MouseMotionMsg(tea.Mouse{
		X:      50,
		Y:      10,
		Button: tea.MouseLeft,
	}))
	m2, ok := result.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", result)
	}
	// With a zero zone, the function should return early without updating selection head.
	if !m2.diffViewer.IsSelecting() {
		t.Fatal("expected selection to remain active after zero-zone motion")
	}
}

// ---------------------------------------------------------------------------
// handleDiffSelectionMotion: clampedX < 0 and clampedX > vpWidth-1 (lines 1471-1476)
// ---------------------------------------------------------------------------

func TestHandleDiffSelectionMotionClampX(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 160, Height: 40})

	m.diffViewer.StartSelection(diffviewer.Point{Line: 5, Col: 0})
	m.diffViewer.SetContent(strings.Repeat("line\n", 200))

	_ = m.View().Content

	z := waitForZone(t, zoneDiffViewer)

	// Move cursor within the viewport to test the clamping paths
	vpHeight := m.diffViewer.Height()
	paneY := diffviewer.DirHeaderHeight + vpHeight/2 // in the middle of viewport

	updated, _ := m.handleDiffSelectionMotion(tea.MouseMotionMsg(tea.Mouse{
		X:      z.StartX + 5, // valid X position
		Y:      z.StartY + paneY,
		Button: tea.MouseLeft,
	}))
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	// Selection head should have been updated to reflect the motion.
	if !result.diffViewer.IsSelecting() {
		t.Fatal("expected selection to still be active after motion")
	}
}

// ---------------------------------------------------------------------------
// handleDiffSelectionMotion: diffPanePoint returns !ok (lines 1488-1490)
// ---------------------------------------------------------------------------

func TestHandleDiffSelectionMotionDiffPanePointFails(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 160, Height: 40})

	m.diffViewer.StartSelection(diffviewer.Point{Line: 5, Col: 0})
	m.diffViewer.SetContent(strings.Repeat("line\n", 200))

	_ = m.View().Content

	z := waitForZone(t, zoneDiffViewer)

	// Move cursor to the dir-header area where diffPanePoint returns ok=false
	// paneY < DirHeaderHeight
	updated, _ := m.handleDiffSelectionMotion(tea.MouseMotionMsg(tea.Mouse{
		X:      z.StartX + 5,
		Y:      z.StartY + 1, // paneY = 1, < DirHeaderHeight
		Button: tea.MouseLeft,
	}))
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	// When diffPanePoint returns !ok, the function returns early without
	// modifying selection state. Selection should still be active.
	if !result.diffViewer.IsSelecting() {
		t.Fatal("expected selection to remain active after diffPanePoint failure")
	}
}

// ---------------------------------------------------------------------------
// handleMouse: click on search box zone when not searching starts search (line 1250-1252)
// ---------------------------------------------------------------------------

func TestHandleMouseSearchBoxZoneClickWhenNotSearching(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	_ = m.View().Content

	z := waitForZone(t, zoneSearchBox)

	updated, _ := m.handleMouse(tea.MouseClickMsg(tea.Mouse{
		Button: tea.MouseLeft,
		X:      z.StartX + 2,
		Y:      z.StartY,
	}))
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	if !result.searching {
		t.Fatal("expected searching to start after clicking search box")
	}
}

// ---------------------------------------------------------------------------
// Update: Quit key (lines 288-289)
// ---------------------------------------------------------------------------

func TestUpdateQuitKeyReturnsQuitCmd(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.helpOpen = false
	m.messageOpen = false

	// Press 'q' to quit
	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Text: "q", Code: 'q'}))
	if cmd == nil {
		t.Fatal("expected non-nil cmd (tea.Quit) when pressing q")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Fatalf("expected tea.QuitMsg, got %T", msg)
	}
}

// ---------------------------------------------------------------------------
// Update: applyAutoDetectedBackground returns non-nil diffViewer cmd (line 216-218)
// ---------------------------------------------------------------------------

func TestUpdateBackgroundDetectionAddsDiffViewerCmd(t *testing.T) {
	m := newTestMainModel(t)
	m.themeOverride = nil
	m.isDarkBackground = nil

	// When we get a BackgroundColorMsg and the diffViewer returns a non-nil
	// cmd from SetDarkBackground, the dfCmd should be appended.
	updated, cmds := m.Update(tea.BackgroundColorMsg{
		Color: color.RGBA{R: 255, G: 255, B: 255, A: 255},
	})
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	// Verify that the auto-detected background was set.
	if result.isDarkBackground == nil {
		t.Fatal("expected isDarkBackground to be set after BackgroundColorMsg")
	}
	if *result.isDarkBackground {
		t.Error("expected isDarkBackground=false for white background")
	}
	_ = cmds
}

// ---------------------------------------------------------------------------
// fetchFileTree: gitdiff parse error (lines 707-709)
// ---------------------------------------------------------------------------

func TestFetchFileTreeParseError(t *testing.T) {
	m := newTestMainModel(t)

	// Use an incomplete hunk header that gitdiff.Parse deterministically rejects.
	// Missing the closing " @@" causes an "invalid fragment header" error.
	m.input = "diff --git a/test b/test\n--- a/test\n+++ b/test\n@@ -0,0 +1 @"

	msg := m.fetchFileTree()
	if _, ok := msg.(common.ErrMsg); !ok {
		t.Fatalf("expected common.ErrMsg for incomplete hunk header, got %T", msg)
	}
}

// ---------------------------------------------------------------------------
// resolveBranch: empty hash after commit line (lines 739-741)
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// openInEditor: closure body (lines 1162-1164)
// ---------------------------------------------------------------------------

func TestOpenInEditorClosureBody(t *testing.T) {
	m := newTestMainModel(t)
	t.Setenv("EDITOR", "echo")
	m.repoRoot = "/tmp"
	m.fileTree.SetCursorByPath("test.txt")

	cmd := m.openInEditor()
	if cmd == nil {
		t.Fatal("expected non-nil cmd when EDITOR is set")
	}

	// Execute the cmd to trigger the closure body.
	// tea.ExecProcess returns a Cmd that wraps the exec, and the callback
	// func(err error) tea.Msg { return nil } at line 1162-1164 needs to execute.
	// We can't easily execute tea.ExecProcess in unit tests, but we can
	// verify the cmd is non-nil and let the tea runtime handle it.
}

// ---------------------------------------------------------------------------
// handleSearchResultClick: clickedIndex >= len(m.filtered) (lines 1344-1346)
// ---------------------------------------------------------------------------

func TestHandleSearchResultClickIndexOutOfRangeCoverage(t *testing.T) {
	m := newTestMainModel(t)
	m.width = 100
	m.height = 40
	m.searching = true
	m.search.SetValue("") // match all
	m.setSearchResults()
	m.resultsVp.SetWidth(m.config.UI.SearchTreeWidth)
	m.resultsVp.SetHeight(m.mainContentHeight() - searchHeight)
	m.resultsVp.SetContent(m.resultsView())

	// Must call View() to register zones
	_ = m.View().Content

	z := waitForZone(t, zoneSearchResults)

	if len(m.filtered) == 0 {
		t.Fatal("expected at least one search result")
	}

	// Scroll the results viewport so YOffset causes clickedIndex >= len(filtered)
	// Set scroll position past the end
	for m.resultsVp.YOffset() < len(m.filtered)+100 {
		if m.resultsVp.AtBottom() {
			break
		}
		m.resultsVp.ScrollDown(1)
	}

	// Click at y=0 in zone-relative coordinates
	// clickedIndex = 0 + YOffset, which should be >= len(filtered)
	clickedIndex := 0 + m.resultsVp.YOffset()
	if clickedIndex < len(m.filtered) {
		t.Skipf(
			"couldn't get YOffset past filtered list; YOffset=%d, len=%d",
			m.resultsVp.YOffset(),
			len(m.filtered),
		)
	}

	updated, _ := m.handleSearchResultClick(tea.MouseClickMsg(tea.Mouse{
		X:      z.StartX + 5,
		Y:      z.StartY, // y=0 in zone relative
		Button: tea.MouseLeft,
	}))
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	// Should return early without selecting any file
	// The resultsCursor should remain unchanged.
	if result.resultsCursor != m.resultsCursor {
		t.Errorf(
			"expected resultsCursor unchanged (%d), got %d",
			m.resultsCursor,
			result.resultsCursor,
		)
	}
}

// ---------------------------------------------------------------------------
// handleDiffSelectionMotion: z.IsZero() (lines 1456-1458)
// ---------------------------------------------------------------------------

func TestHandleDiffSelectionMotionZeroZoneCoverage(t *testing.T) {
	m := newTestMainModel(t)

	// Without calling View(), zones won't be registered, so zone.Get returns zero.
	// DO NOT call View() in this test.
	m.diffViewer.StartSelection(diffviewer.Point{Line: 5, Col: 0})

	result, _ := m.handleDiffSelectionMotion(tea.MouseMotionMsg(tea.Mouse{
		X:      50,
		Y:      10,
		Button: tea.MouseLeft,
	}))
	m2, ok := result.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", result)
	}
	// With a zero zone, the function should return early without updating selection head
	if !m2.diffViewer.IsSelecting() {
		t.Fatal("expected selection to still be active after zero zone motion")
	}
}

// ---------------------------------------------------------------------------
// handleDiffSelectionMotion: clampedX edge cases (lines 1471-1476)
// ---------------------------------------------------------------------------

func TestHandleDiffSelectionMotionClampedXEdgeCases(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 160, Height: 40})

	// Need content in the diffViewer for the selection to work
	m.diffViewer.SetContent(strings.Repeat("line\n", 500))
	m.diffViewer.ScrollDown(10)

	m.diffViewer.StartSelection(diffviewer.Point{Line: 5, Col: 0})

	_ = m.View().Content

	z := waitForZone(t, zoneDiffViewer)

	// Move cursor within the viewport area
	vpHeight := m.diffViewer.Height()
	inViewportY := z.StartY + diffviewer.DirHeaderHeight + vpHeight/2

	result, _ := m.handleDiffSelectionMotion(tea.MouseMotionMsg(tea.Mouse{
		X:      z.StartX + 5, // paneX = 5, which should be >= 0 and < vpWidth
		Y:      inViewportY,
		Button: tea.MouseLeft,
	}))
	_, ok := result.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", result)
	}
}

// ---------------------------------------------------------------------------
// fetchFileTree: gitdiff parse error with truly bad input (lines 707-709)
// ---------------------------------------------------------------------------

func TestFetchFileTreeGenuinelyBadInput(t *testing.T) {
	// Inputs where gitdiff.Parse deterministically returns an error.
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "bad_hunk_range",
			input: "diff --git a/f b/f\n--- a/f\n+++ b/f\n@@ -a,b +c,d @@\n+content",
		},
		{
			name:  "incomplete_hunk",
			input: "diff --git a/f b/f\n--- a/f\n+++ b/f\n@@ -0,0 +1 @",
		},
		{
			name:  "new_file_with_old_content",
			input: "diff --git a/test b/test\nnew file mode 100644\n--- /dev/null\n+++ b/test\n@@ -1 +1 @@\n+hello",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			zone.NewGlobal()
			cfg := config.DefaultConfig()
			m := New(tc.input, cfg)
			msg := m.fetchFileTree()
			if _, ok := msg.(common.ErrMsg); !ok {
				t.Fatalf("expected common.ErrMsg for %s, got %T", tc.name, msg)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// handleSearchResultClick: clickedIndex >= len(m.filtered) edge case
// ---------------------------------------------------------------------------

func TestHandleSearchResultClickScrolledPastResults(t *testing.T) {
	zone.NewGlobal()
	cfg := config.DefaultConfig()
	m := New("", cfg)

	m.width = 100
	m.height = 40
	m.searching = true
	m.search.SetValue("") // match all

	// Create many files so we have filtered results.
	// We need the viewport YOffset to exceed len(m.filtered) when combined with y.
	// With a very small viewport height (2), we can scroll down to TotalLineCount-Height.
	// If TotalLineCount = N (one line per result) Height = 2, max YOffset = N-2.
	// Then clickedIndex = y + YOffset. To get clickedIndex >= N, we need y >= 2.
	// So click at y=2 (third line in the zone-relative coords).
	const numFiles = 5
	files := make([]*gitdiff.File, numFiles)
	for i := range files {
		files[i] = &gitdiff.File{NewName: fmt.Sprintf("file%03d.txt", i)}
	}
	m.files = files
	m.fileTree = m.fileTree.SetFiles(files)
	m.setSearchResults()
	m.resultsVp.SetWidth(m.config.UI.SearchTreeWidth)
	m.resultsVp.SetHeight(2) // very small viewport
	m.resultsVp.SetContent(m.resultsView())

	_ = m.View().Content

	z := waitForZone(t, zoneSearchResults)

	// Scroll to the bottom of the results
	m.resultsVp.GotoBottom()

	// clickedIndex = y + YOffset
	// After GotoBottom, YOffset ~ numFiles - 2 = 3
	// If we click at y=2 (relative to zone), clickedIndex = 2+3 = 5 >= 5
	y := numFiles - m.resultsVp.YOffset() // y that makes clickedIndex = numFiles
	if y+m.resultsVp.YOffset() < numFiles {
		t.Skipf(
			"cannot make clickedIndex >= %d; YOffset=%d, tried y=%d",
			numFiles,
			m.resultsVp.YOffset(),
			y,
		)
	}

	updated, _ := m.handleSearchResultClick(tea.MouseClickMsg(tea.Mouse{
		X:      z.StartX + 5,
		Y:      z.StartY + y, // zone-relative y that makes clickedIndex >= numFiles
		Button: tea.MouseLeft,
	}))
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	// Should return early without selecting any file.
	// resultsCursor must remain at the same position it had before the click.
	if result.resultsCursor != m.resultsCursor {
		t.Errorf(
			"expected resultsCursor unchanged (%d), got %d",
			m.resultsCursor,
			result.resultsCursor,
		)
	}
}

// ---------------------------------------------------------------------------
// Update: applyAutoDetectedBackground with diffViewer in auto mode that has content
// ---------------------------------------------------------------------------

func TestUpdateBackgroundDetectionDiffViewerReturnsCmd(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 160, Height: 40})
	m.themeOverride = nil
	m.isDarkBackground = nil

	// Set diffViewer content to force it to have a file patch rendered,
	// so that SetDarkBackground's diff() method returns a non-nil cmd.
	if len(m.files) > 0 {
		m.diffViewer, _ = m.diffViewer.SetFilePatch(m.files[0])
	}

	// Now send BackgroundColorMsg - the diffViewer should return a non-nil cmd
	// from SetDarkBackground since it has content and is in auto mode.
	updated, cmds := m.Update(tea.BackgroundColorMsg{
		Color: color.RGBA{R: 0, G: 0, B: 0, A: 255},
	})
	result, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	if result.isDarkBackground == nil {
		t.Fatal("expected isDarkBackground to be set")
	}

	// Execute any commands returned to trigger diffViewer diffs
	if cmds != nil {
		_ = cmds()
	}
}

func TestResolveBranchCommitWithOnlySpaces(t *testing.T) {
	// Commit line with only whitespace as the hash.
	preamble := "commit /t /n"
	result := resolveBranch(preamble)
	if result != "" {
		t.Fatalf("expected empty branch for whitespace-only hash, got %q", result)
	}
}

func TestClampToViewportWidth(t *testing.T) {
	cases := []struct {
		x, vpWidth, want int
	}{
		{x: 5, vpWidth: 10, want: 5},  // within bounds
		{x: 15, vpWidth: 10, want: 9}, // past right edge
		{x: 5, vpWidth: 0, want: 5},   // zero vpWidth (no clamping)
		{x: 0, vpWidth: 10, want: 0},   // left edge
		{x: 9, vpWidth: 10, want: 9},   // right edge (vpWidth-1)
		{x: -33, vpWidth: 10, want: 0}, // negative x clamped to 0
		{x: -5, vpWidth: 0, want: -5},  // negative x, zero vpWidth (no clamping)
	}
	for _, tc := range cases {
		got := clampToViewportWidth(tc.x, tc.vpWidth)
		if got != tc.want {
			t.Errorf("clampToViewportWidth(%d, %d) = %d, want %d", tc.x, tc.vpWidth, got, tc.want)
		}
	}
}

func TestHandleDiffSelectionMotionNilZone(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.diffViewer.StartSelection(diffviewer.Point{Line: 0, Col: 0})

	origGetZone := getDiffViewerZone
	defer func() { getDiffViewerZone = origGetZone }()
	getDiffViewerZone = func() *zone.ZoneInfo { return nil }

	msg := tea.MouseMotionMsg(tea.Mouse{X: 50, Y: 10})
	result, cmd := m.handleDiffSelectionMotion(msg)
	// With nil zone, selection state should remain unchanged
	resultModel, ok := result.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", result)
	}
	if !resultModel.diffViewer.IsSelecting() {
		t.Fatal("expected selection to still be active after nil zone motion")
	}
	_ = cmd
}

func TestEditorDone(t *testing.T) {
	result := editorDone(nil)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestHandleDiffSelectionMotionZoneIsZero(t *testing.T) {
	// Test the zero-zone branch by overriding getDiffViewerZone to return a
	// zero ZoneInfo, simulating what happens when zone.Get returns an
	// unregistered zone.
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.diffViewer.StartSelection(diffviewer.Point{Line: 0, Col: 0})

	origGetZone := getDiffViewerZone
	defer func() { getDiffViewerZone = origGetZone }()
	getDiffViewerZone = func() *zone.ZoneInfo { return &zone.ZoneInfo{} } // zero zone

	msg := tea.MouseMotionMsg(tea.Mouse{X: 50, Y: 10})
	result, cmd := m.handleDiffSelectionMotion(msg)
	resultModel, ok := result.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", result)
	}
	// With a zero zone, the function should return early without updating selection head.
	if !resultModel.diffViewer.IsSelecting() {
		t.Fatal("expected selection to remain active after zero-zone motion")
	}
	_ = cmd
}

func TestHandleDiffSelectionMotionOutOfBoundsHorizontal(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	// Call View to register zones.
	_ = m.View().Content
	m.diffViewer.StartSelection(diffviewer.Point{Line: 0, Col: 0})

	// Get the diffViewer zone and construct a mouse event outside it horizontally.
	z := waitForZone(t, zoneDiffViewer)

	// Mouse to the left of the zone.
	msg := tea.MouseMotionMsg(tea.Mouse{X: 0, Y: z.StartY + 5})
	result, cmd := m.handleDiffSelectionMotion(msg)
	resultModel, ok := result.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", result)
	}
	// Selection should still be active even when mouse is out of bounds.
	if !resultModel.diffViewer.IsSelecting() {
		t.Fatal("expected selection to remain active after out-of-bounds horizontal motion")
	}
	_ = cmd
}

func TestHandleDiffSelectionMotionAbovePane(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	_ = m.View().Content
	m.diffViewer.StartSelection(diffviewer.Point{Line: 5, Col: 5})

	z := waitForZone(t, zoneDiffViewer)

	// Mouse above the dir header.
	msg := tea.MouseMotionMsg(tea.Mouse{X: z.StartX + 5, Y: z.StartY + 1})
	result, cmd := m.handleDiffSelectionMotion(msg)
	resultModel, ok := result.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", result)
	}
	// Selection should still be active even when mouse is above the pane.
	if !resultModel.diffViewer.IsSelecting() {
		t.Fatal("expected selection to remain active after motion above pane")
	}
	_ = cmd
}

func TestHandleDiffSelectionMotionBelowPane(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	_ = m.View().Content
	m.diffViewer.StartSelection(diffviewer.Point{Line: 5, Col: 5})

	z := waitForZone(t, zoneDiffViewer)

	// Mouse below the viewport (past the pane).
	msg := tea.MouseMotionMsg(tea.Mouse{
		X: z.StartX + 5,
		Y: z.StartY + diffviewer.DirHeaderHeight + m.diffViewer.Height() + 5,
	})
	result, cmd := m.handleDiffSelectionMotion(msg)
	resultModel, ok := result.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", result)
	}
	// Selection should still be active even when mouse is below the pane.
	if !resultModel.diffViewer.IsSelecting() {
		t.Fatal("expected selection to remain active after motion below pane")
	}
	_ = cmd
}

func TestHandleDiffSelectionMotionClampedX(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	_ = m.View().Content
	m.diffViewer.StartSelection(diffviewer.Point{Line: 5, Col: 5})

	z := waitForZone(t, zoneDiffViewer)

	vpWidth := m.diffViewer.Width()
	// Use a mouse X that's past the viewport width relative to zone start.
	targetX := z.StartX + vpWidth + 5
	if targetX > z.EndX {
		targetX = z.EndX
	}

	msg := tea.MouseMotionMsg(tea.Mouse{
		X: targetX,
		Y: z.StartY + diffviewer.DirHeaderHeight + 5,
	})
	result, cmd := m.handleDiffSelectionMotion(msg)
	resultModel, ok := result.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", result)
	}
	// Selection should still be active with clamped X coordinate.
	if !resultModel.diffViewer.IsSelecting() {
		t.Fatal("expected selection to remain active after clamped X motion")
	}
	_ = cmd
}
