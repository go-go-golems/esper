package monitor

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type resetConfirmOverlay struct {
	sz size

	confirm bool // false: cancel (safe default), true: confirm
}

func newResetConfirmOverlay() *resetConfirmOverlay {
	return &resetConfirmOverlay{confirm: false}
}

func (o *resetConfirmOverlay) setSize(sz size) {
	o.sz = sz
}

func (o *resetConfirmOverlay) open() {
	o.confirm = false
}

func (o *resetConfirmOverlay) Update(msg tea.Msg) (overlayModel, tea.Cmd, overlayOutcome) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return o, nil, overlayOutcome{}
	}

	switch k.Type {
	case tea.KeyEsc:
		return o, nil, overlayOutcome{close: true}
	case tea.KeyLeft, tea.KeyRight, tea.KeyTab:
		o.confirm = !o.confirm
		return o, nil, overlayOutcome{}
	case tea.KeyEnter:
		if !o.confirm {
			return o, nil, overlayOutcome{close: true}
		}
		return o, nil, overlayOutcome{close: true, forward: resetDeviceMsg{}}
	}

	switch k.String() {
	case "h", "l":
		o.confirm = !o.confirm
	case "y", "Y":
		o.confirm = true
		return o, nil, overlayOutcome{close: true, forward: resetDeviceMsg{}}
	case "n", "N":
		o.confirm = false
		return o, nil, overlayOutcome{close: true}
	case "q":
		return o, nil, overlayOutcome{close: true}
	}

	return o, nil, overlayOutcome{}
}

func (o *resetConfirmOverlay) View(st styles) string {
	boxW := max(50, min(78, o.sz.W-6))
	boxH := 11

	innerW := max(0, boxW-st.OverlayBox.GetHorizontalFrameSize())

	title := st.PanelTitle.Render("Reset Device")
	body := lipgloss.NewStyle().Width(innerW).Render(
		"Reset the connected device?\n\n" +
			st.Hint.Render("This may reboot the firmware and interrupt logs."),
	)

	cancel := st.Row.Render("[ Cancel ]")
	reset := st.Row.Render("[ Reset ]")
	if o.confirm {
		reset = st.SelectedRow.Render("[ Reset ]")
	} else {
		cancel = st.SelectedRow.Render("[ Cancel ]")
	}

	actions := lipgloss.JoinHorizontal(lipgloss.Top,
		cancel,
		"   ",
		reset,
	)
	actions = lipgloss.PlaceHorizontal(innerW, lipgloss.Right, actions)

	help := st.Hint.Render(fmt.Sprintf("%-20s%s", "Enter select", "Esc cancel"))

	content := lipgloss.JoinVertical(lipgloss.Left, title, "", body, "", actions, "", help)
	return st.OverlayBox.Copy().Width(boxW).Height(boxH).Render(content)
}
