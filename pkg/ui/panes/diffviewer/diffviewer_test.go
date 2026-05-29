package diffviewer

import (
	"context"
	"fmt"
	"io"
	"math"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/bluekeyes/go-gitdiff/gitdiff"
	"github.com/charmbracelet/x/ansi"

	"github.com/dlvhdr/diffnav/pkg/ui/common"
)

func TestUpdateIgnoresStaleDiffContentMsg(t *testing.T) {
	m := New(false, "auto")
	m.vp.SetWidth(120)
	key := cacheKey("/", false)
	m.cache[key] = &cachedNode{}
	m.renderID = 2

	updated, _ := m.Update(diffContentMsg{
		cacheKey: key,
		text:     "stale",
		renderID: 1,
	})

	if updated.cache[key].diff != "" {
		t.Fatalf("expected stale render to be ignored, got %q", updated.cache[key].diff)
	}
}

func TestUpdateAcceptsCurrentDiffContentMsg(t *testing.T) {
	m := New(false, "auto")
	m.vp.SetWidth(120)
	key := cacheKey("/", false)
	m.cache[key] = &cachedNode{}
	m.renderID = 3

	updated, _ := m.Update(diffContentMsg{
		cacheKey: key,
		text:     "fresh",
		renderID: 3,
	})

	if updated.cache[key].diff != "fresh" {
		t.Fatalf("expected current render to be applied, got %q", updated.cache[key].diff)
	}
}

func TestSetFilePatchRerendersEmptyCachedEntry(t *testing.T) {
	m := New(false, "dark")
	m.Common.Width = 120
	file := &gitdiff.File{NewName: "src/app.go"}
	key := cacheKey("src/app.go", false)
	m.cache[key] = &cachedNode{
		path:  "src/app.go",
		files: []*gitdiff.File{file},
	}

	updated, cmd := m.SetFilePatch(file)

	if cmd == nil {
		t.Fatal("expected empty cached file diff to trigger a new render")
	}
	if updated.file == nil || updated.file.path != "src/app.go" {
		t.Fatalf("expected selected file to be src/app.go, got %#v", updated.file)
	}
}

func TestSetDirPatchRerendersEmptyCachedEntry(t *testing.T) {
	m := New(false, "dark")
	m.Common.Width = 120
	file := &gitdiff.File{NewName: "src/app.go"}
	key := cacheKey("src", false)
	m.cache[key] = &cachedNode{
		path:  "src",
		files: []*gitdiff.File{file},
	}

	updated, cmd := m.SetDirPatch("src", []*gitdiff.File{file})

	if cmd == nil {
		t.Fatal("expected empty cached dir diff to trigger a new render")
	}
	if updated.dir == nil || updated.dir.path != "src" {
		t.Fatalf("expected selected dir to be src, got %#v", updated.dir)
	}
}

func TestDeltaArgsCapsUnifiedLineLength(t *testing.T) {
	args := deltaArgs(120, false, nil, nil)

	if strings.Contains(strings.Join(args, "\x00"), "--max-line-length=0") {
		t.Fatalf("unified delta args must not disable long-line truncation: %#v", args)
	}
	if !containsArg(args, "--max-line-length=4096") {
		t.Fatalf("expected unified delta args to cap long lines, got %#v", args)
	}
	if containsArg(args, "--side-by-side") {
		t.Fatalf("did not expect side-by-side arg for unified render: %#v", args)
	}
}

func TestDeltaArgsCapsSideBySideLineLength(t *testing.T) {
	args := deltaArgs(120, true, nil, nil)

	if !containsArg(args, "--max-line-length=120") {
		t.Fatalf("expected side-by-side delta args to cap long lines, got %#v", args)
	}
	if !containsArg(args, "--side-by-side") {
		t.Fatalf("expected side-by-side arg, got %#v", args)
	}
}

func TestSetFilePatchCancelsInFlightRender(t *testing.T) {
	m := New(false, "dark")
	m.Common.Width = 120

	firstStarted := make(chan struct{})
	firstCanceled := make(chan struct{})
	var closeFirstStarted sync.Once
	var closeFirstCanceled sync.Once
	var calls int
	var callsMu sync.Mutex
	m.renderer.run = func(
		ctx context.Context,
		_ []string,
		writeInput func(io.Writer) error,
	) ([]byte, error) {
		if err := writeInput(io.Discard); err != nil {
			return nil, err
		}
		callsMu.Lock()
		calls++
		call := calls
		callsMu.Unlock()
		if call == 1 {
			closeFirstStarted.Do(func() { close(firstStarted) })
			<-ctx.Done()
			closeFirstCanceled.Do(func() { close(firstCanceled) })
			return nil, ctx.Err()
		}
		return []byte("fresh"), nil
	}

	m, firstCmd := m.SetFilePatch(&gitdiff.File{NewName: "one.go"})
	if firstCmd == nil {
		t.Fatal("expected first render command")
	}
	firstDone := make(chan tea.Msg, 1)
	go func() {
		firstDone <- firstCmd()
	}()

	select {
	case <-firstStarted:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first render to start")
	}

	m, secondCmd := m.SetFilePatch(&gitdiff.File{NewName: "two.go"})
	if secondCmd == nil {
		t.Fatal("expected second render command")
	}

	select {
	case <-firstCanceled:
	case <-time.After(time.Second):
		t.Fatal("expected second render to cancel first render")
	}
	if msg := <-firstDone; msg != nil {
		t.Fatalf("expected canceled render to return nil msg, got %#v", msg)
	}
	msg := secondCmd()
	if msg == nil {
		t.Fatal("expected second render message")
	}
}

func TestSetFilePatchCacheHitCancelsInFlightRender(t *testing.T) {
	m := New(false, "dark")
	m.Common.Width = 120

	firstStarted := make(chan struct{})
	firstCanceled := make(chan struct{})
	m.renderer.run = func(
		ctx context.Context,
		_ []string,
		writeInput func(io.Writer) error,
	) ([]byte, error) {
		if err := writeInput(io.Discard); err != nil {
			return nil, err
		}
		close(firstStarted)
		<-ctx.Done()
		close(firstCanceled)
		return nil, ctx.Err()
	}

	m, firstCmd := m.SetFilePatch(&gitdiff.File{NewName: "one.go"})
	if firstCmd == nil {
		t.Fatal("expected first render command")
	}
	firstDone := make(chan tea.Msg, 1)
	go func() {
		firstDone <- firstCmd()
	}()

	select {
	case <-firstStarted:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first render to start")
	}

	file := &gitdiff.File{NewName: "two.go"}
	key := cacheKey("two.go", false)
	m.cache[key] = &cachedNode{
		path:  "two.go",
		files: []*gitdiff.File{file},
		diff:  "cached",
		ready: true,
	}
	m, cachedCmd := m.SetFilePatch(file)
	if cachedCmd != nil {
		t.Fatal("expected cached file selection to avoid a new render")
	}
	if m.file == nil || m.file.diff != "cached" {
		t.Fatalf("expected cached diff to become active, got %#v", m.file)
	}

	select {
	case <-firstCanceled:
	case <-time.After(time.Second):
		t.Fatal("expected cache hit to cancel in-flight render")
	}
	if msg := <-firstDone; msg != nil {
		t.Fatalf("expected canceled render to return nil msg, got %#v", msg)
	}
}

func containsArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func TestRenderPreamble_Empty(t *testing.T) {
	if got := renderPreamble(""); got != "" {
		t.Fatalf("expected empty string for empty preamble, got %q", got)
	}
	if got := renderPreamble("   \n  \n  "); got != "" {
		t.Fatalf("expected empty string for whitespace-only preamble, got %q", got)
	}
}

func TestRenderPreamble_GitShow(t *testing.T) {
	preamble := `commit abc123def456
Author: Jane Doe <jane@example.com>
Date:   Mon Jan 1 00:00:00 2026 +0000

    feat: add new feature

    This is the body of the commit message.`

	got := renderPreamble(preamble)
	plain := ansi.Strip(got)

	// All original content lines should be preserved in the output.
	for _, want := range []string{
		"commit abc123def456",
		"Author: Jane Doe <jane@example.com>",
		"Date:   Mon Jan 1 00:00:00 2026 +0000",
		"feat: add new feature",
		"This is the body of the commit message.",
	} {
		if !strings.Contains(plain, want) {
			t.Errorf("expected output to contain %q, got:\n%s", want, plain)
		}
	}
}

func TestRenderPreamble_MergeCommit(t *testing.T) {
	preamble := `commit abc123def456
Merge: aaa111 bbb222
Author: Jane Doe <jane@example.com>
Date:   Mon Jan 1 00:00:00 2026 +0000

    Merge branch 'feature' into main`

	got := renderPreamble(preamble)
	plain := ansi.Strip(got)

	for _, want := range []string{
		"Merge: aaa111 bbb222",
		"Merge branch 'feature' into main",
	} {
		if !strings.Contains(plain, want) {
			t.Errorf("expected output to contain %q, got:\n%s", want, plain)
		}
	}
}

// scrollTestModel returns a dark-themed model sized so the diff viewport shows
// vpHeight rows, leaving room to scroll multi-line diffs.
func scrollTestModel(vpHeight int) Model {
	m := New(false, "dark")
	m.Common.Width = 120
	m.Common.Height = vpHeight + DirHeaderHeight
	m.vp.SetWidth(m.contentWidth())
	m.vp.SetHeight(vpHeight)
	return m
}

// cacheReadyFile registers a ready cache entry of lineCount lines so
// SetFilePatch takes the synchronous cached path, and returns the file.
func cacheReadyFile(m Model, name string, lineCount int) *gitdiff.File {
	f := &gitdiff.File{NewName: name}
	m.cache[cacheKey(name, m.sideBySide)] = &cachedNode{
		path:  name,
		files: []*gitdiff.File{f},
		diff:  strings.Repeat("line\n", lineCount),
		ready: true,
	}
	return f
}

func TestScrollPositionRememberedPerFile(t *testing.T) {
	m := scrollTestModel(10)
	a := cacheReadyFile(m, "a.go", 100)
	b := cacheReadyFile(m, "b.go", 100)

	m, _ = m.SetFilePatch(a)
	if m.YOffset() != 0 {
		t.Fatalf("expected a new file to start at the top, got %d", m.YOffset())
	}
	m.vp.ScrollDown(40)
	if m.YOffset() != 40 {
		t.Fatalf("precondition: expected YOffset 40 after scroll, got %d", m.YOffset())
	}

	// A never-visited file starts at the top.
	m, _ = m.SetFilePatch(b)
	if m.YOffset() != 0 {
		t.Fatalf("expected an unvisited file to start at the top, got %d", m.YOffset())
	}

	// Returning to the first file restores its own remembered position.
	m, _ = m.SetFilePatch(a)
	if m.YOffset() != 40 {
		t.Fatalf("expected the first file to restore scroll 40, got %d", m.YOffset())
	}

	// The second file's position is tracked independently.
	m, _ = m.SetFilePatch(b)
	if m.YOffset() != 0 {
		t.Fatalf("expected the second file to stay at the top, got %d", m.YOffset())
	}
}

func TestScrollPositionRememberedPerDirectory(t *testing.T) {
	m := scrollTestModel(10)
	f := &gitdiff.File{NewName: "src/x.go"}
	for _, dir := range []string{"src", "lib"} {
		m.cache[cacheKey(dir, false)] = &cachedNode{
			path:  dir,
			files: []*gitdiff.File{f},
			diff:  strings.Repeat("line\n", 100),
			ready: true,
		}
	}

	m, _ = m.SetDirPatch("src", []*gitdiff.File{f})
	m.vp.ScrollDown(30)
	if m.YOffset() != 30 {
		t.Fatalf("precondition: expected YOffset 30, got %d", m.YOffset())
	}

	m, _ = m.SetDirPatch("lib", []*gitdiff.File{f})
	if m.YOffset() != 0 {
		t.Fatalf("expected an unvisited directory to start at the top, got %d", m.YOffset())
	}

	m, _ = m.SetDirPatch("src", []*gitdiff.File{f})
	if m.YOffset() != 30 {
		t.Fatalf("expected the directory diff to restore scroll 30, got %d", m.YOffset())
	}
}

func TestScrollClampedWhenDiffShrinksAfterInvalidation(t *testing.T) {
	m := scrollTestModel(10)
	a := cacheReadyFile(m, "a.go", 100)
	m, _ = m.SetFilePatch(a)
	m.vp.ScrollDown(80)
	if m.YOffset() != 80 {
		t.Fatalf("precondition: expected YOffset 80, got %d", m.YOffset())
	}

	// Watch refresh: ClearCache snapshots the live offset and drops the render;
	// the fresh diff is far shorter than the viewport is tall.
	m.ClearCache()
	m, _ = m.Update(diffContentMsg{
		cacheKey: cacheKey("a.go", false),
		text:     strings.Repeat("x\n", 3),
		renderID: m.renderID,
	})

	if m.YOffset() != 0 {
		t.Fatalf(
			"expected a stale offset to clamp to the top for a shorter diff, got %d",
			m.YOffset(),
		)
	}
}

func TestScrollRestoredWhenDiffGrowsAfterInvalidation(t *testing.T) {
	m := scrollTestModel(10)
	a := cacheReadyFile(m, "a.go", 100)
	m, _ = m.SetFilePatch(a)
	m.vp.ScrollDown(50)

	// Watch refresh where the fresh diff is still long enough to honor the
	// remembered position.
	m.ClearCache()
	m, _ = m.Update(diffContentMsg{
		cacheKey: cacheKey("a.go", false),
		text:     strings.Repeat("y\n", 300),
		renderID: m.renderID,
	})

	if m.YOffset() != 50 {
		t.Fatalf("expected offset 50 to be restored on a longer diff, got %d", m.YOffset())
	}
}

func TestSideBySideToggleSnapshotsScroll(t *testing.T) {
	m := scrollTestModel(10)
	a := cacheReadyFile(m, "a.go", 100)
	m, _ = m.SetFilePatch(a)
	m.vp.ScrollDown(25)

	// Toggling layout must snapshot the current (unified) layout's position
	// before the cache key gains its side-by-side suffix.
	_ = m.SetSideBySide(true)

	if got := m.scrollOffsets[cacheKey("a.go", false)]; got != 25 {
		t.Fatalf("expected the unified scroll 25 to be snapshotted before toggle, got %d", got)
	}
}

// -------------------------------------------------------------------
// New tests for previously uncovered code paths
// -------------------------------------------------------------------

// SetPreamble stores and retrieves preamble text.
func TestSetPreamble(t *testing.T) {
	m := New(false, "dark")
	if m.preamble != "" {
		t.Fatalf("expected empty preamble on init, got %q", m.preamble)
	}
	m.SetPreamble("commit abc123")
	if m.preamble != "commit abc123" {
		t.Fatalf("expected preamble %q, got %q", "commit abc123", m.preamble)
	}
	m.SetPreamble("")
	if m.preamble != "" {
		t.Fatalf("expected empty preamble after clear, got %q", m.preamble)
	}
}

// applyHighlight() - selection off-screen (above the viewport).
func TestApplyHighlight_SelectionAboveViewport(t *testing.T) {
	m := New(false, "dark")
	m.vp.SetWidth(40)
	m.vp.SetHeight(5)
	m.vp.SetContent(strings.Repeat("line\n", 20))
	m.vp.ScrollDown(15) // viewport shows lines 15..19

	m.sel = selection{
		anchor:  Point{Line: 0, Col: 0},
		head:    Point{Line: 2, Col: 3},
		colBand: [2]int{0, 100},
		active:  true,
		has:     false,
	}

	vpView := m.vp.View()
	out := m.applyHighlight(vpView)
	if strings.Contains(out, "\x1b[7m") {
		t.Fatalf("expected no reverse-video when selection is above viewport")
	}
}

// applyHighlight() - selection off-screen (below the viewport).
func TestApplyHighlight_SelectionBelowViewport(t *testing.T) {
	m := New(false, "dark")
	m.vp.SetWidth(40)
	m.vp.SetHeight(5)
	m.vp.SetContent(strings.Repeat("line\n", 20))

	m.sel = selection{
		anchor:  Point{Line: 20, Col: 0},
		head:    Point{Line: 22, Col: 3},
		colBand: [2]int{0, 100},
		active:  true,
		has:     false,
	}

	vpView := m.vp.View()
	out := m.applyHighlight(vpView)
	if strings.Contains(out, "\x1b[7m") {
		t.Fatalf("expected no reverse-video when selection is below viewport")
	}
}

// applyHighlight() - with a valid selection that overlaps the viewport.
func TestApplyHighlight_ValidSelection(t *testing.T) {
	m := New(false, "dark")
	m.vp.SetWidth(40)
	m.vp.SetHeight(5)
	m.vp.SetContent("line0 content\nline1 content\nline2 content\nline3 content\nline4 content")

	m.sel = selection{
		anchor:  Point{Line: 0, Col: 0},
		head:    Point{Line: 1, Col: 5},
		colBand: [2]int{0, 100},
		active:  false,
		has:     true,
	}

	vpView := m.vp.View()
	out := m.applyHighlight(vpView)
	if !strings.Contains(out, "\x1b[7m") {
		t.Fatalf("expected reverse-video escape in applyHighlight output")
	}
}

// SetSize() sets width/height and calls diff().
func TestSetSize(t *testing.T) {
	m := New(false, "dark")
	m.Common.Width = 80
	m.Common.Height = 20

	cmd := m.SetSize(100, 40)
	if m.Common.Width != 100 {
		t.Fatalf("expected width=100, got %d", m.Common.Width)
	}
	if m.Common.Height != 40 {
		t.Fatalf("expected height=40, got %d", m.Common.Height)
	}
	if m.vp.Width() != 100-scrollbarWidth {
		t.Fatalf("expected vp width=%d, got %d", 100-scrollbarWidth, m.vp.Width())
	}
	if m.vp.Height() != 40-DirHeaderHeight {
		t.Fatalf("expected vp height=%d, got %d", 40-DirHeaderHeight, m.vp.Height())
	}
	// When no file/dir is set, diff() returns nil.
	if cmd != nil {
		t.Fatalf("expected nil cmd when no file/dir set, got %v", cmd)
	}
}

// SetSize() with a file set triggers diff().
func TestSetSize_WithFileTriggersDiff(t *testing.T) {
	m := New(false, "dark")
	m.Common.Width = 80
	m.Common.Height = 30

	var renderCalled bool
	m.renderer.run = func(ctx context.Context, _ []string, writeInput func(io.Writer) error) ([]byte, error) {
		renderCalled = true
		if err := writeInput(io.Discard); err != nil {
			return nil, err
		}
		return []byte("rendered"), nil
	}
	m.file = &cachedNode{
		path:  "test.go",
		files: []*gitdiff.File{{NewName: "test.go"}},
	}
	key := cacheKey("test.go", false)
	m.cache[key] = m.file

	cmd := m.SetSize(120, 50)
	if cmd == nil {
		t.Fatal("expected non-nil cmd when file is set")
	}
	msg := cmd()
	if !renderCalled {
		t.Fatal("expected render to be called")
	}
	_ = msg
}

// SetSize() clears the cache.
func TestSetSize_ClearsCache(t *testing.T) {
	m := New(false, "dark")
	m.Common.Width = 80
	m.Common.Height = 20
	m.cache[cacheKey("test.go", false)] = &cachedNode{path: "test.go", ready: true}
	if len(m.cache) != 1 {
		t.Fatalf("expected 1 cache entry before SetSize")
	}
	_ = m.SetSize(100, 40)
	if len(m.cache) != 0 {
		t.Fatalf("expected 0 cache entries after SetSize, got %d", len(m.cache))
	}
}

// Height() and Width().
func TestHeightAndWidth(t *testing.T) {
	m := New(false, "dark")
	m.Common.Width = 80
	m.Common.Height = 30
	m.vp.SetWidth(m.contentWidth())
	m.vp.SetHeight(m.Common.Height - DirHeaderHeight)

	if m.Height() != 30-DirHeaderHeight {
		t.Fatalf("expected Height=%d, got %d", 30-DirHeaderHeight, m.Height())
	}
	if m.Width() != 80-scrollbarWidth {
		t.Fatalf("expected Width=%d, got %d", 80-scrollbarWidth, m.Width())
	}
}

// SetContent() loads content directly into the viewport.
func TestSetContent(t *testing.T) {
	m := New(false, "dark")
	m.vp.SetWidth(40)
	m.vp.SetHeight(5)

	lines := []string{"first", "second", "third"}
	content := strings.Join(lines, "\n")
	m.SetContent(content)

	if m.vp.TotalLineCount() != 3 {
		t.Fatalf("expected 3 lines, got %d", m.vp.TotalLineCount())
	}
}

// ScrollUp(), ScrollDown(), ScrollBottom(), ScrollTop().
func TestScrollMethods(t *testing.T) {
	m := New(false, "dark")
	m.vp.SetWidth(40)
	m.vp.SetHeight(5)

	var b strings.Builder
	for i := 0; i < 50; i++ {
		fmt.Fprintf(&b, "line-%02d\n", i)
	}
	m.vp.SetContent(strings.TrimSuffix(b.String(), "\n"))

	if m.YOffset() != 0 {
		t.Fatalf("expected initial YOffset=0, got %d", m.YOffset())
	}

	m.ScrollDown(10)
	if m.YOffset() != 10 {
		t.Fatalf("expected YOffset=10 after ScrollDown(10), got %d", m.YOffset())
	}

	m.ScrollUp(3)
	if m.YOffset() != 7 {
		t.Fatalf("expected YOffset=7 after ScrollUp(3), got %d", m.YOffset())
	}

	m.ScrollBottom()
	if m.YOffset() <= 7 {
		t.Fatalf("expected YOffset > 7 after ScrollBottom, got %d", m.YOffset())
	}

	m.ScrollTop()
	if m.YOffset() != 0 {
		t.Fatalf("expected YOffset=0 after ScrollTop, got %d", m.YOffset())
	}
}

// YOffset() and TotalLineCount().
func TestYOffsetAndTotalLineCount(t *testing.T) {
	m := New(false, "dark")
	m.vp.SetWidth(40)
	m.vp.SetHeight(5)

	m.SetContent("a\nb\nc\nd\ne\nf\ng\nh")
	if m.TotalLineCount() != 8 {
		t.Fatalf("expected TotalLineCount=8, got %d", m.TotalLineCount())
	}

	m.ScrollDown(3)
	if m.YOffset() != 3 {
		t.Fatalf("expected YOffset=3, got %d", m.YOffset())
	}
}

// DebugSelection() returns the current selection state.
func TestDebugSelection(t *testing.T) {
	m := New(false, "auto")
	m.sel = selection{
		anchor:  Point{Line: 1, Col: 2},
		head:    Point{Line: 3, Col: 4},
		colBand: [2]int{0, 40},
		active:  true,
	}

	anchor, head, band, active := m.DebugSelection()
	if anchor != (Point{Line: 1, Col: 2}) {
		t.Fatalf("unexpected anchor: %+v", anchor)
	}
	if head != (Point{Line: 3, Col: 4}) {
		t.Fatalf("unexpected head: %+v", head)
	}
	if band != [2]int{0, 40} {
		t.Fatalf("unexpected band: %v", band)
	}
	if !active {
		t.Fatalf("expected active=true")
	}
}

// selectedText() - no content loaded returns empty.
func TestSelectedText_NoContent(t *testing.T) {
	m := New(false, "dark")
	m.sel = selection{
		anchor:  Point{Line: 0, Col: 0},
		head:    Point{Line: 1, Col: 5},
		colBand: [2]int{0, 100},
		active:  false,
		has:     true,
	}
	text := m.selectedText()
	if text != "" {
		t.Fatalf("expected empty text with no file/dir set, got %q", text)
	}
}

// selectedText() - empty diff returns empty.
func TestSelectedText_EmptyDiff(t *testing.T) {
	m := New(false, "dark")
	m.file = &cachedNode{path: "test", diff: ""}
	m.sel = selection{
		anchor:  Point{Line: 0, Col: 0},
		head:    Point{Line: 0, Col: 5},
		colBand: [2]int{0, 100},
		active:  false,
		has:     true,
	}
	text := m.selectedText()
	if text != "" {
		t.Fatalf("expected empty text for empty diff, got %q", text)
	}
}

// selectedText() - selection start line out of range returns empty.
func TestSelectedText_StartOutOfRange(t *testing.T) {
	m := New(false, "dark")
	m.file = &cachedNode{path: "test", diff: "line0\nline1"}
	m.sel = selection{
		anchor:  Point{Line: -1, Col: 0},
		head:    Point{Line: 0, Col: 3},
		colBand: [2]int{0, 100},
		active:  false,
		has:     true,
	}
	text := m.selectedText()
	if text != "" {
		t.Fatalf("expected empty text for out-of-range start line, got %q", text)
	}
}

// selectedText() - selection end line clamped to last line.
func TestSelectedText_EndLineClamped(t *testing.T) {
	m := New(false, "dark")
	m.file = &cachedNode{path: "test", diff: "line0\nline1"}
	m.sel = selection{
		anchor:  Point{Line: 0, Col: 0},
		head:    Point{Line: 100, Col: 3},
		colBand: [2]int{0, 1000},
		active:  false,
		has:     true,
	}
	text := m.selectedText()
	if text == "" {
		t.Fatalf("expected non-empty text with clamped end line")
	}
	// Should contain content from both available lines
	if !strings.Contains(text, "line0") || !strings.Contains(text, "line1") {
		t.Fatalf("expected both lines in text, got %q", text)
	}
}

// selectedText() - with file diff content.
func TestSelectedText_WithFileContent(t *testing.T) {
	m := New(false, "dark")
	m.file = &cachedNode{path: "test", diff: "some line\nanother line"}
	m.sel = selection{
		anchor:  Point{Line: 0, Col: 0},
		head:    Point{Line: 1, Col: 5},
		colBand: [2]int{0, 100},
		active:  false,
		has:     true,
	}
	text := m.selectedText()
	if text == "" {
		t.Fatalf("expected non-empty text from selectedText")
	}
	if !strings.Contains(text, "some") {
		t.Errorf("expected selected text to contain 'some' from first line, got %q", text)
	}
	if !strings.Contains(text, "anot") {
		t.Errorf("expected selected text to contain 'anot' from second line, got %q", text)
	}
}

// selectedText() - with dir diff content.
func TestSelectedText_WithDirContent(t *testing.T) {
	m := New(false, "dark")
	m.dir = &cachedNode{path: "src", diff: "dir line0\ndir line1\ndir line2"}
	m.sel = selection{
		anchor:  Point{Line: 0, Col: 3},
		head:    Point{Line: 2, Col: 5},
		colBand: [2]int{0, 100},
		active:  false,
		has:     true,
	}
	text := m.selectedText()
	if text == "" {
		t.Fatalf("expected non-empty text from dir selectedText")
	}
	// The selection starts at col 3 on line 0, so 'dir '\041is excluded.
	if !strings.Contains(text, "line0") {
		t.Errorf("expected selected text to contain 'line0', got %q", text)
	}
	if !strings.Contains(text, "dir line1") {
		t.Errorf("expected selected text to contain full middle line, got %q", text)
	}
}

// diffFile() - returns nil for zero width.
func TestDiffFile_ZeroWidth(t *testing.T) {
	node := &cachedNode{
		path:  "test.go",
		files: []*gitdiff.File{{NewName: "test.go"}},
	}
	cmd := diffFile(context.Background(), defaultDeltaRenderer(), node, 0, false, nil, 1)
	if cmd != nil {
		t.Fatalf("expected nil cmd for zero width, got %v", cmd)
	}
}

// diffFile() - returns nil for nil node.
func TestDiffFile_NilNode(t *testing.T) {
	cmd := diffFile(context.Background(), defaultDeltaRenderer(), nil, 80, false, nil, 1)
	if cmd != nil {
		t.Fatalf("expected nil cmd for nil node, got %v", cmd)
	}
}

// diffFile() - returns nil for node with wrong file count.
func TestDiffFile_WrongFileCount(t *testing.T) {
	node := &cachedNode{
		path:  "test.go",
		files: []*gitdiff.File{},
	}
	cmd := diffFile(context.Background(), defaultDeltaRenderer(), node, 80, false, nil, 1)
	if cmd != nil {
		t.Fatalf("expected nil cmd for wrong file count, got %v", cmd)
	}
}

// diffDir() - returns nil for zero width.
func TestDiffDir_ZeroWidth(t *testing.T) {
	node := &cachedNode{
		path:  "src",
		files: []*gitdiff.File{{NewName: "a.go"}},
	}
	cmd := diffDir(
		context.Background(),
		defaultDeltaRenderer(),
		node,
		0,
		false,
		nil,
		lipgloss.Color("8"),
		"",
		1,
	)
	if cmd != nil {
		t.Fatalf("expected nil cmd for zero width, got %v", cmd)
	}
}

// diffDir() - returns nil for nil dir.
func TestDiffDir_NilDir(t *testing.T) {
	cmd := diffDir(
		context.Background(),
		defaultDeltaRenderer(),
		nil,
		80,
		false,
		nil,
		lipgloss.Color("8"),
		"",
		1,
	)
	if cmd != nil {
		t.Fatalf("expected nil cmd for nil dir, got %v", cmd)
	}
}

// deltaMaxLineLength() - various width inputs.
func TestDeltaMaxLineLength(t *testing.T) {
	// Side-by-side with positive width less than maxDeltaLineLength
	if got := deltaMaxLineLength(100, true); got != 100 {
		t.Fatalf("expected 100 for sbs width=100, got %d", got)
	}

	// Side-by-side with width exceeding maxDeltaLineLength
	if got := deltaMaxLineLength(5000, true); got != maxDeltaLineLength {
		t.Fatalf("expected %d for sbs width=5000, got %d", maxDeltaLineLength, got)
	}

	// Side-by-side with zero width
	if got := deltaMaxLineLength(0, true); got != maxDeltaLineLength {
		t.Fatalf("expected %d for sbs width=0, got %d", maxDeltaLineLength, got)
	}

	// Unified mode always returns maxDeltaLineLength
	if got := deltaMaxLineLength(100, false); got != maxDeltaLineLength {
		t.Fatalf("expected %d for unified, got %d", maxDeltaLineLength, got)
	}
	if got := deltaMaxLineLength(0, false); got != maxDeltaLineLength {
		t.Fatalf("expected %d for unified width=0, got %d", maxDeltaLineLength, got)
	}
}

// SetDarkBackground() - auto theme (the only mode that has effect).
func TestSetDarkBackground_AutoTheme(t *testing.T) {
	m := New(false, "auto")
	m.Common.Width = 80

	// Initially nil, setting to true should set the field
	cmd := m.SetDarkBackground(true)
	if m.isDarkBackground == nil || !*m.isDarkBackground {
		t.Fatalf("expected isDarkBackground=true")
	}
	// No file/dir set, so diff() returns nil.
	if cmd != nil {
		t.Fatalf("expected nil cmd when no file/dir, got %v", cmd)
	}
}

// SetDarkBackground() - auto theme, change value.
func TestSetDarkBackground_AutoThemeChangeValue(t *testing.T) {
	m := New(false, "auto")
	m.Common.Width = 80

	_ = m.SetDarkBackground(true)
	cmd := m.SetDarkBackground(false)
	if m.isDarkBackground == nil || *m.isDarkBackground {
		t.Fatalf("expected isDarkBackground=false after change")
	}
	if cmd != nil {
		t.Fatalf("expected nil cmd when no file/dir, got %v", cmd)
	}
}

// SetDarkBackground() - auto theme, same value is no-op.
func TestSetDarkBackground_AutoThemeSameValue(t *testing.T) {
	m := New(false, "auto")
	m.Common.Width = 80

	_ = m.SetDarkBackground(true)
	cmd := m.SetDarkBackground(true)
	if cmd != nil {
		t.Fatalf("expected nil cmd when setting same value, got %v", cmd)
	}
}

// SetDarkBackground() - light theme (non-auto theme is a no-op).
func TestSetDarkBackground_LightTheme(t *testing.T) {
	m := New(false, "light")
	m.Common.Width = 80

	cmd := m.SetDarkBackground(false)
	if cmd != nil {
		t.Fatalf("expected nil cmd for light theme, got %v", cmd)
	}
}

// SetDarkBackground() - dark theme (non-auto theme is a no-op).
func TestSetDarkBackground_DarkTheme(t *testing.T) {
	m := New(false, "dark")
	m.Common.Width = 80

	cmd := m.SetDarkBackground(true)
	if cmd != nil {
		t.Fatalf("expected nil cmd for dark theme, got %v", cmd)
	}
}

// SetDarkBackground() - clears cache on value change.
func TestSetDarkBackground_ClearsCache(t *testing.T) {
	m := New(false, "auto")
	m.Common.Width = 80
	m.cache[cacheKey("/", false)] = &cachedNode{path: "/", ready: true}
	if len(m.cache) != 1 {
		t.Fatalf("expected 1 cache entry before SetDarkBackground")
	}

	_ = m.SetDarkBackground(true)
	if len(m.cache) != 0 {
		t.Fatalf("expected 0 cache entries after SetDarkBackground change, got %d", len(m.cache))
	}
}

// deltaThemeArgs() - dark background returns nil.
func TestDeltaThemeArgs_DarkMode(t *testing.T) {
	m := New(false, "dark")
	args := m.deltaThemeArgs()
	if args != nil {
		t.Fatalf("expected nil args for dark background, got %v", args)
	}
}

// deltaThemeArgs() - nil isDarkBackground returns nil.
func TestDeltaThemeArgs_NilBackground(t *testing.T) {
	m := New(false, "auto")
	args := m.deltaThemeArgs()
	if args != nil {
		t.Fatalf("expected nil args for nil isDarkBackground, got %v", args)
	}
}

// deltaThemeArgs() - light mode returns args.
func TestDeltaThemeArgs_LightMode(t *testing.T) {
	m := New(false, "light")
	args := m.deltaThemeArgs()
	if args == nil {
		t.Fatalf("expected non-nil args for light background")
	}
	found := false
	for _, a := range args {
		if a == "--light" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected --light in args, got %v", args)
	}
	foundSyntax := false
	for _, a := range args {
		if strings.Contains(a, "--syntax-theme=") {
			foundSyntax = true
			break
		}
	}
	if !foundSyntax {
		t.Fatalf("expected --syntax-theme in args, got %v", args)
	}
}

// parseThemeMode() - test all paths.
func TestParseThemeMode(t *testing.T) {
	cases := []struct {
		input string
		want  themeMode
	}{
		{"auto", themeAuto},
		{"light", themeLight},
		{"dark", themeDark},
		{"", themeAuto},
		{"bogus", themeAuto},
		{"  light  ", themeLight},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := parseThemeMode(tc.input)
			if got != tc.want {
				t.Fatalf("parseThemeMode(%q) = %d, want %d", tc.input, got, tc.want)
			}
		})
	}
}

// RootDiffStats() - cache hit.
func TestRootDiffStats_CacheHit(t *testing.T) {
	m := New(false, "dark")
	m.cache[cacheKey("/", false)] = &cachedNode{
		path:      "/",
		additions: 42,
		deletions: 7,
		ready:     true,
	}

	add, del := m.RootDiffStats()
	if add != 42 || del != 7 {
		t.Fatalf("expected (42, 7), got (%d, %d)", add, del)
	}
}

// RootDiffStats() - cache miss.
func TestRootDiffStats_CacheMiss(t *testing.T) {
	m := New(false, "dark")

	add, del := m.RootDiffStats()
	if add != 0 || del != 0 {
		t.Fatalf("expected (0, 0) on cache miss, got (%d, %d)", add, del)
	}
}

// View() - with finalized selection overlay.
func TestView_WithFinalizedSelection(t *testing.T) {
	m := New(false, "dark")
	m.vp.SetWidth(40)
	m.vp.SetHeight(5)
	m.vp.SetContent("line0 content\nline1 content\nline2 content\nline3 content\nline4 content")

	m.sel = selection{
		anchor:  Point{Line: 0, Col: 0},
		head:    Point{Line: 1, Col: 5},
		colBand: [2]int{0, 100},
		active:  false,
		has:     true,
	}

	out := m.View()
	if !strings.Contains(out, "\x1b[7m") {
		t.Fatalf("expected reverse-video escape in View() with finalized selection")
	}
}

// View() - with active (in-progress) selection.
func TestView_WithActiveSelection(t *testing.T) {
	m := New(false, "dark")
	m.vp.SetWidth(40)
	m.vp.SetHeight(5)
	m.vp.SetContent("alpha\nbeta\ngamma\ndelta\nepsilon")

	m.sel = selection{
		anchor:  Point{Line: 0, Col: 0},
		head:    Point{Line: 2, Col: 3},
		colBand: [2]int{0, 100},
		active:  true,
		has:     false,
	}

	out := m.View()
	if !strings.Contains(out, "\x1b[7m") {
		t.Fatalf("expected reverse-video escape in View() with active selection")
	}
}

// diff() - returns nil when themeAuto and isDarkBackground is nil.
func TestDiff_AutoThemeNoBackgroundDetection(t *testing.T) {
	m := New(false, "auto")
	m.Common.Width = 80
	m.file = &cachedNode{path: "test.go", files: []*gitdiff.File{{NewName: "test.go"}}}

	cmd := m.diff()
	if cmd != nil {
		t.Fatalf("expected nil cmd when auto theme and no background detection, got %v", cmd)
	}
}

// diff() - cached file path returns nil cmd and sets content.
func TestDiff_CachedFilePath(t *testing.T) {
	m := New(false, "dark")
	m.Common.Width = 80

	f := &gitdiff.File{NewName: "cached.go"}
	key := cacheKey("cached.go", false)
	m.cache[key] = &cachedNode{
		path:  "cached.go",
		files: []*gitdiff.File{f},
		diff:  "cached diff content",
		ready: true,
	}

	m.file = &cachedNode{path: "cached.go", files: []*gitdiff.File{f}}
	cmd := m.diff()
	if cmd != nil {
		t.Fatalf("expected nil cmd for cached file, got %v", cmd)
	}
	if m.vp.TotalLineCount() < 1 {
		t.Fatalf("expected viewport to have content after cache hit")
	}
}

// diff() - cached dir path returns nil cmd.
func TestDiff_CachedDirPath(t *testing.T) {
	m := New(false, "dark")
	m.Common.Width = 80

	f := &gitdiff.File{NewName: "a.go"}
	key := cacheKey("src", false)
	m.cache[key] = &cachedNode{
		path:  "src",
		files: []*gitdiff.File{f},
		diff:  "dir diff content",
		ready: true,
	}

	m.dir = &cachedNode{path: "src", files: []*gitdiff.File{f}}
	cmd := m.diff()
	if cmd != nil {
		t.Fatalf("expected nil cmd for cached dir, got %v", cmd)
	}
}

// diff() - no file and no dir set returns nil.
func TestDiff_NoFileNoDir(t *testing.T) {
	m := New(false, "dark")
	m.Common.Width = 80

	cmd := m.diff()
	if cmd != nil {
		t.Fatalf("expected nil cmd when no file/dir, got %v", cmd)
	}
}

// headerView() - dir header.
func TestHeaderView_Dir(t *testing.T) {
	m := New(false, "dark")
	m.Common.Width = 80
	m.dir = &cachedNode{
		path:      "src",
		additions: 10,
		deletions: 3,
	}

	header := m.headerView()
	if header == "" {
		t.Fatalf("expected non-empty header for dir")
	}
	headerPlain := ansi.Strip(header)
	if !strings.Contains(headerPlain, "src") {
		t.Errorf("expected header to contain dir path %q, got %q", "src", headerPlain)
	}
	if !strings.Contains(headerPlain, "+10") {
		t.Errorf("expected header to contain additions +10, got %q", headerPlain)
	}
	if !strings.Contains(headerPlain, "-3") {
		t.Errorf("expected header to contain deletions -3, got %q", headerPlain)
	}
}

// headerView() - file header.
func TestHeaderView_File(t *testing.T) {
	m := New(false, "dark")
	m.Common.Width = 80
	m.file = &cachedNode{
		path:  "main.go",
		files: []*gitdiff.File{{NewName: "main.go", NewMode: 0o644}},
	}

	header := m.headerView()
	if header == "" {
		t.Fatalf("expected non-empty header for file")
	}
	headerPlain := ansi.Strip(header)
	if !strings.Contains(headerPlain, "main.go") {
		t.Errorf("expected header to contain file name %q, got %q", "main.go", headerPlain)
	}
}

// headerView() - no file and no dir.
func TestHeaderView_NoFileNoDir(t *testing.T) {
	m := New(false, "dark")
	m.Common.Width = 80

	header := m.headerView()
	if header != "" {
		t.Fatalf("expected empty header when no file or dir, got %q", header)
	}
}

// headerView() - file with wrong file count.
func TestHeaderView_FileWrongCount(t *testing.T) {
	m := New(false, "dark")
	m.Common.Width = 80
	m.file = &cachedNode{
		path:  "main.go",
		files: []*gitdiff.File{},
	}

	header := m.headerView()
	if header != "" {
		t.Fatalf("expected empty header for file with 0 files, got %q", header)
	}
}

// ClearSelection() drops all selection state.
func TestClearSelection(t *testing.T) {
	m := New(false, "auto")
	m.sel = selection{
		anchor:  Point{Line: 0, Col: 0},
		head:    Point{Line: 1, Col: 5},
		colBand: [2]int{0, 40},
		active:  false,
		has:     true,
	}

	m.ClearSelection()
	if m.HasSelection() {
		t.Fatalf("expected HasSelection() == false after ClearSelection")
	}
	if m.IsSelecting() {
		t.Fatalf("expected IsSelecting() == false after ClearSelection")
	}
}

// ClearCache() drops cache entries.
func TestClearCache(t *testing.T) {
	m := New(false, "dark")
	m.Common.Width = 80

	m.cache[cacheKey("test.go", false)] = &cachedNode{path: "test.go", diff: "content", ready: true}
	if len(m.cache) != 1 {
		t.Fatalf("expected 1 cache entry, got %d", len(m.cache))
	}

	m.ClearCache()
	if len(m.cache) != 0 {
		t.Fatalf("expected 0 cache entries after ClearCache, got %d", len(m.cache))
	}
}

// cacheKey() - unified vs side-by-side.
func TestCacheKey(t *testing.T) {
	unified := cacheKey("test.go", false)
	sbs := cacheKey("test.go", true)
	if unified == sbs {
		t.Fatalf("expected different keys for same path with different sbs mode")
	}
	if unified != "test.go" {
		t.Fatalf("expected %q for unified, got %q", "test.go", unified)
	}
	if sbs != "test.go:sbs" {
		t.Fatalf("expected %q for sbs, got %q", "test.go:sbs", sbs)
	}
}

// cacheReady() - nil, unready, ready.
func TestCacheReady(t *testing.T) {
	if cacheReady(nil) {
		t.Fatalf("expected false for nil node")
	}
	node := &cachedNode{ready: false}
	if cacheReady(node) {
		t.Fatalf("expected false for unready node")
	}
	readyNode := &cachedNode{ready: true}
	if !cacheReady(readyNode) {
		t.Fatalf("expected true for ready node")
	}
}

// currentKey() - no file, with file, with dir.
func TestCurrentKey(t *testing.T) {
	m := New(false, "dark")

	key, ok := m.currentKey()
	if ok {
		t.Fatalf("expected false when no file/dir, got key=%q ok=%v", key, ok)
	}

	m.file = &cachedNode{path: "test.go"}
	key, ok = m.currentKey()
	if !ok || key != cacheKey("test.go", false) {
		t.Fatalf("expected key=%q ok=true, got key=%q ok=%v", cacheKey("test.go", false), key, ok)
	}

	m.file = nil
	m.dir = &cachedNode{path: "src"}
	key, ok = m.currentKey()
	if !ok || key != cacheKey("src", false) {
		t.Fatalf("expected key=%q ok=true, got key=%q ok=%v", cacheKey("src", false), key, ok)
	}
}

// diffFile() - new file (IsNew) forces unified even when sbs=true.
func TestDiffFile_NewFileForcesUnified(t *testing.T) {
	var capturedArgs []string
	r := deltaRenderer{
		run: func(ctx context.Context, args []string, writeInput func(io.Writer) error) ([]byte, error) {
			capturedArgs = args
			_ = writeInput(io.Discard)
			return []byte("output"), nil
		},
	}

	newFile := &gitdiff.File{NewName: "new.go", IsNew: true}
	node := &cachedNode{
		path:  "new.go",
		files: []*gitdiff.File{newFile},
	}

	cmd := diffFile(context.Background(), r, node, 80, true, nil, 1)
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	msg := cmd()

	for _, a := range capturedArgs {
		if a == "--side-by-side" {
			t.Fatalf("expected no --side-by-side for new file, args: %v", capturedArgs)
		}
	}

	if msg == nil {
		t.Fatal("expected non-nil message")
	}
}

// diffFile() - deleted file forces unified.
func TestDiffFile_DeletedFileForcesUnified(t *testing.T) {
	var capturedArgs []string
	r := deltaRenderer{
		run: func(ctx context.Context, args []string, writeInput func(io.Writer) error) ([]byte, error) {
			capturedArgs = args
			_ = writeInput(io.Discard)
			return []byte("output"), nil
		},
	}

	delFile := &gitdiff.File{NewName: "del.go", IsDelete: true}
	node := &cachedNode{
		path:  "del.go",
		files: []*gitdiff.File{delFile},
	}

	cmd := diffFile(context.Background(), r, node, 80, true, nil, 1)
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	_ = cmd()

	for _, a := range capturedArgs {
		if a == "--side-by-side" {
			t.Fatalf("expected no --side-by-side for deleted file")
		}
	}
}

// diffFile() - canceled context returns nil msg.
func TestDiffFile_CanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	node := &cachedNode{
		path:  "test.go",
		files: []*gitdiff.File{{NewName: "test.go"}},
	}

	cmd := diffFile(ctx, defaultDeltaRenderer(), node, 80, false, nil, 1)
	if cmd == nil {
		t.Fatal("expected non-nil cmd even with canceled ctx")
	}
	msg := cmd()
	if msg != nil {
		t.Fatalf("expected nil msg for canceled context, got %v", msg)
	}
}

// diffDir() - canceled context returns nil msg.
func TestDiffDir_CanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	node := &cachedNode{
		path:  "src",
		files: []*gitdiff.File{{NewName: "a.go"}},
	}

	cmd := diffDir(ctx, defaultDeltaRenderer(), node, 80, false, nil, lipgloss.Color("8"), "", 1)
	if cmd == nil {
		t.Fatal("expected non-nil cmd even with canceled ctx")
	}
	msg := cmd()
	if msg != nil {
		t.Fatalf("expected nil msg for canceled context, got %v", msg)
	}
}

// diffDir() - preamble is prepended for root dir.
func TestDiffDir_PreamblePrepended(t *testing.T) {
	m := New(false, "dark")
	m.Common.Width = 80
	m.preamble = "commit abc123\nAuthor: Test"

	m.renderer.run = func(ctx context.Context, args []string, writeInput func(io.Writer) error) ([]byte, error) {
		_ = writeInput(io.Discard)
		return []byte("dir content\n"), nil
	}

	m.dir = &cachedNode{
		path:  "/",
		files: []*gitdiff.File{{NewName: "a.go"}},
	}
	key := cacheKey("/", false)
	m.cache[key] = m.dir

	cmd := m.diff()
	if cmd == nil {
		t.Fatal("expected cmd for dir diff")
	}
	msg := cmd()

	dcm, ok := msg.(diffContentMsg)
	if !ok {
		t.Fatalf("expected diffContentMsg, got %T", msg)
	}
	plain := ansi.Strip(dcm.text)
	if !strings.Contains(plain, "commit abc123") {
		t.Fatalf("expected preamble content in diff output, got: %q", plain)
	}
}

// diffDir() - non-root dir does not include preamble.
func TestDiffDir_NonRootNoPreamble(t *testing.T) {
	m := New(false, "dark")
	m.Common.Width = 80
	m.preamble = "commit abc123\nAuthor: Test"

	m.renderer.run = func(ctx context.Context, args []string, writeInput func(io.Writer) error) ([]byte, error) {
		_ = writeInput(io.Discard)
		return []byte("dir content\n"), nil
	}

	m.dir = &cachedNode{
		path:  "src",
		files: []*gitdiff.File{{NewName: "a.go"}},
	}
	key := cacheKey("src", false)
	m.cache[key] = m.dir

	cmd := m.diff()
	if cmd == nil {
		t.Fatal("expected cmd for dir diff")
	}
	msg := cmd()

	dcm, ok := msg.(diffContentMsg)
	if !ok {
		t.Fatalf("expected diffContentMsg, got %T", msg)
	}
	plain := ansi.Strip(dcm.text)
	if strings.Contains(plain, "commit abc123") {
		t.Fatalf("expected no preamble in non-root dir diff, got: %q", plain)
	}
}

// contentWidth().
func TestContentWidth(t *testing.T) {
	m := New(false, "dark")
	m.Common.Width = 80
	if m.contentWidth() != 80-scrollbarWidth {
		t.Fatalf("expected contentWidth=%d, got %d", 80-scrollbarWidth, m.contentWidth())
	}
}

// rememberScroll() and restoreScroll().
func TestRememberAndRestoreScroll(t *testing.T) {
	m := scrollTestModel(10)
	a := cacheReadyFile(m, "a.go", 100)
	m, _ = m.SetFilePatch(a)

	m.vp.ScrollDown(25)
	m.rememberScroll()

	key := cacheKey("a.go", false)
	if m.scrollOffsets[key] != 25 {
		t.Fatalf("expected scroll offset 25, got %d", m.scrollOffsets[key])
	}

	m.vp.GotoTop()
	if m.YOffset() != 0 {
		t.Fatalf("expected YOffset=0 after GotoTop, got %d", m.YOffset())
	}

	m.restoreScroll(key)
	if m.YOffset() != 25 {
		t.Fatalf("expected YOffset=25 after restoreScroll, got %d", m.YOffset())
	}
}

// isSBSContentLine() - various cases.
func TestIsSBSContentLine(t *testing.T) {
	m := New(true, "dark")

	// No file/dir set
	if m.isSBSContentLine(0) {
		t.Fatalf("expected false with no content")
	}

	// Negative line
	m.file = &cachedNode{path: "test", diff: "│ some content\nplain line"}
	if m.isSBSContentLine(-1) {
		t.Fatalf("expected false for negative line")
	}

	// SBS content line (starts with │)
	if !m.isSBSContentLine(0) {
		t.Fatalf("expected true for line starting with │")
	}

	// Plain line (doesn't start with │)
	if m.isSBSContentLine(1) {
		t.Fatalf("expected false for line not starting with │")
	}

	// Line out of range
	if m.isSBSContentLine(5) {
		t.Fatalf("expected false for line out of range")
	}

	// Empty diff
	m.file = &cachedNode{path: "test", diff: ""}
	if m.isSBSContentLine(0) {
		t.Fatalf("expected false for empty diff")
	}

	// Dir content
	m.file = nil
	m.dir = &cachedNode{path: "src", diff: "│ dir content\nplain"}
	if !m.isSBSContentLine(0) {
		t.Fatalf("expected true for dir SBS line")
	}
	if m.isSBSContentLine(1) {
		t.Fatalf("expected false for dir plain line")
	}
}

// SetSideBySide() updates the flag.
func TestSetSideBySide(t *testing.T) {
	m := New(false, "dark")
	if m.sideBySide {
		t.Fatalf("expected sideBySide=false initially")
	}

	cmd := m.SetSideBySide(true)
	if !m.sideBySide {
		t.Fatalf("expected sideBySide=true after SetSideBySide(true)")
	}
	if cmd != nil {
		t.Fatalf("expected nil cmd when no file/dir, got %v", cmd)
	}
}

// startRender() increments renderID and sets cancelRender.
func TestStartRender(t *testing.T) {
	m := New(false, "dark")
	m.Common.Width = 80

	initialID := m.renderID
	ctx, newID := m.startRender()
	if newID <= initialID {
		t.Fatalf("expected renderID to increment, got %d -> %d", initialID, newID)
	}
	if m.cancelRender == nil {
		t.Fatalf("expected cancelRender to be set after startRender")
	}

	// Clean up
	m.cancelRender()
	<-ctx.Done() // verify ctx is cancellable
}

// defaultDeltaRenderer() has non-nil run.
func TestDefaultDeltaRenderer(t *testing.T) {
	r := defaultDeltaRenderer()
	if r.run == nil {
		t.Fatalf("expected default renderer to have non-nil run")
	}
}

// StartSelection() - column clamping when Col < lo.
func TestStartSelection_ColClampBelowLo(t *testing.T) {
	m := New(true, "dark")
	m.gutterCol = 40
	m.leftContentCol = 6
	m.rightContentCol = 46
	m.file = &cachedNode{
		path: "test",
		diff: "│  1 │left content                │  1 │right content",
	}

	m.StartSelection(Point{Line: 0, Col: 2})
	if m.sel.anchor.Col != 6 {
		t.Fatalf("expected anchor.Col=6 after clamp, got %d", m.sel.anchor.Col)
	}
}

// StartSelection() - click on leading border column snaps to leftContentCol.
func TestStartSelection_LeadingBorderSnap(t *testing.T) {
	m := New(true, "dark")
	m.gutterCol = 40
	m.leftContentCol = 6
	m.rightContentCol = 46
	m.file = &cachedNode{
		path: "test",
		diff: "│  1 │left content                │  1 │right content",
	}

	m.StartSelection(Point{Line: 0, Col: 0})
	if m.sel.anchor.Col != 6 {
		t.Fatalf("expected anchor.Col=6 after clamp from col 0, got %d", m.sel.anchor.Col)
	}
}

// StartSelection() - SBS right side click.
func TestStartSelection_SBSRightSide(t *testing.T) {
	m := New(true, "dark")
	m.gutterCol = 40
	m.leftContentCol = 6
	m.rightContentCol = 46
	m.file = &cachedNode{
		path: "test",
		diff: "│  1 │left content                │  1 │right content",
	}

	m.StartSelection(Point{Line: 0, Col: 50})
	if m.sel.colBand[0] <= m.gutterCol {
		t.Fatalf("expected band to start after gutter for right side click")
	}
}

// StartSelection() - unified mode (gutterCol <= 0) uses unconstrained band.
func TestStartSelection_UnifiedModeNoClamp(t *testing.T) {
	m := New(false, "dark")
	m.vp.SetWidth(80)
	m.vp.SetHeight(5)

	m.StartSelection(Point{Line: 0, Col: 10})
	if m.sel.colBand[0] != 0 || m.sel.colBand[1] != math.MaxInt {
		t.Fatalf("expected unconstrained band [0, MaxInt), got %v", m.sel.colBand)
	}
	if m.sel.anchor.Col != 10 {
		t.Fatalf("expected anchor.Col=10 (no clamp), got %d", m.sel.anchor.Col)
	}
}

// StartSelection() - click on gutter snaps to left side.
func TestStartSelection_ClickOnGutter(t *testing.T) {
	m := New(true, "dark")
	m.gutterCol = 40
	m.leftContentCol = 6
	m.rightContentCol = 46
	m.file = &cachedNode{
		path: "test",
		diff: "│  1 │left content                │  1 │right content",
	}

	m.StartSelection(Point{Line: 0, Col: 40}) // exactly on gutter
	// Should be treated as left side (p.Col not > gutterCol)
	if m.sel.colBand[1] != 40 {
		t.Fatalf(
			"expected right edge of band = gutterCol=40 for left-side snap, got %d",
			m.sel.colBand[1],
		)
	}
}

// DiffFile with a successful render returns diffContentMsg.
func TestDiffFile_SuccessfulRender(t *testing.T) {
	r := deltaRenderer{
		run: func(ctx context.Context, args []string, writeInput func(io.Writer) error) ([]byte, error) {
			_ = writeInput(io.Discard)
			return []byte("rendered diff"), nil
		},
	}

	node := &cachedNode{
		path:  "test.go",
		files: []*gitdiff.File{{NewName: "test.go"}},
	}

	cmd := diffFile(context.Background(), r, node, 80, false, nil, 1)
	msg := cmd()

	dcm, ok := msg.(diffContentMsg)
	if !ok {
		t.Fatalf("expected diffContentMsg, got %T", msg)
	}
	if dcm.text != "rendered diff" {
		t.Fatalf("expected text 'rendered diff', got %q", dcm.text)
	}
}

// DiffDir with a successful render returns diffContentMsg.
func TestDiffDir_SuccessfulRender(t *testing.T) {
	r := deltaRenderer{
		run: func(ctx context.Context, args []string, writeInput func(io.Writer) error) ([]byte, error) {
			_ = writeInput(io.Discard)
			return []byte("dir rendered"), nil
		},
	}

	node := &cachedNode{
		path:  "src",
		files: []*gitdiff.File{{NewName: "a.go"}},
	}

	cmd := diffDir(context.Background(), r, node, 80, false, nil, lipgloss.Color("8"), "", 1)
	msg := cmd()

	dcm, ok := msg.(diffContentMsg)
	if !ok {
		t.Fatalf("expected diffContentMsg, got %T", msg)
	}
	if dcm.text != "dir rendered" {
		t.Fatalf("expected text 'dir rendered', got %q", dcm.text)
	}
}

// DiffDir with renderer error returns ErrMsg.
func TestDiffDir_RendererError(t *testing.T) {
	r := deltaRenderer{
		run: func(ctx context.Context, args []string, writeInput func(io.Writer) error) ([]byte, error) {
			_ = writeInput(io.Discard)
			return nil, fmt.Errorf("delta crashed")
		},
	}

	node := &cachedNode{
		path:  "src",
		files: []*gitdiff.File{{NewName: "a.go"}},
	}

	cmd := diffDir(context.Background(), r, node, 80, false, nil, lipgloss.Color("8"), "", 1)
	msg := cmd()

	errMsg, ok := msg.(common.ErrMsg)
	if !ok {
		t.Fatalf("expected ErrMsg, got %T", msg)
	}
	if errMsg.Err == nil {
		t.Fatalf("expected non-nil error in ErrMsg")
	}
}

// GutterCol() accessor returns the current gutter column.
func TestGutterCol(t *testing.T) {
	m := New(false, "auto")
	if m.GutterCol() != -1 {
		t.Fatalf("expected -1 for unified mode, got %d", m.GutterCol())
	}

	m.gutterCol = 40
	if m.GutterCol() != 40 {
		t.Fatalf("expected 40 after set, got %d", m.GutterCol())
	}
}

// deltaRenderer.Run with nil run function falls back to runDelta.
func TestDeltaRenderer_RunWithNilRun(t *testing.T) {
	r := deltaRenderer{run: nil}
	// Use a canceled context so runDelta returns quickly without
	// depending on the external delta binary.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := r.Run(ctx, []string{}, func(w io.Writer) error { return nil })
	if err == nil {
		t.Fatal("expected error with canceled context")
	}
}

// diffDir: direct call with preamble != "" includes preamble in output.
func TestDiffDir_PreambleInClosure(t *testing.T) {
	r := deltaRenderer{
		run: func(ctx context.Context, args []string, writeInput func(io.Writer) error) ([]byte, error) {
			_ = writeInput(io.Discard)
			return []byte("dir content"), nil
		},
	}
	node := &cachedNode{
		path:  "/",
		files: []*gitdiff.File{{NewName: "a.go"}},
	}
	cmd := diffDir(
		context.Background(),
		r,
		node,
		80,
		false,
		nil,
		lipgloss.Color("8"),
		"commit abc123\nAuthor: Test",
		1,
	)
	msg := cmd()
	dcm, ok := msg.(diffContentMsg)
	if !ok {
		t.Fatalf("expected diffContentMsg, got %T", msg)
	}
	plain := ansi.Strip(dcm.text)
	if !strings.Contains(plain, "commit abc123") {
		t.Fatalf("expected preamble in direct diffDir output, got: %q", plain)
	}
}

// diffDir: writeInput error — when the renderer's writeInput callback
// receives a failing writer, the writeInput closure inside diffDir returns
// the error, covering the io.WriteString error path.
func TestDiffDir_WriteInputError(t *testing.T) {
	r := deltaRenderer{
		run: func(ctx context.Context, args []string, writeInput func(io.Writer) error) ([]byte, error) {
			err := writeInput(&failingWriter{})
			if err != nil {
				return nil, err
			}
			return []byte("output"), nil
		},
	}

	node := &cachedNode{
		path:  "src",
		files: []*gitdiff.File{{NewName: "a.go"}},
	}
	cmd := diffDir(context.Background(), r, node, 80, false, nil, lipgloss.Color("8"), "", 1)
	msg := cmd()
	errMsg, ok := msg.(common.ErrMsg)
	if !ok {
		t.Fatalf("expected ErrMsg, got %T", msg)
	}
	if errMsg.Err == nil {
		t.Fatal("expected non-nil error")
	}
}

// failingWriter is an io.Writer that always returns an error.
type failingWriter struct{}

func (f *failingWriter) Write(p []byte) (int, error) { return 0, fmt.Errorf("pipe closed") }

// diffDir: sideBySide=true passes the flag through args.
func TestDiffDir_SideBySideFlag(t *testing.T) {
	var capturedArgs []string
	r := deltaRenderer{
		run: func(ctx context.Context, args []string, writeInput func(io.Writer) error) ([]byte, error) {
			capturedArgs = args
			_ = writeInput(io.Discard)
			return []byte("sbs dir"), nil
		},
	}
	node := &cachedNode{
		path:  "src",
		files: []*gitdiff.File{{NewName: "a.go"}},
	}
	cmd := diffDir(context.Background(), r, node, 80, true, nil, lipgloss.Color("8"), "", 1)
	msg := cmd()
	dcm, ok := msg.(diffContentMsg)
	if !ok {
		t.Fatalf("expected diffContentMsg, got %T", msg)
	}
	if !containsArg(capturedArgs, "--side-by-side") {
		t.Fatalf("expected --side-by-side in args for SBS diffDir, got %v", capturedArgs)
	}
	_ = dcm
}

// diffFile: write input error path covers io.WriteString returning an error.
func TestDiffFile_WriteInputError(t *testing.T) {
	r := deltaRenderer{
		run: func(ctx context.Context, args []string, writeInput func(io.Writer) error) ([]byte, error) {
			err := writeInput(&failingWriter{})
			if err != nil {
				return nil, err
			}
			return []byte("output"), nil
		},
	}

	newFile := &gitdiff.File{NewName: "test.go"}
	node := &cachedNode{
		path:  "test.go",
		files: []*gitdiff.File{newFile},
	}
	cmd := diffFile(context.Background(), r, node, 80, false, nil, 1)
	msg := cmd()
	errMsg, ok := msg.(common.ErrMsg)
	if !ok {
		t.Fatalf("expected ErrMsg, got %T", msg)
	}
	if errMsg.Err == nil {
		t.Fatal("expected non-nil error")
	}
}

// ExtendSelection: when p.Col >= hi, it is clamped to hi-1.
func TestExtendSelection_ColClampedToHiMinusOne(t *testing.T) {
	m := New(false, "auto")
	m.sel = selection{
		anchor:  Point{Line: 0, Col: 5},
		colBand: [2]int{0, 10},
		active:  true,
	}
	m.ExtendSelection(Point{Line: 0, Col: 100})
	if m.sel.head.Col != 9 {
		t.Fatalf("expected head.Col=9 (hi-1), got %d", m.sel.head.Col)
	}
}

// ExtendSelection: no-op when not active.
func TestExtendSelection_NoOpWhenNotActive(t *testing.T) {
	m := New(false, "auto")
	m.sel = selection{
		anchor:  Point{Line: 0, Col: 5},
		colBand: [2]int{0, 10},
		active:  false,
	}
	m.ExtendSelection(Point{Line: 0, Col: 50})
	if m.sel.head.Col != 0 {
		t.Fatalf("expected head.Col unchanged at 0 when not active, got %d", m.sel.head.Col)
	}
}

// EndSelection: not active returns empty.
func TestEndSelection_NotActive(t *testing.T) {
	m := New(false, "auto")
	text, ok := m.EndSelection()
	if ok || text != "" {
		t.Fatalf("expected (\"\", false) when not active, got (%q, %v)", text, ok)
	}
}

// runDelta: context canceled after process starts covers the ctxErr path.
// We cancel the context while the process is running so Wait() returns a
// context error, exercising the ctxErr branch.
func TestRunDelta_ContextCanceledMidRun(t *testing.T) {
	origNewCmd := newDeltaCmd
	defer func() { newDeltaCmd = origNewCmd }()

	newDeltaCmd = func(ctx context.Context, args []string) *exec.Cmd {
		// Use a long-running command that will be killed by context cancellation.
		return exec.CommandContext(ctx, "sleep", "10")
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	_, err := runDelta(ctx, []string{}, func(w io.Writer) error {
		time.Sleep(200 * time.Millisecond)
		return nil
	})
	if err == nil {
		t.Fatal("expected error when context is canceled mid-run")
	}
	if ctx.Err() == nil {
		t.Fatal("expected context to be canceled")
	}
}

// runDelta: bad args cause the process to exit with an error and stderr,
// covering the waitErr != nil and stderr branches.
func TestRunDelta_BadArgsError(t *testing.T) {
	origNewCmd := newDeltaCmd
	defer func() { newDeltaCmd = origNewCmd }()

	newDeltaCmd = func(ctx context.Context, args []string) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", "echo 'error: invalid flag' >&2; exit 1")
	}
	_, err := runDelta(
		context.Background(),
		[]string{},
		func(w io.Writer) error {
			return nil
		},
	)
	if err == nil {
		t.Fatal("expected error with process failure")
	}
	if !strings.Contains(err.Error(), "invalid flag") {
		t.Fatalf("expected stderr message in error, got %v", err)
	}
}

// runDelta: StdinPipe error path — injected via stdinPipeFunc var.
func TestRunDelta_StdinPipeError(t *testing.T) {
	origStdinPipe := stdinPipeFunc
	defer func() { stdinPipeFunc = origStdinPipe }()

	stdinPipeFunc = func(c *exec.Cmd) (io.WriteCloser, error) {
		return nil, fmt.Errorf("mock stdin pipe error")
	}

	_, err := runDelta(context.Background(), []string{}, func(w io.Writer) error {
		return nil
	})
	if err == nil {
		t.Fatal("expected error from StdinPipe failure")
	}
	if !strings.Contains(err.Error(), "mock stdin pipe error") {
		t.Fatalf("expected mock error, got %v", err)
	}
}

// runDelta: waitErr != nil with empty stderr — a process that exits non-zero
// but writes nothing to stderr. Injected via newDeltaCmd var.
func TestRunDelta_WaitErrNoStderr(t *testing.T) {
	origNewCmd := newDeltaCmd
	defer func() { newDeltaCmd = origNewCmd }()

	newDeltaCmd = func(ctx context.Context, args []string) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", "exit 1")
	}

	_, err := runDelta(context.Background(), []string{}, func(w io.Writer) error {
		return nil
	})
	if err == nil {
		t.Fatal("expected error from process exit code 1")
	}
}

// runDelta: stdin pipe write error from writeInput.
func TestRunDelta_StdinWriteError(t *testing.T) {
	origNewCmd := newDeltaCmd
	defer func() { newDeltaCmd = origNewCmd }()
	newDeltaCmd = func(ctx context.Context, args []string) *exec.Cmd {
		return exec.CommandContext(ctx, "cat")
	}
	_, err := runDelta(context.Background(), []string{}, func(w io.Writer) error {
		return fmt.Errorf("stdin write failed")
	})
	if err == nil {
		t.Fatal("expected error when writeInput fails")
	}
}

// runDelta: successful invocation produces output.
func TestRunDelta_Success(t *testing.T) {
	origNewCmd := newDeltaCmd
	defer func() { newDeltaCmd = origNewCmd }()
	newDeltaCmd = func(ctx context.Context, args []string) *exec.Cmd {
		return exec.CommandContext(ctx, "cat")
	}
	out, err := runDelta(context.Background(), []string{}, func(w io.Writer) error {
		_, err := io.WriteString(w, "diff --git a/f b/f\nnew file mode 100644\n")
		return err
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("expected non-empty output from command")
	}
}

// runDelta: writeInput returns an error but the process exits successfully
// (waitErr == nil). This exercises the stdinErr != nil && waitErr == nil branch
// that returns stdinErr.
func TestRunDelta_WriteInputErrorProcessSucceeds(t *testing.T) {
	// runDelta: writeInput returns an error but the process exits successfully
	// (waitErr == nil). This exercises the stdinErr != nil && waitErr == nil branch.
	origNewCmd := newDeltaCmd
	defer func() { newDeltaCmd = origNewCmd }()

	newDeltaCmd = func(ctx context.Context, args []string) *exec.Cmd {
		// A command that ignores stdin and exits successfully after a short delay,
		// ensuring the writeInput goroutine runs before Wait() returns.
		return exec.CommandContext(ctx, "sleep", "0.5")
	}

	_, err := runDelta(context.Background(), []string{}, func(w io.Writer) error {
		return fmt.Errorf("stdin write failed")
	})
	if err == nil {
		t.Fatal("expected stdinErr to be returned")
	}
	if !strings.Contains(err.Error(), "stdin write failed") {
		t.Fatalf("expected stdin write error, got %v", err)
	}
}

// runDelta: writeInput returning an error when the process also fails covers
// the waitErr != nil precedence over stdinErr.
func TestRunDelta_WriteInputError(t *testing.T) {
	origNewCmd := newDeltaCmd
	defer func() { newDeltaCmd = origNewCmd }()

	newDeltaCmd = func(ctx context.Context, args []string) *exec.Cmd {
		// A command that fails with stderr when stdin is incomplete.
		return exec.CommandContext(ctx, "sh", "-c", "cat; exit 1")
	}

	out, err := runDelta(context.Background(), []string{}, func(w io.Writer) error {
		return fmt.Errorf("stdin write failed")
	})
	if err == nil {
		t.Fatal("expected error when writeInput fails")
	}
	if len(out) != 0 {
		t.Fatalf("expected empty output when writeInput fails, got %d bytes", len(out))
	}
}

// applyHighlight: selection entirely within a visible viewport with
// a >= b after clamp produces no reverse-video for that line.
func TestApplyHighlight_ClampedToEmptyRange(t *testing.T) {
	m := New(false, "dark")
	m.vp.SetWidth(40)
	m.vp.SetHeight(5)
	m.vp.SetContent("short\nanother short\nthird")

	// Selection on line 0 with a large start col and small end col that,
	// after clampToLine, produces a >= b.
	m.sel = selection{
		anchor:  Point{Line: 0, Col: 0},
		head:    Point{Line: 0, Col: 2},
		colBand: [2]int{100, 200}, // band starts at col 100, clamps both a,b to 100,100
		active:  true,
		has:     false,
	}

	vpView := m.vp.View()
	out := m.applyHighlight(vpView)
	// With a >= b, no reverse-video should be applied, so
	// the output should be identical to the input.
	if out != vpView {
		t.Fatal("expected output to equal input when highlight range is clamped to empty")
	}
}

// selectedText: valid selection on a single line returns the expected text.
func TestSelectedText_ValidSingleLineSelection(t *testing.T) {
	m := New(false, "dark")
	m.file = &cachedNode{path: "test", diff: "normal content"}
	m.sel = selection{
		anchor:  Point{Line: 0, Col: 0},
		head:    Point{Line: 0, Col: 5},
		colBand: [2]int{0, 100},
		active:  false,
		has:     true,
	}

	text := m.selectedText()
	if text != "norma" {
		t.Fatalf("expected selected text %q, got %q", "norma", text)
	}
}

// ---------------------------------------------------------------------------
// HasFile / HasDir / CurrentFilePath
// -----------

func TestHasFile_CurrentFilePath(t *testing.T) {
	m := New(false, "dark")
	if m.HasFile() {
		t.Error("expected HasFile=false with no file set")
	}
	if m.HasDir() {
		t.Error("expected HasDir=false with no dir set")
	}
	if m.CurrentFilePath() != "" {
		t.Errorf("expected empty CurrentFilePath with no file, got %q", m.CurrentFilePath())
	}

	m.file = &cachedNode{path: "main.go", files: []*gitdiff.File{{NewName: "main.go"}}}
	if !m.HasFile() {
		t.Error("expected HasFile=true after setting file")
	}
	if m.CurrentFilePath() != "main.go" {
		t.Errorf("expected CurrentFilePath=main.go, got %q", m.CurrentFilePath())
	}
}

func TestHasDir(t *testing.T) {
	m := New(false, "dark")
	m.dir = &cachedNode{path: "src", additions: 5, deletions: 1}
	if !m.HasDir() {
		t.Error("expected HasDir=true after setting dir")
	}
	if m.HasFile() {
		t.Error("expected HasFile=false when dir is set")
	}
}

// ---------------------------------------------------------------------------
// Init
// ---------------------------------------------------------------------------

func TestInitReturnsNil(t *testing.T) {
	m := New(false, "dark")
	if cmd := m.Init(); cmd != nil {
		t.Fatalf("expected Init to return nil, got %v", cmd)
	}
}

// ---------------------------------------------------------------------------
// Robustness: View still renders when internal functions panic
// ---------------------------------------------------------------------------

func TestViewStillRendersWhenColumnDetectionPanics(t *testing.T) {
	orig := detectGutterColFunc
	detectGutterColFunc = func(string, int) int { panic("gutter detection failed") }
	defer func() { detectGutterColFunc = orig }()

	m := New(true, "dark")
	m.Common.Width = 80
	m.Common.Height = 20
	m.SetSize(80, 20)

	key := cacheKey("/", true)
	m.dir = &cachedNode{path: "/"}
	m.cache[key] = m.dir
	m.renderID = 1

	m, _ = m.Update(diffContentMsg{
		cacheKey: key,
		text:     "some diff content\nmore lines",
		renderID: 1,
	})

	// View must still render without crashing.
	view := m.View()
	if view == "" {
		t.Fatal("expected non-empty view after column detection panic")
	}
}

func TestViewStillRendersWhenSideContentColsPanics(t *testing.T) {
	orig := detectSideContentColsFunc
	detectSideContentColsFunc = func(string, int) (int, int) {
		panic("side content cols detection failed")
	}
	defer func() { detectSideContentColsFunc = orig }()

	m := New(true, "dark")
	m.Common.Width = 80
	m.Common.Height = 20
	m.SetSize(80, 20)

	key := cacheKey("/", true)
	m.dir = &cachedNode{path: "/"}
	m.cache[key] = m.dir
	m.renderID = 1

	m, _ = m.Update(diffContentMsg{
		cacheKey: key,
		text:     "some diff content\nmore lines",
		renderID: 1,
	})

	view := m.View()
	if view == "" {
		t.Fatal("expected non-empty view after side-content-cols panic")
	}
}

func TestViewStillRendersWhenHighlightPanics(t *testing.T) {
	orig := spliceReverseFunc
	spliceReverseFunc = func(string, int, int, int) string {
		panic("spliceReverse failed")
	}
	defer func() { spliceReverseFunc = orig }()

	m := New(false, "dark")
	m.Common.Width = 80
	m.Common.Height = 20
	m.SetSize(80, 20)
	m.SetContent("line0 content\nline1 content\nline2 content")

	// Start a selection through the public API to trigger applyHighlight
	// during View().
	m.StartSelection(Point{Line: 0, Col: 0})
	m.ExtendSelection(Point{Line: 1, Col: 5})

	view := m.View()
	if view == "" {
		t.Fatal("expected non-empty view even when highlight panics")
	}
	// The view should still contain the original content (just unhighlighted).
	plain := ansi.Strip(view)
	if !strings.Contains(plain, "line0") {
		t.Fatal("expected view to contain original content after highlight panic")
	}
}

// ---------------------------------------------------------------------------
// Narrow viewport: column detection fallback
// ---------------------------------------------------------------------------

func TestNarrowViewportFallsBackToUnifiedSelection(t *testing.T) {
	// Force detectGutterCol to return 0 so detectSideContentCols
	// hits its gutterCol <= 0 early return.
	orig := detectGutterColFunc
	detectGutterColFunc = func(string, int) int { return 0 }
	defer func() { detectGutterColFunc = orig }()

	m := New(true, "dark")
	m.Common.Width = 80
	m.Common.Height = 20
	m.SetSize(80, 20)

	key := cacheKey("/", true)
	m.dir = &cachedNode{path: "/"}
	m.cache[key] = m.dir
	m.renderID = 1

	m, _ = m.Update(diffContentMsg{
		cacheKey: key,
		text:     "some content here",
		renderID: 1,
	})

	// View must render; selection must work with unified band.
	view := m.View()
	if view == "" {
		t.Fatal("expected non-empty view when gutter detection fails")
	}

	// Starting a selection should not panic even with no detected columns.
	m.StartSelection(Point{Line: 0, Col: 0})
	m.ExtendSelection(Point{Line: 0, Col: 0})
}

// ---------------------------------------------------------------------------
// Column detection on cached SBS content via SetFilePatch/SetDirPatch
// ---------------------------------------------------------------------------

func TestSetFilePatchCachedSBS(t *testing.T) {
	m := New(true, "dark")
	m.Common.Width = 80
	m.Common.Height = 20
	m.SetSize(80, 20)

	pad := func(n int) string { return strings.Repeat(" ", n) }
	sbsLine := "│" + "  1 " + "│" + pad(24) + "│" + "  1 " + "│" + pad(24)
	sbsContent := strings.Repeat(sbsLine+"\n", 4)

	f := &gitdiff.File{NewName: "test.go"}
	key := cacheKey("test.go", true)
	m.cache[key] = &cachedNode{
		path:  "test.go",
		files: []*gitdiff.File{f},
		diff:  sbsContent,
		ready: true,
	}

	m2, cmd := m.SetFilePatch(f)
	if cmd != nil {
		t.Fatalf("expected nil cmd for cached hit, got %v", cmd)
	}

	// Selection should be scoped to the left side when clicking left of gutter.
	m2.StartSelection(Point{Line: 0, Col: 6})
	m2.ExtendSelection(Point{Line: 1, Col: 6})

	text, ok := m2.EndSelection()
	if !ok || text == "" {
		t.Fatal("expected selected text from cached SBS file")
	}
}

func TestSetDirPatchCachedSBS(t *testing.T) {
	m := New(true, "dark")
	m.Common.Width = 80
	m.Common.Height = 20
	m.SetSize(80, 20)

	pad := func(n int) string { return strings.Repeat(" ", n) }
	sbsLine := "│" + "  1 " + "│" + pad(24) + "│" + "  1 " + "│" + pad(24)
	sbsContent := strings.Repeat(sbsLine+"\n", 4)

	f := &gitdiff.File{NewName: "a.go"}
	key := cacheKey("src", true)
	m.cache[key] = &cachedNode{
		path:  "src",
		files: []*gitdiff.File{f},
		diff:  sbsContent,
		ready: true,
	}

	m2, cmd := m.SetDirPatch("src", []*gitdiff.File{f})
	if cmd != nil {
		t.Fatalf("expected nil cmd for cached hit, got %v", cmd)
	}

	// View must render the cached content.
	view := m2.View()
	if view == "" {
		t.Fatal("expected non-empty view for cached SBS directory")
	}
}

// ---------------------------------------------------------------------------
// Update with non-diffContentMsg
// ---------------------------------------------------------------------------

func TestUpdatePropagatesKeyToViewport(t *testing.T) {
	m := New(false, "dark")
	m.Common.Width = 60
	m.Common.Height = 10
	m.SetSize(60, 10)
	m.SetContent("hello\nworld\nmore")

	m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))

	// The viewport should have scrolled; the view must still render.
	view := m.View()
	if view == "" {
		t.Fatal("expected non-empty view after key down")
	}
}

// DiffFile with renderer error returns ErrMsg.
func TestDiffFile_RendererError(t *testing.T) {
	r := deltaRenderer{
		run: func(ctx context.Context, args []string, writeInput func(io.Writer) error) ([]byte, error) {
			_ = writeInput(io.Discard)
			return nil, fmt.Errorf("delta crashed")
		},
	}

	node := &cachedNode{
		path:  "test.go",
		files: []*gitdiff.File{{NewName: "test.go"}},
	}

	cmd := diffFile(context.Background(), r, node, 80, false, nil, 1)
	msg := cmd()

	errMsg, ok := msg.(common.ErrMsg)
	if !ok {
		t.Fatalf("expected ErrMsg, got %T", msg)
	}
	if errMsg.Err == nil {
		t.Fatalf("expected non-nil error in ErrMsg")
	}
}
