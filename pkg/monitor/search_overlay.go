package monitor

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type searchOverlayModel struct {
	sz size

	input textinput.Model

	query   string
	matches []int
	cur     int

	lastErr string
}

func newSearchOverlayModel() searchOverlayModel {
	ti := textinput.New()
	ti.Prompt = "/ "
	ti.Placeholder = "search..."
	ti.Focus()
	return searchOverlayModel{input: ti}
}

func (m *searchOverlayModel) setSize(sz size) {
	m.sz = sz
	m.input.Width = max(10, sz.W-12)
}

func (m *searchOverlayModel) open() {
	m.query = ""
	m.matches = nil
	m.cur = 0
	m.lastErr = ""
	m.input.SetValue("")
	m.input.Focus()
}

type searchOverlayResultKind int

const (
	searchOverlayNone searchOverlayResultKind = iota
	searchOverlayClose
	searchOverlayJump
	searchOverlayNext
	searchOverlayPrev
)

type searchOverlayResult struct {
	kind searchOverlayResultKind
}

func (m searchOverlayModel) Update(msg tea.KeyMsg) (searchOverlayModel, tea.Cmd, searchOverlayResult) {
	switch msg.Type {
	case tea.KeyEsc:
		return m, nil, searchOverlayResult{kind: searchOverlayClose}
	case tea.KeyEnter:
		return m, nil, searchOverlayResult{kind: searchOverlayJump}
	}

	switch msg.String() {
	case "n", "ctrl+n":
		return m, nil, searchOverlayResult{kind: searchOverlayNext}
	case "N", "ctrl+p":
		return m, nil, searchOverlayResult{kind: searchOverlayPrev}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.query = m.input.Value()
	return m, cmd, searchOverlayResult{kind: searchOverlayNone}
}

func (m searchOverlayModel) View(st styles) string {
	title := st.PanelTitle.Render("Search")
	body := m.input.View()

	hint := ""
	if m.query == "" {
		hint = st.Hint.Render("Enter: jump  n/N: next/prev  Esc: close")
	} else {
		hint = st.Hint.Render("Enter: jump  n/N: next/prev  Esc: close")
	}

	content := lipgloss.JoinVertical(lipgloss.Left, title, "", body, "", hint)
	return st.OverlayBox.Render(content)
}

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

func stripANSI(s string) string {
	return ansiRE.ReplaceAllString(s, "")
}

func containsQuery(line, query string) bool {
	if query == "" {
		return false
	}
	return strings.Contains(strings.ToLower(stripANSI(line)), strings.ToLower(query))
}
