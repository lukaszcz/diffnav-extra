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
			t.Errorf("help view missing column %q on narrow terminal:\n%s", want, view)
		}
	}
}
