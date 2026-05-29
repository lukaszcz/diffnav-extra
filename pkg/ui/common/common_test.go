package common

import (
	"errors"
	"image/color"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// --- component.go tests ---

func TestCommonStructFields(t *testing.T) {
	c := Common{Width: 80, Height: 24}
	if c.Width != 80 {
		t.Errorf("expected Width 80, got %d", c.Width)
	}
	if c.Height != 24 {
		t.Errorf("expected Height 24, got %d", c.Height)
	}
}

// mockComponent implements the Component interface for testing.
type mockComponent struct {
	width, height int
}

func (m *mockComponent) SetSize(width, height int) tea.Cmd {
	m.width = width
	m.height = height
	return nil
}

func TestComponentInterface(t *testing.T) {
	var c Component = &mockComponent{}
	cmd := c.SetSize(100, 50)
	if cmd != nil {
		t.Errorf("expected nil cmd, got %v", cmd)
	}
	mc := c.(*mockComponent)
	if mc.width != 100 {
		t.Errorf("expected width 100, got %d", mc.width)
	}
	if mc.height != 50 {
		t.Errorf("expected height 50, got %d", mc.height)
	}
}

// --- msgs.go tests ---

func TestErrMsg(t *testing.T) {
	err := errors.New("something went wrong")
	e := ErrMsg{Err: err}
	if e.Err == nil {
		t.Fatal("expected non-nil Err")
	}
	if e.Err.Error() != "something went wrong" {
		t.Errorf("expected 'something went wrong', got '%s'", e.Err.Error())
	}
}

// --- scrollbar.go tests ---

// stripANSI removes common ANSI escape sequences for simpler content inspection.
func stripANSI(s string) string {
	var out strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && ((s[j] >= '0' && s[j] <= '9') || s[j] == ';') {
				j++
			}
			if j < len(s) {
				j++ // skip the terminating letter
			}
			i = j
		} else {
			out.WriteByte(s[i])
			i++
		}
	}
	return out.String()
}

func TestRenderScrollbarAllFits(t *testing.T) {
	got := RenderScrollbar(10, 5, 0)
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestRenderScrollbarExactFit(t *testing.T) {
	got := RenderScrollbar(10, 10, 0)
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestRenderScrollbarOverflowYOffsetZero(t *testing.T) {
	viewHeight := 10
	totalLines := 100
	yOffset := 0

	got := RenderScrollbar(viewHeight, totalLines, yOffset)
	cleaned := stripANSI(got)

	// thumbSize = max(1, 10*10/100) = 1, scrollableLines=90
	// thumbPos = 0*(10-1)/90 = 0
	thumbCount := strings.Count(cleaned, "┃")
	trackCount := strings.Count(cleaned, "│")
	if thumbCount != 1 {
		t.Errorf("expected 1 thumb char, got %d", thumbCount)
	}
	if thumbCount+trackCount != viewHeight {
		t.Errorf("expected %d total chars, got %d", viewHeight, thumbCount+trackCount)
	}
}

func TestRenderScrollbarEdgeCaseYOffsetPositiveThumbPosZero(t *testing.T) {
	// yOffset > 0 && thumbPos == 0 → thumbPos corrected to 1
	// viewHeight=10, totalLines=20: thumbSize = max(1, 10*10/20) = 5
	// scrollableLines=10, yOffset=1: thumbPos = 1*(10-5)/10 = 0 → corrected to 1
	viewHeight := 10
	totalLines := 20
	yOffset := 1

	got := RenderScrollbar(viewHeight, totalLines, yOffset)
	cleaned := stripANSI(got)

	charCount := strings.Count(cleaned, "┃") + strings.Count(cleaned, "│")
	if charCount != viewHeight {
		t.Errorf("expected %d chars, got %d", viewHeight, charCount)
	}

	// First char should be track since thumbPos is corrected from 0 to 1
	if strings.HasPrefix(cleaned, "┃") {
		t.Error("first character should be track, not thumb (thumbPos corrected from 0 to 1)")
	}
}

func TestRenderScrollbarLargeYOffset(t *testing.T) {
	viewHeight := 10
	totalLines := 100
	yOffset := 50

	got := RenderScrollbar(viewHeight, totalLines, yOffset)
	cleaned := stripANSI(got)

	// thumbSize = max(1, 10*10/100) = 1, scrollableLines=90
	// thumbPos = 50*(10-1)/90 = 5
	charCount := strings.Count(cleaned, "┃") + strings.Count(cleaned, "│")
	if charCount != viewHeight {
		t.Errorf("expected %d chars, got %d", viewHeight, charCount)
	}
	if strings.Count(cleaned, "┃") != 1 {
		t.Errorf("expected 1 thumb char, got %d", strings.Count(cleaned, "┃"))
	}
}

func TestRenderScrollbarThumbAtEnd(t *testing.T) {
	viewHeight := 10
	totalLines := 30
	yOffset := 20

	got := RenderScrollbar(viewHeight, totalLines, yOffset)
	cleaned := stripANSI(got)

	// thumbSize = max(1, 10*10/30) = 3, scrollableLines=20
	// thumbPos = 20*(10-3)/20 = 7
	charCount := strings.Count(cleaned, "┃") + strings.Count(cleaned, "│")
	if charCount != viewHeight {
		t.Errorf("expected %d chars, got %d", viewHeight, charCount)
	}
	if strings.Count(cleaned, "┃") != 3 {
		t.Errorf("expected 3 thumb chars, got %d", strings.Count(cleaned, "┃"))
	}
}

func TestRenderScrollbarSmallOverflow(t *testing.T) {
	viewHeight := 5
	totalLines := 6
	yOffset := 0

	got := RenderScrollbar(viewHeight, totalLines, yOffset)
	cleaned := stripANSI(got)

	// thumbSize = max(1, 5*5/6) = 4, scrollableLines=1
	// thumbPos = 0*(5-4)/1 = 0
	charCount := strings.Count(cleaned, "┃") + strings.Count(cleaned, "│")
	if charCount != viewHeight {
		t.Errorf("expected %d chars, got %d", viewHeight, charCount)
	}
	if strings.Count(cleaned, "┃") != 4 {
		t.Errorf("expected 4 thumb chars, got %d", strings.Count(cleaned, "┃"))
	}
}

func TestRenderScrollbarNewlineCount(t *testing.T) {
	viewHeight := 4
	totalLines := 40
	yOffset := 0

	got := RenderScrollbar(viewHeight, totalLines, yOffset)
	if got == "" {
		t.Fatal("expected non-empty scrollbar")
	}
	// Should have viewHeight-1 newlines between the viewHeight visual lines
	newlineCount := strings.Count(got, "\n")
	if newlineCount != viewHeight-1 {
		t.Errorf("expected %d newlines, got %d", viewHeight-1, newlineCount)
	}
}

// --- styles.go tests ---

func TestKeyConstants(t *testing.T) {
	// Verify that every defined Key constant has a corresponding Colors and
	// BgStyles entry, ensuring the mapping is complete.
	allKeys := []Key{Selected, DarkerSelected}
	for _, k := range allKeys {
		if _, ok := Colors[k]; !ok {
			t.Errorf("Colors map missing key %v", k)
		}
		if _, ok := BgStyles[k]; !ok {
			t.Errorf("BgStyles map missing key %v", k)
		}
	}
}

func TestColorsMapKeys(t *testing.T) {
	if _, ok := Colors[Selected]; !ok {
		t.Error("Colors map missing Selected key")
	}
	if _, ok := Colors[DarkerSelected]; !ok {
		t.Error("Colors map missing DarkerSelected key")
	}
}

func TestBgStylesMapKeys(t *testing.T) {
	if _, ok := BgStyles[Selected]; !ok {
		t.Error("BgStyles map missing Selected key")
	}
	if _, ok := BgStyles[DarkerSelected]; !ok {
		t.Error("BgStyles map missing DarkerSelected key")
	}
}

func TestSelectionColorNilIsDarkBackground(t *testing.T) {
	c := SelectionColor(Selected, nil)
	if c == nil {
		t.Fatal("expected non-nil color")
	}
	expected := color.Color(Colors[Selected])
	r1, g1, b1, _ := c.RGBA()
	r2, g2, b2, _ := expected.RGBA()
	if r1 != r2 || g1 != g2 || b1 != b2 {
		t.Errorf("nil isDarkBackground: got (%d,%d,%d), want (%d,%d,%d)", r1, g1, b1, r2, g2, b2)
	}
}

func TestSelectionColorDarkBackground(t *testing.T) {
	dark := true
	c := SelectionColor(Selected, &dark)
	expected := color.Color(Colors[Selected])
	r1, g1, b1, _ := c.RGBA()
	r2, g2, b2, _ := expected.RGBA()
	if r1 != r2 || g1 != g2 || b1 != b2 {
		t.Errorf("dark background: got (%d,%d,%d), want (%d,%d,%d)", r1, g1, b1, r2, g2, b2)
	}
}

func TestSelectionColorDarkBackgroundDarkerSelected(t *testing.T) {
	dark := true
	c := SelectionColor(DarkerSelected, &dark)
	expected := color.Color(Colors[DarkerSelected])
	r1, g1, b1, _ := c.RGBA()
	r2, g2, b2, _ := expected.RGBA()
	if r1 != r2 || g1 != g2 || b1 != b2 {
		t.Errorf(
			"dark background DarkerSelected: got (%d,%d,%d), want (%d,%d,%d)",
			r1,
			g1,
			b1,
			r2,
			g2,
			b2,
		)
	}
}

func TestSelectionColorLightBackgroundSelected(t *testing.T) {
	dark := false
	c := SelectionColor(Selected, &dark)
	hex := LipglossColorToHex(c)
	if hex != "#dadada" {
		t.Errorf("light background Selected: expected #dadada, got %s", hex)
	}
}

func TestSelectionColorLightBackgroundDarkerSelected(t *testing.T) {
	dark := false
	c := SelectionColor(DarkerSelected, &dark)
	hex := LipglossColorToHex(c)
	if hex != "#bcbcbc" {
		t.Errorf("light background DarkerSelected: expected #bcbcbc, got %s", hex)
	}
}

func TestSelectionColorLightBackgroundUnknownKey(t *testing.T) {
	dark := false
	unknownKey := Key(999)
	c := SelectionColor(unknownKey, &dark)
	// Unknown key → default branch returns Colors[999] which is zero-value color.RGBA{}
	hex := LipglossColorToHex(c)
	if hex != "#000000" {
		t.Errorf("unknown key with light background: expected #000000, got %s", hex)
	}
}

func TestSelectionColorNilIsDarkBackgroundDarkerSelected(t *testing.T) {
	c := SelectionColor(DarkerSelected, nil)
	expected := color.Color(Colors[DarkerSelected])
	r1, g1, b1, _ := c.RGBA()
	r2, g2, b2, _ := expected.RGBA()
	if r1 != r2 || g1 != g2 || b1 != b2 {
		t.Errorf(
			"nil isDarkBackground DarkerSelected: got (%d,%d,%d), want (%d,%d,%d)",
			r1,
			g1,
			b1,
			r2,
			g2,
			b2,
		)
	}
}

func TestLipglossColorToHex(t *testing.T) {
	c := color.RGBA{R: 0xAB, G: 0xCD, B: 0xEF, A: 0xFF}
	got := LipglossColorToHex(c)
	if got != "#abcdef" {
		t.Errorf("expected #abcdef, got %s", got)
	}
}

func TestLipglossColorToHexZeroValues(t *testing.T) {
	c := color.RGBA{R: 0, G: 0, B: 0, A: 0xFF}
	got := LipglossColorToHex(c)
	if got != "#000000" {
		t.Errorf("expected #000000, got %s", got)
	}
}

func TestLipglossColorToHexMaxValues(t *testing.T) {
	c := color.RGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
	got := LipglossColorToHex(c)
	if got != "#ffffff" {
		t.Errorf("expected #ffffff, got %s", got)
	}
}

func TestLipglossColorToHexWithLipglossColor(t *testing.T) {
	lc := lipgloss.Color("#ff0000")
	hex := LipglossColorToHex(lc)
	if hex != "#ff0000" {
		t.Errorf("expected #ff0000, got %s", hex)
	}
}

func TestLipglossColorToHexRoundTrip(t *testing.T) {
	// Use Colors[Selected] and verify format
	origColor := Colors[Selected]
	hex := LipglossColorToHex(origColor)
	if len(hex) != 7 || hex[0] != '#' {
		t.Errorf("expected #rrggbb format, got %s", hex)
	}
	// Verify consistency: calling LipglossColorToHex twice yields same result
	hex2 := LipglossColorToHex(origColor)
	if hex != hex2 {
		t.Errorf("round-trip inconsistency: %s vs %s", hex, hex2)
	}
}
