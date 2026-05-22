package diffviewer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/color"
	"io"
	"math"
	"os"
	"os/exec"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"charm.land/log/v2"
	"github.com/bluekeyes/go-gitdiff/gitdiff"
	"github.com/charmbracelet/x/ansi"

	"github.com/dlvhdr/diffnav/pkg/config"
	"github.com/dlvhdr/diffnav/pkg/filenode"
	"github.com/dlvhdr/diffnav/pkg/icons"
	"github.com/dlvhdr/diffnav/pkg/ui/common"
	"github.com/dlvhdr/diffnav/pkg/utils"
)

// DirHeaderHeight is the height of the header band rendered above the
// diff viewport. Exported so callers translating mouse coordinates into
// content rows (see tui.go's diffPanePoint) can skip past it.
const DirHeaderHeight = 3

const maxDeltaLineLength = 4096

type cachedNode struct {
	path      string
	files     []*gitdiff.File
	additions int64
	deletions int64
	diff      string
	ready     bool
}

type nodeCache map[string]*cachedNode

func cacheKey(path string, sideBySide bool) string {
	if sideBySide {
		return path + ":sbs"
	}
	return path
}

func cacheReady(node *cachedNode) bool {
	return node != nil && node.ready
}

// currentKey returns the cache/scroll key of the node currently shown in the
// viewport (a file or a directory), or false when nothing is selected.
func (m *Model) currentKey() (string, bool) {
	switch {
	case m.file != nil:
		return cacheKey(m.file.path, m.sideBySide), true
	case m.dir != nil:
		return cacheKey(m.dir.path, m.sideBySide), true
	default:
		return "", false
	}
}

// rememberScroll snapshots the viewport's current scroll position for the
// node being navigated away from. Call this before swapping the selected node.
func (m *Model) rememberScroll() {
	if key, ok := m.currentKey(); ok {
		m.scrollOffsets[key] = m.vp.YOffset()
	}
}

// restoreScroll positions the viewport at the node's remembered offset (the
// top if never visited). The viewport clamps stale offsets to the current
// content, so a diff that shrank — e.g. after a watch refresh — lands at the
// bottom rather than out of bounds. Must run after the content is set so the
// clamp sees the new line count.
func (m *Model) restoreScroll(key string) {
	m.vp.SetYOffset(m.scrollOffsets[key])
}

type deltaRenderer struct {
	run func(context.Context, []string, func(io.Writer) error) ([]byte, error)
}

func defaultDeltaRenderer() deltaRenderer {
	return deltaRenderer{run: runDelta}
}

func (r deltaRenderer) Run(
	ctx context.Context,
	args []string,
	writeInput func(io.Writer) error,
) ([]byte, error) {
	if r.run == nil {
		return runDelta(ctx, args, writeInput)
	}
	return r.run(ctx, args, writeInput)
}

type Model struct {
	common.Common
	vp    viewport.Model
	file  *cachedNode
	dir   *cachedNode
	cache nodeCache
	// scrollOffsets remembers the diff viewport's vertical scroll position per
	// node (keyed identically to cache). Switching files/dirs restores the
	// node's last position; first-time visits default to the top. Offsets are
	// kept across cache invalidation (resize, theme, watch refresh) and the
	// viewport clamps any stale value to the new content's range.
	scrollOffsets map[string]int
	// renderer is injectable so tests can assert render lifecycle behavior
	// without launching the external delta binary.
	renderer deltaRenderer
	// cancelRender stops the currently running delta process, if any.
	cancelRender context.CancelFunc
	// Monotonic render generation used to drop stale async results.
	renderID   uint64
	sideBySide bool
	themeMode  themeMode
	// nil means unknown (leave delta behavior unchanged).
	isDarkBackground *bool
	preamble         string
	sel              selection
	// gutterCol is the visual column of the side-by-side center divider.
	// -1 means "no column constraint" (unified mode or not yet detected).
	gutterCol int
	// leftContentCol / rightContentCol are the first visual column inside
	// each side's content area (i.e. just past the post-line-number "│"
	// border). -1 means "undetected" — selections then fall back to the
	// full half band starting at 0 / gutterCol+1.
	leftContentCol  int
	rightContentCol int
	// testHookSelectedTextPanic, if non-nil, is called at the start of
	// selectedTextInner to test the panic recovery path. Not for production.
	testHookSelectedTextPanic func()
}

// SetPreamble stores the preamble text (e.g. commit metadata from git show).
func (m *Model) SetPreamble(preamble string) {
	m.preamble = preamble
}

type themeMode uint8

const (
	themeAuto themeMode = iota
	themeLight
	themeDark
)

func New(sideBySide bool, theme string) Model {
	m := Model{
		vp:              viewport.Model{},
		sideBySide:      sideBySide,
		cache:           map[string]*cachedNode{},
		scrollOffsets:   map[string]int{},
		renderer:        defaultDeltaRenderer(),
		themeMode:       parseThemeMode(theme),
		gutterCol:       -1,
		leftContentCol:  -1,
		rightContentCol: -1,
	}
	switch m.themeMode {
	case themeLight:
		isDark := false
		m.isDarkBackground = &isDark
	case themeDark:
		isDark := true
		m.isDarkBackground = &isDark
	}
	return m
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	cmds := make([]tea.Cmd, 0)
	switch msg := msg.(type) {
	case diffContentMsg:
		if msg.renderID != m.renderID {
			break
		}
		m.cancelRender = nil
		// Clear before gutter detection so a mid-drag content swap leaves
		// sel.active == false; the next MouseMotionMsg is a no-op until the
		// next click.
		m.sel = selection{}
		// Wrap long lines so unified delta output (which delta does not wrap
		// itself) folds at the viewport edge instead of overflowing. The wrap
		// uses delta's wrap-left-symbol '↵', so joinWrappedLines collapses
		// the resulting rows back to one logical line on selection/copy.
		diff := wrapLongLines(msg.text, m.vp.Width())
		if _, ok := m.cache[msg.cacheKey]; ok {
			m.cache[msg.cacheKey].diff = diff
			m.cache[msg.cacheKey].ready = true
		}
		m.vp.SetContent(diff)
		m.refreshColumnDetection(diff)
		// The renderID guard above means this content belongs to the node the
		// user is currently viewing, so restore its remembered scroll position
		// now that the line count (and thus the clamp range) is known.
		m.restoreScroll(msg.cacheKey)
	}

	vp, vpCmd := m.vp.Update(msg)
	cmds = append(cmds, vpCmd)
	m.vp = vp

	return m, tea.Batch(cmds...)
}

// refreshColumnDetection re-runs gutter and per-side content column detection
// on the supplied diff content (typically the freshly rendered or cached
// payload). Keeping this in one place ensures every code path that swaps the
// viewport content also refreshes the selection bands — without this, loading
// a cached file or toggling between unified and side-by-side via cached
// content would leave stale column metadata in place and make the right side
// effectively unselectable.
//
// All three values stay at -1 unless we can detect *both* a gutter divider
// AND the post-line-number borders on each side. Delta renders new/deleted
// files as unified even when --side-by-side is requested, which would
// otherwise leave gutterCol stuck at the vpWidth/2 fallback and split every
// selection in half over empty space.
//
// Robustness: a defer/recover guard wraps the parse so a future delta layout
// change can't crash the whole TUI — on any panic we leave the columns at
// -1 and the selection falls back to the unified [0, MaxInt) band.
func (m *Model) refreshColumnDetection(content string) {
	m.gutterCol = -1
	m.leftContentCol = -1
	m.rightContentCol = -1
	if !m.sideBySide || m.vp.Width() <= 0 {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			log.Warn(
				"diffviewer: column detection panicked; falling back to unified selection band",
				"panic",
				r,
			)
			m.gutterCol = -1
			m.leftContentCol = -1
			m.rightContentCol = -1
		}
	}()
	stripped := ansi.Strip(content)
	g := detectGutterColFunc(stripped, m.vp.Width())
	l, r := detectSideContentColsFunc(stripped, g)
	// Strict plausibility check — beyond the obvious negative-value guards,
	// reject any layout where the detected columns don't form a coherent
	// (leadingBorder=0 < leftContent < gutter < rightContent < vpWidth)
	// sequence. Anything outside that window means delta produced something
	// we don't understand; treat it as "unparseable" rather than risk
	// clamping selections to nonsense ranges.
	w := m.vp.Width()
	if g <= 0 || g >= w || l <= 0 || l >= g || r <= g || r >= w {
		return
	}
	m.gutterCol = g
	m.leftContentCol = l
	m.rightContentCol = r
}

const scrollbarWidth = 3 // 1 space + 1 scrollbar character + 1 padding

func (m Model) View() string {
	vpView := m.vp.View()
	if m.sel.active || m.sel.has {
		vpView = m.applyHighlight(vpView)
	}
	scrollbar := common.RenderScrollbar(m.vp.Height(), m.vp.TotalLineCount(), m.vp.YOffset())
	if scrollbar != "" {
		vpView = lipgloss.JoinHorizontal(lipgloss.Top, vpView, " ", scrollbar)
	}
	return lipgloss.JoinVertical(lipgloss.Left, m.headerView(), vpView)
}

// applyHighlight overlays SGR-reverse video on the visible viewport rows that
// fall inside the active or finalized selection. Returns the input unchanged
// when the selection is fully off-screen.
//
// A defer/recover guard protects the live render path from any panic that a
// malformed line or unexpected delta output could trigger inside the slice
// and ansi.Cut operations below — on failure the unhighlighted view is
// returned so the TUI keeps working.
func (m Model) applyHighlight(vpView string) (out string) {
	out = vpView
	defer func() {
		if r := recover(); r != nil {
			log.Warn(
				"diffviewer: applyHighlight panicked; rendering unhighlighted view",
				"panic",
				r,
			)
			out = vpView
		}
	}()
	start, end := m.sel.normalized()
	top := m.vp.YOffset()
	bottom := top + m.vp.Height()
	if end.Line < top || start.Line >= bottom {
		return vpView
	}
	visible := strings.Split(vpView, "\n")
	for screenRow, line := range visible {
		contentLine := top + screenRow
		if contentLine < start.Line || contentLine > end.Line {
			continue
		}
		w := lipgloss.Width(line)
		a, b := highlightRange(contentLine, start, end, w)
		a, b = clampToBand(a, b, m.sel.colBand)
		a, b = clampToLine(a, b, w)
		if a >= b {
			continue
		}
		visible[screenRow] = spliceReverseFunc(line, a, b, w)
	}
	return strings.Join(visible, "\n")
}

func (m *Model) SetSize(width, height int) tea.Cmd {
	m.Common.Width = width
	m.Common.Height = height
	m.vp.SetWidth(m.contentWidth())
	m.vp.SetHeight(m.Common.Height - DirHeaderHeight)
	// ClearCache snapshots the live scroll position before invalidating the
	// render, so the reflowed content lands at the same place (clamped to fit).
	m.ClearCache()
	return m.diff()
}

func (m Model) contentWidth() int {
	return m.Common.Width - scrollbarWidth
}

// Height returns the diff viewport's visible row count (excluding the
// dir-header band). Mouse-edge logic uses this to detect dragging past the
// pane edge.
func (m *Model) Height() int {
	return m.vp.Height()
}

// Width returns the diff viewport's content column count (excluding the
// scrollbar gutter).
func (m *Model) Width() int {
	return m.vp.Width()
}

func (m *Model) startRender() (context.Context, uint64) {
	m.cancelPendingRender()
	ctx, cancel := context.WithCancel(context.Background())
	m.cancelRender = cancel
	m.renderID++
	return ctx, m.renderID
}

func (m *Model) cancelPendingRender() {
	if m.cancelRender != nil {
		m.cancelRender()
		m.cancelRender = nil
		m.renderID++
	}
}

func (m *Model) diff() tea.Cmd {
	if m.themeMode == themeAuto && m.isDarkBackground == nil {
		return nil
	}

	if m.file != nil {
		key := cacheKey(m.file.path, m.sideBySide)
		if cached, ok := m.cache[key]; ok && cacheReady(cached) {
			m.cancelPendingRender()
			m.file = cached
			m.vp.SetContent(cached.diff)
			m.refreshColumnDetection(cached.diff)
			m.restoreScroll(key)
			return nil
		}
		node := &cachedNode{
			path:      m.file.path,
			files:     m.file.files,
			additions: m.file.additions,
			deletions: m.file.deletions,
		}
		m.file = node
		m.cache[key] = node
		ctx, renderID := m.startRender()
		return diffFile(
			ctx,
			m.renderer,
			node,
			m.contentWidth(),
			m.sideBySide,
			m.deltaThemeArgs(),
			renderID,
		)
	} else if m.dir != nil {
		key := cacheKey(m.dir.path, m.sideBySide)
		if cached, ok := m.cache[key]; ok && cacheReady(cached) {
			m.cancelPendingRender()
			m.dir = cached
			m.vp.SetContent(cached.diff)
			m.refreshColumnDetection(cached.diff)
			m.restoreScroll(key)
			return nil
		}
		node := &cachedNode{
			path:      m.dir.path,
			files:     m.dir.files,
			additions: m.dir.additions,
			deletions: m.dir.deletions,
		}
		m.dir = node
		m.cache[key] = node
		preamble := ""
		if m.dir.path == "/" {
			preamble = m.preamble
		}
		ctx, renderID := m.startRender()
		return diffDir(
			ctx,
			m.renderer,
			node,
			m.contentWidth(),
			m.sideBySide,
			m.deltaThemeArgs(),
			common.SelectionColor(common.Selected, m.isDarkBackground),
			preamble,
			renderID,
		)
	}

	return nil
}

func (m Model) headerView() string {
	if m.dir != nil {
		return m.dirHeaderView()
	}

	if m.file == nil || len(m.file.files) != 1 {
		return ""
	}
	name := m.file.path
	base := lipgloss.NewStyle()

	fileIcon := icons.GetIcon(name, false)
	prefix := base.Render(fileIcon) + base.Render(" ")
	name = utils.TruncateString(name, m.Common.Width-lipgloss.Width(prefix))
	top := prefix + base.Bold(true).Render(name)

	bottom := filenode.ViewFileDiffStats(m.file.files[0], base)

	return base.
		Width(m.Common.Width).
		Height(DirHeaderHeight - 1).
		BorderStyle(lipgloss.NormalBorder()).
		BorderBottom(true).
		BorderForeground(lipgloss.Color("8")).
		Render(lipgloss.JoinVertical(lipgloss.Left, top, bottom))
}

func (m Model) dirHeaderView() string {
	base := lipgloss.NewStyle().Foreground(lipgloss.Blue)
	prefix := base.Render(" ")
	name := utils.TruncateString(m.dir.path, m.Common.Width-lipgloss.Width(prefix))

	top := prefix + base.Bold(true).Render(name)
	bottom := filenode.ViewDiffStats(m.dir.additions, m.dir.deletions, base)
	return base.
		Width(m.Common.Width).
		Height(DirHeaderHeight - 1).
		BorderStyle(lipgloss.NormalBorder()).
		BorderBottom(true).
		BorderForeground(lipgloss.Color("8")).
		Render(lipgloss.JoinVertical(lipgloss.Left, top, bottom))
}

func (m Model) SetFilePatch(file *gitdiff.File) (Model, tea.Cmd) {
	m.rememberScroll()
	m.sel = selection{}
	m.dir = nil

	fname := filenode.GetFileName(file)
	key := cacheKey(fname, m.sideBySide)
	if cached, ok := m.cache[key]; ok && cacheReady(cached) {
		m.cancelPendingRender()
		m.file = cached
		m.vp.SetContent(cached.diff)
		m.refreshColumnDetection(cached.diff)
		m.restoreScroll(key)
		return m, nil
	}

	files := make([]*gitdiff.File, 1)
	files[0] = file
	additions, deletions := filenode.DiffStats(file)
	m.file = &cachedNode{
		path:      fname,
		files:     files,
		additions: additions,
		deletions: deletions,
	}
	m.cache[key] = m.file

	cmd := m.diff()
	return m, cmd
}

func (m Model) SetDirPatch(dirPath string, files []*gitdiff.File) (Model, tea.Cmd) {
	m.rememberScroll()
	m.sel = selection{}
	m.file = nil

	key := cacheKey(dirPath, m.sideBySide)
	if cached, ok := m.cache[key]; ok && cacheReady(cached) {
		m.cancelPendingRender()
		m.dir = cached
		m.vp.SetContent(cached.diff)
		m.refreshColumnDetection(cached.diff)
		m.restoreScroll(key)
		return m, nil
	}

	var added, deleted int64
	for _, file := range files {
		na, nd := filenode.DiffStats(file)
		added += na
		deleted += nd
	}
	m.dir = &cachedNode{
		path:      dirPath,
		files:     files,
		additions: added,
		deletions: deleted,
	}
	m.cache[key] = m.dir
	cmd := m.diff()
	return m, cmd
}

// SetSideBySide updates the diff view mode and re-renders.
func (m *Model) SetSideBySide(sideBySide bool) tea.Cmd {
	// Snapshot the current layout's scroll before the key changes (the cache
	// key carries a side-by-side suffix), so toggling back returns to where
	// the user left each layout.
	m.rememberScroll()
	m.sideBySide = sideBySide
	return m.diff()
}

// SetContent loads a string directly into the underlying viewport, bypassing
// the delta render pipeline. Intended for tests that need scrollable content
// without invoking the external delta binary.
func (m *Model) SetContent(s string) {
	m.vp.SetContent(s)
}

// ScrollUp scrolls the viewport up by the given number of lines.
func (m *Model) ScrollUp(lines int) {
	m.vp.ScrollUp(lines)
}

// ScrollDown scrolls the viewport down by the given number of lines.
func (m *Model) ScrollDown(lines int) {
	m.vp.ScrollDown(lines)
}

// ScrollBottom scrolls the viewport to the bottom.
func (m *Model) ScrollBottom() {
	m.vp.GotoBottom()
}

// ScrollTop scrolls the viewport to its top.
func (m *Model) ScrollTop() {
	m.vp.GotoTop()
}

// YOffset returns the viewport's current top content row.
func (m *Model) YOffset() int {
	return m.vp.YOffset()
}

// TotalLineCount returns the number of content lines currently loaded into the
// viewport.
func (m *Model) TotalLineCount() int {
	return m.vp.TotalLineCount()
}

// StartSelection begins a new selection anchored at p. Derives colBand from
// p.Col, m.gutterCol, and the detected per-side content columns. The band
// excludes the leading border, line numbers, and post-LN border on whichever
// side the click landed.
//
// Lines without the standard side-by-side layout — hunk-header boxes
// ("60: struct …"), filename decorations, separators — opt out of side-
// specific clamping so the user can select content that lives outside the
// post-line-number borders. Sets active=true, has=false. Any prior selection
// is replaced.
func (m *Model) StartSelection(p Point) {
	var lo, hi int
	sbs := m.gutterCol > 0 && m.isSBSContentLine(p.Line)
	switch {
	case !sbs:
		// Unified mode, undetected, or click on a non-SBS line (hunk header,
		// filename decoration, separator): no column constraint.
		lo, hi = 0, math.MaxInt
	case p.Col > m.gutterCol:
		lo = m.gutterCol + 1
		if m.rightContentCol > lo {
			lo = m.rightContentCol
		}
		hi = math.MaxInt
	default:
		// Left side (p.Col < gutterCol) or directly on the divider — snap left.
		lo = 0
		if m.leftContentCol > 0 {
			lo = m.leftContentCol
		}
		hi = m.gutterCol
	}
	if p.Col < lo {
		p.Col = lo
	}
	if p.Col >= hi {
		p.Col = hi - 1
	}
	m.sel = selection{
		anchor:  p,
		head:    p,
		colBand: [2]int{lo, hi},
		active:  true,
	}
}

// isSBSContentLine reports whether the cached diff line at `line` matches the
// side-by-side structure (leading "│" border at col 0). Hunk-header boxes,
// filename decorations, separators, and other delta chrome do not start with
// "│" and are treated as plain text for selection clamping purposes.
func (m *Model) isSBSContentLine(line int) bool {
	if line < 0 {
		return false
	}
	var src string
	switch {
	case m.file != nil:
		src = m.file.diff
	case m.dir != nil:
		src = m.dir.diff
	}
	if src == "" {
		return false
	}
	lines := strings.Split(src, "\n")
	if line >= len(lines) {
		return false
	}
	return strings.HasPrefix(ansi.Strip(lines[line]), "│")
}

// ExtendSelection moves the selection head to p, clamping p.Col into the
// active colBand. The line axis is never clamped — vertical drag spans the
// full content. No-op when no drag is active.
func (m *Model) ExtendSelection(p Point) {
	if !m.sel.active {
		return
	}
	lo, hi := m.sel.colBand[0], m.sel.colBand[1]
	if p.Col < lo {
		p.Col = lo
	}
	if p.Col >= hi {
		// colBand is half-open [lo, hi); the last valid head column is hi-1.
		p.Col = hi - 1
	}
	m.sel.head = p
}

// EndSelection finalizes an active selection. If the head never moved away
// from the anchor it returns ("", false) with the selection cleared.
// Otherwise it returns (selectedText, true) and leaves the finalized
// highlight in place until the next event clears it.
func (m *Model) EndSelection() (string, bool) {
	if !m.sel.active {
		return "", false
	}
	m.sel.active = false
	if m.sel.anchor == m.sel.head {
		m.sel.has = false
		return "", false
	}
	m.sel.has = true
	return m.selectedText(), true
}

// ClearSelection drops all selection state.
func (m *Model) ClearSelection() {
	m.sel = selection{}
}

// HasSelection reports whether a finalized non-empty selection exists.
func (m *Model) HasSelection() bool {
	return m.sel.has
}

// IsSelecting reports whether a drag is currently in progress.
func (m *Model) IsSelecting() bool {
	return m.sel.active
}

// GutterCol returns the detected side-by-side divider column, or -1 for
// unified mode / undetected.
func (m *Model) GutterCol() int {
	return m.gutterCol
}

// DebugSelection exposes the current selection's anchor/head/band for tests.
// Only intended for diagnostics — not part of any public contract.
func (m *Model) DebugSelection() (anchor, head Point, band [2]int, active bool) {
	return m.sel.anchor, m.sel.head, m.sel.colBand, m.sel.active
}

// selectedText extracts the ANSI-stripped plaintext under the current
// selection from the cached diff content. Returns "" when no content is
// loaded or the relevant cached node has no diff text.
//
// A defer/recover guard turns any unexpected panic in the ANSI-cut path
// into an empty result rather than crashing the TUI on clipboard copy.
func (m *Model) selectedText() (result string) {
	defer func() {
		if r := recover(); r != nil {
			log.Warn("diffviewer: selectedText panicked; copying empty string", "panic", r)
			result = ""
		}
	}()
	return m.selectedTextInner()
}

func (m *Model) selectedTextInner() string {
	if m.testHookSelectedTextPanic != nil {
		m.testHookSelectedTextPanic()
	}
	var src string
	switch {
	case m.file != nil:
		src = m.file.diff
	case m.dir != nil:
		src = m.dir.diff
	default:
		return ""
	}
	if src == "" {
		return ""
	}
	start, end := m.sel.normalized()
	lines := strings.Split(src, "\n")
	if start.Line < 0 || start.Line >= len(lines) {
		return ""
	}
	endLine := end.Line
	if endLine >= len(lines) {
		endLine = len(lines) - 1
	}
	out := make([]string, 0, endLine-start.Line+1)
	for i := start.Line; i <= endLine; i++ {
		line := lines[i]
		w := lipgloss.Width(line)
		a, b := highlightRange(i, start, end, w)
		a, b = clampToBand(a, b, m.sel.colBand)
		a, b = clampToLine(a, b, w)
		if a >= b {
			out = append(out, "")
			continue
		}
		out = append(out, ansi.Strip(ansi.Cut(line, a, b)))
	}
	return joinWrappedLines(out)
}

func diffFile(
	ctx context.Context,
	renderer deltaRenderer,
	node *cachedNode,
	width int,
	sideBySide bool,
	themeArgs []string,
	renderID uint64,
) tea.Cmd {
	if width <= 0 || node == nil || len(node.files) != 1 {
		return nil
	}

	file := node.files[0]
	key := cacheKey(node.path, sideBySide)
	return func() tea.Msg {
		// Only use side-by-side if preference is true AND file is not new/deleted
		useSideBySide := sideBySide && !file.IsNew && !file.IsDelete
		args := deltaArgs(width, useSideBySide, themeArgs, nil)
		out, err := renderer.Run(ctx, args, func(w io.Writer) error {
			if _, err := io.WriteString(w, file.String()); err != nil {
				return err
			}
			_, err := io.WriteString(w, "\n")
			return err
		})
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return common.ErrMsg{Err: err}
		}
		return diffContentMsg{cacheKey: key, text: string(out), renderID: renderID}
	}
}

func diffDir(
	ctx context.Context,
	renderer deltaRenderer,
	dir *cachedNode,
	width int,
	sideBySide bool,
	themeArgs []string,
	selectedBg color.Color,
	preamble string,
	renderID uint64,
) tea.Cmd {
	if width <= 0 || dir == nil {
		return nil
	}
	key := cacheKey(dir.path, sideBySide)
	return func() tea.Msg {
		s := lipgloss.NewStyle().Background(selectedBg)
		c := common.LipglossColorToHex(selectedBg)
		useSideBySide := sideBySide
		dirArgs := []string{
			fmt.Sprintf("--file-modified-label=%s",
				utils.RemoveReset(s.Foreground(lipgloss.Yellow).Render(" "))),
			fmt.Sprintf("--file-removed-label=%s",
				utils.RemoveReset(s.Foreground(lipgloss.Red).Render(" "))),
			fmt.Sprintf("--file-added-label=%s",
				utils.RemoveReset(s.Foreground(lipgloss.Green).Render(" "))),
			fmt.Sprintf("--file-style='%s bold %s'", c, c),
			fmt.Sprintf("--file-decoration-style='%s box %s'", c, c),
		}
		args := deltaArgs(width, useSideBySide, themeArgs, dirArgs)
		out, err := renderer.Run(ctx, args, func(w io.Writer) error {
			for _, file := range dir.files {
				if _, err := io.WriteString(w, file.String()); err != nil {
					return err
				}
			}
			_, err := io.WriteString(w, "\n")
			return err
		})
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return common.ErrMsg{Err: err}
		}

		text := string(out)
		if preamble != "" {
			text = renderPreamble(preamble) + "\n" + text
		}
		return diffContentMsg{cacheKey: key, text: text, renderID: renderID}
	}
}

func deltaArgs(width int, sideBySide bool, themeArgs, extraArgs []string) []string {
	args := []string{
		"--paging=never",
		fmt.Sprintf("-w=%d", width),
		fmt.Sprintf("--max-line-length=%d", deltaMaxLineLength(width, sideBySide)),
	}
	args = append(args, extraArgs...)
	args = append(args, themeArgs...)
	if sideBySide {
		args = append(args, "--side-by-side")
	}
	return args
}

func deltaMaxLineLength(width int, sideBySide bool) int {
	if sideBySide && width > 0 && width < maxDeltaLineLength {
		return width
	}
	return maxDeltaLineLength
}

// newDeltaCmd creates the *exec.Cmd for the delta binary. Injected so tests
// can substitute commands that exercise hard-to-reach error paths.
var newDeltaCmd = defaultNewDeltaCmd

func defaultNewDeltaCmd(ctx context.Context, args []string) *exec.Cmd {
	return exec.CommandContext(ctx, "delta", args...)
}

// stdinPipeFunc wraps (*exec.Cmd).StdinPipe so tests can inject a failing stub
// to exercise the StdinPipe error path.
var stdinPipeFunc = defaultStdinPipeFunc

func defaultStdinPipeFunc(c *exec.Cmd) (io.WriteCloser, error) {
	return c.StdinPipe()
}

func runDelta(
	ctx context.Context,
	args []string,
	writeInput func(io.Writer) error,
) ([]byte, error) {
	deltac := newDeltaCmd(ctx, args)
	deltac.Env = os.Environ()

	stdin, err := stdinPipeFunc(deltac)
	if err != nil {
		return nil, err
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	deltac.Stdout = &stdout
	deltac.Stderr = &stderr

	if err := deltac.Start(); err != nil {
		return nil, err
	}

	writeErr := make(chan error, 1)
	go func() {
		err := writeInput(stdin)
		if closeErr := stdin.Close(); err == nil {
			err = closeErr
		}
		writeErr <- err
	}()

	waitErr := deltac.Wait()
	stdinErr := <-writeErr
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}
	if stdinErr != nil && waitErr == nil {
		return nil, stdinErr
	}
	if waitErr != nil {
		if stderr.Len() > 0 {
			return nil, fmt.Errorf("%w: %s", waitErr, strings.TrimSpace(stderr.String()))
		}
		return nil, waitErr
	}
	return stdout.Bytes(), nil
}

func (m *Model) SetDarkBackground(isDark bool) tea.Cmd {
	if m.themeMode != themeAuto {
		return nil
	}
	if m.isDarkBackground != nil && *m.isDarkBackground == isDark {
		return nil
	}
	m.isDarkBackground = &isDark
	m.rememberScroll()
	m.cache = make(nodeCache)
	return m.diff()
}

func (m Model) deltaThemeArgs() []string {
	if m.isDarkBackground == nil || *m.isDarkBackground {
		return nil
	}

	// Let delta drive the light-mode colors. Its --light defaults use named
	// ANSI colors that terminals render natively, which avoids the grey
	// downsampling we'd hit if we sent desaturated 24-bit hex like #ffebe9
	// through a piped subprocess.
	return []string{
		"--light",
		"--syntax-theme=GitHub",
	}
}

func parseThemeMode(v string) themeMode {
	switch config.NormalizeTheme(v) {
	case config.ThemeLight:
		return themeLight
	case config.ThemeDark:
		return themeDark
	default:
		return themeAuto
	}
}

func renderPreamble(preamble string) string {
	preamble = strings.TrimSpace(preamble)
	if preamble == "" {
		return ""
	}

	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	yellow := lipgloss.NewStyle().Foreground(lipgloss.Yellow)

	var out []string
	for _, line := range strings.Split(preamble, "\n") {
		switch {
		case strings.HasPrefix(line, "commit "):
			out = append(
				out,
				dim.Render("commit ")+yellow.Render(strings.TrimPrefix(line, "commit ")),
			)
		case strings.HasPrefix(line, "Author:"),
			strings.HasPrefix(line, "AuthorDate:"),
			strings.HasPrefix(line, "Date:"),
			strings.HasPrefix(line, "Commit:"),
			strings.HasPrefix(line, "CommitDate:"),
			strings.HasPrefix(line, "Merge:"):
			out = append(out, dim.Render(line))
		default:
			out = append(out, line)
		}
	}

	return strings.Join(out, "\n")
}

type diffContentMsg struct {
	cacheKey string
	text     string
	renderID uint64
}

func (m *Model) ClearCache() {
	// Snapshot the current node's scroll before discarding renders so the
	// position survives the re-render (resize, watch refresh). Remembered
	// offsets intentionally outlive the cache; restoreScroll clamps any value
	// that no longer fits the new content.
	m.rememberScroll()
	m.cancelPendingRender()
	m.cache = make(nodeCache)
}

func (m *Model) RootDiffStats() (int64, int64) {
	if item, ok := m.cache[cacheKey("/", m.sideBySide)]; ok {
		return item.additions, item.deletions
	}

	return 0, 0
}
