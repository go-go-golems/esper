package monitor

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type paletteCommandKind int

const (
	cmdNone paletteCommandKind = iota
	cmdOpenSearch
	cmdOpenFilter
	cmdToggleInspector
	cmdToggleWrap
	cmdResetDevice
	cmdSendBreak
	cmdDisconnect
	cmdClearViewport
	cmdToggleSessionLog
	cmdShowHelp
	cmdQuit
)

type paletteRowKind int

const (
	paletteRowCommand paletteRowKind = iota
	paletteRowSeparator
)

type paletteRow struct {
	kind paletteRowKind

	label    string
	shortcut string

	cmd paletteCommandKind
}

type paletteOverlayModel struct {
	sz size

	input textinput.Model

	all      []paletteRow
	filtered []paletteRow
	list     selectList
}

func newPaletteOverlayModel() paletteOverlayModel {
	ti := textinput.New()
	ti.Prompt = "> "
	ti.Placeholder = "Type to filter commands..."
	ti.Focus()

	sep := paletteRow{kind: paletteRowSeparator}
	all := []paletteRow{
		{kind: paletteRowCommand, label: "Search log output", shortcut: "/", cmd: cmdOpenSearch},
		{kind: paletteRowCommand, label: "Filter by level/regex", shortcut: "f", cmd: cmdOpenFilter},
		{kind: paletteRowCommand, label: "Toggle Inspector", shortcut: "i", cmd: cmdToggleInspector},
		{kind: paletteRowCommand, label: "Toggle line wrap", shortcut: "w", cmd: cmdToggleWrap},
		sep,
		{kind: paletteRowCommand, label: "Reset device", shortcut: "Ctrl-R", cmd: cmdResetDevice},
		// Note: Ctrl-] is reserved as an unconditional exit in non-TUI tail mode.
		// In the TUI we expose "Send break" via palette execution only (no direct Ctrl-] binding).
		{kind: paletteRowCommand, label: "Send break", shortcut: "Ctrl-]", cmd: cmdSendBreak},
		{kind: paletteRowCommand, label: "Disconnect", shortcut: "Ctrl-D", cmd: cmdDisconnect},
		sep,
		{kind: paletteRowCommand, label: "Clear viewport", shortcut: "Ctrl-L", cmd: cmdClearViewport},
		{kind: paletteRowCommand, label: "Toggle session logging", shortcut: "Ctrl-S", cmd: cmdToggleSessionLog},
		sep,
		{kind: paletteRowCommand, label: "Help", shortcut: "?", cmd: cmdShowHelp},
		{kind: paletteRowCommand, label: "Quit", shortcut: "Ctrl-C", cmd: cmdQuit},
	}

	m := paletteOverlayModel{
		input: ti,
		all:   all,
	}
	m.refilter()
	return m
}

func (m *paletteOverlayModel) setSize(sz size) {
	m.sz = sz
	m.input.Width = max(10, sz.W-10)
}

func (m *paletteOverlayModel) open() {
	m.input.SetValue("")
	m.input.Focus()
	m.refilter()
	m.list.Selected = 0
	m.ensureSelectableSelection(1)
}

type paletteOverlayResultKind int

const (
	paletteOverlayNone paletteOverlayResultKind = iota
	paletteOverlayClose
	paletteOverlayExec
)

type paletteOverlayResult struct {
	kind paletteOverlayResultKind
	cmd  paletteCommandKind
}

func (m paletteOverlayModel) Update(msg tea.KeyMsg) (paletteOverlayModel, tea.Cmd, paletteOverlayResult) {
	switch msg.Type {
	case tea.KeyEsc:
		return m, nil, paletteOverlayResult{kind: paletteOverlayClose}
	case tea.KeyUp:
		m.moveSelection(-1)
		return m, nil, paletteOverlayResult{}
	case tea.KeyDown:
		m.moveSelection(1)
		return m, nil, paletteOverlayResult{}
	case tea.KeyEnter:
		if len(m.filtered) == 0 || m.list.Selected < 0 || m.list.Selected >= len(m.filtered) {
			return m, nil, paletteOverlayResult{kind: paletteOverlayClose}
		}
		row := m.filtered[m.list.Selected]
		if row.kind != paletteRowCommand {
			return m, nil, paletteOverlayResult{kind: paletteOverlayNone}
		}
		return m, nil, paletteOverlayResult{kind: paletteOverlayExec, cmd: row.cmd}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.refilter()
	m.list.SetLen(len(m.filtered))
	m.ensureSelectableSelection(1)
	return m, cmd, paletteOverlayResult{}
}

func (m *paletteOverlayModel) refilter() {
	q := strings.TrimSpace(strings.ToLower(m.input.Value()))
	if q == "" {
		m.filtered = append([]paletteRow{}, m.all...)
		return
	}
	var out []paletteRow
	for _, r := range m.all {
		if r.kind != paletteRowCommand {
			continue
		}
		if strings.Contains(strings.ToLower(r.label), q) || strings.Contains(strings.ToLower(r.shortcut), q) {
			out = append(out, r)
		}
	}
	m.filtered = out
}

func (m paletteOverlayModel) View(st styles) string {
	title := st.PanelTitle.Render("Commands")

	input := m.input.View()

	var rows []string
	labelW := max(10, m.sz.W-22)
	for i, r := range m.filtered {
		if r.kind == paletteRowSeparator {
			rows = append(rows, st.Hint.Render(strings.Repeat("─", max(10, min(m.sz.W-14, m.sz.W)))))
			continue
		}

		prefix := "  "
		if i == m.list.Selected {
			prefix = "→ "
		}
		label := padOrTrim(r.label, labelW)
		short := st.Hint.Render(padOrTrim(r.shortcut, 10))
		line := prefix + label + " " + short
		if i == m.list.Selected {
			line = st.SelectedRow.Render(line)
		}
		rows = append(rows, line)
	}
	if len(rows) == 0 {
		rows = append(rows, st.Hint.Render("No matching commands"))
	}

	list := lipgloss.JoinVertical(lipgloss.Left, rows...)
	hint := st.Hint.Render("↑/↓ select  Enter run  Esc close")

	content := lipgloss.JoinVertical(lipgloss.Left, title, "", input, "", list, "", hint)
	return st.OverlayBox.Render(content)
}

func (m *paletteOverlayModel) ensureSelectableSelection(dir int) {
	if len(m.filtered) == 0 {
		m.list.Selected = 0
		return
	}
	m.list.Selected = clamp(m.list.Selected, 0, len(m.filtered)-1)
	if m.filtered[m.list.Selected].kind == paletteRowSeparator {
		m.moveSelection(dir)
	}
}

func (m *paletteOverlayModel) moveSelection(delta int) {
	if len(m.filtered) == 0 || delta == 0 {
		return
	}

	// Try a bounded number of steps to find the next selectable row.
	for steps := 0; steps < len(m.filtered); steps++ {
		m.list.Move(delta, len(m.filtered))
		if m.list.Selected >= 0 && m.list.Selected < len(m.filtered) && m.filtered[m.list.Selected].kind == paletteRowCommand {
			return
		}
	}
}
