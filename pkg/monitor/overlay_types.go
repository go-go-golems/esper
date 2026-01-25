package monitor

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type monitorOverlayKind int

const (
	monitorOverlayNone monitorOverlayKind = iota
	monitorOverlaySearch
	monitorOverlayFilter
	monitorOverlayPalette
)

type ctrlTPrefixTimeoutMsg struct {
	id int
}

func ctrlTPrefixTimeoutCmd(id int, d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg {
		return ctrlTPrefixTimeoutMsg{id: id}
	})
}
