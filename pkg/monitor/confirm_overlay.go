package monitor

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type confirmOverlayConfig struct {
	title        string
	message      string
	confirmLabel string
	forward      tea.Msg
}

type confirmOverlay struct {
	sz size

	cfg confirmOverlayConfig

	confirm bool // false: cancel (safe default), true: confirm
}

func newConfirmOverlay(cfg confirmOverlayConfig) *confirmOverlay {
	if cfg.confirmLabel == "" {
		cfg.confirmLabel = "Confirm"
	}
	return &confirmOverlay{cfg: cfg, confirm: false}
}

func (o *confirmOverlay) setSize(sz size) { o.sz = sz }
func (o *confirmOverlay) open()           { o.confirm = false }

func (o *confirmOverlay) Update(msg tea.Msg) (overlayModel, tea.Cmd, overlayOutcome) {
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
		return o, nil, overlayOutcome{close: true, forward: o.cfg.forward}
	}

	switch k.String() {
	case "y", "Y":
		o.confirm = true
		return o, nil, overlayOutcome{close: true, forward: o.cfg.forward}
	case "n", "N":
		return o, nil, overlayOutcome{close: true}
	case "q":
		return o, nil, overlayOutcome{close: true}
	}

	return o, nil, overlayOutcome{}
}

func (o *confirmOverlay) View(st styles) string {
	boxW := max(50, min(78, o.sz.W-6))
	boxH := 11
	innerW := max(0, boxW-st.OverlayBox.GetHorizontalFrameSize())

	title := st.PanelTitle.Render(o.cfg.title)
	body := lipgloss.NewStyle().Width(innerW).Render(o.cfg.message)

	cancel := st.Row.Render("[ Cancel ]")
	confirm := st.Row.Render("[ " + o.cfg.confirmLabel + " ]")
	if o.confirm {
		confirm = st.SelectedRow.Render("[ " + o.cfg.confirmLabel + " ]")
	} else {
		cancel = st.SelectedRow.Render("[ Cancel ]")
	}

	actions := lipgloss.JoinHorizontal(lipgloss.Top, cancel, "   ", confirm)
	actions = lipgloss.PlaceHorizontal(innerW, lipgloss.Right, actions)

	help := st.Hint.Render("Enter select   Esc cancel")

	content := lipgloss.JoinVertical(lipgloss.Left, title, "", body, "", actions, "", help)
	return st.OverlayBox.Copy().Width(boxW).Height(boxH).Render(content)
}
