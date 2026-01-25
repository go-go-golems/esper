package monitor

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type helpOverlayModel struct {
	sz size
}

func newHelpOverlayModel() helpOverlayModel {
	return helpOverlayModel{}
}

func (m *helpOverlayModel) setSize(sz size) {
	m.sz = sz
}

func (m helpOverlayModel) View(st styles) string {
	innerW, innerH := m.sz.W, m.sz.H
	if innerW < 1 || innerH < 1 {
		return ""
	}

	boxW := min(72, max(30, innerW-6))
	boxH := min(18, max(10, innerH-6))

	boxInnerW := max(0, boxW-st.OverlayBox.GetHorizontalBorderSize())
	boxInnerH := max(0, boxH-st.OverlayBox.GetVerticalBorderSize())

	content := m.helpText(boxInnerW)
	return st.OverlayBox.
		Width(boxInnerW).
		Height(boxInnerH).
		Render(content)
}

func (m helpOverlayModel) helpText(w int) string {
	lines := []string{
		"Help / Keymap",
		"",
		"Global:",
		"  ?        Toggle help",
		"  Ctrl-C   Quit",
		"",
		"Port Picker:",
		"  ↑/↓      Select port",
		"  Tab      Next field",
		"  Enter    Connect",
		"  r        Rescan ports",
		"",
		"Monitor:",
		"  Ctrl-T   Toggle HOST/DEVICE mode",
		"  PgUp/Dn  Scroll (HOST)",
		"  G        Resume follow (DEVICE)",
		"  i        Toggle inspector (HOST)",
		"  Tab      Toggle focus (HOST, inspector open)",
		"",
		"Close help: Esc or q",
	}
	text := strings.Join(lines, "\n")
	return lipgloss.NewStyle().Width(w).Render(text)
}
