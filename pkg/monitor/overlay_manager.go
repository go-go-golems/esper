package monitor

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type overlayOutcome struct {
	close   bool
	forward tea.Msg
}

type overlayModel interface {
	setSize(sz size)
	open()
	Update(msg tea.Msg) (overlayModel, tea.Cmd, overlayOutcome)
	View(st styles) string // returns the overlay box content (not placed)
}

type openOverlayMsg struct {
	overlay overlayModel
}

func openOverlayCmd(overlay overlayModel) tea.Cmd {
	return func() tea.Msg {
		return openOverlayMsg{overlay: overlay}
	}
}

type closeOverlayMsg struct{}

func closeOverlayCmd() tea.Cmd {
	return func() tea.Msg {
		return closeOverlayMsg{}
	}
}

// renderOverlayOver dims the background and places the overlay box centered within the
// given size. This uses lipgloss placement and line replacement (Bubble Tea can't truly
// "layer" strings).
func renderOverlayOver(st styles, sz size, background string, box string) string {
	bg := st.OverlayDim.Render(background)
	ov := lipgloss.Place(sz.W, sz.H, lipgloss.Center, lipgloss.Center, box)

	bgLines := splitLinesN(bg, sz.H)
	ovLines := splitLinesN(ov, sz.H)
	for i := 0; i < sz.H && i < len(bgLines) && i < len(ovLines); i++ {
		if strings.TrimSpace(stripANSI(ovLines[i])) != "" {
			bgLines[i] = ovLines[i]
		}
	}
	return strings.Join(bgLines, "\n")
}
