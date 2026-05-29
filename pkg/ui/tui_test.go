package ui

import (
	"fmt"
	"image/color"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/bluekeyes/go-gitdiff/gitdiff"
	zone "github.com/lrstanley/bubblezone/v2"

	"github.com/dlvhdr/diffnav/pkg/config"

	"github.com/dlvhdr/diffnav/pkg/filenode"
	"github.com/dlvhdr/diffnav/pkg/ui/common"
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

func TestRelativeTime(t *testing.T) {
	cases := []struct {
		name string
		ago  time.Duration
		want string
	}{
		{"now", 0, "now"},
		{"minutes", 5 * time.Minute, "5m"},
		{"minute_boundary", 59 * time.Second, "now"},
		{"hours", 2 * time.Hour, "2h"},
		{"hour_boundary", 59 * time.Minute, "59m"},
		{"days", 5 * 24 * time.Hour, "5d"},
		{"day_boundary", 23 * time.Hour, "23h"},
		{"months", 90 * 24 * time.Hour, "3mo"},
		{"month_boundary", 29 * 24 * time.Hour, "29d"},
		{"years", 400 * 24 * time.Hour, "1y"},
		{"multiple_years", 800 * 24 * time.Hour, "2y"},
		{"year_boundary", 364 * 24 * time.Hour, "12mo"},
		{"exactly_one_hour", 60 * time.Minute, "1h"},
		{"exactly_one_day", 24 * time.Hour, "1d"},
		{"exactly_30_days", 30 * 24 * time.Hour, "1mo"},
		{"future", 5 * time.Minute, "now"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			var ts time.Time
			if tc.name == "future" {
				ts = time.Now().Add(tc.ago) // positive duration = future time
			} else {
				ts = time.Now().Add(-tc.ago)
			}
			got := relativeTime(ts)
			if got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// abs tests
// ---------------------------------------------------------------------------

func TestAbs(t *testing.T) {
	cases := []struct {
		name  string
		input int
		want  int
	}{
		{"positive", 5, 5},
		{"negative", -5, 5},
		{"zero", 0, 0},
		{"large_negative", -1000, 1000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := abs(tc.input); got != tc.want {
				t.Fatalf("expected %d, got %d", tc.want, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// sortFiles tests
// ---------------------------------------------------------------------------

func TestSortFiles(t *testing.T) {
	cases := []struct {
		name      string
		files     []*gitdiff.File
		wantNames []string
	}{
		{
			"same_directory_sorted_alphabetically",
			[]*gitdiff.File{{NewName: "dir/z.txt"}, {NewName: "dir/a.txt"}},
			[]string{"dir/a.txt", "dir/z.txt"},
		},
		{
			"top_level_same_directory_alphabetically",
			[]*gitdiff.File{{NewName: "b.txt"}, {NewName: "a.txt"}},
			[]string{"a.txt", "b.txt"},
		},
		{
			"parent_dir_before_child_dir",
			[]*gitdiff.File{{NewName: "parent/file.txt"}, {NewName: "parent/child/file.txt"}},
			[]string{"parent/child/file.txt", "parent/file.txt"},
		},
		{
			"sibling_directories_sorted_by_full_path",
			[]*gitdiff.File{{NewName: "b-dir/file.txt"}, {NewName: "a-dir/file.txt"}},
			[]string{"a-dir/file.txt", "b-dir/file.txt"},
		},
		{
			"top_level_and_nested_different_dirs",
			[]*gitdiff.File{{NewName: "z.txt"}, {NewName: "a-dir/file.txt"}},
			[]string{"a-dir/file.txt", "z.txt"},
		},
		{
			"empty_slice",
			[]*gitdiff.File{},
			[]string{},
		},
		{
			"case_insensitive",
			[]*gitdiff.File{{NewName: "B.txt"}, {NewName: "a.txt"}},
			[]string{"a.txt", "B.txt"},
		},
		{
			"non_root_before_root_name_dir",
			[]*gitdiff.File{{NewName: "/root-file.txt"}, {NewName: "sub/file.txt"}},
			[]string{"sub/file.txt", "/root-file.txt"},
		},
		{
			"root_name_dir_after_non_root",
			[]*gitdiff.File{{NewName: "sub/file.txt"}, {NewName: "/root-file.txt"}},
			[]string{"sub/file.txt", "/root-file.txt"},
		},
		{
			"parent_dir_comes_before_child_dir",
			[]*gitdiff.File{{NewName: "parent/child/file.txt"}, {NewName: "parent/file.txt"}},
			[]string{"parent/child/file.txt", "parent/file.txt"},
		},
		{
			"child_dir_after_parent_same_dir",
			[]*gitdiff.File{{NewName: "sub/z.txt"}, {NewName: "sub/a.txt"}},
			[]string{"sub/a.txt", "sub/z.txt"},
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			sortFiles(tc.files)
			var gotNames []string
			for _, f := range tc.files {
				gotNames = append(gotNames, f.NewName)
			}
			if len(gotNames) != len(tc.wantNames) {
				t.Fatalf("expected %d files, got %d", len(tc.wantNames), len(gotNames))
			}
			for i, got := range gotNames {
				if got != tc.wantNames[i] {
					t.Fatalf("position %d: expected %q, got %q", i, tc.wantNames[i], got)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// cycleIconStyle tests
// ---------------------------------------------------------------------------

func TestCycleIconStyle(t *testing.T) {
	t.Run("Steps", func(t *testing.T) {
		cases := []struct {
			name  string
			input string
			want  string
		}{
			{"ascii_to_unicode", filenode.IconsASCII, filenode.IconsUnicode},
			{"unicode_to_nerd_status", filenode.IconsUnicode, filenode.IconsNerdStatus},
			{"nerd_status_to_nerd_simple", filenode.IconsNerdStatus, filenode.IconsNerdSimple},
			{"nerd_simple_to_nerd_filetype", filenode.IconsNerdSimple, filenode.IconsNerdFiletype},
			{"nerd_filetype_to_nerd_full", filenode.IconsNerdFiletype, filenode.IconsNerdFull},
			{"nerd_full_wraps_to_ascii", filenode.IconsNerdFull, filenode.IconsASCII},
			{"unknown_resets_to_ascii", "unknown-style", filenode.IconsASCII},
		}
		for _, tc := range cases {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				m := newTestMainModel(t)
				m.iconStyle = tc.input
				m.cycleIconStyle()
				if m.iconStyle != tc.want {
					t.Fatalf(
						"expected %q after cycling from %q, got %q",
						tc.want,
						tc.input,
						m.iconStyle,
					)
				}
			})
		}
	})

	t.Run("FullCycle", func(t *testing.T) {
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
	})
}

// ---------------------------------------------------------------------------
// parseCommitMeta tests
// ---------------------------------------------------------------------------

func TestParseCommitMeta(t *testing.T) {
	strPtr := func(s string) *string { return &s }

	t.Run("Static", func(t *testing.T) {
		cases := []struct {
			name       string
			preamble   string
			wantHash   *string
			wantAuthor *string
			wantDate   *string
		}{
			{
				"full_preamble",
				"commit abcdef1234567890 (HEAD -> main)\nAuthor: John Doe <john@example.com>\nAuthorDate: Mon Jan 2 15:04:05 2006 -0700",
				strPtr("abcdef1"),
				strPtr("JDoe"),
				nil,
			},
			{"empty_preamble", "", strPtr(""), strPtr(""), strPtr("")},
			{
				"short_hash",
				"commit abc\nAuthor: Test User <test@test.com>",
				strPtr("abc"), nil, nil,
			},
			{
				"hash_truncated",
				"commit abcdefghijklmnop\nAuthor: Test <t@t.com>",
				strPtr("abcdefg"), nil, nil,
			},
			{
				"hash_with_refs_decoration",
				"commit abcdef1234567890 (HEAD -> main, tag: v1.0)\nAuthor: Jane Smith <jane@example.com>",
				strPtr("abcdef1"),
				nil,
				nil,
			},
			{
				"author_with_email_only",
				"commit abc1234567890\nAuthor: <user@example.com>",
				nil, strPtr("user"), nil,
			},
			{
				"author_single_name",
				"commit abc1234567890\nAuthor: SingleName <s@test.com>",
				nil, strPtr("SingleName"), nil,
			},
			{
				"date_invalid_format",
				"commit abc1234567890\nAuthor: Test <t@t.com>\nAuthorDate: not-a-date",
				nil, nil, strPtr(""),
			},
		}
		for _, tc := range cases {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				m := newTestMainModel(t)
				m.preamble = tc.preamble
				meta := m.parseCommitMeta()
				if tc.wantHash != nil && meta.hash != *tc.wantHash {
					t.Fatalf("expected hash %q, got %q", *tc.wantHash, meta.hash)
				}
				if tc.wantAuthor != nil && meta.author != *tc.wantAuthor {
					t.Fatalf("expected author %q, got %q", *tc.wantAuthor, meta.author)
				}
				if tc.wantDate != nil && meta.date != *tc.wantDate {
					t.Fatalf("expected date %q, got %q", *tc.wantDate, meta.date)
				}
			})
		}
	})

	t.Run("DateAuthorDate", func(t *testing.T) {
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
	})

	t.Run("DateDate", func(t *testing.T) {
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
	})

	t.Run("AuthorDatePreferredOverDate", func(t *testing.T) {
		m := newTestMainModel(t)
		past := time.Now().Add(-30 * time.Minute)
		m.preamble = fmt.Sprintf(
			"commit abc1234567890\nAuthor: Test <t@t.com>\nAuthorDate: %s\nDate: Mon Jan 1 00:00:00 2000 +0000",
			past.Format("Mon Jan 2 15:04:05 2006 -0700"),
		)
		meta := m.parseCommitMeta()
		if meta.date == "" {
			t.Fatal("expected date to be set from AuthorDate")
		}
	})

	t.Run("DateWithDatePrefix", func(t *testing.T) {
		m := newTestMainModel(t)
		past := time.Now().Add(-1 * time.Hour)
		m.preamble = "commit abc123\nDate: " + past.Format("Mon Jan 2 15:04:05 2006 -0700")
		meta := m.parseCommitMeta()
		if meta.date == "" {
			t.Fatal("expected date to be parsed from 'Date:' prefix")
		}
	})
}

// ---------------------------------------------------------------------------
// commitSubject tests
// ---------------------------------------------------------------------------

func TestCommitSubject(t *testing.T) {
	cases := []struct {
		name     string
		preamble string
		want     string
	}{
		{"simple", "commit abc123\nAuthor: Test\nDate: now\n\nFix the bug", "Fix the bug"},
		{"empty_preamble", "", ""},
		{
			"skips_metadata_lines",
			"commit abc123\nAuthor: Test\nAuthorDate: now\nDate: now\nCommit: Test\nCommitDate: now\nMerge: abc def\n\nThe actual subject",
			"The actual subject",
		},
		{"no_subject_only_metadata", "commit abc123\nAuthor: Test\nDate: now", ""},
		{"trims_whitespace", "commit abc123\n  \n  Hello world  ", "Hello world"},
		{
			"with_merge",
			"commit abc123\nMerge: def456\nAuthor: Test\n\nMerge branch 'feature'",
			"Merge branch 'feature'",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestMainModel(t)
			m.preamble = tc.preamble
			got := m.commitSubject()
			if got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// resolveBranch tests
// ---------------------------------------------------------------------------

func TestResolveBranch(t *testing.T) {
	cases := []struct {
		name     string
		preamble string
		want     string
	}{
		{"with_head_arrow", "commit abc123 (HEAD -> feature-branch)", "feature-branch"},
		{"with_multiple_refs", "commit abc123 (HEAD -> main, tag: v1.0, origin/main)", "main"},
		{"no_decoration", "commit 0000000000000000000000000000000000000000", ""},
		{"empty_preamble", "", ""},
		{
			"no_head_arrow",
			"commit 0000000000000000000000000000000000000000 (tag: v1.0, origin/main)",
			"",
		},
		{
			"commit_line_with_extra_content",
			"commit abc123 extra content (HEAD -> develop)",
			"develop",
		},
		{"with_commit_line_and_ref", "commit abc (HEAD -> feature)", "feature"},
		{"only_tag", "commit abc (tag: v1.0)", ""},
		{"commit_hash_truncated_by_space", "commit abc123456 (HEAD -> test-branch)", "test-branch"},
		{"empty_hash_single_space", "commit ", ""},
		{"empty_hash_double_space", "commit  ", ""},
		{"commit_with_only_spaces", "commit \t ", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveBranch(tc.preamble)
			if got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// KeyGroups tests
// ---------------------------------------------------------------------------

func TestKeyGroups(t *testing.T) {
	t.Run("ThreeGroups", func(t *testing.T) {
		groups := KeyGroups()
		if len(groups) != 3 {
			t.Fatalf("expected 3 key groups, got %d", len(groups))
		}
	})

	t.Run("NonEmptyGroups", func(t *testing.T) {
		groups := KeyGroups()
		for i, g := range groups {
			if len(g) == 0 {
				t.Fatalf("expected group %d to have bindings", i)
			}
		}
	})

	t.Run("AllBindingsHaveHelp", func(t *testing.T) {
		groups := KeyGroups()
		for i, group := range groups {
			for j, binding := range group {
				help := binding.Help()
				if help.Desc == "" {
					t.Fatalf("group %d binding %d: expected non-empty help description", i, j)
				}
			}
		}
	})
}
