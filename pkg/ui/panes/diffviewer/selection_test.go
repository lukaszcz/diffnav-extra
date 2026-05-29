package diffviewer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/bluekeyes/go-gitdiff/gitdiff"
	"github.com/charmbracelet/x/ansi"
)

// (1) Single-line selection: ansi.Strip of the spliced output contains the
// expected substring.
func TestSpliceReverse_SingleLineContainsSubstring(t *testing.T) {
	line := "hello world"
	w := lipgloss.Width(line)

	out := spliceReverse(line, 6, 11, w)

	plain := ansi.Strip(out)
	if !strings.Contains(plain, "world") {
		t.Fatalf("expected stripped output to contain %q, got %q", "world", plain)
	}
	if !strings.Contains(out, "\x1b[7m") || !strings.Contains(out, "\x1b[27m") {
		t.Fatalf("expected SGR 7 / 27 in output, got %q", out)
	}
}

// (2) Multi-line selection: highlightRange + clampToBand produce correct
// ranges for first / interior / last lines.
func TestHighlightRange_MultiLineWithBandClamp(t *testing.T) {
	start := Point{Line: 2, Col: 5}
	end := Point{Line: 4, Col: 8}
	lineWidth := 40
	band := [2]int{0, 30}

	// First line: from start.Col to lineWidth.
	a, b := highlightRange(2, start, end, lineWidth)
	if a != 5 || b != 40 {
		t.Fatalf("first line pre-clamp: want (5, 40), got (%d, %d)", a, b)
	}
	a, b = clampToBand(a, b, band)
	if a != 5 || b != 30 {
		t.Fatalf("first line clamped: want (5, 30), got (%d, %d)", a, b)
	}

	// Interior line: [0, lineWidth).
	a, b = highlightRange(3, start, end, lineWidth)
	if a != 0 || b != 40 {
		t.Fatalf("interior pre-clamp: want (0, 40), got (%d, %d)", a, b)
	}
	a, b = clampToBand(a, b, band)
	if a != 0 || b != 30 {
		t.Fatalf("interior clamped: want (0, 30), got (%d, %d)", a, b)
	}

	// Last line: [0, end.Col).
	a, b = highlightRange(4, start, end, lineWidth)
	if a != 0 || b != 8 {
		t.Fatalf("last line pre-clamp: want (0, 8), got (%d, %d)", a, b)
	}
	a, b = clampToBand(a, b, band)
	if a != 0 || b != 8 {
		t.Fatalf("last line clamped: want (0, 8), got (%d, %d)", a, b)
	}
}

// (2 cont.) Single-line selection through highlightRange.
func TestHighlightRange_SingleLine(t *testing.T) {
	start := Point{Line: 5, Col: 3}
	end := Point{Line: 5, Col: 12}

	a, b := highlightRange(5, start, end, 80)
	if a != 3 || b != 12 {
		t.Fatalf("single line: want (3, 12), got (%d, %d)", a, b)
	}
}

// (3) Reverse drag (anchor below head) normalizes correctly.
func TestSelection_NormalizedReverseDrag(t *testing.T) {
	s := selection{
		anchor: Point{Line: 7, Col: 4},
		head:   Point{Line: 2, Col: 10},
	}
	start, end := s.normalized()
	if start != (Point{Line: 2, Col: 10}) {
		t.Fatalf("start: want {2 10}, got %+v", start)
	}
	if end != (Point{Line: 7, Col: 4}) {
		t.Fatalf("end: want {7 4}, got %+v", end)
	}
}

// (3 cont.) Same-line reverse drag: anchor.Col > head.Col normalizes by col.
func TestSelection_NormalizedSameLineReverse(t *testing.T) {
	s := selection{
		anchor: Point{Line: 3, Col: 20},
		head:   Point{Line: 3, Col: 5},
	}
	start, end := s.normalized()
	if start != (Point{Line: 3, Col: 5}) || end != (Point{Line: 3, Col: 20}) {
		t.Fatalf("normalize same-line reverse: got start=%+v end=%+v", start, end)
	}
}

// (3 cont.) Forward drag is a no-op.
func TestSelection_NormalizedForwardDrag(t *testing.T) {
	s := selection{
		anchor: Point{Line: 1, Col: 1},
		head:   Point{Line: 3, Col: 3},
	}
	start, end := s.normalized()
	if start != s.anchor || end != s.head {
		t.Fatalf("forward drag should be unchanged: got start=%+v end=%+v", start, end)
	}
}

// (6) ANSI round-trip — single line: ansi.Strip(spliceReverse(line, a, b, w))
// == ansi.Strip(line). Catches leaking SGR state.
func TestSpliceReverse_AnsiRoundTripSingleLine(t *testing.T) {
	// Styled line: red "foo", default " bar", green "baz".
	line := "\x1b[31mfoo\x1b[0m bar \x1b[32mbaz\x1b[0m"
	w := lipgloss.Width(line)

	out := spliceReverse(line, 2, 7, w)

	if got, want := ansi.Strip(out), ansi.Strip(line); got != want {
		t.Fatalf("ansi-strip mismatch:\n  got:  %q\n  want: %q", got, want)
	}
}

// (6 cont.) ANSI round-trip — multi-line: for every line in the selected
// range, ansi.Strip(spliced) == ansi.Strip(original). This catches SGR
// state that delta opens on one line and (unusually) doesn't re-open on the
// next — our \x1b[27m reset must not leak across the join.
func TestSpliceReverse_AnsiRoundTripMultiLine(t *testing.T) {
	lines := []string{
		"\x1b[31mfirst line content\x1b[0m",
		"\x1b[32msecond line of stuff\x1b[0m",
		"\x1b[33mthird and final line\x1b[0m",
	}
	start := Point{Line: 0, Col: 6}
	end := Point{Line: 2, Col: 5}
	band := [2]int{0, 1 << 30}

	for i, line := range lines {
		w := lipgloss.Width(line)
		a, b := highlightRange(i, start, end, w)
		a, b = clampToBand(a, b, band)
		a, b = clampToLine(a, b, w)
		spliced := spliceReverse(line, a, b, w)
		if got, want := ansi.Strip(spliced), ansi.Strip(line); got != want {
			t.Fatalf("line %d: ansi-strip mismatch:\n  got:  %q\n  want: %q", i, got, want)
		}
	}
}

// (4) Click-without-drag leaves no selection: StartSelection followed by
// EndSelection with anchor unchanged returns (_, false) and HasSelection
// remains false.
func TestSelection_ClickWithoutDragLeavesNoSelection(t *testing.T) {
	m := New(false, "auto")
	m.StartSelection(Point{Line: 3, Col: 5})
	if !m.IsSelecting() {
		t.Fatalf("expected IsSelecting() == true immediately after StartSelection")
	}

	text, ok := m.EndSelection()
	if ok {
		t.Fatalf("expected ok == false for click-without-drag, got text=%q", text)
	}
	if text != "" {
		t.Fatalf("expected empty text, got %q", text)
	}
	if m.HasSelection() {
		t.Fatalf("expected HasSelection() == false after click-without-drag")
	}
	if m.IsSelecting() {
		t.Fatalf("expected IsSelecting() == false after EndSelection")
	}
}

// (5) Selection survives viewport scroll: the highlighted screen row shifts
// by the scroll delta and the underlying content is unchanged.
func TestSelection_SurvivesViewportScroll(t *testing.T) {
	m := New(false, "auto")
	m.vp.SetWidth(40)
	m.vp.SetHeight(5)

	var b strings.Builder
	for i := 0; i < 20; i++ {
		fmt.Fprintf(&b, "line-%02d-some-content\n", i)
	}
	m.vp.SetContent(strings.TrimSuffix(b.String(), "\n"))

	// Single-line selection on content line 7 covering "line-07".
	m.StartSelection(Point{Line: 7, Col: 0})
	m.ExtendSelection(Point{Line: 7, Col: 7})

	// YOffset=5 → visible content rows 5..9 → line 7 at screen row 2.
	m.ScrollDown(5)
	view1 := m.View()

	// YOffset=6 → visible content rows 6..10 → line 7 at screen row 1.
	m.ScrollDown(1)
	view2 := m.View()

	findHighlight := func(label, s string) (int, string) {
		for i, l := range strings.Split(s, "\n") {
			if strings.Contains(l, "\x1b[7m") {
				return i, l
			}
		}
		t.Fatalf("%s: no highlighted row found in:\n%s", label, s)
		return -1, ""
	}

	row1, line1 := findHighlight("view1", view1)
	row2, line2 := findHighlight("view2", view2)

	if row2 != row1-1 {
		t.Fatalf(
			"expected highlight row to shift up by 1 after ScrollDown(1): row1=%d row2=%d",
			row1,
			row2,
		)
	}
	// Trailing scrollbar character may differ between views (it tracks scroll
	// position); compare only the content portion that came from the viewport.
	if !strings.Contains(ansi.Strip(line1), "line-07") {
		t.Fatalf(
			"view1: expected highlighted row to contain %q, got %q",
			"line-07",
			ansi.Strip(line1),
		)
	}
	if !strings.Contains(ansi.Strip(line2), "line-07") {
		t.Fatalf(
			"view2: expected highlighted row to contain %q, got %q",
			"line-07",
			ansi.Strip(line2),
		)
	}
}

// (7) Loading a new file via SetFilePatch clears any prior finalized
// selection. Guards against stale selection state surviving a content swap.
func TestSelection_SetFilePatchClearsSelection(t *testing.T) {
	m := New(false, "auto")
	m.vp.SetWidth(40)
	m.vp.SetHeight(5)
	m.file = &cachedNode{path: "old.go", diff: "first line\nsecond line\nthird line"}

	m.StartSelection(Point{Line: 0, Col: 0})
	m.ExtendSelection(Point{Line: 1, Col: 5})
	if _, ok := m.EndSelection(); !ok {
		t.Fatalf("setup: expected EndSelection ok=true after a real drag")
	}
	if !m.HasSelection() {
		t.Fatalf("setup: expected HasSelection() == true before SetFilePatch")
	}

	m, _ = m.SetFilePatch(&gitdiff.File{NewName: "new.go"})
	if m.HasSelection() {
		t.Fatalf("expected HasSelection() == false after SetFilePatch")
	}
}

// (8) Column detection on a known delta side-by-side render fixture.
// Expect gutterCol close to width/2 and stable across content lines.
func TestDetectGutterCol_DeltaSideBySideFixture(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "sbs_render_w120.ansi"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	stripped := ansi.Strip(string(raw))

	const width = 120
	got := detectGutterCol(stripped, width)

	target := width / 2
	if absInt(got-target) > 10 {
		t.Fatalf("gutterCol=%d not within ±10 of width/2=%d", got, target)
	}

	// Stability: every content line (≥3 dividers) must place its center
	// '│' at the same column the detector picked.
	contentLines := 0
	for _, line := range strings.Split(stripped, "\n") {
		positions := visualColumnsOf(line, '│')
		if len(positions) < 3 {
			continue
		}
		contentLines++
		nearest := positions[0]
		for _, p := range positions {
			if absInt(p-target) < absInt(nearest-target) {
				nearest = p
			}
		}
		if nearest != got {
			t.Fatalf("line %q: nearest-to-midpoint %d != detected %d", line, nearest, got)
		}
	}
	if contentLines < 2 {
		t.Fatalf("fixture only has %d content lines; detector input is too thin", contentLines)
	}
}

// (14) Wide-character gutter detection. Feed a side-by-side render with CJK
// and emoji to the left of the gutter; assert gutterCol (visual column)
// lands on the actual '│' divider, not on a rune-index that skewed left.
func TestDetectGutterCol_WideCharsToLeftOfGutter(t *testing.T) {
	// Hand-construct lines where the center '│' sits at visual column 30,
	// with CJK / emoji to its left. Each line has ≥3 '│' so it passes the
	// skip-line gate. The runes-to-the-left would skew a rune-index-based
	// detector by 6+ columns; a visual-column detector must see col 30.
	//
	// Layout per line:
	//   '│'  cols 0
	//   '  1 '  cols 1..4
	//   '│'  col 5
	//   CJK/emoji block (each rune width 2) + padding to reach col 30
	//   '│'  col 30  <-- center divider
	//   right-side filler + '│' at the right border
	//
	// We pick width=60 so target=30 aligns with the planted center column.
	pad := func(width int) string { return strings.Repeat(" ", width) }
	// Left side: 24 visual cols of content after the post-LN '│' (col 5),
	// putting the center '│' at col 5+1+24 = 30.
	leftBlocks := []string{
		"日本語テスト" + pad(12),  // 6 CJK * 2 = 12 visual, +12 spaces = 24
		"こんにちは世界" + pad(10), // 7 CJK * 2 = 14 visual, +10 spaces = 24
		"🐶🐱🐭🐹" + pad(16),    // 4 emoji * 2 = 8 visual, +16 spaces = 24
	}
	rightFiller := pad(24)
	lines := make([]string, 0, len(leftBlocks))
	for _, lb := range leftBlocks {
		// "│" + "  1 " + "│" + leftBlock(24) + "│" + rightFiller(24) + "│"
		lines = append(lines, "│"+"  1 "+"│"+lb+"│"+rightFiller+"│")
	}

	// Sanity: every line must have the center '│' at visual col 30.
	for _, l := range lines {
		positions := visualColumnsOf(l, '│')
		if len(positions) < 3 {
			t.Fatalf("line missing dividers (have %d): %q", len(positions), l)
		}
		found := false
		for _, p := range positions {
			if p == 30 {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected a '│' at visual col 30; positions=%v line=%q", positions, l)
		}
	}

	got := detectGutterCol(strings.Join(lines, "\n"), 60)
	if got != 30 {
		t.Fatalf("gutterCol: want 30 (visual col of center '│'), got %d", got)
	}
}

// (15) Tabs in diff content: delta's current config expands tabs to spaces in
// the output we read from m.file.diff. This test is a TRIPWIRE — if a future
// delta version or config option ships raw tabs, the col↔visual-column math
// in the selection layer (visualColumnsOf, ansi.Cut, etc.) needs revisiting
// because a single '\t' rune occupies multiple visual columns and is not
// width-1 like the rest of our coordinate math assumes.
func TestDeltaOutput_NoRawTabs_Tripwire(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "sbs_render_w120.ansi"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	stripped := ansi.Strip(string(raw))
	// The input fixture (sbs_input.diff) contains literal '\t' indentation,
	// so a delta that *didn't* expand tabs would surface them here.
	if strings.ContainsRune(stripped, '\t') {
		t.Fatalf(
			"delta output unexpectedly contains raw '\\t' — tab expansion is off; selection col math needs revisiting",
		)
	}
}

// (16) First-N robustness: a diff whose first 10 content lines mix hunk-
// header decorations (which have <3 '│' and are skipped by the gate) and
// real diff rows still converges on the same gutterCol as a clean run.
func TestDetectGutterCol_HunkHeaderRobustness(t *testing.T) {
	// "Clean" content line: '│' at cols 0, 5, 30, 55.
	pad := func(n int) string { return strings.Repeat(" ", n) }
	clean := "│" + "  1 " + "│" + pad(24) + "│" + pad(24) + "│"
	// Hunk-header / decoration lines: 0 or 1 '│', skipped by the gate.
	headers := []string{
		"───┐",
		"42: │", // 1 '│' — exactly at the gate's lower bound, still skipped.
		"───┘",
		"@@ -1,8 +1,9 @@",
	}

	// Reference: a "clean run" with only content lines.
	cleanRun := strings.Repeat(clean+"\n", 4)
	want := detectGutterCol(cleanRun, 60)
	if want != 30 {
		t.Fatalf("sanity: clean run want gutter at 30, got %d", want)
	}

	// Mixed: 4 header decorations interleaved with 4 content lines, well
	// within the first-10-content-lines window.
	mixed := strings.Join([]string{
		headers[0],
		headers[1],
		clean,
		headers[2],
		clean,
		headers[3],
		clean,
		clean,
	}, "\n")
	got := detectGutterCol(mixed, 60)
	if got != want {
		t.Fatalf("mixed-headers run: got %d, want %d (same as clean run)", got, want)
	}
}

// (9) Column clamp left: with gutterCol=40, a drag starting at col 10
// that extends to col 60 lands head.Col on 39 (gutterCol-1, the last
// column inside the half-open [0, gutterCol) left band).
func TestSelection_ColumnClampLeft(t *testing.T) {
	m := New(true, "auto")
	m.gutterCol = 40
	// SBS-structured line so the clamp applies; without leading '│' the
	// selection now treats the row as a hunk-header / decoration and skips
	// side-specific column clamping.
	m.file = &cachedNode{
		path: "test",
		diff: "│  1 │left content                  │  1 │right content",
	}

	m.StartSelection(Point{Line: 0, Col: 10})
	m.ExtendSelection(Point{Line: 0, Col: 60})

	if m.sel.head.Col != 39 {
		t.Fatalf("expected head.Col == 39, got %d", m.sel.head.Col)
	}
}

// (10) Column clamp right: with gutterCol=40, a drag starting at col 60
// that extends to col 10 lands head.Col on 41 (gutterCol+1, the first
// column inside the right band).
func TestSelection_ColumnClampRight(t *testing.T) {
	m := New(true, "auto")
	m.gutterCol = 40
	m.file = &cachedNode{
		path: "test",
		diff: "│  1 │left content                  │  1 │right content",
	}

	m.StartSelection(Point{Line: 0, Col: 60})
	m.ExtendSelection(Point{Line: 0, Col: 10})

	if m.sel.head.Col != 41 {
		t.Fatalf("expected head.Col == 41, got %d", m.sel.head.Col)
	}
}

// (11) Column-aware text extraction: a multi-line drag in the left column
// yields plaintext that does NOT contain the gutter '│' nor any rune from
// a column >= gutterCol.
func TestSelection_LeftColumnExtractionExcludesGutterAndRight(t *testing.T) {
	m := New(true, "auto")
	m.vp.SetWidth(70)
	m.vp.SetHeight(10)
	m.gutterCol = 30
	m.leftContentCol = 6
	m.rightContentCol = 36

	// Synthetic side-by-side content with realistic layout:
	//   col 0:  '│'         leading border
	//   col 1-4 '  1 '      line number
	//   col 5:  '│'         post-line-number border
	//   col 6+: left content (up to gutter at col 30)
	//   col 30: '│'         center divider
	//   col 31-34 '  1 '    right line number
	//   col 35: '│'         post-line-number border
	//   col 36+: right content tokens
	lines := []string{
		"│  1 │alpha leftcol content   │  1 │BETA-RIGHT-LINE-ZERO  ",
		"│  2 │gamma leftcol content   │  2 │DELTA-RIGHT-LINE-ONE  ",
		"│  3 │epsilon leftcol content │  3 │ZETA-RIGHT-LINE-TWO   ",
	}
	for i, ln := range lines {
		positions := visualColumnsOf(ln, '│')
		if !containsCol(positions, 30) {
			t.Fatalf("line %d: expected '│' at col 30, positions=%v line=%q", i, positions, ln)
		}
	}
	content := strings.Join(lines, "\n")
	m.file = &cachedNode{path: "test", diff: content}

	m.StartSelection(Point{Line: 0, Col: 6})
	m.ExtendSelection(Point{Line: 2, Col: 20})

	text, ok := m.EndSelection()
	if !ok {
		t.Fatalf("expected EndSelection ok=true")
	}
	if strings.ContainsRune(text, '│') {
		t.Fatalf("expected no '│' in extracted text, got %q", text)
	}
	for _, tok := range []string{"BETA", "DELTA", "ZETA", "RIGHT"} {
		if strings.Contains(text, tok) {
			t.Fatalf("expected text to exclude right-column token %q, got %q", tok, text)
		}
	}
}

// (12) Unified mode: gutterCol == -1, no column clamping. A single-line
// drag from col 5 to col 70 yields the whole substring between those
// columns.
func TestSelection_UnifiedModeNoClamping(t *testing.T) {
	m := New(false, "auto")
	if m.gutterCol != -1 {
		t.Fatalf("expected gutterCol == -1 in unified mode, got %d", m.gutterCol)
	}
	m.vp.SetWidth(80)
	m.vp.SetHeight(5)

	// 80-column line of distinct ASCII so we can verify the substring.
	const line = "0123456789ABCDEFGHIJ0123456789ABCDEFGHIJ0123456789ABCDEFGHIJ0123456789ABCDEFGHIJ"
	m.file = &cachedNode{path: "test", diff: line}

	m.StartSelection(Point{Line: 0, Col: 5})
	m.ExtendSelection(Point{Line: 0, Col: 70})

	if m.sel.head.Col != 70 {
		t.Fatalf("expected head.Col == 70 (no clamp), got %d", m.sel.head.Col)
	}
	text, ok := m.EndSelection()
	if !ok {
		t.Fatalf("expected EndSelection ok=true")
	}
	if want := line[5:70]; text != want {
		t.Fatalf("expected text %q, got %q", want, text)
	}
}

// (13) Mode-toggle: after rendering side-by-side (gutterCol > 0), toggle
// to unified, feed a fresh diffContentMsg through Update, then verify
// gutterCol == -1 and that a new selection is not clamped against the
// stale divider. PLAN.md notes that detection only runs in the
// diffContentMsg branch — reading gutterCol immediately after
// SetSideBySide would see the stale value.
func TestSelection_ModeToggleResetsGutterCol(t *testing.T) {
	m := New(true, "dark")
	m.vp.SetWidth(60)
	m.vp.SetHeight(10)

	// Build a synthetic side-by-side content that the detector will lock
	// onto: ≥3 '│' per line with the center divider at col 30.
	pad := func(n int) string { return strings.Repeat(" ", n) }
	sbsLine := "│" + "  1 " + "│" + pad(24) + "│" + pad(24) + "│"
	sbsContent := strings.Repeat(sbsLine+"\n", 4)

	sbsKey := cacheKey("/", true)
	m.dir = &cachedNode{path: "/"}
	m.cache[sbsKey] = m.dir
	m.renderID = 1
	m, _ = m.Update(diffContentMsg{
		cacheKey: sbsKey,
		text:     sbsContent,
		renderID: 1,
	})
	if m.gutterCol <= 0 {
		t.Fatalf("expected gutterCol > 0 after side-by-side render, got %d", m.gutterCol)
	}

	// Sample selection while side-by-side: clamps against the divider.
	m.StartSelection(Point{Line: 0, Col: 10})
	m.ExtendSelection(Point{Line: 0, Col: 200})
	if m.sel.head.Col >= m.gutterCol {
		t.Fatalf(
			"expected head.Col clamped below gutter, got head.Col=%d gutterCol=%d",
			m.sel.head.Col, m.gutterCol,
		)
	}
	m.ClearSelection()

	// Toggle to unified. SetSideBySide flips m.sideBySide and asks diff()
	// for a refresh; if it returns a cmd we execute it, otherwise we
	// simulate the eventual diffContentMsg ourselves. Either way, the
	// gutter reset only happens when a new diffContentMsg lands.
	if cmd := m.SetSideBySide(false); cmd != nil {
		if msg := cmd(); msg != nil {
			m, _ = m.Update(msg)
		}
	}
	unifiedKey := cacheKey("/", false)
	if _, ok := m.cache[unifiedKey]; !ok {
		m.cache[unifiedKey] = &cachedNode{path: "/"}
	}
	m.renderID++
	m, _ = m.Update(diffContentMsg{
		cacheKey: unifiedKey,
		text:     "plain unified content without any divider runes at all here",
		renderID: m.renderID,
	})
	if m.gutterCol != -1 {
		t.Fatalf("expected gutterCol == -1 in unified mode, got %d", m.gutterCol)
	}

	// New selection is not clamped against the previous divider.
	m.StartSelection(Point{Line: 0, Col: 5})
	m.ExtendSelection(Point{Line: 0, Col: 50})
	if m.sel.head.Col != 50 {
		t.Fatalf("expected head.Col == 50 (no clamp), got %d", m.sel.head.Col)
	}
}

// (17) detectSideContentCols on the delta side-by-side fixture: the returned
// (leftContentCol, rightContentCol) must sit just past each side's post-LN
// '│' border, so a selection clamped to them excludes the line-number column.
func TestDetectSideContentCols_DeltaFixture(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "sbs_render_w120.ansi"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	stripped := ansi.Strip(string(raw))
	gutter := detectGutterCol(stripped, 120)

	left, right := detectSideContentCols(stripped, gutter)
	if left <= 0 || left >= gutter {
		t.Fatalf("leftContentCol out of range: got %d, want in (0, %d)", left, gutter)
	}
	if right <= gutter {
		t.Fatalf("rightContentCol must be > gutterCol(%d), got %d", gutter, right)
	}
	// Each detected column must point to a content position whose preceding
	// column carries '│' (i.e. the post-LN border).
	for _, ln := range firstNContentLines(stripped, 5) {
		positions := visualColumnsOf(ln, '│')
		if len(positions) < 4 {
			continue
		}
		if !containsCol(positions, left-1) {
			t.Fatalf("expected '│' at col %d in content line %q (positions=%v)",
				left-1, ln, positions)
		}
		if !containsCol(positions, right-1) {
			t.Fatalf("expected '│' at col %d in content line %q (positions=%v)",
				right-1, ln, positions)
		}
	}
}

func containsCol(positions []int, target int) bool {
	for _, p := range positions {
		if p == target {
			return true
		}
	}
	return false
}

// (18) Side-by-side LEFT selection excludes leading border, line number, and
// post-LN '│' border. The extracted text contains the content tokens and none
// of the structural characters.
func TestSelection_SideBySide_LeftExcludesLineNumbersAndBorders(t *testing.T) {
	m := New(true, "auto")
	m.vp.SetWidth(80)
	m.vp.SetHeight(10)
	// Layout per line: "│" + "  1 " + "│" + LEFT(24) + "│" + "  1 " + "│" + RIGHT(24)
	//   col 0:  '│'
	//   col 5:  '│' (left post-LN)
	//   col 30: '│' (gutter / center divider)
	//   col 35: '│' (right post-LN)
	lines := []string{
		"│" + "  1 " + "│" + "alpha-left-content      " + "│" + "  1 " + "│" + "alpha-right-content     ",
		"│" + "  2 " + "│" + "beta-left-content       " + "│" + "  2 " + "│" + "beta-right-content      ",
		"│" + "  3 " + "│" + "gamma-left-content      " + "│" + "  3 " + "│" + "gamma-right-content     ",
	}
	content := strings.Join(lines, "\n")
	m.file = &cachedNode{path: "test", diff: content}
	m.gutterCol = 30
	m.leftContentCol = 6
	m.rightContentCol = 36

	// Click on the leading-border column; the band should still snap to col 6.
	m.StartSelection(Point{Line: 0, Col: 0})
	m.ExtendSelection(Point{Line: 2, Col: 29})

	text, ok := m.EndSelection()
	if !ok {
		t.Fatalf("expected EndSelection ok=true")
	}
	if strings.ContainsRune(text, '│') {
		t.Fatalf("expected no '│' in extracted text, got %q", text)
	}
	for _, num := range []string{" 1 ", " 2 ", " 3 "} {
		if strings.Contains(text, num) {
			t.Fatalf("expected no line-number token %q in extracted text, got %q", num, text)
		}
	}
	for _, tok := range []string{"alpha-left-content", "beta-left-content", "gamma-left-content"} {
		if !strings.Contains(text, tok) {
			t.Fatalf("expected text to contain %q, got %q", tok, text)
		}
	}
	for _, tok := range []string{"alpha-right-content", "beta-right-content", "gamma-right-content"} {
		if strings.Contains(text, tok) {
			t.Fatalf("expected text to exclude right-side token %q, got %q", tok, text)
		}
	}
}

// (19) Side-by-side RIGHT selection excludes center divider, right line
// number, and the post-LN '│'.
func TestSelection_SideBySide_RightExcludesLineNumbersAndBorders(t *testing.T) {
	m := New(true, "auto")
	m.vp.SetWidth(80)
	m.vp.SetHeight(10)
	lines := []string{
		"│" + "  1 " + "│" + "alpha-left-content      " + "│" + "  1 " + "│" + "alpha-right-content     ",
		"│" + "  2 " + "│" + "beta-left-content       " + "│" + "  2 " + "│" + "beta-right-content      ",
		"│" + "  3 " + "│" + "gamma-left-content      " + "│" + "  3 " + "│" + "gamma-right-content     ",
	}
	content := strings.Join(lines, "\n")
	m.file = &cachedNode{path: "test", diff: content}
	m.gutterCol = 30
	m.leftContentCol = 6
	m.rightContentCol = 36

	// Click on the right line-number column (col 32). The band should snap to
	// rightContentCol=36 and the drag must stay there even when extended back
	// toward the gutter.
	m.StartSelection(Point{Line: 0, Col: 32})
	if m.sel.anchor.Col != 36 {
		t.Fatalf("expected anchor.Col=36 after clamp, got %d", m.sel.anchor.Col)
	}
	m.ExtendSelection(Point{Line: 2, Col: 200})

	text, ok := m.EndSelection()
	if !ok {
		t.Fatalf("expected EndSelection ok=true")
	}
	if strings.ContainsRune(text, '│') {
		t.Fatalf("expected no '│' in extracted text, got %q", text)
	}
	for _, num := range []string{" 1 ", " 2 ", " 3 "} {
		if strings.Contains(text, num) {
			t.Fatalf("expected no line-number token %q in extracted text, got %q", num, text)
		}
	}
	for _, tok := range []string{"alpha-right-content", "beta-right-content", "gamma-right-content"} {
		if !strings.Contains(text, tok) {
			t.Fatalf("expected text to contain %q, got %q", tok, text)
		}
	}
	for _, tok := range []string{"alpha-left-content", "beta-left-content", "gamma-left-content"} {
		if strings.Contains(text, tok) {
			t.Fatalf("expected text to exclude left-side token %q, got %q", tok, text)
		}
	}
}

// (20) End-to-end through Update: feeding a side-by-side diffContentMsg
// populates leftContentCol/rightContentCol so a subsequent left-side
// selection excludes the line-number column.
func TestSelection_SideBySideUpdateDetectsContentCols(t *testing.T) {
	m := New(true, "dark")
	// Viewport width 60 with each side ~30 cols wide puts the center divider
	// at col 30, matching width/2 so detectGutterCol picks it.
	m.vp.SetWidth(60)
	m.vp.SetHeight(10)

	pad := func(s string, w int) string {
		if len(s) >= w {
			return s[:w]
		}
		return s + strings.Repeat(" ", w-len(s))
	}
	// Layout: "│" + "  1 " + "│" + LEFT(24) + "│" + "  1 " + "│" + RIGHT(24)
	// Pipes at cols 0, 5, 30, 35.
	line := func(ln string, rn string) string {
		return "│" + "  1 " + "│" + pad(ln, 24) + "│" + "  1 " + "│" + pad(rn, 24)
	}
	sbsContent := strings.Join([]string{
		line("alpha-left", "alpha-right"),
		line("beta-left", "beta-right"),
		line("gamma-left", "gamma-right"),
	}, "\n")

	key := cacheKey("/", true)
	m.dir = &cachedNode{path: "/"}
	m.cache[key] = m.dir
	m.renderID = 1
	m, _ = m.Update(diffContentMsg{cacheKey: key, text: sbsContent, renderID: 1})

	if m.leftContentCol <= 0 || m.leftContentCol >= m.gutterCol {
		t.Fatalf("leftContentCol=%d not in (0, gutterCol=%d)", m.leftContentCol, m.gutterCol)
	}
	if m.rightContentCol <= m.gutterCol {
		t.Fatalf("rightContentCol=%d not > gutterCol=%d", m.rightContentCol, m.gutterCol)
	}

	// Click on the leading border and drag past the gutter; selection must
	// land entirely in the left content area, free of '│' and line numbers.
	m.StartSelection(Point{Line: 0, Col: 0})
	m.ExtendSelection(Point{Line: 2, Col: 200})
	text, ok := m.EndSelection()
	if !ok {
		t.Fatalf("expected EndSelection ok=true")
	}
	if strings.ContainsRune(text, '│') {
		t.Fatalf("expected no '│' in extracted text, got %q", text)
	}
	if !strings.Contains(text, "alpha-left") || !strings.Contains(text, "gamma-left") {
		t.Fatalf("expected left content tokens in text, got %q", text)
	}
	if strings.Contains(text, "alpha-right") {
		t.Fatalf("expected no right content token in text, got %q", text)
	}
}

// (21) End-to-end: feed the real delta side-by-side fixture through Update
// and assert that a click on the right half produces a non-empty highlight
// AND a non-empty extracted text — the production failure mode the user
// reported.
func TestSelection_SideBySide_RightHalfFromDeltaFixture(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "sbs_render_w120.ansi"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	m := New(true, "dark")
	m.vp.SetWidth(120)
	m.vp.SetHeight(20)

	key := cacheKey("/", true)
	m.dir = &cachedNode{path: "/"}
	m.cache[key] = m.dir
	m.renderID = 1
	m, _ = m.Update(diffContentMsg{cacheKey: key, text: string(raw), renderID: 1})

	if m.gutterCol <= 0 {
		t.Fatalf("expected gutterCol > 0, got %d", m.gutterCol)
	}
	if m.rightContentCol <= m.gutterCol {
		t.Fatalf("rightContentCol=%d must be > gutterCol=%d", m.rightContentCol, m.gutterCol)
	}

	// Pick a content line — first row in the cached diff with ≥4 pipes — and
	// click somewhere inside the right half of that line.
	cachedLines := strings.Split(m.dir.diff, "\n")
	contentLine := -1
	for i, ln := range cachedLines {
		if len(visualColumnsOf(ansi.Strip(ln), '│')) >= 4 {
			contentLine = i
			break
		}
	}
	if contentLine < 0 {
		t.Fatalf("no content line with ≥4 pipes found in cached diff")
	}

	rightClickCol := m.rightContentCol + 2 // a couple cols into the right content
	m.StartSelection(Point{Line: contentLine, Col: rightClickCol})
	m.ExtendSelection(Point{Line: contentLine, Col: rightClickCol + 6})

	// View() must contain a reverse-video escape — without one, nothing
	// is visually selected.
	out := m.View()
	if !strings.Contains(out, "\x1b[7m") {
		t.Fatalf(
			"expected reverse-video escape in View() after right-side selection, got:\n%s",
			out,
		)
	}

	text, ok := m.EndSelection()
	if !ok || text == "" {
		t.Fatalf(
			"expected non-empty extracted text from right-side selection, got ok=%v text=%q",
			ok,
			text,
		)
	}
	if strings.ContainsRune(text, '│') {
		t.Fatalf("expected no '│' in extracted text, got %q", text)
	}
}

// (22) joinWrappedLines pure-logic tests covering all 4 glyph cases.
func TestJoinWrappedLines_Symbols(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want string
	}{
		{
			name: "no symbols passes through with newlines",
			in:   []string{"first", "second", "third"},
			want: "first\nsecond\nthird",
		},
		{
			name: "trailing ↵ folds next row",
			in:   []string{"var s = \"this is a very long stri↵", "ng continues here"},
			want: "var s = \"this is a very long string continues here",
		},
		{
			name: "trailing ↵ on multiple rows folds them all",
			in:   []string{"aaa↵", "bbb↵", "ccc"},
			want: "aaabbbccc",
		},
		{
			name: "trailing → keeps newline (truncation, not continuation)",
			in:   []string{"long content cut→", "next logical line"},
			want: "long content cut\nnext logical line",
		},
		{
			name: "trailing ↴ folds like ↵",
			in:   []string{"abc↴", "def"},
			want: "abcdef",
		},
		{
			name: "leading … stripped before joining",
			in:   []string{"begin↵", "…end"},
			want: "beginend",
		},
		{
			name: "leading whitespace + … (right-aligned wrap) stripped",
			in:   []string{"longish-content-that-wraps↵", "                          …rest"},
			want: "longish-content-that-wrapsrest",
		},
		{
			name: "plain left-aligned wrap keeps content whitespace",
			in:   []string{"foo()↵", "  return bar"},
			want: "foo()  return bar",
		},
		{
			name: "standalone … is not a continuation, leave alone",
			in:   []string{"…explicit ellipsis content"},
			want: "…explicit ellipsis content",
		},
		{
			name: "mixed wrap and truncation",
			in:   []string{"first↵", "wrap-cont→", "third"},
			want: "firstwrap-cont\nthird",
		},
		{
			name: "empty rows preserved",
			in:   []string{"a", "", "b"},
			want: "a\n\nb",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := joinWrappedLines(tc.in)
			if got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

// (23) Integration: a selection spanning wrapped rows from delta-formatted
// content yields a single logical line in the extracted text.
func TestSelection_StripsWrapSymbolsOnExtract(t *testing.T) {
	m := New(true, "auto")
	m.vp.SetWidth(80)
	m.vp.SetHeight(10)
	m.gutterCol = 30
	m.leftContentCol = 6
	m.rightContentCol = 36

	// Three physical rows, all left-side content, where the first two end in
	// '↵' to indicate wrap and the third terminates normally. The left
	// content area is 24 visual cols wide (cols 6..29), so wrap symbols at
	// col 29 fall inside the [leftContentCol=6, gutterCol=30) band.
	row := func(left string) string {
		// `left` must already be 24 visual cols wide (ASCII + a single-width
		// glyph like '↵' both count as 1 visual col).
		return "│" + "  1 " + "│" + left + "│" + "  1 " + "│" + "right-side-fill        "
	}
	lines := []string{
		row("var s = first-fragment-↵"),
		row("then-middle-fragment-x-↵"),
		row("then-final-fragment.    "),
	}
	m.file = &cachedNode{path: "test", diff: strings.Join(lines, "\n")}

	m.StartSelection(Point{Line: 0, Col: 6})
	m.ExtendSelection(Point{Line: 2, Col: 29})
	text, ok := m.EndSelection()
	if !ok {
		t.Fatalf("expected EndSelection ok=true")
	}
	if strings.Contains(text, "↵") {
		t.Fatalf("expected ↵ stripped, got %q", text)
	}
	if strings.Count(text, "\n") != 0 {
		t.Fatalf("expected wrapped rows joined to one line, got %q", text)
	}
	want := "var s = first-fragment-then-middle-fragment-x-then-final-fragment."
	if !strings.Contains(text, want) {
		t.Fatalf("expected joined text to contain %q, got %q", want, text)
	}
}

// (24) wrapLongLines pure-logic tests.
func TestWrapLongLines(t *testing.T) {
	cases := []struct {
		name  string
		in    string
		width int
		want  []string // each entry is one physical row (after split on '\n')
	}{
		{
			name:  "line at limit passes through unchanged",
			in:    "abcdefghij",
			width: 10,
			want:  []string{"abcdefghij"},
		},
		{
			name:  "line under limit passes through unchanged",
			in:    "abc",
			width: 10,
			want:  []string{"abc"},
		},
		{
			name:  "line over limit wraps into chunks ending in ↵",
			in:    "abcdefghij" + "klmnopqrst" + "uvw",
			width: 10,
			want: []string{
				"abcdefghi↵",
				"jklmnopqr↵",
				"stuvw",
			},
		},
		{
			name:  "multiple lines wrap independently",
			in:    "short\n" + strings.Repeat("x", 12) + "\nshort2",
			width: 6,
			want: []string{
				"short",
				"xxxxx↵",
				"xxxxx↵",
				"xx",
				"short2",
			},
		},
		{
			name:  "empty input is preserved",
			in:    "",
			width: 10,
			want:  []string{""},
		},
		{
			name:  "non-positive width returns input verbatim",
			in:    "anything goes",
			width: 0,
			want:  []string{"anything goes"},
		},
		{
			name:  "width=1 falls back to truncate (cannot fit content + symbol)",
			in:    "abcdef",
			width: 1,
			want:  []string{"a"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := wrapLongLines(tc.in, tc.width)
			gotRows := strings.Split(got, "\n")
			// Compare stripped (no ANSI) representations.
			stripped := make([]string, len(gotRows))
			for i, r := range gotRows {
				stripped[i] = ansi.Strip(r)
			}
			if !equalStringSlices(stripped, tc.want) {
				t.Fatalf("got rows %q want %q", stripped, tc.want)
			}
			// Every wrapped row (those ending in '↵') must fit within width.
			for _, r := range stripped {
				if w := lipgloss.Width(r); w > tc.width && tc.width > 0 {
					t.Fatalf("row %q has width %d > %d", r, w, tc.width)
				}
			}
		})
	}
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// (25) wrapLongLines preserves ANSI style escapes so the wrapped row reopens
// the original styling — and the selection extractor strips both the wrap
// symbol and the styles when collapsing.
func TestWrapLongLines_AnsiPreservedAndCollapsesOnExtract(t *testing.T) {
	// A red-foreground line longer than the width.
	red := "\x1b[31m"
	reset := "\x1b[0m"
	long := red + "aaaaabbbbbcccccddddd" + reset
	out := wrapLongLines(long, 10)
	rows := strings.Split(out, "\n")
	if len(rows) < 2 {
		t.Fatalf("expected wrap to produce >=2 rows, got %d: %q", len(rows), rows)
	}
	for _, r := range rows {
		// At minimum the SGR 31 (red) escape should appear somewhere in the
		// row that contains visible content from the original line.
		plain := ansi.Strip(r)
		if plain == "" {
			continue
		}
		if !strings.Contains(r, "\x1b[") {
			t.Fatalf("expected ANSI escapes preserved in row %q", r)
		}
	}

	// joinWrappedLines should collapse the rows back to one logical line
	// after ANSI stripping.
	plainRows := make([]string, len(rows))
	for i, r := range rows {
		plainRows[i] = ansi.Strip(r)
	}
	joined := joinWrappedLines(plainRows)
	if strings.Contains(joined, "↵") {
		t.Fatalf("expected ↵ stripped from joined output, got %q", joined)
	}
	if strings.Contains(joined, "\n") {
		t.Fatalf("expected wrapped rows joined to one line, got %q", joined)
	}
	if joined != "aaaaabbbbbcccccddddd" {
		t.Fatalf("expected joined %q, got %q", "aaaaabbbbbcccccddddd", joined)
	}
}

// (24) Cross-mode cache reuse: after rendering a file in unified mode (which
// resets gutterCol=-1) and then re-rendering it side-by-side from a cached
// payload, the side-by-side columns must be re-detected. Without this fix the
// right-side band logic would treat every click as "unified mode, no clamp",
// hiding line-numbered selections and feeling like the right half doesn't
// respond to drags in some configurations.
func TestSelection_CachedSideBySideRedetectsColumns(t *testing.T) {
	m := New(true, "dark")
	m.vp.SetWidth(60)
	m.vp.SetHeight(10)

	pad := func(n int) string { return strings.Repeat(" ", n) }
	sbsLine := "│" + "  1 " + "│" + pad(24) + "│" + "  1 " + "│" + pad(24)
	sbsContent := strings.Repeat(sbsLine+"\n", 4)

	// Render once via diffContentMsg to populate the cache.
	key := cacheKey("/", true)
	m.dir = &cachedNode{path: "/"}
	m.cache[key] = m.dir
	m.renderID = 1
	m, _ = m.Update(diffContentMsg{cacheKey: key, text: sbsContent, renderID: 1})

	gutter := m.gutterCol
	leftCol := m.leftContentCol
	rightCol := m.rightContentCol
	if gutter <= 0 || leftCol <= 0 || rightCol <= gutter {
		t.Fatalf("initial detection bad: gutter=%d left=%d right=%d", gutter, leftCol, rightCol)
	}

	// Simulate a mode toggle to unified that resets the detection metadata.
	m.gutterCol = -1
	m.leftContentCol = -1
	m.rightContentCol = -1

	// Now load the same path again — cache hit. Detection must refresh.
	files := []*gitdiff.File{}
	m, _ = m.SetDirPatch("/", files)
	if m.gutterCol != gutter || m.leftContentCol != leftCol || m.rightContentCol != rightCol {
		t.Fatalf("cache-hit failed to refresh columns: gutter=%d left=%d right=%d (want %d/%d/%d)",
			m.gutterCol, m.leftContentCol, m.rightContentCol, gutter, leftCol, rightCol)
	}
}

// (25) Unified content rendered while m.sideBySide==true (delta forces
// unified for new/deleted files): detection must NOT pin gutterCol at the
// vpWidth/2 fallback — otherwise the right-side selection band points into
// empty space past the actual line widths and "the right side doesn't work".
func TestSelection_UnifiedContentInSBSModeFallsBackToUnifiedBand(t *testing.T) {
	m := New(true, "dark")
	m.vp.SetWidth(80)
	m.vp.SetHeight(10)

	// Unified delta output: plain content without side-by-side structure.
	unifiedContent := strings.Join([]string{
		"package main",
		"",
		"func main() {}",
		"  return",
	}, "\n")

	key := cacheKey("/", true)
	m.dir = &cachedNode{path: "/"}
	m.cache[key] = m.dir
	m.renderID = 1
	m, _ = m.Update(diffContentMsg{cacheKey: key, text: unifiedContent, renderID: 1})

	if m.gutterCol != -1 {
		t.Fatalf(
			"expected gutterCol == -1 for unified content (got %d) — would otherwise split selections in half over empty space",
			m.gutterCol,
		)
	}
	if m.leftContentCol != -1 || m.rightContentCol != -1 {
		t.Fatalf(
			"expected left/right content cols both -1, got %d/%d",
			m.leftContentCol,
			m.rightContentCol,
		)
	}

	// A click "on the right half" must now use the unified band [0, MaxInt),
	// extracting the full visible content.
	m.StartSelection(Point{Line: 0, Col: 50})
	m.ExtendSelection(Point{Line: 0, Col: 5})
	text, ok := m.EndSelection()
	if !ok || text == "" {
		t.Fatalf(
			"expected non-empty selection when SBS-but-unified content is clicked on the right, got ok=%v text=%q",
			ok,
			text,
		)
	}
}

// (26) Click on a hunk-header line ("60: struct Foo { │") must NOT apply the
// SBS side-band clamp — otherwise the leading "60: " prefix becomes
// unselectable. The line lacks the leading '│' that marks an SBS content
// row, so StartSelection should fall back to the unified [0, MaxInt) band.
func TestSelection_HunkHeaderUsesUnifiedBand(t *testing.T) {
	m := New(true, "dark")
	m.vp.SetWidth(80)
	m.vp.SetHeight(10)
	m.gutterCol = 40
	m.leftContentCol = 6
	m.rightContentCol = 46

	hunkHeader := "60: struct Foo {                       │"
	sbsRow := "│  1 │left side content                │  1 │right side content"
	m.file = &cachedNode{path: "test", diff: hunkHeader + "\n" + sbsRow}

	// Click on "struct" inside the hunk header (col 4).
	m.StartSelection(Point{Line: 0, Col: 4})
	if m.sel.colBand[0] != 0 {
		t.Fatalf("expected unified band lo=0 on hunk-header line, got %v", m.sel.colBand)
	}
	if m.sel.anchor.Col != 4 {
		t.Fatalf("expected anchor.Col=4 (no clamp on hunk header), got %d", m.sel.anchor.Col)
	}
	m.ExtendSelection(Point{Line: 0, Col: 16})

	text, ok := m.EndSelection()
	if !ok {
		t.Fatalf("expected EndSelection ok=true")
	}
	if !strings.Contains(text, "struct Foo {") {
		t.Fatalf("expected hunk-header content 'struct Foo {' in extracted text, got %q", text)
	}

	// And a click on the SBS row below still clamps as before.
	m.StartSelection(Point{Line: 1, Col: 10})
	if m.sel.colBand[0] != 6 || m.sel.colBand[1] != 40 {
		t.Fatalf("expected SBS left band [6,40) on row 1, got %v", m.sel.colBand)
	}
}

// (27) Robustness: garbled or unfamiliar delta output (zero pipes, weird
// glyphs, line widths far outside the expected layout) must not crash the
// TUI. Detection should land at -1 columns and selection should fall back
// to the unified band so the user can still drag, even if line-number
// exclusion no longer applies.
func TestSelection_GarbledDeltaOutputFallsBackToUnified(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{name: "no pipes at all", content: "just text\nmore text\nno divider markers anywhere"},
		{name: "wrong divider char", content: "║  1 ║left║  1 ║right\n║  2 ║left║  2 ║right"},
		{name: "implausibly wide gutter", content: strings.Repeat("│", 200)},
		{name: "binary-ish garbage", content: "\x00\x01\x02\x03\x04\x05"},
		{name: "empty content", content: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := New(true, "dark")
			m.vp.SetWidth(80)
			m.vp.SetHeight(10)
			key := cacheKey("/", true)
			m.dir = &cachedNode{path: "/"}
			m.cache[key] = m.dir
			m.renderID = 1
			// Update must not panic.
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("Update panicked on %q content: %v", tc.name, r)
				}
			}()
			m, _ = m.Update(diffContentMsg{cacheKey: key, text: tc.content, renderID: 1})
			if m.gutterCol != -1 || m.leftContentCol != -1 || m.rightContentCol != -1 {
				t.Fatalf("expected all columns at -1 on unparseable content, got g=%d l=%d r=%d",
					m.gutterCol, m.leftContentCol, m.rightContentCol)
			}

			// Selection still functions in unified mode.
			m.StartSelection(Point{Line: 0, Col: 0})
			m.ExtendSelection(Point{Line: 0, Col: 5})
			if _, err := safeEndSelection(&m); err != nil {
				t.Fatalf("EndSelection panicked on %q content: %v", tc.name, err)
			}
		})
	}
}

func safeEndSelection(m *Model) (text string, recovered any) {
	defer func() {
		if r := recover(); r != nil {
			recovered = r
		}
	}()
	t, _ := m.EndSelection()
	return t, nil
}

// joinWrappedLines: explicit ellipsis "…" content that IS a continuation row
// (previous row ended in ↵) but does NOT have the continuation prefix format
// (leading whitespace + "…"). The ellipsis is embedded in the content.
func TestJoinWrappedLines_ContinuationWithEllipsisInContent(t *testing.T) {
	// Row 1 ends in ↵, row 2 is a continuation. Row 2 contains "…" in the middle
	// of actual content, not as the continuation prefix. stripContinuationPrefix
	// trims leading whitespace, tries CutPrefix("hello…world", "…") which fails,
	// returns the original row.
	got := joinWrappedLines([]string{"abc↵", "hello…world"})
	want := "abchello…world"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// EndSelection boundary conditions and robustness
// ---------------------------------------------------------------------------

func TestEndSelectionSafeOnPanic(t *testing.T) {
	m := New(false, "dark")
	m.Common.Width = 60
	m.Common.Height = 20
	m.SetSize(60, 20)
	m.SetContent("some content to select")

	m.StartSelection(Point{Line: 0, Col: 0})
	m.ExtendSelection(Point{Line: 0, Col: 5})

	// Inject panic through the production code's test seam.
	m.testHookSelectedTextPanic = func() { panic("selectedTextInner failed") }

	text, ok := m.EndSelection()
	// The panic must be recovered: EndSelection returns safe values.
	if ok && text != "" {
		t.Fatalf("expected empty result on panic, got %q", text)
	}
}

// ---------------------------------------------------------------------------
// ANSI edge cases in selection highlighting
// ---------------------------------------------------------------------------

func TestHighlightWithSGRResetInContent(t *testing.T) {
	// Delta emits \x1b[m (empty-params SGR reset = full reset) and
	// \x1b[27m (explicit reverse-off). Both should be handled by
	// reapplyReverseAfterResets so the selection highlight is restored.
	cases := []struct {
		name    string
		content string
	}{
		{"empty-params-reset", "\x1b[31mhello\x1b[m world\nline2\nline3"},
		{"sgr27-reverse-off", "\x1b[31mhello\x1b[27m world\nline2\nline3"},
		{"composite-reset", "\x1b[31mhello\x1b[0;1m bold world\nline2\nline3"},
		{"semicolon-empty-params", "hello\x1b[;m world\nline2"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := New(false, "dark")
			m.Common.Width = 60
			m.Common.Height = 10
			m.SetSize(60, 10)
			m.SetContent(tc.content)

			m.StartSelection(Point{Line: 0, Col: 0})
			m.ExtendSelection(Point{Line: 0, Col: 11})

			// View should render without panic and include reverse-video
			// highlighting (the highlight must be re-asserted after the reset).
			view := m.View()
			if view == "" {
				t.Fatal("expected non-empty view")
			}
			if !strings.Contains(view, "\x1b[7m") {
				t.Fatal("expected reverse-video escape in highlighted view")
			}
		})
	}
}

func TestHighlightWithIncompleteCSISequence(t *testing.T) {
	// Malformed ANSI: incomplete CSI (\x1b[3 with no closing byte) at end of
	// a line. This exercises the end >= len(s) branch in reapplyReverseAfterResets.
	m := New(false, "dark")
	m.Common.Width = 60
	m.Common.Height = 10
	m.SetSize(60, 10)
	m.SetContent("hello\x1b[3\nline2\nline3")

	m.StartSelection(Point{Line: 0, Col: 0})
	m.ExtendSelection(Point{Line: 0, Col: 5})

	view := m.View()
	if view == "" {
		t.Fatal("expected non-empty view with incomplete CSI in content")
	}
}

func TestHighlightWithNonSGRCSISequence(t *testing.T) {
	// Non-SGR CSI sequence (device status report \x1b[5n). The final byte
	// is 'n' not 'm', so sgrClearsReverse must NOT be called. The view
	// should still render correctly.
	m := New(false, "dark")
	m.Common.Width = 60
	m.Common.Height = 10
	m.SetSize(60, 10)
	m.SetContent("hello\x1b[5n world\nline2")

	m.StartSelection(Point{Line: 0, Col: 0})
	m.ExtendSelection(Point{Line: 0, Col: 12})

	view := m.View()
	if view == "" {
		t.Fatal("expected non-empty view with non-SGR CSI")
	}
}

// ---------------------------------------------------------------------------
// Selection on short/narrow lines in side-by-side mode
// ---------------------------------------------------------------------------

func TestViewHighlightsShortSBSLineWithoutPanic(t *testing.T) {
	// A side-by-side diff with a very short middle line (separator or
	// decoration) between longer SBS lines. The selection band [6,40)
	// starts past the short line's width, so clampToLine's a > lineWidth
	// guard fires. View() must not panic.
	m := New(true, "dark")
	m.Common.Width = 80
	m.Common.Height = 20
	m.SetSize(80, 20)

	content := "│  1 │long-content-left              │  1 │right1\n" +
		"x\n" + // short line, width=1
		"│  2 │second-line                    │  2 │right2"
	m.dir = &cachedNode{path: "src", diff: content}
	m.SetContent(content)

	// Set column detection so StartSelection uses SBS band.
	m.gutterCol = 40
	m.leftContentCol = 6
	m.rightContentCol = 46

	m.StartSelection(Point{Line: 0, Col: 6})
	m.ExtendSelection(Point{Line: 2, Col: 39})

	view := m.View()
	if view == "" {
		t.Fatal("expected non-empty view")
	}
}

func TestSelectionOnShortSBSLines(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{
			"short-middle-line",
			"│  1 │long-content-left              │  1 │right1\n" +
				"x\n" +
				"│  2 │second-line                    │  2 │right2",
		},
		{
			"empty-middle-line",
			"│  1 │long-content                │  1 │right1\n" +
				"\n" +
				"│  2 │another                     │  2 │right2",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := New(true, "dark")
			m.Common.Width = 80
			m.Common.Height = 20
			m.SetSize(80, 20)
			m.SetContent(tc.content)

			// Multi-line selection covering all three lines.
			m.StartSelection(Point{Line: 0, Col: 6})
			m.ExtendSelection(Point{Line: 2, Col: 39})

			// View must not panic on the short/empty middle line.
			view := m.View()
			if view == "" {
				t.Fatal("expected non-empty view")
			}
		})
	}
}

func TestEndSelectionOnShortSBSLines(t *testing.T) {
	// Same scenario but exercising EndSelection which goes through
	// selectedText → clampToLine with the a > lineWidth branch.
	m := New(true, "dark")
	m.Common.Width = 80
	m.Common.Height = 20
	m.SetSize(80, 20)

	content := "│  1 │alpha-content                  │  1 │right1\n" +
		"│  2 │xy                             │  2 │right2"
	m.dir = &cachedNode{path: "src", diff: content}
	m.SetContent(content)

	m.StartSelection(Point{Line: 0, Col: 6})
	m.ExtendSelection(Point{Line: 1, Col: 39})

	text, ok := m.EndSelection()
	if !ok {
		t.Fatal("expected EndSelection to succeed")
	}
	if text == "" {
		t.Fatal("expected non-empty selected text")
	}
}

// ---------------------------------------------------------------------------
// EndSelection boundary conditions: no content, empty diff, out-of-range
// ---------------------------------------------------------------------------

func TestEndSelectionNoFileOrDir(t *testing.T) {
	m := New(false, "dark")
	m.StartSelection(Point{Line: 0, Col: 0})
	m.ExtendSelection(Point{Line: 0, Col: 5})

	text, ok := m.EndSelection()
	if ok && text != "" {
		t.Fatalf("expected empty text with no content loaded, got %q", text)
	}
}

func TestEndSelectionEmptyDiff(t *testing.T) {
	m := New(false, "dark")
	m.SetContent("")

	m.StartSelection(Point{Line: 0, Col: 0})
	m.ExtendSelection(Point{Line: 0, Col: 5})

	text, ok := m.EndSelection()
	if ok && text != "" {
		t.Fatalf("expected empty text for empty diff, got %q", text)
	}
}

func TestEndSelectionStartLineOutOfRange(t *testing.T) {
	cases := []struct {
		name  string
		start Point
	}{
		{"negative-line", Point{Line: -1, Col: 0}},
		{"beyond-content", Point{Line: 100, Col: 0}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := New(false, "dark")
			m.SetContent("line0\nline1")

			m.StartSelection(tc.start)
			m.ExtendSelection(Point{Line: 0, Col: 3})

			text, ok := m.EndSelection()
			if ok && text != "" {
				t.Fatalf("expected empty text for out-of-range start, got %q", text)
			}
		})
	}
}

func TestEndSelectionEndToEnd(t *testing.T) {
	m := New(false, "dark")
	m.Common.Width = 60
	m.Common.Height = 10
	m.SetSize(60, 10)

	content := "first line of text\nsecond line\nthird line"
	m.file = &cachedNode{path: "test", diff: content}
	m.SetContent(content)

	m.StartSelection(Point{Line: 0, Col: 5})
	m.ExtendSelection(Point{Line: 1, Col: 6})

	text, ok := m.EndSelection()
	if !ok {
		t.Fatal("expected EndSelection to succeed")
	}
	if !strings.Contains(text, "line of text") {
		t.Fatalf("expected 'line of text' in selected text, got %q", text)
	}
	if !strings.Contains(text, "second") {
		t.Fatalf("expected 'second' in selected text, got %q", text)
	}
}

// ---------------------------------------------------------------------------
// Multi-line SBS selection with mixed line lengths
// ---------------------------------------------------------------------------

func TestEndSelectionSBSWithMixedLineLengths(t *testing.T) {
	m := New(true, "dark")
	m.Common.Width = 80
	m.Common.Height = 20
	m.SetSize(80, 20)

	content := "│  1 │long-content-here              │  1 │right1\n" +
		"│  2 │ab                             │  2 │right2"
	m.dir = &cachedNode{path: "src", diff: content}
	m.SetContent(content)

	m.StartSelection(Point{Line: 0, Col: 6})
	m.ExtendSelection(Point{Line: 1, Col: 39})

	text, ok := m.EndSelection()
	if !ok {
		t.Fatal("expected EndSelection to succeed")
	}
	if !strings.Contains(text, "long-content-here") {
		t.Fatalf("expected 'long-content-here' in selected text, got %q", text)
	}
	if !strings.Contains(text, "ab") {
		t.Fatalf("expected 'ab' from short line, got %q", text)
	}
}

func TestView_NoSelectionNoOverlay(t *testing.T) {
	m := New(false, "auto")
	m.vp.SetWidth(40)
	m.vp.SetHeight(5)
	m.vp.SetContent("alpha\nbeta\ngamma")

	out := m.View()
	if strings.Contains(out, "\x1b[7m") || strings.Contains(out, "\x1b[27m") {
		t.Fatalf(
			"expected no SGR-reverse escapes when sel.active==false && sel.has==false, got:\n%q",
			out,
		)
	}
}
