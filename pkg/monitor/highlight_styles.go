package monitor

import "github.com/charmbracelet/lipgloss"

type highlightStyleKind int

const (
	highlightStyleNone highlightStyleKind = iota
	highlightStyleRedBackground
	highlightStyleYellowUnderline
	highlightStyleGreen
	highlightStyleCyan
	highlightStyleBold
)

type highlightStyleOption struct {
	Label string
	Kind  highlightStyleKind
	Style lipgloss.Style
}

func highlightStyleOptions() []highlightStyleOption {
	return []highlightStyleOption{
		{Label: "none", Kind: highlightStyleNone, Style: lipgloss.NewStyle()},
		{
			Label: "red background",
			Kind:  highlightStyleRedBackground,
			Style: lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Background(lipgloss.Color("1")),
		},
		{
			Label: "yellow underline",
			Kind:  highlightStyleYellowUnderline,
			Style: lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Underline(true),
		},
		{Label: "green", Kind: highlightStyleGreen, Style: lipgloss.NewStyle().Foreground(lipgloss.Color("10"))},
		{Label: "cyan", Kind: highlightStyleCyan, Style: lipgloss.NewStyle().Foreground(lipgloss.Color("14"))},
		{Label: "bold", Kind: highlightStyleBold, Style: lipgloss.NewStyle().Bold(true)},
	}
}

func highlightStyleLabel(kind highlightStyleKind) string {
	for _, opt := range highlightStyleOptions() {
		if opt.Kind == kind {
			return opt.Label
		}
	}
	return "none"
}

func highlightStyleFor(kind highlightStyleKind) lipgloss.Style {
	for _, opt := range highlightStyleOptions() {
		if opt.Kind == kind {
			return opt.Style
		}
	}
	return lipgloss.NewStyle()
}

func highlightStyleNext(kind highlightStyleKind) highlightStyleKind {
	opts := highlightStyleOptions()
	if len(opts) == 0 {
		return kind
	}
	for i, opt := range opts {
		if opt.Kind == kind {
			return opts[(i+1)%len(opts)].Kind
		}
	}
	return opts[0].Kind
}
