package monitor

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
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

func (m helpOverlayModel) Update(msg tea.KeyMsg) (helpOverlayModel, tea.Cmd) {
	// Overlay key handling is done in appModel (Esc/q to close).
	_ = msg
	return m, nil
}

func (m helpOverlayModel) RenderOver(st styles, winW, winH int, background string) string {
	_ = background

	innerW := winW - st.ScreenFrame.GetHorizontalBorderSize()
	innerH := winH - st.ScreenFrame.GetVerticalBorderSize()
	if innerW < 1 || innerH < 1 {
		return ""
	}

	boxW := min(72, max(30, innerW-6))
	boxH := min(18, max(10, innerH-6))

	content := m.helpText(boxW - st.OverlayBox.GetHorizontalBorderSize())
	box := st.OverlayBox.
		Width(boxW).
		Height(boxH).
		Render(content)

	overlay := lipgloss.Place(winW, winH, lipgloss.Center, lipgloss.Center, box)
	return overlay
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
		"",
		"Close help: Esc or q",
	}
	text := strings.Join(lines, "\n")
	return lipgloss.NewStyle().Width(w).Render(text)
}
