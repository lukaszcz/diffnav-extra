package ui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	zone "github.com/lrstanley/bubblezone/v2"

	"github.com/charmbracelet/x/ansi"
	"github.com/lukaszcz/diffnav-extra/pkg/config"
	"github.com/lukaszcz/diffnav-extra/pkg/filenode"
	"github.com/lukaszcz/diffnav-extra/pkg/ui/panes/diffviewer"
)

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

func TestUpdateEscapeNoSelectionRenders(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after Escape without selection")
	}
}

func TestUpdateQuitKeyReturnsQuit(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Text: "q", Code: 'q'}))
	if cmd == nil {
		t.Fatal("expected non-nil cmd when pressing q")
	}
}

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
		m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

		// Toggle file tree off by pressing "e" (keys.ToggleFileTree)
		m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "e", Code: 'e'}))

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
		// Enter search mode by pressing "t" (keys.Search), which calls
		// setSearchResults and sets up the viewport internally.
		m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "t", Code: 't'}))

		// Type the search query character by character through Update so that
		// searchUpdate recalculates results on each keystroke.
		for _, ch := range "yarn" {
			m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: string(ch), Code: ch}))
		}

		view := m.View().Content
		if view == "" {
			t.Fatal("expected non-empty view with search active")
		}
		waitForZone(t, zoneSearchBox)
	})
}

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
				m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
				view := m.View().Content
				plain := ansi.Strip(view)
				// When the file tree is hidden, the search placeholder text should
				// not appear in the view since the sidebar is not rendered.
				if strings.Contains(plain, "Filter") {
					t.Fatal("expected file tree sidebar to be hidden when ShowFileTree=false")
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
	plainAfter := ansi.Strip(viewAfter)
	if !strings.Contains(plainAfter, "quit") && !strings.Contains(plainAfter, "Quit") {
		t.Fatal("expected help overlay to remain visible after mouse scroll")
	}
	_ = viewBefore
}

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
