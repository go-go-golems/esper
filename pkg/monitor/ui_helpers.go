package monitor

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// padOrTrim renders a string to exactly width w, padding or truncating as needed.
func padOrTrim(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) > w {
		return lipgloss.NewStyle().Width(w).MaxWidth(w).Render(s)
	}
	return lipgloss.NewStyle().Width(w).Render(s)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// clamp already exists in app_model.go

// splitLinesN splits a string into exactly n lines, padding or truncating as needed.
func splitLinesN(s string, n int) []string {
	lines := strings.Split(s, "\n")
	// lipgloss output sometimes has a trailing newline; drop it if it would add an extra empty row.
	if len(lines) == n+1 && lines[len(lines)-1] == "" {
		lines = lines[:n]
	}
	if len(lines) > n {
		lines = lines[:n]
	}
	for len(lines) < n {
		lines = append(lines, "")
	}
	return lines
}

// stringsJoinVertical joins lines vertically using lipgloss.
func stringsJoinVertical(lines []string) string {
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// splitKeepNewline splits a string by newlines, keeping the newline characters.
func splitKeepNewline(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.SplitAfter(s, "\n")
	// If string doesn't end with newline, last part won't include it.
	// Keep it anyway (viewport may still show partial).
	return parts
}
