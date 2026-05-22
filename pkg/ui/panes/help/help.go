package help

import (
	helpBubble "charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type Model struct {
	help   helpBubble.Model
	width  int
	height int
	keys   [][]key.Binding
}

func New() Model {
	m := Model{}
	m.help = helpBubble.New()
	helpSt := lipgloss.NewStyle()
	m.help.ShortSeparator = " · "
	m.help.Styles.FullKey = helpSt
	m.help.Styles.FullDesc = helpSt
	m.help.Styles.FullSeparator = helpSt
	m.help.Styles.FullKey = helpSt.Foreground(lipgloss.Blue)
	m.help.Styles.FullDesc = helpSt
	m.help.Styles.FullSeparator = helpSt
	m.help.Styles.Ellipsis = helpSt
	return m
}

// overlayChrome is the horizontal space the surrounding overlay border and
// padding consume (see overlayStyle in the tui package).
const overlayChrome = 8

func (m *Model) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = msg.Height / 2
		// Size to the natural width of all columns, but never wider than the
		// terminal can hold, so every column stays visible even when narrow.
		avail := max(0, msg.Width-overlayChrome)
		m.width = min(m.naturalWidth(), avail)
		m.help.SetWidth(m.width)
	}

	return nil
}

func (m *Model) SetKeys(groups [][]key.Binding) {
	m.keys = groups
}

// naturalWidth measures the width needed to render every help column without
// truncation.
func (m *Model) naturalWidth() int {
	help := m.help
	help.SetWidth(0)
	return lipgloss.Width(help.FullHelpView(m.keys))
}

func (m *Model) Width() int {
	return m.width
}

func (m *Model) Height() int {
	return m.height
}

func (m *Model) View() string {
	return m.help.FullHelpView(m.keys)
}
