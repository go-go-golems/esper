package monitor

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func (m monitorModel) readSerialCmd() tea.Cmd {
	return func() tea.Msg {
		if m.session == nil || m.session.port == nil {
			return serialErrMsg{err: fmt.Errorf("not connected")}
		}
		buf := make([]byte, 4096)
		n, err := m.session.port.Read(buf)
		if err != nil {
			return serialErrMsg{err: err}
		}
		if n <= 0 {
			return serialChunkMsg{b: nil}
		}
		return serialChunkMsg{b: append([]byte{}, buf[:n]...)}
	}
}

func (m monitorModel) tickCmd() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg{t: t}
	})
}

func (m monitorModel) resetDeviceCmd() tea.Cmd {
	return func() tea.Msg {
		if m.session == nil {
			return resetResultMsg{err: fmt.Errorf("not connected")}
		}
		return resetResultMsg{err: m.session.ResetPulse()}
	}
}

func (m monitorModel) sendBreakCmd() tea.Cmd {
	return func() tea.Msg {
		if m.session == nil {
			return sendBreakResultMsg{err: fmt.Errorf("not connected")}
		}
		return sendBreakResultMsg{err: m.session.SendBreak(250 * time.Millisecond)}
	}
}
