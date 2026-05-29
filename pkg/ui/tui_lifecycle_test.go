package ui

import (
	"fmt"
	"image/color"
	"os"
	"os/exec"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/bluekeyes/go-gitdiff/gitdiff"
	zone "github.com/lrstanley/bubblezone/v2"

	"github.com/dlvhdr/diffnav/pkg/config"
	"github.com/dlvhdr/diffnav/pkg/ui/common"
)

func TestBackgroundColorDetectionStillWorksWhileSearching(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m = updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Text: "t", Code: 't'}))
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

func TestUpdateRepoRootMsgSetsRoot(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	m = updateMainModel(t, m, repoRootMsg("/home/user/repo"))
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after repoRootMsg")
	}
}

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

func TestUpdateErrMsgRendersView(t *testing.T) {
	m := newTestMainModel(t)
	m = updateMainModel(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	m = updateMainModel(t, m, common.ErrMsg{Err: fmt.Errorf("test error")})
	view := m.View().Content
	if view == "" {
		t.Fatal("expected non-empty view after ErrMsg")
	}
}

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

func TestFetchFileTreeErrorPath(t *testing.T) {
	m := newTestMainModel(t)
	m.input = "diff --git a/foo b/bar\nindex broken\n"

	msg := m.fetchFileTree()
	if _, ok := msg.(common.ErrMsg); !ok {
		t.Fatalf("expected common.ErrMsg for invalid diff input, got %T", msg)
	}
}

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

func TestScheduleWatchTickProducesMsg(t *testing.T) {
	m := newTestMainModel(t)
	m.watchInterval = 1 * time.Millisecond
	cmd := m.scheduleWatchTick()
	msg := cmd()
	if _, ok := msg.(watchTickMsg); !ok {
		t.Fatalf("expected watchTickMsg, got %T", msg)
	}
}

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
