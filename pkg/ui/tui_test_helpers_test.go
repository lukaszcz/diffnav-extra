package ui

import (
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/bluekeyes/go-gitdiff/gitdiff"
	zone "github.com/lrstanley/bubblezone/v2"

	"github.com/dlvhdr/diffnav/pkg/config"
)

// newTestMainModel creates a mainModel populated with the standard
// multiple_files.txt fixture for use in tests.
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

// updateMainModel calls m.Update(msg) and returns the resulting mainModel.
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

// switchToPanel drives the model to the desired panel through Update
// by sending Tab key presses. Assumes the model starts on FileTreePanel
// (the default from New()).
func switchToPanel(t *testing.T, m mainModel, target Panel) mainModel {
	t.Helper()
	if target == DiffViewerPanel {
		return updateMainModel(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	}
	return m
}

// Tiny helper so the test doesn't need to import diffviewer just for the
// header height constant.
func diffviewerDirHeaderHeight() int { return 3 }
