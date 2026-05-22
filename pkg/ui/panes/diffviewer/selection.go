package diffviewer

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
)

// Point identifies a position within the rendered diff content.
//
// Col is a *visual column* — the same coordinate space as paneX and
// ansi.Cut/lipgloss.Width — never a rune index or byte offset. Callers must
// translate any rune-index-derived value into visual columns before storing
// it in a Point.
type Point struct {
	Line int
	Col  int
}

// selection captures an in-progress or finalized text selection in the diff
// viewport. colBand is [lo, hi) — half-open — matching the conventions used
// throughout highlightRange and ansi.Cut.
type selection struct {
	anchor  Point
	head    Point
	colBand [2]int
	active  bool
	has     bool
}

// normalized returns (start, end) with start <= end in (line, col) order.
func (s selection) normalized() (Point, Point) {
	a, b := s.anchor, s.head
	if a.Line > b.Line || (a.Line == b.Line && a.Col > b.Col) {
		a, b = b, a
	}
	return a, b
}

// highlightRange returns the half-open [a, b) visual-column range to
// highlight on a given content line, before colBand and line-width clamps.
//
// Rules (from PLAN.md):
//   - Single-line selection:    a, b = start.Col, end.Col
//   - First line of multi-line: a, b = start.Col, lineWidth
//   - Interior line:            a, b = 0,         lineWidth
//   - Last line:                a, b = 0,         end.Col
func highlightRange(line int, start, end Point, lineWidth int) (int, int) {
	if start.Line == end.Line {
		return start.Col, end.Col
	}
	switch line {
	case start.Line:
		return start.Col, lineWidth
	case end.Line:
		return 0, end.Col
	default:
		return 0, lineWidth
	}
}

// clampToBand clips [a, b) to the selection's colBand [lo, hi). Used for
// multi-line selections so interior lines don't extend beyond the active
// column.
func clampToBand(a, b int, band [2]int) (int, int) {
	lo, hi := band[0], band[1]
	if a < lo {
		a = lo
	}
	if b > hi {
		b = hi
	}
	if a > b {
		a = b
	}
	return a, b
}

// clampToLine clips b to min(b, lineWidth). MUST be applied after
// clampToBand — protects against drag-right-past-EOL when the selection's
// end column exceeds the actual rendered line width.
func clampToLine(a, b, lineWidth int) (int, int) {
	if b > lineWidth {
		b = lineWidth
	}
	if a > lineWidth {
		a = lineWidth
	}
	if a > b {
		a = b
	}
	return a, b
}

// spliceReverse returns line with the visual column range [a, b) wrapped in
// SGR 7 (reverse video) / SGR 27 (reset). Uses ansi.Cut so ANSI escapes and
// wide characters are preserved.
//
// Mid-chunk SGR resets (e.g. delta's `\x1b[0m`) would otherwise clear our
// reverse-video attribute and leave the selection invisible after the cell
// renderer normalizes the output; reapplyReverseAfterResets rewrites the
// middle so reverse stays asserted across any internal reset.
func spliceReverse(line string, a, b, lineWidth int) string {
	middle := reapplyReverseAfterResets(ansi.Cut(line, a, b))
	return ansi.Cut(line, 0, a) + "\x1b[7m" + middle + "\x1b[27m" + ansi.Cut(line, b, lineWidth)
}

// reapplyReverseAfterResets walks s looking for SGR sequences that include
// parameter 0 (a full reset) or the canonical "disable reverse" 27 code, and
// appends `\x1b[7m` after each so the surrounding reverse-video span keeps
// applying to the cells that follow. Non-SGR escapes and SGR sequences that
// don't reset reverse are left untouched.
func reapplyReverseAfterResets(s string) string {
	if s == "" {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		c := s[i]
		if c != 0x1b || i+1 >= len(s) || s[i+1] != '[' {
			b.WriteByte(c)
			i++
			continue
		}
		// Found the start of a CSI sequence: \x1b[<params>m...
		end := i + 2
		for end < len(s) {
			r := s[end]
			if r == 'm' || (r >= 0x40 && r <= 0x7e) {
				break
			}
			end++
		}
		if end >= len(s) {
			b.WriteString(s[i:])
			break
		}
		seq := s[i : end+1]
		b.WriteString(seq)
		if seq[len(seq)-1] == 'm' && sgrClearsReverse(seq[2:len(seq)-1]) {
			b.WriteString("\x1b[7m")
		}
		i = end + 1
	}
	return b.String()
}

// sgrClearsReverse returns true when an SGR parameter list — i.e. the bytes
// between `\x1b[` and `m` — would clear the reverse-video attribute. The
// empty list (`\x1b[m`) and any list containing a `0` or `27` parameter both
// reset reverse and need a re-assert afterwards.
func sgrClearsReverse(params string) bool {
	if params == "" {
		return true
	}
	for _, p := range strings.Split(params, ";") {
		switch strings.TrimSpace(p) {
		case "", "0", "27":
			return true
		}
	}
	return false
}

// Delta's default line-continuation glyphs. We strip these from extracted
// text and, for the wrap symbols, fold the next physical line into the
// current logical one.
const (
	wrapLeftSymbol        = "↵" // ↵   wrap-left-symbol (end-of-line continuation)
	wrapRightSymbol       = "↴" // ↴   wrap-right-symbol (right-aligned wrap continuation)
	wrapRightPrefixSymbol = "…" // …   wrap-right-prefix-symbol (continuation prefix)
	truncationSymbol      = "→" // →   end-of-line truncation indicator
)

// wrapLongLines folds physical rows wider than `width` into multiple rows of
// at most `width` visual columns, joined by delta's wrap-left-symbol "↵". The
// symbol takes one column, so content chunks are sized at width-1. Lines that
// already fit are passed through unchanged.
//
// Output is compatible with joinWrappedLines: a row ending in '↵' is the
// continuation marker that selection/copy uses to merge the next row back into
// the same logical line. ANSI styles are preserved via ansi.Cut, with a SGR
// reset emitted before each '↵' so the symbol itself doesn't pick up a
// trailing background color from the chunk.
func wrapLongLines(text string, width int) string {
	if width <= 0 {
		return text
	}
	lines := strings.Split(text, "\n")
	var b strings.Builder
	b.Grow(len(text))
	for i, line := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		w := ansi.StringWidth(line)
		if w <= width {
			b.WriteString(line)
			continue
		}
		chunkWidth := width - 1
		if chunkWidth < 1 {
			// Viewport too narrow to fit content + symbol; fall back to a
			// plain truncate so we don't emit zero-width chunks forever.
			b.WriteString(ansi.Truncate(line, width, ""))
			continue
		}
		for start := 0; start < w; start += chunkWidth {
			end := start + chunkWidth
			if end >= w {
				b.WriteString(ansi.Cut(line, start, w))
				break
			}
			b.WriteString(ansi.Cut(line, start, end))
			b.WriteString("\x1b[0m")
			b.WriteString(wrapLeftSymbol)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// joinWrappedLines folds delta-rendered line continuations back into single
// logical lines. Input is a slice of ANSI-stripped row substrings (one per
// physical viewport row in the selection); output is the joined plaintext.
//
// Rules:
//   - Trailing '↵' / '↴' on a row mean "continues on next row" — the glyph is
//     stripped and no newline is emitted between this row and the next.
//   - Trailing '→' means "truncated at the row edge but the logical line is
//     done here" — strip the glyph, emit a newline.
//   - The row *immediately following* a wrap glyph is a continuation. Strip
//     any leading whitespace + '…' (delta's right-aligned wrap prefix) before
//     appending so the join reads as one logical line instead of e.g.
//     "abc                 …def".
func joinWrappedLines(rows []string) string {
	if len(rows) == 0 {
		return ""
	}
	var b strings.Builder
	continuing := false
	for i, row := range rows {
		if continuing {
			row = stripContinuationPrefix(row)
		}
		joinNext := false
		switch {
		case strings.HasSuffix(row, wrapLeftSymbol):
			row = strings.TrimSuffix(row, wrapLeftSymbol)
			joinNext = true
		case strings.HasSuffix(row, wrapRightSymbol):
			row = strings.TrimSuffix(row, wrapRightSymbol)
			joinNext = true
		case strings.HasSuffix(row, truncationSymbol):
			row = strings.TrimSuffix(row, truncationSymbol)
		}
		b.WriteString(row)
		if i < len(rows)-1 && !joinNext {
			b.WriteByte('\n')
		}
		continuing = joinNext
	}
	return b.String()
}

// stripContinuationPrefix removes the leading whitespace and optional '…'
// glyph that delta inserts when right-aligning a wrapped fragment. Returns
// the row unchanged if no continuation prefix is present.
func stripContinuationPrefix(row string) string {
	trimmed := strings.TrimLeft(row, " \t")
	if rest, ok := strings.CutPrefix(trimmed, wrapRightPrefixSymbol); ok {
		return rest
	}
	// Plain left-aligned wrap: no '…' to strip and the leading whitespace
	// is real indentation belonging to the wrapped content — keep it.
	return row
}

// Function vars allow tests to inject panicking implementations to exercise
// defer/recover safety nets in refreshColumnDetection and applyHighlight.
var (
	detectGutterColFunc       = detectGutterCol
	detectSideContentColsFunc = detectSideContentCols
	spliceReverseFunc         = spliceReverse
)

// visualColumnsOf walks s rune-by-rune accumulating runewidth.RuneWidth and
// returns the visual column at which each occurrence of r begins.
//
// Operates in visual-column space (not rune index) so that CJK / emoji to
// the left of the matched rune are accounted for. The returned columns are
// in the same coordinate space as paneX and ansi.Cut.
func visualColumnsOf(s string, r rune) []int {
	var out []int
	col := 0
	for _, c := range s {
		if c == r {
			out = append(out, col)
		}
		col += runewidth.RuneWidth(c)
	}
	return out
}

// detectGutterCol scans the first ~10 content lines of the stripped diff
// and returns the visual column at which the center "│" divider sits.
// Returns the fallback (paneWidth/2) if no consistent column emerges or
// if fewer than ~2 lines pass the skip-line gate.
//
// Skip-line gate: a "content line" has at least 3 occurrences of '│'
// (leading border, post-line-number, center divider). Header/decoration
// lines have 0 or 1 and are skipped.
//
// All positions are visual columns, via visualColumnsOf — NOT rune
// indexes. CJK / emoji to the left of the gutter must not skew the
// result.
func detectGutterCol(stripped string, paneWidth int) int {
	fallback := paneWidth / 2
	target := fallback
	counts := map[int]int{}
	lines := firstNContentLines(stripped, 10)
	for _, raw := range lines {
		positions := visualColumnsOf(raw, '│')
		if len(positions) < 3 {
			continue
		}
		best := positions[0]
		for _, p := range positions {
			if absInt(p-target) < absInt(best-target) {
				best = p
			}
		}
		counts[best]++
	}
	return modeOrFallback(counts, fallback)
}

// detectSideContentCols finds the first visual column of the content area on
// each side of a side-by-side render — i.e. the column just past the
// post-line-number "│" border. Returns (-1, -1) when nothing matches.
//
// For each content line (≥4 occurrences of '│') it picks:
//   - leftPostLN  = the rightmost '│' strictly before gutterCol
//   - rightPostLN = the leftmost '│' strictly after gutterCol
//
// and returns (leftPostLN+1, rightPostLN+1) as the mode across content lines.
// gutterCol must already have been resolved via [detectGutterCol].
func detectSideContentCols(stripped string, gutterCol int) (int, int) {
	if gutterCol <= 0 {
		return -1, -1
	}
	leftCounts := map[int]int{}
	rightCounts := map[int]int{}
	for _, raw := range firstNContentLines(stripped, 10) {
		positions := visualColumnsOf(raw, '│')
		if len(positions) < 4 {
			continue
		}
		leftPostLN := -1
		rightPostLN := -1
		for _, p := range positions {
			switch {
			case p > 0 && p < gutterCol:
				if p > leftPostLN {
					leftPostLN = p
				}
			case p > gutterCol:
				if rightPostLN == -1 || p < rightPostLN {
					rightPostLN = p
				}
			}
		}
		if leftPostLN > 0 {
			leftCounts[leftPostLN]++
		}
		if rightPostLN > 0 {
			rightCounts[rightPostLN]++
		}
	}
	left := modeOrFallback(leftCounts, -1)
	right := modeOrFallback(rightCounts, -1)
	if left < 0 || right < 0 {
		return -1, -1
	}
	return left + 1, right + 1
}

// firstNContentLines returns up to n non-empty lines from stripped, split on
// '\n'. Caller is expected to pass already-ansi.Stripped content.
func firstNContentLines(stripped string, n int) []string {
	out := make([]string, 0, n)
	for _, line := range strings.Split(stripped, "\n") {
		if line == "" {
			continue
		}
		out = append(out, line)
		if len(out) >= n {
			break
		}
	}
	return out
}

// modeOrFallback returns the most-counted key in counts, breaking ties by
// the smaller key. Returns fallback if counts is empty.
func modeOrFallback(counts map[int]int, fallback int) int {
	if len(counts) == 0 {
		return fallback
	}
	bestKey := 0
	bestCount := -1
	for k, c := range counts {
		if c > bestCount || (c == bestCount && k < bestKey) {
			bestKey = k
			bestCount = c
		}
	}
	return bestKey
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
