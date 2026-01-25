package monitor

import (
	"github.com/charmbracelet/lipgloss"
)

type styles struct {
	ScreenFrame lipgloss.Style

	TitleBar  lipgloss.Style
	StatusBar lipgloss.Style

	Panel      lipgloss.Style
	PanelTitle lipgloss.Style

	InputBox lipgloss.Style

	ErrorBanner lipgloss.Style
	InlineError lipgloss.Style
	Hint        lipgloss.Style

	OverlayDim  lipgloss.Style
	OverlayBox  lipgloss.Style
	OverlayText lipgloss.Style

	SelectedRow lipgloss.Style
	Row         lipgloss.Style
}

func defaultStyles() styles {
	border := lipgloss.NormalBorder()

	return styles{
		ScreenFrame: lipgloss.NewStyle().
			Border(border).
			Padding(0, 0),

		TitleBar: lipgloss.NewStyle().Bold(true),
		StatusBar: lipgloss.NewStyle().
			Faint(true),

		Panel: lipgloss.NewStyle().
			Border(border).
			Padding(0, 1),

		PanelTitle: lipgloss.NewStyle().Bold(true),

		InputBox: lipgloss.NewStyle().
			Border(border).
			Padding(0, 1),

		ErrorBanner: lipgloss.NewStyle().
			Border(lipgloss.ThickBorder()).
			Padding(0, 1),

		InlineError: lipgloss.NewStyle().
			Foreground(lipgloss.Color("1")),

		Hint: lipgloss.NewStyle().Faint(true),

		OverlayDim: lipgloss.NewStyle().Faint(true),
		OverlayBox: lipgloss.NewStyle().
			Border(border).
			Padding(1, 2),

		OverlayText: lipgloss.NewStyle(),

		SelectedRow: lipgloss.NewStyle().Bold(true),
		Row:         lipgloss.NewStyle(),
	}
}
