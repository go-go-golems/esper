package monitor

import (
	"github.com/charmbracelet/lipgloss"
)

// Color palette for esper TUI
var (
	// Primary accent color (purple/violet)
	colorPrimary = lipgloss.Color("#7D56F4")

	// Semantic colors
	colorError   = lipgloss.Color("#FF5F56") // Red
	colorWarning = lipgloss.Color("#FFBD2E") // Yellow/Orange
	colorSuccess = lipgloss.Color("#27C93F") // Green

	// Neutral colors
	colorDim       = lipgloss.Color("#626262") // Dimmed text
	colorBorder    = lipgloss.Color("#444444") // Subtle border
	colorHighlight = lipgloss.Color("#3A3A5C") // Selection background
	colorTitleBg   = lipgloss.Color("#1E1E2E") // Title bar background
	colorStatusBg  = lipgloss.Color("#181825") // Status bar background
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
	roundedBorder := lipgloss.RoundedBorder()

	return styles{
		ScreenFrame: lipgloss.NewStyle().
			Border(roundedBorder).
			BorderForeground(colorBorder).
			Padding(0, 0),

		TitleBar: lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary),

		StatusBar: lipgloss.NewStyle().
			Foreground(colorDim),

		Panel: lipgloss.NewStyle().
			Border(roundedBorder).
			BorderForeground(colorBorder).
			Padding(0, 1),

		PanelTitle: lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary),

		InputBox: lipgloss.NewStyle().
			Border(roundedBorder).
			BorderForeground(colorBorder).
			Padding(0, 1),

		ErrorBanner: lipgloss.NewStyle().
			Border(lipgloss.ThickBorder()).
			BorderForeground(colorError).
			Foreground(colorError).
			Padding(0, 1),

		InlineError: lipgloss.NewStyle().
			Foreground(colorError),

		Hint: lipgloss.NewStyle().
			Foreground(colorDim),

		OverlayDim: lipgloss.NewStyle().Faint(true),

		OverlayBox: lipgloss.NewStyle().
			Border(roundedBorder).
			BorderForeground(colorPrimary).
			Padding(1, 2),

		OverlayText: lipgloss.NewStyle(),

		SelectedRow: lipgloss.NewStyle().
			Bold(true).
			Background(colorHighlight),

		Row: lipgloss.NewStyle(),
	}
}
