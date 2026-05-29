package ui

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	zone "github.com/lrstanley/bubblezone/v2"

	"github.com/dlvhdr/diffnav/pkg/config"
)

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
				m = switchToPanel(t, m, panel)
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
				// The scroll keys route to the diff pane regardless of which
				// panel is active, so the file tree staying unchanged and the
				// diff offset changing already proves the panel didn't switch.
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
				m = switchToPanel(t, m, panel)
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
				// The file tree staying unchanged already proves the panel
				// didn't switch; these scroll keys always target the diff pane.
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
	m = drainCmds(t, m, m.fetchFileTree)

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

	// Default panel is FileTreePanel; no need to set it explicitly.
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
	// Default panel is FileTreePanel; no need to set it explicitly.

	// Select the large file (yarn.lock) and render its diff via keyboard
	// navigation through Update, which internally calls setNodeDiff.
	m.fileTree.SetCursorByPath("yarn.lock")
	// Navigate away then back to trigger setNodeDiff through Update.
	m = drainCmds(t, m, keyCmd(t, &m, tea.Key{Text: "p", Code: 'p'}))
	if m.fileTree.CurrNodePath() == "yarn.lock" {
		t.Fatalf(
			"setup: expected prev-file to leave yarn.lock, still on %q",
			m.fileTree.CurrNodePath(),
		)
	}
	m = drainCmds(t, m, keyCmd(t, &m, tea.Key{Text: "n", Code: 'n'}))
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

// Enter on a highlighted file should launch $EDITOR and skip the
// directory-toggle path the filetree pane uses for Enter on dirs.
func TestEnterOnFileOpensEditor(t *testing.T) {
	t.Setenv("EDITOR", "true")
	m := newTestMainModel(t)
	// Default panel is FileTreePanel; no need to set it explicitly.
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
	// Default panel is FileTreePanel; no need to set it explicitly.
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
	// Default panel is FileTreePanel; no need to set it explicitly.
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
				// Default panel is FileTreePanel; no need to set it explicitly.
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

				result := updateMainModel(t, m, tea.MouseClickMsg(tea.Mouse{
					X:      z.StartX + nameX,
					Y:      z.StartY + localY,
					Button: tea.MouseLeft,
				}))

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
				// Default panel is FileTreePanel; no need to set it explicitly.
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

				result := updateMainModel(t, m, tea.MouseClickMsg(tea.Mouse{
					X:      z.StartX + iconX,
					Y:      z.StartY + localY,
					Button: tea.MouseLeft,
				}))

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

func TestUpdateUpMovesInFileTreePanel(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	// Default panel is FileTreePanel; no need to set it explicitly.

	// Move down first, then up
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after up/down in file tree panel")
	}
}

func TestUpdateDownMovesInFileTreePanel(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	// Default panel is FileTreePanel; no need to set it explicitly.

	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after down in file tree panel")
	}
}

func TestUpdateGoesToBottom(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	// Default panel is FileTreePanel; no need to set it explicitly.

	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "G", Code: 'G'}))
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after G")
	}
}

func TestUpdateGoesToTop(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	// Default panel is FileTreePanel; no need to set it explicitly.

	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "g", Code: 'g'}))
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after g")
	}
}

func TestUpdatePrevNextFile(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))

	// Prev file (J in some keymaps; actual binding may vary)
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after prev/next file")
	}
}

func TestUpdatePrevNextFileNoMovementInDiffViewer(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	// Switch to DiffViewerPanel through Update (Tab key).
	m = switchToPanel(t, m, DiffViewerPanel)

	// Navigate to first file then try prev
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))

	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after prev/next file key")
	}
}

func TestUpdatePrevFileAtBoundary(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	// Navigate to first file then press PrevFile (p) which should be a no-op
	for i := 0; i < 50; i++ {
		m.fileTree.PrevFile()
	}

	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "p", Code: 'p'}))
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after prev file at boundary")
	}
}

func TestUpdateNextFileAtBoundary(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	// Navigate to last file then press NextFile (n) which should be a no-op
	for i := 0; i < 50; i++ {
		m.fileTree.NextFile()
	}

	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "n", Code: 'n'}))
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after next file at boundary")
	}
}

func TestToggleIconStyleKey(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	view1 := m.View().Content
	if view1 == "" {
		t.Fatal("expected non-empty view before icon style toggle")
	}

	// Press "i" to cycle icon style
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "i", Code: 'i'}))
	view2 := m.View().Content
	if view2 == "" {
		t.Fatal("expected non-empty view after toggling icon style")
	}
}

func TestToggleDiffView(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 160, Height: 40})

	view1 := m.View().Content
	if view1 == "" {
		t.Fatal("expected non-empty view before toggle")
	}

	// Press "s" to toggle side-by-side
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "s", Code: 's'}))
	view2 := m.View().Content
	if view2 == "" {
		t.Fatal("expected non-empty view after toggling side-by-side")
	}

	// Toggle back
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "s", Code: 's'}))
	view3 := m.View().Content
	if view3 == "" {
		t.Fatal("expected non-empty view after toggling side-by-side back")
	}
}

func TestUpdateToggleNodeInFileTreePanel(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	// Default panel is FileTreePanel; no need to set it explicitly.

	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after Enter in file tree panel")
	}
}

func TestUpdateToggleNodeInDiffViewerPanel(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	// Switch to DiffViewerPanel through Update (Tab key).
	m = switchToPanel(t, m, DiffViewerPanel)

	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after Enter in diff viewer panel")
	}
}

func TestUpdateDefaultKeyInDiffViewer(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	// Switch to DiffViewerPanel through Update (Tab key).
	m = switchToPanel(t, m, DiffViewerPanel)

	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "x", Code: 'x'}))
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after unhandled key in diff viewer")
	}
}

func TestUpdateDefaultKeyInFileTree(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	// Default panel is FileTreePanel; no need to set it explicitly.

	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "x", Code: 'x'}))
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after unhandled key in file tree")
	}
}

func TestUpdateDownInDiffViewerPanel(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	// Switch to DiffViewerPanel through Update (Tab key).
	m = switchToPanel(t, m, DiffViewerPanel)
	m.diffViewer.SetContent(strings.Repeat("line\n", 500))

	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after Down in diff viewer panel")
	}
}

func TestUpdateUpInDiffViewerPanel(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	// Switch to DiffViewerPanel through Update (Tab key).
	m = switchToPanel(t, m, DiffViewerPanel)
	m.diffViewer.SetContent(strings.Repeat("line\n", 500))
	m.diffViewer.ScrollDown(30)

	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after Up in diff viewer panel")
	}
}

func TestUpdateUpDownInDiffViewerPanel(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	// Switch to DiffViewerPanel through Update (Tab key).
	m = switchToPanel(t, m, DiffViewerPanel)
	m.diffViewer.SetContent(strings.Repeat("line\n", 500))

	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after up/down in diff viewer")
	}
}

func TestUpdateDiffLineUpDown(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	// Switch to DiffViewerPanel through Update (Tab key).
	m = switchToPanel(t, m, DiffViewerPanel)
	m.diffViewer.SetContent(strings.Repeat("line\n", 500))

	// j = DiffLineDown
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "j", Code: 'j'}))
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after j")
	}

	// k = DiffLineUp
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "k", Code: 'k'}))
	view = m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after k")
	}
}

func TestUpdateDiffCtrlDCtrlUInDiffPanel(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	// Switch to DiffViewerPanel through Update (Tab key).
	m = switchToPanel(t, m, DiffViewerPanel)
	m.diffViewer.SetContent(strings.Repeat("line\n", 500))

	// Ctrl+D in diff viewer (not in overlay)
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: 'd', Mod: tea.ModCtrl}))
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after Ctrl+D in diff viewer")
	}

	// Ctrl+U
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: 'u', Mod: tea.ModCtrl}))
	view = m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after Ctrl+U in diff viewer")
	}
}

func TestUpdateDiffPageDownUp(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	// Switch to DiffViewerPanel through Update (Tab key).
	m = switchToPanel(t, m, DiffViewerPanel)
	m.diffViewer.SetContent(strings.Repeat("line\n", 500))

	// PageDown
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyPgDown}))
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after PageDown in diff viewer")
	}

	// PageUp
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyPgUp}))
	view = m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after PageUp in diff viewer")
	}
}

func TestUpdateDiffTopBottom(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	// Switch to DiffViewerPanel through Update (Tab key).
	m = switchToPanel(t, m, DiffViewerPanel)
	m.diffViewer.SetContent(strings.Repeat("line\n", 500))

	// G (bottom)
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "G", Code: 'G'}))
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after G in diff viewer")
	}

	// g (top)
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "g", Code: 'g'}))
	view = m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after g in diff viewer")
	}
}

func TestUpdateOpenInEditorKeyWithEditor(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	t.Setenv("EDITOR", "true")

	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Text: "o", Code: 'o'}))
	if cmd == nil {
		t.Fatal("expected non-nil cmd when pressing o with EDITOR set")
	}
}

func TestUpdateOpenInEditorNoEditor(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	t.Setenv("EDITOR", "")

	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Text: "o", Code: 'o'}))
	// cmd should be nil since no editor configured
	_ = cmd
}

func TestOpenInEditorNoFilesBehavior(t *testing.T) {
	// Create a model without files so openInEditor returns nil.
	zone.NewGlobal()
	m := New("", config.DefaultConfig())
	t.Setenv("EDITOR", "true")
	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Text: "o", Code: 'o'}))
	if cmd != nil {
		t.Fatal("expected nil cmd when no files")
	}
}

func TestOpenInEditorWithRepoRootSet(t *testing.T) {
	m := newTestMainModel(t)
	t.Setenv("EDITOR", "true")
	// Drive repoRoot assignment through Update instead of setting private field.
	m = updateMainModel(t, m, repoRootMsg("/tmp"))

	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Text: "o", Code: 'o'}))
	if cmd == nil {
		t.Fatal("expected non-nil cmd when EDITOR is set and repoRoot is provided")
	}
}

func TestEditorDoneCallback(t *testing.T) {
	if result := editorDone(nil); result != nil {
		t.Fatalf("expected editorDone(nil) to return nil, got %v", result)
	}
	if result := editorDone(fmt.Errorf("some error")); result != nil {
		t.Fatalf("expected editorDone(err) to return nil, got %v", result)
	}
}

func TestUpdateCopyKeyRendersView(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "y", Code: 'y'}))
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after pressing y (copy)")
	}
}

func TestSetNodeDiffNilNode(t *testing.T) {
	// NOTE: setNodeDiff is a private method, but the nil-node branch cannot be
	// triggered through Update because fileTree.GetCurrNode() never returns nil
	// on a properly constructed model. Leaving private access for coverage.
	result, cmd := newTestMainModel(t).setNodeDiff(nil)
	if cmd != nil {
		t.Fatal("expected nil cmd for nil node")
	}
	_ = result
}

func TestSetNodeDiffNilNodeViaMoveToFile(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	// Press up many times in file tree to reach top, then keep pressing up
	for i := 0; i < 50; i++ {
		m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	}

	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after navigating to top of file tree")
	}
}

func TestUpdateTopBottomInDiffViewer(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	// Switch to DiffViewerPanel through Update (Tab key).
	m = switchToPanel(t, m, DiffViewerPanel)
	m.diffViewer.SetContent(strings.Repeat("line\n", 500))

	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "G", Code: 'G'}))
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "g", Code: 'g'}))
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after G/g in diff viewer")
	}
}
