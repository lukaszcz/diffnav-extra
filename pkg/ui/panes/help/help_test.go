package help

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

func threeColumnKeys() [][]key.Binding {
	return [][]key.Binding{
		{key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "alphaalpha"))},
		{key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "bravobravo"))},
		{key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "charliecha"))},
	}
}

// On a narrow terminal the help window must still show every column.
func TestAllColumnsVisibleOnNarrowTerminal(t *testing.T) {
	m := New()
	m.SetKeys(threeColumnKeys())
	m.Update(tea.WindowSizeMsg{Width: 54, Height: 24})

	view := m.View()
	for _, want := range []string{"alphaalpha", "bravobravo", "charliecha"} {
		if !strings.Contains(view, want) {
			t.Errorf("help view missing column %q on narrow terminal: %s", want, view)
		}
	}
}

func TestWidthHeightBeforeUpdate(t *testing.T) {
	m := New()
	if m.Width() != 0 {
		t.Errorf("expected initial Width 0, got %d", m.Width())
	}
	if m.Height() != 0 {
		t.Errorf("expected initial Height 0, got %d", m.Height())
	}
}

func TestUpdateSetsWidthHeightFromWindowSize(t *testing.T) {
	m := New()
	m.SetKeys(threeColumnKeys())
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})

	if m.Width() <= 0 {
		t.Errorf("expected Width > 0 after WindowSizeMsg, got %d", m.Width())
	}
	if m.Height() != 12 {
		t.Errorf("expected Height 12 (half of 24), got %d", m.Height())
	}
}

func TestUpdateNonWindowSizeMsg(t *testing.T) {
	m := New()
	m.SetKeys(threeColumnKeys())
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})

	widthBefore := m.Width()
	heightBefore := m.Height()

	cmd := m.Update(tea.KeyPressMsg{})
	if cmd != nil {
		t.Errorf("expected nil cmd for non-WindowSizeMsg, got %v", cmd)
	}
	if m.Width() != widthBefore {
		t.Errorf("Width changed after non-WindowSizeMsg: %d -> %d", widthBefore, m.Width())
	}
	if m.Height() != heightBefore {
		t.Errorf("Height changed after non-WindowSizeMsg: %d -> %d", heightBefore, m.Height())
	}
}

func TestNaturalWidthWithoutKeys(t *testing.T) {
	m := New()
	nw := m.naturalWidth()
	// naturalWidth with no keys set should return 0 (no columns to render)
	if nw != 0 {
		t.Errorf("expected naturalWidth 0 with no keys, got %d", nw)
	}
}

func TestViewWithoutKeys(t *testing.T) {
	m := New()
	view := m.View()
	// Without keys, the view should be empty or contain only whitespace
	if len(strings.TrimSpace(view)) > 0 {
		t.Errorf("expected empty view with no keys, got %q", view)
	}
}

func TestViewWithKeys(t *testing.T) {
	m := New()
	m.SetKeys(threeColumnKeys())
	view := m.View()
	if view == "" {
		t.Error("expected non-empty view with keys set")
	}
	for _, want := range []string{"alphaalpha", "bravobravo", "charliecha"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q: %s", want, view)
		}
	}
}

func TestWidthClampsToAvailable(t *testing.T) {
	m := New()
	m.SetKeys(threeColumnKeys())
	// Use a very wide terminal so naturalWidth() < available
	m.Update(tea.WindowSizeMsg{Width: 1000, Height: 40})

	// Width should be min(naturalWidth, available) = naturalWidth since
	// 1000 - overlayChrome is very large.
	nw := m.naturalWidth()
	if m.Width() != nw {
		t.Errorf("expected Width == naturalWidth (%d) on wide terminal, got %d", nw, m.Width())
	}
}

func TestWidthClampsToZeroOnTinyTerminal(t *testing.T) {
	m := New()
	// Width smaller than overlayChrome: available = max(0, width-overlayChrome) = 0
	m.Update(tea.WindowSizeMsg{Width: 1, Height: 10})

	if m.Width() != 0 {
		t.Errorf("expected Width 0 on tiny terminal, got %d", m.Width())
	}
}

func TestSetKeysOverwrite(t *testing.T) {
	m := New()
	first := threeColumnKeys()
	second := [][]key.Binding{
		{key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "xray"))},
	}

	m.SetKeys(first)
	m.SetKeys(second)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})

	view := m.View()
	if strings.Contains(view, "alphaalpha") {
		t.Error("first key group should be overwritten")
	}
	if !strings.Contains(view, "xray") {
		t.Errorf("second key group missing from view: %s", view)
	}
}
