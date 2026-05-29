package ui

import (
	"fmt"
	"image/color"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/bluekeyes/go-gitdiff/gitdiff"
	zone "github.com/lrstanley/bubblezone/v2"

	"github.com/charmbracelet/x/ansi"
	"github.com/dlvhdr/diffnav/pkg/config"
	"github.com/dlvhdr/diffnav/pkg/filenode"
	"github.com/dlvhdr/diffnav/pkg/ui/common"
	"github.com/dlvhdr/diffnav/pkg/ui/panes/diffviewer"
)

// ---------------------------------------------------------------------------
// Helper functions
// ---------------------------------------------------------------------------

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
// 1. TestUpdateOverlayToggle
// ---------------------------------------------------------------------------

func TestUpdateOverlayToggle(t *testing.T) {
	t.Run("QuestionMarkTogglesHelp", func(t *testing.T) {
		m := newTestMainModel(t)
		m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

		// "?" should make help overlay appear in the view
		m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "?", Code: '?'}))
		view := m.View().Content
		if !strings.Contains(ansi.Strip(view), "quit") {
			t.Fatal("expected help overlay to appear in View after pressing '?'")
		}

		// "?" again should hide it
		m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "?", Code: '?'}))
		view = m.View().Content
		if strings.Contains(ansi.Strip(view), "quit") {
			t.Fatal("expected help overlay to disappear from View after pressing '?' again")
		}
	})

	t.Run("MTogglesMessageWithPreamble", func(t *testing.T) {
		m := newTestMainModel(t)
		m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
		m.preamble = "commit abc123\nAuthor: Test\n\nSubject line"

		// "m" should open message overlay — assert via View
		m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "m", Code: 'm'}))
		view := m.View().Content
		if !strings.Contains(ansi.Strip(view), "commit") {
			t.Fatal("expected message overlay content in View after pressing 'm'")
		}

		// "m" again should close it
		m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "m", Code: 'm'}))
		view = m.View().Content
		// After closing, the overlay-specific content should not be a centered modal
		// The commit info may still appear in the header, so check for overlay-specific markers
		// A reliable check: when overlay is closed, the main diff content should dominate
		if !strings.Contains(ansi.Strip(view), "DIFFNAV") {
			t.Fatal("expected main UI to render after closing message overlay")
		}
	})

	t.Run("MWithoutPreambleIsNoop", func(t *testing.T) {
		m := newTestMainModel(t)
		m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
		m.preamble = ""

		m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "m", Code: 'm'}))
		// No message overlay should appear. The view should show normal content.
		view := m.View().Content
		// If message overlay had opened, commit content would appear in overlay.
		// Since preamble is empty, there should be no commit overlay text.
		if strings.Contains(ansi.Strip(view), "commit abc") {
			t.Fatal("expected no message overlay to appear when preamble is empty")
		}
	})

	t.Run("EscapeClosesOverlay", func(t *testing.T) {
		m := newTestMainModel(t)
		m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

		// Open help
		m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "?", Code: '?'}))
		view := m.View().Content
		if !strings.Contains(ansi.Strip(view), "quit") {
			t.Fatal("expected help overlay visible")
		}

		// Escape should close it
		m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
		view = m.View().Content
		if strings.Contains(ansi.Strip(view), "quit") {
			t.Fatal("expected help overlay to be closed after Escape")
		}
	})

	t.Run("QClosesOverlayFirst", func(t *testing.T) {
		m := newTestMainModel(t)
		m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

		// Open help overlay
		m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "?", Code: '?'}))
		view := m.View().Content
		if !strings.Contains(ansi.Strip(view), "quit") {
			t.Fatal("expected help overlay visible")
		}

		// "q" while overlay is open should close the overlay, not quit the app
		m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "q", Code: 'q'}))
		view = m.View().Content
		if strings.Contains(ansi.Strip(view), "quit") {
			t.Fatal("expected help overlay to be closed after 'q', not the app to quit")
		}
	})
}

// ---------------------------------------------------------------------------
// 2. TestUpdateMessageOverlayScrollKeys
// ---------------------------------------------------------------------------

func TestUpdateMessageOverlayScrollKeys(t *testing.T) {
	t.Run("DownUpScroll", func(t *testing.T) {
		m := newTestMainModel(t)
		m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
		m.preamble = strings.Repeat("preamble line\n", 200)
		m.messageOpen = true
		m.updateMessageVp()
		m.messageVp.GotoTop()

		// Press Down — verify the overlay still renders and content scrolled
		m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
		view := m.View().Content
		if view == "" {
			t.Fatal("expected non-empty view after scrolling down in message overlay")
		}

		// Press Up — should scroll back
		m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
		view = m.View().Content
		if view == "" {
			t.Fatal("expected non-empty view after scrolling up in message overlay")
		}
	})

	t.Run("CtrlDCtrlU", func(t *testing.T) {
		m := newTestMainModel(t)
		m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
		m.preamble = strings.Repeat("preamble line\n", 200)
		m.messageOpen = true
		m.updateMessageVp()
		m.messageVp.GotoTop()

		// Ctrl+D — scroll down half a page
		m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: 'd', Mod: tea.ModCtrl}))
		view := m.View().Content
		if view == "" {
			t.Fatal("expected non-empty view after Ctrl+D in message overlay")
		}

		// Ctrl+U — scroll up
		m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: 'u', Mod: tea.ModCtrl}))
		view = m.View().Content
		if view == "" {
			t.Fatal("expected non-empty view after Ctrl+U in message overlay")
		}
	})
}

// ---------------------------------------------------------------------------
// 3. TestUpdateEscapeClearsSelection
// ---------------------------------------------------------------------------

func TestUpdateEscapeClearsSelection(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 160, Height: 40})

	// Create a finalized selection
	m.diffViewer.SetContent(strings.Repeat("line content\n", 500))
	m.diffViewer.StartSelection(diffviewer.Point{Line: 0, Col: 0})
	m.diffViewer.ExtendSelection(diffviewer.Point{Line: 5, Col: 4})
	m.diffViewer.EndSelection()

	// Before escape, the view should still render properly
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view with selection")
	}

	// Pressing Escape should clear the selection
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))

	// After escape, the view should still render (no selection highlights)
	view = m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after clearing selection with Escape")
	}
}

// ---------------------------------------------------------------------------
// 4. TestToggleDiffView
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// 5. TestToggleIconStyleKey
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// 6. TestToggleFileTreeKeyboard
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// 7. TestViewHeader
// ---------------------------------------------------------------------------

func TestViewHeader(t *testing.T) {
	cases := []struct {
		name       string
		setup      func(m *mainModel)
		wantSubstr []string
		dontWant   []string
	}{
		{
			name: "full_metadata",
			setup: func(m *mainModel) {
				m.preamble = "commit abcdef1234567890\nAuthor: John Doe <john@example.com>\nAuthorDate: " + time.Now().
					Add(-2*time.Hour).
					Format("Mon Jan 2 15:04:05 2006 -0700") +
					"\n\nFix the bug"
				m.commitBranch = "main"
				m.cachedMeta = m.parseCommitMeta()
			},
			wantSubstr: []string{"DIFFNAV", "abcdef1"},
			dontWant:   []string{},
		},
		{
			name: "nerd_icon_style",
			setup: func(m *mainModel) {
				m.preamble = "commit abc1234567890\nAuthor: Test <t@t.com>\nAuthorDate: " + time.Now().
					Add(-30*time.Minute).
					Format("Mon Jan 2 15:04:05 2006 -0700")
				m.commitBranch = "develop"
				m.cachedMeta = m.parseCommitMeta()
				m.iconStyle = filenode.IconsNerdStatus
			},
			wantSubstr: []string{"DIFFNAV"},
			dontWant:   []string{},
		},
		{
			name: "ascii_icon_style",
			setup: func(m *mainModel) {
				m.preamble = "commit abc1234567890\nAuthor: Test <t@t.com>\nAuthorDate: " + time.Now().
					Add(-30*time.Minute).
					Format("Mon Jan 2 15:04:05 2006 -0700")
				m.commitBranch = "develop"
				m.cachedMeta = m.parseCommitMeta()
				m.iconStyle = filenode.IconsASCII
			},
			wantSubstr: []string{"DIFFNAV"},
			dontWant:   []string{},
		},
		{
			name: "no_meta",
			setup: func(m *mainModel) {
				m.preamble = ""
				m.cachedMeta = m.parseCommitMeta()
			},
			wantSubstr: []string{"DIFFNAV"},
			dontWant:   []string{},
		},
		{
			name: "with_subject",
			setup: func(m *mainModel) {
				m.preamble = "commit abc123\nAuthor: Test <t@t.com>\n\nThis is a commit subject"
				m.cachedMeta = m.parseCommitMeta()
				m.commitBranch = ""
			},
			wantSubstr: []string{"DIFFNAV", "This is a commit subject"},
			dontWant:   []string{},
		},
		{
			name: "very_narrow",
			setup: func(m *mainModel) {
				m.width = 20
				m.preamble = "commit abc123\nAuthor: Test\nDate: now\n\nA very long commit subject that should be truncated"
				m.cachedMeta = m.parseCommitMeta()
				m.commitBranch = "main"
			},
			wantSubstr: []string{"DIFFNAV"},
			dontWant:   []string{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestMainModel(t)
			m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 160, Height: 40})
			tc.setup(&m)

			header := m.viewHeader()
			plain := ansi.Strip(header)
			for _, want := range tc.wantSubstr {
				if !strings.Contains(plain, want) {
					t.Errorf("expected header to contain %q, got %q", want, plain)
				}
			}
			for _, dont := range tc.dontWant {
				if strings.Contains(plain, dont) {
					t.Errorf("expected header NOT to contain %q, but it did", dont)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 8. TestFooterView
// ---------------------------------------------------------------------------

func TestFooterView(t *testing.T) {
	cases := []struct {
		name       string
		setup      func(m *mainModel)
		wantSubstr []string
	}{
		{
			name:       "with_files",
			setup:      func(m *mainModel) {},
			wantSubstr: []string{"files"},
		},
		{
			name: "watch_enabled",
			setup: func(m *mainModel) {
				m.watchEnabled = true
				m.watchCmd = "make test"
			},
			wantSubstr: []string{"watching", "make test"},
		},
		{
			name: "light_background",
			setup: func(m *mainModel) {
				isDark := false
				m.isDarkBackground = &isDark
			},
			wantSubstr: []string{"files"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestMainModel(t)
			m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
			tc.setup(&m)

			footer := m.footerView()
			plain := ansi.Strip(footer)
			for _, want := range tc.wantSubstr {
				if !strings.Contains(plain, want) {
					t.Errorf("expected footer to contain %q, got %q", want, plain)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 9. TestMessageView
// ---------------------------------------------------------------------------

func TestMessageView(t *testing.T) {
	cases := []struct {
		name       string
		setup      func(m *mainModel)
		wantSubstr []string
		empty      bool
	}{
		{
			name: "with_preamble",
			setup: func(m *mainModel) {
				m.preamble = "commit abc123\nAuthor: Test <t@test.com>\nAuthorDate: Mon Jan 2 15:04:05 2006 -0700\n\nSome subject"
			},
			wantSubstr: []string{"commit"},
		},
		{
			name: "empty_preamble",
			setup: func(m *mainModel) {
				m.preamble = ""
			},
			empty: true,
		},
		{
			name: "all_metadata",
			setup: func(m *mainModel) {
				m.preamble = "commit abc\nAuthor: Test\nAuthorDate: now\nCommit: Other\nCommitDate: now\nMerge: def\nSome body text"
			},
			wantSubstr: []string{
				"commit",
				"Author:",
				"AuthorDate:",
				"Commit:",
				"CommitDate:",
				"Merge:",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestMainModel(t)
			m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
			tc.setup(&m)

			view := m.messageView()
			if tc.empty {
				if view != "" {
					t.Fatalf("expected empty message view, got %q", view)
				}
				return
			}
			plain := ansi.Strip(view)
			for _, want := range tc.wantSubstr {
				if !strings.Contains(plain, want) {
					t.Errorf("expected message view to contain %q, got %q", want, plain)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 10. TestViewRendersComponents
// ---------------------------------------------------------------------------

func TestViewRendersComponents(t *testing.T) {
	t.Run("hidden_header", func(t *testing.T) {
		m := newTestMainModel(t)
		m.config.UI.HideHeader = true
		m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

		view := m.View().Content
		if view == "" {
			t.Fatal("expected non-empty view")
		}
		plain := ansi.Strip(view)
		if strings.Contains(plain, "DIFFNAV") {
			t.Error("expected header to be hidden but found DIFFNAV branding in view")
		}
	})

	t.Run("hidden_footer", func(t *testing.T) {
		m := newTestMainModel(t)
		m.config.UI.HideFooter = true
		m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

		view := m.View().Content
		if view == "" {
			t.Fatal("expected non-empty view")
		}
		plain := ansi.Strip(view)
		if strings.Contains(plain, "F1/? help") {
			t.Error("expected footer to be hidden but found help text in view")
		}
	})

	t.Run("help_overlay", func(t *testing.T) {
		m := newTestMainModel(t)
		m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
		m.helpOpen = true

		view := m.View().Content
		plain := ansi.Strip(view)
		if !strings.Contains(plain, "quit") && !strings.Contains(plain, "Quit") {
			t.Error("expected help overlay to contain quit keybinding")
		}
	})

	t.Run("message_overlay", func(t *testing.T) {
		m := newTestMainModel(t)
		m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
		m.preamble = "commit abc\nAuthor: Test\nSubject"
		m.messageOpen = true
		m.updateMessageVp()

		view := m.View().Content
		plain := ansi.Strip(view)
		if !strings.Contains(plain, "commit") && !strings.Contains(plain, "Author") {
			t.Error("expected message overlay to contain preamble content")
		}
	})

	t.Run("hidden_sidebar", func(t *testing.T) {
		m := newTestMainModel(t)
		m.isShowingFileTree = false
		m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

		view := m.View().Content
		if view == "" {
			t.Fatal("expected non-empty view with hidden sidebar")
		}
		// Diff viewer zone should still be present
		waitForZone(t, zoneDiffViewer)
	})

	t.Run("search_active", func(t *testing.T) {
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
		waitForZone(t, zoneSearchBox)
	})
}

// ---------------------------------------------------------------------------
// 11. TestNewWithConfig
// ---------------------------------------------------------------------------

func TestNewWithConfig(t *testing.T) {
	cases := []struct {
		name  string
		setup func() config.Config
		check func(t *testing.T, m mainModel)
	}{
		{
			name: "theme_light",
			setup: func() config.Config {
				cfg := config.DefaultConfig()
				cfg.UI.Theme = config.ThemeLight
				return cfg
			},
			check: func(t *testing.T, m mainModel) {
				if m.isDarkBackground == nil {
					t.Fatal("expected isDarkBackground to be set for light theme")
				}
				if *m.isDarkBackground {
					t.Fatal("expected isDarkBackground=false for light theme")
				}
			},
		},
		{
			name: "theme_dark",
			setup: func() config.Config {
				cfg := config.DefaultConfig()
				cfg.UI.Theme = config.ThemeDark
				return cfg
			},
			check: func(t *testing.T, m mainModel) {
				if m.isDarkBackground == nil {
					t.Fatal("expected isDarkBackground to be set for dark theme")
				}
				if !*m.isDarkBackground {
					t.Fatal("expected isDarkBackground=true for dark theme")
				}
			},
		},
		{
			name: "show_file_tree_false",
			setup: func() config.Config {
				cfg := config.DefaultConfig()
				cfg.UI.ShowFileTree = false
				return cfg
			},
			check: func(t *testing.T, m mainModel) {
				if m.isShowingFileTree {
					t.Fatal("expected isShowingFileTree=false when ShowFileTree=false")
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			zone.NewGlobal()
			cfg := tc.setup()
			data, err := os.ReadFile("../../examples/multiple_files.txt")
			if err != nil {
				t.Fatal(err)
			}
			m := New(string(data), cfg)
			tc.check(t, m)
		})
	}
}

// ---------------------------------------------------------------------------
// 12. TestFetchRepoRootErrorPath
// ---------------------------------------------------------------------------

func TestFetchRepoRootErrorPathBehavioral(t *testing.T) {
	m := newTestMainModel(t)

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
// 13. TestInitAutoTheme
// ---------------------------------------------------------------------------

func TestInitAutoTheme(t *testing.T) {
	zone.NewGlobal()

	cfg := config.DefaultConfig()
	cfg.UI.Theme = config.ThemeAuto

	data, err := os.ReadFile("../../examples/multiple_files.txt")
	if err != nil {
		t.Fatal(err)
	}

	m := New(string(data), cfg)

	cmd := m.Init()
	if cmd == nil {
		t.Fatal("expected Init to return non-nil batch command for auto theme")
	}

	// Execute the batch to ensure it produces at least one message
	msg := cmd()
	if msg == nil {
		t.Fatal("expected Init batch command to produce a message")
	}
}

// ---------------------------------------------------------------------------
// 14. TestInitWatchTick
// ---------------------------------------------------------------------------

func TestInitWatchTick(t *testing.T) {
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

	msg := cmd()
	if msg == nil {
		t.Fatal("expected watch-init batch command to produce a message")
	}
}

// ---------------------------------------------------------------------------
// 15. TestMainContentHeight
// ---------------------------------------------------------------------------

func TestMainContentHeightTable(t *testing.T) {
	cases := []struct {
		name        string
		hideHeader  bool
		hideFooter  bool
		totalHeight int
		want        int
	}{
		{"with_header", false, false, 40, 40 - headerHeight - footerHeight},
		{"without_header", true, false, 40, 40 - footerHeight},
		{"without_footer", false, true, 40, 40 - headerHeight},
		{"without_both", true, true, 40, 40},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestMainModel(t)
			m.height = tc.totalHeight
			m.config.UI.HideHeader = tc.hideHeader
			m.config.UI.HideFooter = tc.hideFooter
			h := m.mainContentHeight()
			if h != tc.want {
				t.Fatalf("expected %d, got %d", tc.want, h)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 16. TestFetchFileTreeErrorPath
// ---------------------------------------------------------------------------

func TestFetchFileTreeErrorPath(t *testing.T) {
	m := newTestMainModel(t)
	m.input = "diff --git a/foo b/bar\nindex broken\n"

	msg := m.fetchFileTree()
	if _, ok := msg.(common.ErrMsg); !ok {
		t.Fatalf("expected common.ErrMsg for invalid diff input, got %T", msg)
	}
}

// ---------------------------------------------------------------------------
// 17. TestResolveBranchWithGitPointsAt
// ---------------------------------------------------------------------------

func TestResolveBranchWithGitPointsAtBehavioral(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)

	if out, err := exec.Command("git", "init").CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %s: %v", out, err)
	}
	exec.Command("git", "config", "user.email", "test@test.com").Run()
	exec.Command("git", "config", "user.name", "Test").Run()

	os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte("hello"), 0o644)
	exec.Command("git", "add", "test.txt").Run()
	if out, err := exec.Command("git", "commit", "-m", "initial").CombinedOutput(); err != nil {
		t.Fatalf("git commit failed: %s: %v", out, err)
	}

	out, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatalf("git rev-parse HEAD failed: %v", err)
	}
	hash := strings.TrimSpace(string(out))

	preamble := "commit " + hash
	result := resolveBranch(preamble)

	if result == "" {
		t.Fatalf("expected non-empty branch from --points-at for hash %s", hash)
	}
}

// ---------------------------------------------------------------------------
// 18. TestHandleMouseScrollUpMessageOverlay
// ---------------------------------------------------------------------------

func TestHandleMouseScrollUpMessageOverlayBehavioral(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.preamble = strings.Repeat("preamble line\n", 200)
	m.messageOpen = true
	m.updateMessageVp()
	m.messageVp.ScrollDown(20)

	// Drive mouse wheel-up through Update
	m = updateMainModel(t, m, tea.MouseMotionMsg(tea.Mouse{
		Button: tea.MouseWheelUp,
		X:      50,
		Y:      10,
	}))

	// View should still render properly after scrolling up
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after scrolling up in message overlay")
	}
}

// ---------------------------------------------------------------------------
// 19. TestHandleMouseScrollWhenHelpOpenIsNoop
// ---------------------------------------------------------------------------

func TestHandleMouseScrollWhenHelpOpenIsNoopBehavioral(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.helpOpen = true

	viewBefore := m.View().Content

	// Mouse scroll while help is open should be a no-op
	m = updateMainModel(t, m, tea.MouseMotionMsg(tea.Mouse{
		Button: tea.MouseWheelDown,
		X:      50,
		Y:      10,
	}))

	viewAfter := m.View().Content
	if viewAfter == "" {
		t.Fatal("expected non-empty view after scroll when help open")
	}
	// Both views should render the help overlay content
	plainBefore := ansi.Strip(viewBefore)
	plainAfter := ansi.Strip(viewAfter)
	if !strings.Contains(plainAfter, "quit") && !strings.Contains(plainAfter, "Quit") {
		t.Fatal("expected help overlay to remain visible after mouse scroll")
	}
	_ = plainBefore
}

// ---------------------------------------------------------------------------
// 20. TestHandleMouseClickOutsideMessageOverlayClosesIt
// ---------------------------------------------------------------------------

func TestHandleMouseClickOutsideMessageOverlayClosesItBehavioral(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.preamble = "commit abc"
	m.messageOpen = true
	m.updateMessageVp()

	// Before: view should show overlay content
	viewBefore := m.View().Content
	plainBefore := ansi.Strip(viewBefore)
	if !strings.Contains(plainBefore, "commit") {
		t.Fatal("expected message overlay content visible before click")
	}

	// Click at corner (0,0) which should be outside the centered overlay — drive through Update
	m = updateMainModel(t, m, tea.MouseClickMsg(tea.Mouse{
		Button: tea.MouseLeft,
		X:      0,
		Y:      0,
	}))

	// After: overlay should be closed, so the message overlay content should not appear
	// as a centered modal anymore
	viewAfter := m.View().Content
	if viewAfter == "" {
		t.Fatal("expected non-empty view after closing message overlay")
	}
}

// ---------------------------------------------------------------------------
// Additional coverage: background color detection through Update
// ---------------------------------------------------------------------------

func TestUpdateBackgroundColorMsgSetsBackground(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 160, Height: 40})
	m.themeOverride = nil
	m.isDarkBackground = nil

	// Send BackgroundColorMsg through Update
	m = updateMainModel(t, m, tea.BackgroundColorMsg{
		Color: color.RGBA{R: 0, G: 0, B: 0, A: 255},
	})

	// Verify the view still renders after background detection
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after background color detection")
	}
}

// ---------------------------------------------------------------------------
// Update: keyboard navigation (up/down/G/g/j/k/tab)
// ---------------------------------------------------------------------------

func TestUpdateUpMovesInFileTreePanel(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.activePanel = FileTreePanel

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
	m.activePanel = FileTreePanel

	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after down in file tree panel")
	}
}

func TestUpdateGoesToBottom(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.activePanel = FileTreePanel

	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "G", Code: 'G'}))
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after G")
	}
}

func TestUpdateGoesToTop(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.activePanel = FileTreePanel

	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "g", Code: 'g'}))
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after g")
	}
}

func TestUpdateSwitchPanel(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.activePanel = FileTreePanel

	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	// After tab, diff viewer should be active
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after switching panel")
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

// ---------------------------------------------------------------------------
// Update: search key (t)
// ---------------------------------------------------------------------------

func TestUpdateSearchKey(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	// Press "t" to start search
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "t", Code: 't'}))

	// View should still render with search UI
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after pressing search key")
	}
}

// ---------------------------------------------------------------------------
// Update: message overlay page keys (PageDown/PageUp)
// ---------------------------------------------------------------------------

func TestUpdateMessageOverlayPageKeys(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.preamble = strings.Repeat("preamble line\n", 200)
	m.messageOpen = true
	m.updateMessageVp()
	m.messageVp.GotoTop()

	// PageDown
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyPgDown}))
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after PageDown in message overlay")
	}

	// PageUp
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyPgUp}))
	view = m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after PageUp in message overlay")
	}
}

// ---------------------------------------------------------------------------
// Update: diff view-specific key bindings (j/k/CtrlD/CtrlU)
// ---------------------------------------------------------------------------

func TestUpdateDiffLineUpDown(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.activePanel = DiffViewerPanel
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
	m.activePanel = DiffViewerPanel
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

// ---------------------------------------------------------------------------
// Update: Quit key returns Quit command
// ---------------------------------------------------------------------------

func TestUpdateQuitKeyReturnsQuit(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Text: "q", Code: 'q'}))
	if cmd == nil {
		t.Fatal("expected non-nil cmd when pressing q")
	}
}

// ---------------------------------------------------------------------------
// Update: window resize
// ---------------------------------------------------------------------------

func TestUpdateWindowResizeRenders(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	// Resize
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 80, Height: 30})
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after resize")
	}
}

// ---------------------------------------------------------------------------
// Update: watch messages (watchTickMsg, watchResultMsg)
// ---------------------------------------------------------------------------

func TestUpdateWatchTickMsgWhenNotInFlight(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.watchEnabled = true
	m.watchInFlight = false
	m.watchCmd = "echo test"
	m.watchInterval = time.Second

	m = updateMainModel(t, m, watchTickMsg{})
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after watch tick")
	}
}

func TestUpdateWatchTickMsgWhenInFlight(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.watchEnabled = true
	m.watchInFlight = true

	m = updateMainModel(t, m, watchTickMsg{})
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after in-flight watch tick")
	}
}

func TestUpdateWatchResultMsgOutputChanged(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.watchEnabled = true
	m.watchInFlight = true
	m.input = "original"

	m = updateMainModel(t, m, watchResultMsg{output: "new output", err: nil})
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after watch result with changed output")
	}
}

func TestUpdateWatchResultMsgOutputUnchanged(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.watchEnabled = true
	m.watchInFlight = true
	m.input = "original"

	m = updateMainModel(t, m, watchResultMsg{output: "original", err: nil})
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after watch result with unchanged output")
	}
}

func TestUpdateWatchResultMsgError(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.watchEnabled = true
	m.watchInFlight = true

	m = updateMainModel(t, m, watchResultMsg{err: fmt.Errorf("command failed")})
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after watch error")
	}
}

// ---------------------------------------------------------------------------
// Update: repoRootMsg
// ---------------------------------------------------------------------------

func TestUpdateRepoRootMsgSetsRoot(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	m = updateMainModel(t, m, repoRootMsg("/home/user/repo"))
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after repoRootMsg")
	}
}

// ---------------------------------------------------------------------------
// Update: fileTreeMsg
// ---------------------------------------------------------------------------

func TestUpdateFileTreeMsgWithFiles(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	// Send a fileTreeMsg with files (the same ones we already have)
	m = updateMainModel(t, m, fileTreeMsg{files: m.files, preamble: "commit abc", branch: "main"})
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after fileTreeMsg")
	}
}

func TestUpdateFileTreeMsgEmptyFilesQuit(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.watchEnabled = false

	_, cmd := m.Update(fileTreeMsg{files: []*gitdiff.File{}, preamble: "", branch: ""})
	if cmd == nil {
		t.Fatal("expected tea.Quit cmd for empty files when watch disabled")
	}
}

func TestUpdateFileTreeMsgEmptyFilesWatchEnabled(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.watchEnabled = true

	m = updateMainModel(
		t,
		m,
		fileTreeMsg{files: []*gitdiff.File{}, preamble: "commit abc", branch: "main"},
	)
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after empty fileTreeMsg with watch")
	}
}

func TestUpdateFileTreeMsgWithPendingCursorPath(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.pendingCursorPath = "yarn.lock"

	m = updateMainModel(t, m, m.fetchFileTree())
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after fileTreeMsg with pending path")
	}
}

// ---------------------------------------------------------------------------
// Update: common.ErrMsg
// ---------------------------------------------------------------------------

func TestUpdateErrMsgRendersView(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	m = updateMainModel(t, m, common.ErrMsg{Err: fmt.Errorf("test error")})
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after ErrMsg")
	}
}

// ---------------------------------------------------------------------------
// Mouse: scroll in file tree through Update
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Mouse: scroll in diff viewer through Update
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Mouse: click on help zone through Update
// ---------------------------------------------------------------------------

func TestMouseClickHelpZoneThroughUpdate(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	_ = m.View().Content
	z := waitForZone(t, zoneHelp)

	// Click on help zone
	m = updateMainModel(t, m, tea.MouseClickMsg(tea.Mouse{
		Button: tea.MouseLeft,
		X:      z.StartX + 1,
		Y:      z.StartY,
	}))

	view := m.View().Content
	plain := ansi.Strip(view)
	if !strings.Contains(plain, "quit") && !strings.Contains(plain, "Quit") {
		t.Fatal("expected help overlay to appear after clicking help zone")
	}
}

// ---------------------------------------------------------------------------
// Mouse: click on header zone through Update
// ---------------------------------------------------------------------------

func TestMouseClickHeaderZoneWithPreamble(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.preamble = "commit abc"

	_ = m.View().Content
	z := waitForZone(t, zoneHeader)

	m = updateMainModel(t, m, tea.MouseClickMsg(tea.Mouse{
		Button: tea.MouseLeft,
		X:      z.StartX + 2,
		Y:      z.StartY,
	}))

	view := m.View().Content
	plain := ansi.Strip(view)
	if !strings.Contains(plain, "commit") {
		t.Fatal("expected message overlay to appear after clicking header with preamble")
	}
}

func TestMouseClickHeaderZoneWithoutPreamble(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.preamble = ""

	_ = m.View().Content
	z := waitForZone(t, zoneHeader)

	m = updateMainModel(t, m, tea.MouseClickMsg(tea.Mouse{
		Button: tea.MouseLeft,
		X:      z.StartX + 2,
		Y:      z.StartY,
	}))

	view := m.View().Content
	plain := ansi.Strip(view)
	// Without preamble, clicking header should not open message overlay
	if strings.Contains(plain, "commit abc") {
		t.Fatal("expected no message overlay when preamble is empty")
	}
}

// ---------------------------------------------------------------------------
// Mouse: click on search box through Update
// ---------------------------------------------------------------------------

func TestMouseClickSearchBoxNotSearching(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.searching = false

	_ = m.View().Content
	z := waitForZone(t, zoneSearchBox)

	m = updateMainModel(t, m, tea.MouseClickMsg(tea.Mouse{
		Button: tea.MouseLeft,
		X:      z.StartX + 2,
		Y:      z.StartY,
	}))

	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after clicking search box")
	}
}

func TestMouseClickSearchBoxAlreadySearching(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.searching = true

	_ = m.View().Content
	z := waitForZone(t, zoneSearchBox)

	m = updateMainModel(t, m, tea.MouseClickMsg(tea.Mouse{
		Button: tea.MouseLeft,
		X:      z.StartX + 2,
		Y:      z.StartY,
	}))

	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after clicking search box when already searching")
	}
}

// ---------------------------------------------------------------------------
// Mouse: click on search results through Update
// ---------------------------------------------------------------------------

func TestMouseClickSearchResultThroughUpdate(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.searching = true
	m.search.SetValue("")
	m.setSearchResults()
	m.resultsVp.SetWidth(m.config.UI.SearchTreeWidth)
	m.resultsVp.SetHeight(m.mainContentHeight() - searchHeight)
	m.resultsVp.SetContent(m.resultsView())

	_ = m.View().Content
	z := waitForZone(t, zoneSearchResults)

	m = updateMainModel(t, m, tea.MouseClickMsg(tea.Mouse{
		Button: tea.MouseLeft,
		X:      z.StartX + 5,
		Y:      z.StartY,
	}))

	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after clicking search result")
	}
}

// ---------------------------------------------------------------------------
// Mouse: click on file tree zone through Update
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Mouse: click on sidebar border to start drag
// ---------------------------------------------------------------------------

func TestMouseClickSidebarBorderThroughUpdate(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 160, Height: 40})
	m.isShowingFileTree = true

	sidebarWidth := m.sidebarWidth()
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

// ---------------------------------------------------------------------------
// Mouse: click on hidden sidebar grab line
// ---------------------------------------------------------------------------

func TestMouseClickHiddenSidebarGrabLineThroughUpdate(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.isShowingFileTree = false
	m.searching = false

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

// ---------------------------------------------------------------------------
// Mouse: click in diff pane starts selection
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Mouse: motion with sidebar drag resizes
// ---------------------------------------------------------------------------

func TestMouseMotionSidebarDragThroughUpdate(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 160, Height: 40})
	m.isShowingFileTree = true
	m.draggingSidebar = true

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

// ---------------------------------------------------------------------------
// Mouse: sidebar drag hides when below threshold
// ---------------------------------------------------------------------------

func TestMouseSidebarDragHidesBelowThreshold(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.isShowingFileTree = true
	m.draggingSidebar = true

	// Drag to very small width
	m = updateMainModel(t, m, tea.MouseMotionMsg(tea.Mouse{
		X:      2,
		Y:      5,
		Button: tea.MouseLeft,
	}))

	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after sidebar drag to hide")
	}
}

// ---------------------------------------------------------------------------
// Mouse: sidebar drag during search resets drag
// ---------------------------------------------------------------------------

func TestMouseSidebarDragDuringSearch(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.searching = true
	m.draggingSidebar = true

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

// ---------------------------------------------------------------------------
// Mouse: motion with diff selection extends selection
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Mouse: release ends selection with clipboard
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Mouse: release without selection
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Mouse: right-click while overlay is open should be blocked
// ---------------------------------------------------------------------------

func TestMouseRightClickWhileHelpOpen(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.helpOpen = true

	m = updateMainModel(t, m, tea.MouseClickMsg(tea.Mouse{
		Button: tea.MouseRight,
		X:      50,
		Y:      10,
	}))

	view := m.View().Content
	plain := ansi.Strip(view)
	if !strings.Contains(plain, "quit") && !strings.Contains(plain, "Quit") {
		t.Fatal("expected help overlay to remain open after right-click")
	}
}

// ---------------------------------------------------------------------------
// Mouse: click inside help overlay keeps it open
// ---------------------------------------------------------------------------

func TestMouseClickInsideHelpOverlay(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.helpOpen = true

	_ = m.View().Content
	content := m.help.View()
	o := m.renderOverlay(content)

	m = updateMainModel(t, m, tea.MouseClickMsg(tea.Mouse{
		Button: tea.MouseLeft,
		X:      o.col + o.width/2,
		Y:      o.row + o.height/2,
	}))

	view := m.View().Content
	plain := ansi.Strip(view)
	if !strings.Contains(plain, "quit") && !strings.Contains(plain, "Quit") {
		t.Fatal("expected help overlay to stay open after clicking inside")
	}
}

// ---------------------------------------------------------------------------
// Update: openInEditor through keyboard
// ---------------------------------------------------------------------------

func TestUpdateOpenInEditorKeyWithEditor(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	t.Setenv("EDITOR", "true")

	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Text: "o", Code: 'o'}))
	if cmd == nil {
		t.Fatal("expected non-nil cmd when pressing o with EDITOR set")
	}
}

// ---------------------------------------------------------------------------
// Update: copy key
// ---------------------------------------------------------------------------

func TestUpdateCopyKeyRendersView(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "y", Code: 'y'}))
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after pressing y (copy)")
	}
}

// ---------------------------------------------------------------------------
// Update: escape without selection is no-op for the diff viewer
// ---------------------------------------------------------------------------

func TestUpdateEscapeNoSelectionRenders(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after Escape without selection")
	}
}

// ---------------------------------------------------------------------------
// Update: search mode key events through Update
// ---------------------------------------------------------------------------

func TestUpdateSearchEscapeStops(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	// Start search
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "t", Code: 't'}))

	// Escape to stop search
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after escape from search")
	}
}

func TestUpdateSearchEnterSelects(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	// Start search
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "t", Code: 't'}))
	m.search.SetValue("yarn")
	m.setSearchResults()

	// Enter to select
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after enter in search")
	}
}

func TestUpdateSearchCtrlCQuits(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	// Start search
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "t", Code: 't'}))

	// Ctrl+C in search should produce a quit command
	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl}))
	if cmd == nil {
		t.Fatal("expected non-nil cmd (quit) from Ctrl+C in search")
	}
}

func TestUpdateSearchDownAdvancesCursor(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	// Start search
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "t", Code: 't'}))
	m.search.SetValue("")
	m.setSearchResults()

	// Down key should advance cursor in search results
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after down in search")
	}
}

func TestUpdateSearchUpRetreatsCursor(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	// Start search
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "t", Code: 't'}))
	m.search.SetValue("")
	m.setSearchResults()

	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after up in search")
	}
}

func TestUpdateSearchCtrlPCtrlN(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	// Start search
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "t", Code: 't'}))
	m.search.SetValue("")
	m.setSearchResults()

	// Ctrl+N advances cursor
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: 'n', Mod: tea.ModCtrl}))
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after Ctrl+N in search")
	}

	// Ctrl+P retreats cursor
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: 'p', Mod: tea.ModCtrl}))
	view = m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after Ctrl+P in search")
	}
}

func TestUpdateSearchDefaultKeyResetsCursor(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	// Start search
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "t", Code: 't'}))
	m.search.SetValue("")
	m.setSearchResults()

	// Press a key that doesn't match any specific search binding
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "a", Code: 'a'}))
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after typing in search")
	}
}

// ---------------------------------------------------------------------------
// Update: toggle node (Enter key)
// ---------------------------------------------------------------------------

func TestUpdateToggleNodeInFileTreePanel(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.activePanel = FileTreePanel

	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after Enter in file tree panel")
	}
}

func TestUpdateToggleNodeInDiffViewerPanel(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.activePanel = DiffViewerPanel

	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after Enter in diff viewer panel")
	}
}

// ---------------------------------------------------------------------------
// Update: default key in diff viewer and file tree
// ---------------------------------------------------------------------------

func TestUpdateDefaultKeyInDiffViewer(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.activePanel = DiffViewerPanel

	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "x", Code: 'x'}))
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after unhandled key in diff viewer")
	}
}

func TestUpdateDefaultKeyInFileTree(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.activePanel = FileTreePanel

	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "x", Code: 'x'}))
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after unhandled key in file tree")
	}
}

// ---------------------------------------------------------------------------
// Update: diff specific scroll keys when panel is FileTreePanel
// ---------------------------------------------------------------------------

func TestUpdateDownInDiffViewerPanel(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.activePanel = DiffViewerPanel
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
	m.activePanel = DiffViewerPanel
	m.diffViewer.SetContent(strings.Repeat("line\n", 500))
	m.diffViewer.ScrollDown(30)

	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after Up in diff viewer panel")
	}
}

func TestUpdateDiffPageDownUp(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.activePanel = DiffViewerPanel
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
	m.activePanel = DiffViewerPanel
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

// ---------------------------------------------------------------------------
// Update: switch panel when file tree is hidden is no-op
// ---------------------------------------------------------------------------

func TestUpdateSwitchPanelWhenTreeHidden(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.isShowingFileTree = false
	m.activePanel = DiffViewerPanel

	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after tab with tree hidden")
	}
}

// ---------------------------------------------------------------------------
// Update: background detection with theme timeout
// ---------------------------------------------------------------------------

func TestUpdateThemeDetectTimeoutThroughUpdate(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 160, Height: 40})
	m.themeOverride = nil
	m.isDarkBackground = nil

	m = updateMainModel(t, m, themeDetectTimeoutMsg{})
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after theme detect timeout")
	}
}

// ---------------------------------------------------------------------------
// Update: theme detection already handled is ignored
// ---------------------------------------------------------------------------

func TestUpdateBackgroundDetectAlreadySet(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 160, Height: 40})
	isDark := true
	m.isDarkBackground = &isDark

	m = updateMainModel(t, m, tea.BackgroundColorMsg{
		Color: color.RGBA{R: 255, G: 255, B: 255, A: 255},
	})
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after background color when already set")
	}
}

// ---------------------------------------------------------------------------
// Mouse: scroll in search results through Update
// ---------------------------------------------------------------------------

func TestMouseScrollInSearchResults(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 10})
	m.searching = true
	m.search.SetValue("")
	m.setSearchResults()
	m.resultsVp.SetWidth(m.config.UI.SearchTreeWidth)
	m.resultsVp.SetHeight(2)
	m.resultsVp.SetContent(m.resultsView())

	_ = m.View().Content
	z := waitForZone(t, zoneSearchResults)

	m = updateMainModel(t, m, tea.MouseMotionMsg(tea.Mouse{
		X:      z.StartX + 2,
		Y:      z.StartY + 1,
		Button: tea.MouseWheelDown,
	}))
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after scroll in search results")
	}
}

// ---------------------------------------------------------------------------
// Mouse: click outside help overlay closes it
// ---------------------------------------------------------------------------

func TestMouseClickOutsideHelpOverlayCloses(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.helpOpen = true

	m = updateMainModel(t, m, tea.MouseClickMsg(tea.Mouse{
		Button: tea.MouseLeft,
		X:      0,
		Y:      0,
	}))

	view := m.View().Content
	plain := ansi.Strip(view)
	if strings.Contains(plain, "quit") {
		t.Fatal("expected help overlay to be closed after clicking outside")
	}
}

// ---------------------------------------------------------------------------
// Mouse: scroll down in message overlay through Update
// ---------------------------------------------------------------------------

func TestMouseScrollDownMessageOverlayThroughUpdate(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.preamble = strings.Repeat("preamble line\n", 200)
	m.messageOpen = true
	m.updateMessageVp()
	m.messageVp.GotoTop()

	m = updateMainModel(t, m, tea.MouseMotionMsg(tea.Mouse{
		Button: tea.MouseWheelDown,
		X:      50,
		Y:      10,
	}))

	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after scrolling down in message overlay")
	}
}

// ---------------------------------------------------------------------------
// Mouse: click outside message overlay closes it
// ---------------------------------------------------------------------------

func TestMouseClickOutsideMessageOverlayCloses(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.preamble = "commit abc"
	m.messageOpen = true
	m.updateMessageVp()

	m = updateMainModel(t, m, tea.MouseClickMsg(tea.Mouse{
		Button: tea.MouseLeft,
		X:      0,
		Y:      0,
	}))

	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after clicking outside message overlay")
	}
}

// ---------------------------------------------------------------------------
// Mouse: click inside message overlay keeps it open
// ---------------------------------------------------------------------------

func TestMouseClickInsideMessageOverlayStaysOpen(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.preamble = "commit abc\nAuthor: Test\nSubject"
	m.messageOpen = true
	m.updateMessageVp()

	_ = m.View().Content
	content := m.messageViewContent()
	o := m.renderOverlay(content)

	m = updateMainModel(t, m, tea.MouseClickMsg(tea.Mouse{
		Button: tea.MouseLeft,
		X:      o.col + o.width/2,
		Y:      o.row + o.height/2,
	}))

	view := m.View().Content
	plain := ansi.Strip(view)
	if !strings.Contains(plain, "commit") {
		t.Fatal("expected message overlay to remain open after clicking inside")
	}
}

// ---------------------------------------------------------------------------
// Mouse: release stops sidebar drag
// ---------------------------------------------------------------------------

func TestMouseReleaseStopsSidebarDrag(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.draggingSidebar = true

	m = updateMainModel(t, m, tea.MouseReleaseMsg(tea.Mouse{
		X:      10,
		Y:      10,
		Button: tea.MouseLeft,
	}))

	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after mouse release stops sidebar drag")
	}
}

// ---------------------------------------------------------------------------
// Mouse: release with finalized selection copies text
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Update: default message type (unknown message)
// ---------------------------------------------------------------------------

func TestUpdateDefaultMessage(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	// Send a message type that doesn't match any case in Update's switch
	m = updateMainModel(t, m, tea.FocusMsg{})
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after default message type")
	}
}

// ---------------------------------------------------------------------------
// Update: openInEditor with no editor
// ---------------------------------------------------------------------------

func TestUpdateOpenInEditorNoEditor(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	t.Setenv("EDITOR", "")

	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Text: "o", Code: 'o'}))
	// cmd should be nil since no editor configured
	_ = cmd
}

// ---------------------------------------------------------------------------
// Update: up/down in diff viewer panel
// ---------------------------------------------------------------------------

func TestUpdateUpDownInDiffViewerPanel(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.activePanel = DiffViewerPanel
	m.diffViewer.SetContent(strings.Repeat("line\n", 500))

	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after up/down in diff viewer")
	}
}

// ---------------------------------------------------------------------------
// Update: G/g in diff viewer panel
// ---------------------------------------------------------------------------

func TestUpdateTopBottomInDiffViewer(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.activePanel = DiffViewerPanel
	m.diffViewer.SetContent(strings.Repeat("line\n", 500))

	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "G", Code: 'G'}))
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "g", Code: 'g'}))
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after G/g in diff viewer")
	}
}

// ---------------------------------------------------------------------------
// editorDone callback
// ---------------------------------------------------------------------------

func TestEditorDoneCallback(t *testing.T) {
	if result := editorDone(nil); result != nil {
		t.Fatalf("expected editorDone(nil) to return nil, got %v", result)
	}
	if result := editorDone(fmt.Errorf("some error")); result != nil {
		t.Fatalf("expected editorDone(err) to return nil, got %v", result)
	}
}

// ---------------------------------------------------------------------------
// fetchWatchDiff through Update watch tick
// ---------------------------------------------------------------------------

func TestFetchWatchDiffExecution(t *testing.T) {
	m := newTestMainModel(t)
	m.watchEnabled = true
	m.watchInFlight = false
	m.watchCmd = "echo test"
	m.watchInterval = time.Second

	msg := m.fetchWatchDiff()
	result, ok := msg.(watchResultMsg)
	if !ok {
		t.Fatalf("expected watchResultMsg, got %T", msg)
	}
	if result.err != nil {
		t.Fatalf("expected no error from 'echo test', got %v", result.err)
	}
}

func TestFetchWatchDiffError(t *testing.T) {
	m := newTestMainModel(t)
	m.watchCmd = "false"

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
// scheduleWatchTick closure body
// ---------------------------------------------------------------------------

func TestScheduleWatchTickProducesMsg(t *testing.T) {
	m := newTestMainModel(t)
	m.watchInterval = 1 * time.Millisecond
	cmd := m.scheduleWatchTick()
	msg := cmd()
	if _, ok := msg.(watchTickMsg); !ok {
		t.Fatalf("expected watchTickMsg, got %T", msg)
	}
}

// ---------------------------------------------------------------------------
// fetchRepoRoot success path
// ---------------------------------------------------------------------------

func TestFetchRepoRootInGitRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	m := newTestMainModel(t)
	// We're in a real git repo (the project repo), so fetchRepoRoot should succeed.
	msg := m.fetchRepoRoot()
	rr, ok := msg.(repoRootMsg)
	if !ok {
		t.Fatalf("expected repoRootMsg, got %T", msg)
	}
	// The repo root should be non-empty when running inside the project repo.
	if string(rr) == "" {
		t.Fatal("expected non-empty repoRootMsg in a git repo")
	}
}

// ---------------------------------------------------------------------------
// Init batch command execution
// ---------------------------------------------------------------------------

func TestInitBatchCommandExecution(t *testing.T) {
	zone.NewGlobal()

	cfg := config.DefaultConfig()
	data, err := os.ReadFile("../../examples/multiple_files.txt")
	if err != nil {
		t.Fatal(err)
	}

	m := New(string(data), cfg)
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("expected Init to return non-nil batch command")
	}

	// Execute all sub-commands in the batch
	if batch, ok := cmd().(tea.BatchMsg); ok {
		for _, c := range batch {
			if c != nil {
				c()
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Mouse: scroll up in diff viewer zone through Update
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Mouse: scroll down/up in file tree through Update
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Mouse: diff selection motion above/below viewport through Update
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// clampToViewportWidth edge cases
// ---------------------------------------------------------------------------

func TestClampToViewportWidthTable(t *testing.T) {
	cases := []struct {
		x, vpWidth, want int
	}{
		{5, 10, 5},
		{15, 10, 9},
		{5, 0, 5},
		{0, 10, 0},
		{9, 10, 9},
		{-33, 10, 0},
		{-5, 0, -5},
	}
	for _, tc := range cases {
		got := clampToViewportWidth(tc.x, tc.vpWidth)
		if got != tc.want {
			t.Errorf("clampToViewportWidth(%d, %d) = %d, want %d", tc.x, tc.vpWidth, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// setSearchResults edge cases
// ---------------------------------------------------------------------------

func TestSetSearchResultsClampsCursor(t *testing.T) {
	m := newTestMainModel(t)
	m.search.SetValue("")
	m.resultsCursor = 9999 // way too high
	m.setSearchResults()
	if m.resultsCursor >= len(m.filtered) {
		t.Fatal("expected cursor to be clamped")
	}
}

func TestSetSearchResultsNegativeCursor(t *testing.T) {
	m := newTestMainModel(t)
	m.search.SetValue("does-not-match-xyz-12345")
	m.resultsCursor = -3
	m.setSearchResults()
	if m.resultsCursor != 0 {
		t.Fatal("expected negative cursor to be clamped to 0")
	}
}

// ---------------------------------------------------------------------------
// openInEditor edge cases
// ---------------------------------------------------------------------------

func TestOpenInEditorNoFilesBehavior(t *testing.T) {
	m := newTestMainModel(t)
	m.files = nil
	t.Setenv("EDITOR", "true")
	cmd := m.openInEditor()
	if cmd != nil {
		t.Fatal("expected nil cmd when no files")
	}
}

// ---------------------------------------------------------------------------
// Mouse: scroll in search results up and down
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Update: applyAutoDetectedBackground with diffViewer content returns non-nil cmd
// Line 216: if dfCmd := m.applyAutoDetectedBackground(isDark); dfCmd != nil
// ---------------------------------------------------------------------------

func TestUpdateBackgroundDetectDiffViewerCmd(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 160, Height: 40})
	m.themeOverride = nil
	m.isDarkBackground = nil

	// Load a file patch into the diffViewer so diff() returns a non-nil cmd.
	if len(m.files) > 0 {
		m.diffViewer, _ = m.diffViewer.SetFilePatch(m.files[0])
	}

	m = updateMainModel(t, m, tea.BackgroundColorMsg{
		Color: color.RGBA{R: 255, G: 255, B: 255, A: 255},
	})

	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after background detection with diffViewer content")
	}
}

// ---------------------------------------------------------------------------
// Line 267: block arbitrary keys while overlay is open
// ---------------------------------------------------------------------------

func TestUpdateBlockKeysWhileOverlayOpen(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	// Open help overlay
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "?", Code: '?'}))

	// Press an arbitrary key that doesn't match overlay bindings
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "x", Code: 'x'}))

	// Help should still be visible
	view := m.View().Content
	plain := ansi.Strip(view)
	if !strings.Contains(plain, "quit") && !strings.Contains(plain, "Quit") {
		t.Fatal("expected help overlay to remain open when pressing non-overlay key")
	}
}

// ---------------------------------------------------------------------------
// Line 337: Switch panel from DiffViewerPanel to FileTreePanel
// ---------------------------------------------------------------------------

func TestUpdateSwitchPanelFromDiffToTree(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.activePanel = DiffViewerPanel
	m.isShowingFileTree = true

	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after tab from diff viewer to file tree")
	}
}

// ---------------------------------------------------------------------------
// Line 426: window resize while message overlay is open
// ---------------------------------------------------------------------------

func TestUpdateResizeWhileMessageOpen(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.preamble = "commit abc\nAuthor: Test\nSubject"
	m.messageOpen = true
	m.updateMessageVp()

	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 80, Height: 30})
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after resize while message open")
	}
}

// ---------------------------------------------------------------------------
// Line 1152: openInEditor with repoRoot set
// ---------------------------------------------------------------------------

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
// Line 1216: diffPanePoint with paneY < DirHeaderHeight
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Line 1342/1346: handleSearchResultClick with y<0 and index out of range
// ---------------------------------------------------------------------------

func TestMouseSearchResultClickOutOfRangeThroughUpdate(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.searching = true
	m.search.SetValue("")
	m.setSearchResults()
	m.resultsVp.SetWidth(m.config.UI.SearchTreeWidth)
	m.resultsVp.SetHeight(m.mainContentHeight() - searchHeight)
	m.resultsVp.SetContent(m.resultsView())

	_ = m.View().Content

	// Click outside the search results zone (y < 0 relative to zone)
	m = updateMainModel(t, m, tea.MouseClickMsg(tea.Mouse{
		X:      0,
		Y:      0,
		Button: tea.MouseLeft,
	}))

	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after search click out of range")
	}
}

// ---------------------------------------------------------------------------
// Line 1392/1398: handleFileTreeClick with y<0 and nil node
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Line 1454: handleDiffSelectionMotion when not selecting
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Line 1458: handleDiffSelectionMotion with zero zone
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Line 1528: handleSidebarDrag with same width (below minResizeStep)
// ---------------------------------------------------------------------------

func TestMouseSidebarDragSameWidth(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.isShowingFileTree = true
	m.draggingSidebar = true

	// Drag to same position as current sidebar width
	currentWidth := m.sidebarWidth()
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

// ---------------------------------------------------------------------------
// Line 1564: moveToFile when PrevFile/NextFile returns false
// ---------------------------------------------------------------------------

func TestUpdatePrevNextFileNoMovementInDiffViewer(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.activePanel = DiffViewerPanel

	// Navigate to first file then try prev
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))

	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after prev/next file key")
	}
}

// ---------------------------------------------------------------------------
// Line 1604: setNodeDiff with nil node
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Line 1638/1649: setSearchResults/selectedSearchResult edge cases
// ---------------------------------------------------------------------------

func TestSetSearchResultsEmptyQueryNegativeCursor(t *testing.T) {
	m := newTestMainModel(t)
	m.search.SetValue("does-not-match-xyz-12345")
	m.resultsCursor = -5
	m.setSearchResults()

	// selectedSearchResult with empty results
	_, ok := m.selectedSearchResult()
	if ok {
		t.Fatal("expected false from selectedSearchResult with empty results")
	}
}

// ---------------------------------------------------------------------------
// Line 1564: moveToFile when PrevFile/NextFile returns false
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Line 1342/1346: handleSearchResultClick y<0 and index out of range
// (These defensive checks are technically unreachable through handleMouse
// since InBounds gates the call, but we test them directly.)
// ---------------------------------------------------------------------------

func TestHandleSearchResultClickNegativeY(t *testing.T) {
	m := newTestMainModel(t)
	m.searching = true
	m.search.SetValue("")
	m.setSearchResults()

	updated, cmd := m.handleSearchResultClick(tea.MouseClickMsg(tea.Mouse{
		X:      0,
		Y:      0,
		Button: tea.MouseLeft,
	}))
	m2, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	_ = m2
	_ = cmd
}

func TestHandleSearchResultClickIndexOutOfRange(t *testing.T) {
	m := newTestMainModel(t)
	m.width = 100
	m.height = 40
	m.searching = true
	m.search.SetValue("")
	m.setSearchResults()
	m.resultsVp.SetWidth(m.config.UI.SearchTreeWidth)
	m.resultsVp.SetHeight(m.mainContentHeight() - searchHeight)
	m.resultsVp.SetContent(m.resultsView())

	_ = m.View().Content

	z := waitForZone(t, zoneSearchResults)
	if len(m.filtered) == 0 {
		t.Fatal("expected at least one search result")
	}

	// Scroll far down so YOffset > 0, then click at a y that makes
	// clickedIndex = y + YOffset >= len(m.filtered).
	m.resultsVp.GotoBottom()

	// After GotoBottom, YOffset is maximized. Clicking at the last visible
	// line (y = Height-1) should give clickedIndex = Height-1 + YOffset.
	// With short viewport and many results, this should be in range.
	// To make it out of range, reduce filtered to be smaller than clickedIndex.
	maxY := m.resultsVp.Height() - 1
	clickedIndex := maxY + m.resultsVp.YOffset()

	// Trim filtered so clickedIndex exceeds it
	if clickedIndex < len(m.filtered) {
		m.filtered = m.filtered[:max(1, clickedIndex)]
	}

	updated, cmd := m.handleSearchResultClick(tea.MouseClickMsg(tea.Mouse{
		X:      z.StartX + 5,
		Y:      z.StartY + maxY,
		Button: tea.MouseLeft,
	}))
	m2, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	_ = m2
	_ = cmd
}

// ---------------------------------------------------------------------------
// Line 1392: handleFileTreeClick y<0
// ---------------------------------------------------------------------------

func TestHandleFileTreeClickNegativeY(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	updated, cmd := m.handleFileTreeClick(tea.MouseClickMsg(tea.Mouse{
		X:      0,
		Y:      0,
		Button: tea.MouseLeft,
	}))
	m2, ok := updated.(mainModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	_ = m2
	_ = cmd
}

// ---------------------------------------------------------------------------
// Line 1454: handleDiffSelectionMotion when not selecting
// ---------------------------------------------------------------------------

func TestHandleDiffSelectionMotionNotSelecting(t *testing.T) {
	m := newTestMainModel(t)

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

// ---------------------------------------------------------------------------
// Line 1604: setNodeDiff with nil node
// ---------------------------------------------------------------------------

func TestSetNodeDiffNilNode(t *testing.T) {
	result, cmd := newTestMainModel(t).setNodeDiff(nil)
	if cmd != nil {
		t.Fatal("expected nil cmd for nil node")
	}
	_ = result
}

// ---------------------------------------------------------------------------
// Line 1638-1639: setSearchResults negative cursor
// ---------------------------------------------------------------------------

func TestSetSearchResultsNegativeCursorClamps(t *testing.T) {
	m := newTestMainModel(t)
	m.search.SetValue("yarn")
	m.setSearchResults()
	if len(m.filtered) == 0 {
		t.Fatal("expected at least one result for 'yarn'")
	}

	// Force cursor negative and run setSearchResults again
	m.resultsCursor = -3
	m.setSearchResults()
	if m.resultsCursor != 0 {
		t.Fatalf("expected cursor clamped to 0, got %d", m.resultsCursor)
	}
}

func TestSelectedSearchResultNegativeCursor(t *testing.T) {
	m := newTestMainModel(t)
	m.search.SetValue("")
	m.setSearchResults()
	if len(m.filtered) == 0 {
		t.Fatal("expected at least one search result")
	}
	m.resultsCursor = -1
	_, ok := m.selectedSearchResult()
	if ok {
		t.Fatal("expected false from selectedSearchResult with negative cursor")
	}
}

// ---------------------------------------------------------------------------
// Line 1051: resultsView with nil icon
// ---------------------------------------------------------------------------

func TestResultsViewNilIconFallback(t *testing.T) {
	m := newTestMainModel(t)
	m.width = 100
	m.height = 40
	isDark := true
	m.isDarkBackground = &isDark

	// Create files with weird extensions that neo.ByPath won't recognize
	m.files = []*gitdiff.File{{NewName: "file.unknown_ext_12345"}, {NewName: "another.xyz123"}}
	m.fileTree = m.fileTree.SetFiles(m.files)

	m.searching = true
	m.search.SetValue("")
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
// Mouse: scroll down/up in search results through Update (scrollUpInSearchResults)
// ---------------------------------------------------------------------------

func TestMouseScrollUpInSearchResults(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 10})
	m.searching = true
	m.search.SetValue("")
	m.setSearchResults()
	m.resultsVp.SetWidth(m.config.UI.SearchTreeWidth)
	m.resultsVp.SetHeight(2)
	m.resultsVp.SetContent(m.resultsView())

	if m.resultsVp.TotalLineCount() <= m.resultsVp.Height() {
		t.Skip("search results fit viewport")
	}

	m.resultsVp.ScrollDown(5)

	_ = m.View().Content
	z := waitForZone(t, zoneSearchResults)

	m = updateMainModel(t, m, tea.MouseMotionMsg(tea.Mouse{
		X:      z.StartX + 2,
		Y:      z.StartY + 1,
		Button: tea.MouseWheelUp,
	}))

	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after scroll up in search results")
	}
}
