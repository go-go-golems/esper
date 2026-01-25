package monitor

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type ctrlTPrefixTimeoutMsg struct {
	id int
}

func ctrlTPrefixTimeoutCmd(id int, d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg {
		return ctrlTPrefixTimeoutMsg{id: id}
	})
}
