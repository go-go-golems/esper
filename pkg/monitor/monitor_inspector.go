package monitor

import (
	"fmt"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func (m *monitorModel) addEvent(kind, title, body string) {
	if body == "" {
		body = title
	}
	m.events = append(m.events, monitorEvent{
		At:    time.Now(),
		Kind:  kind,
		Title: title,
		Body:  body,
	})
	const maxEvents = 200
	if len(m.events) > maxEvents {
		m.events = append([]monitorEvent{}, m.events[len(m.events)-maxEvents:]...)
		m.eventList.SetLen(len(m.events))
	}
	if len(m.events) == 1 {
		m.eventList.Selected = 0
	}
}

func (m *monitorModel) setToast(text string, d time.Duration) {
	if d <= 0 {
		m.toastText = ""
		m.toastUntil = time.Time{}
		return
	}
	m.toastText = text
	m.toastUntil = time.Now().Add(d)
}

func (m monitorModel) renderInspectorPanel(st styles, sz size) string {
	if sz.W < 10 || sz.H < 4 {
		return ""
	}
	panelInnerW := max(0, sz.W-st.Panel.GetHorizontalBorderSize())
	panelInnerH := max(0, sz.H-st.Panel.GetVerticalBorderSize())
	if len(m.events) == 0 {
		return st.Panel.Width(panelInnerW).Height(panelInnerH).Render(st.Hint.Render("No events yet."))
	}

	header := st.PanelTitle.Render("Inspector")
	focus := "view"
	if m.hostFocus == hostFocusInspector {
		focus = "inspector"
	}
	sub := st.Hint.Render(fmt.Sprintf("focus: %s  events:%d", focus, len(m.events)))

	listH := max(3, panelInnerH-6)
	start, end := m.eventList.Window(len(m.events), listH)

	var rows []string
	for i := start; i < end; i++ {
		e := m.events[i]
		prefix := fmt.Sprintf("%s %-7s ", e.At.Format("15:04:05"), e.Kind)
		line := padOrTrim(prefix+e.Title, max(0, panelInnerW-2))
		if i == m.eventList.Selected {
			line = st.SelectedRow.Render(line)
		} else {
			line = st.Row.Render(line)
		}
		rows = append(rows, line)
	}
	for len(rows) < listH {
		rows = append(rows, "")
	}

	body := stringsJoinVertical(rows)

	detail := ""
	if m.eventList.Selected >= 0 && m.eventList.Selected < len(m.events) {
		detail = m.events[m.eventList.Selected].Body
	}
	detail = padOrTrim(detail, max(0, panelInnerW-2))

	content := lipgloss.JoinVertical(lipgloss.Left, header, sub, "", body, "", detail)
	return st.Panel.Width(panelInnerW).Height(panelInnerH).Render(content)
}
