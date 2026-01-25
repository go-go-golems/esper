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
	cmdDisconnect
	cmdClearViewport
	cmdShowHelp
	cmdQuit
)

type paletteCommand struct {
	Label    string
	Shortcut string
	Kind     paletteCommandKind
}

type paletteOverlayModel struct {
	sz size

	input textinput.Model

	all      []paletteCommand
	filtered []paletteCommand
	list     selectList
}

func newPaletteOverlayModel() paletteOverlayModel {
	ti := textinput.New()
	ti.Prompt = "> "
	ti.Placeholder = "Type to filter commands..."
	ti.Focus()

	all := []paletteCommand{
		{Label: "Search log output", Shortcut: "/", Kind: cmdOpenSearch},
		{Label: "Filter by level/regex", Shortcut: "f", Kind: cmdOpenFilter},
		{Label: "Toggle Inspector", Shortcut: "i", Kind: cmdToggleInspector},
		{Label: "Disconnect", Shortcut: "Ctrl-D", Kind: cmdDisconnect},
		{Label: "Clear viewport", Shortcut: "Ctrl-L", Kind: cmdClearViewport},
		{Label: "Help", Shortcut: "?", Kind: cmdShowHelp},
		{Label: "Quit", Shortcut: "Ctrl-C", Kind: cmdQuit},
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
	m.list.Selected = 0
	m.refilter()
}

type paletteOverlayResultKind int

const (
	paletteOverlayNone paletteOverlayResultKind = iota
	paletteOverlayClose
	paletteOverlayExec
)

type paletteOverlayResult struct {
	kind paletteOverlayResultKind
	cmd  paletteCommand
}

func (m paletteOverlayModel) Update(msg tea.KeyMsg) (paletteOverlayModel, tea.Cmd, paletteOverlayResult) {
	switch msg.Type {
	case tea.KeyEsc:
		return m, nil, paletteOverlayResult{kind: paletteOverlayClose}
	case tea.KeyUp:
		m.list.Move(-1, len(m.filtered))
		return m, nil, paletteOverlayResult{}
	case tea.KeyDown:
		m.list.Move(1, len(m.filtered))
		return m, nil, paletteOverlayResult{}
	case tea.KeyEnter:
		if len(m.filtered) == 0 {
			return m, nil, paletteOverlayResult{kind: paletteOverlayClose}
		}
		return m, nil, paletteOverlayResult{kind: paletteOverlayExec, cmd: m.filtered[m.list.Selected]}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.refilter()
	m.list.SetLen(len(m.filtered))
	return m, cmd, paletteOverlayResult{}
}

func (m *paletteOverlayModel) refilter() {
	q := strings.TrimSpace(strings.ToLower(m.input.Value()))
	if q == "" {
		m.filtered = append([]paletteCommand{}, m.all...)
		return
	}
	var out []paletteCommand
	for _, c := range m.all {
		if strings.Contains(strings.ToLower(c.Label), q) || strings.Contains(strings.ToLower(c.Shortcut), q) {
			out = append(out, c)
		}
	}
	m.filtered = out
}

func (m paletteOverlayModel) View(st styles) string {
	title := st.PanelTitle.Render("Commands")

	input := m.input.View()

	var rows []string
	for i, c := range m.filtered {
		label := padOrTrim(c.Label, max(10, m.sz.W-20))
		short := st.Hint.Render(padOrTrim(c.Shortcut, 10))
		line := label + " " + short
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
