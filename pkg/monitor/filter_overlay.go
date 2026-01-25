package monitor

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type filterConfig struct {
	levelE bool
	levelW bool
	levelI bool

	include *regexp.Regexp
	exclude *regexp.Regexp

	includeRaw string
	excludeRaw string
}

func defaultFilterConfig() filterConfig {
	return filterConfig{levelE: true, levelW: true, levelI: true}
}

type filterOverlayFocus int

const (
	filterFocusLevelE filterOverlayFocus = iota
	filterFocusLevelW
	filterFocusLevelI
	filterFocusInclude
	filterFocusExclude
	filterFocusApply
	filterFocusClear
	filterFocusCancel
)

type filterOverlayModel struct {
	sz size

	cfg filterConfig

	includeInput textinput.Model
	excludeInput textinput.Model
	focus        filterOverlayFocus

	err string
}

func newFilterOverlayModel() filterOverlayModel {
	inc := textinput.New()
	inc.Prompt = ""
	inc.Placeholder = "wifi|http"

	exc := textinput.New()
	exc.Prompt = ""
	exc.Placeholder = "heartbeat"

	return filterOverlayModel{
		cfg:          defaultFilterConfig(),
		includeInput: inc,
		excludeInput: exc,
		focus:        filterFocusLevelE,
	}
}

func (m *filterOverlayModel) setSize(sz size) {
	m.sz = sz
	w := max(10, sz.W-20)
	m.includeInput.Width = w
	m.excludeInput.Width = w
}

func (m *filterOverlayModel) openFrom(cfg filterConfig) {
	m.cfg = cfg
	m.includeInput.SetValue(cfg.includeRaw)
	m.excludeInput.SetValue(cfg.excludeRaw)
	m.focus = filterFocusLevelE
	m.err = ""
	m.includeInput.Blur()
	m.excludeInput.Blur()
}

type filterOverlayResultKind int

const (
	filterOverlayNone filterOverlayResultKind = iota
	filterOverlayCancel
	filterOverlayApply
	filterOverlayClear
)

type filterOverlayResult struct {
	kind filterOverlayResultKind
	cfg  filterConfig
}

func (m filterOverlayModel) Update(msg tea.KeyMsg) (filterOverlayModel, tea.Cmd, filterOverlayResult) {
	switch msg.Type {
	case tea.KeyEsc:
		return m, nil, filterOverlayResult{kind: filterOverlayCancel}
	case tea.KeyTab:
		m.focus = (m.focus + 1) % 8
		return m, nil, filterOverlayResult{kind: filterOverlayNone}
	case tea.KeyShiftTab:
		m.focus = (m.focus + 8 - 1) % 8
		return m, nil, filterOverlayResult{kind: filterOverlayNone}
	}

	switch msg.Type {
	case tea.KeyEnter:
		switch m.focus {
		case filterFocusApply:
			cfg, err := m.compile()
			if err != nil {
				m.err = err.Error()
				return m, nil, filterOverlayResult{kind: filterOverlayNone}
			}
			return m, nil, filterOverlayResult{kind: filterOverlayApply, cfg: cfg}
		case filterFocusClear:
			return m, nil, filterOverlayResult{kind: filterOverlayClear, cfg: defaultFilterConfig()}
		case filterFocusCancel:
			return m, nil, filterOverlayResult{kind: filterOverlayCancel}
		default:
			// fallthrough: in fields, Enter applies
			cfg, err := m.compile()
			if err != nil {
				m.err = err.Error()
				return m, nil, filterOverlayResult{kind: filterOverlayNone}
			}
			return m, nil, filterOverlayResult{kind: filterOverlayApply, cfg: cfg}
		}
	}

	// Field/toggle behavior.
	if msg.String() == " " {
		switch m.focus {
		case filterFocusLevelE:
			m.cfg.levelE = !m.cfg.levelE
			return m, nil, filterOverlayResult{}
		case filterFocusLevelW:
			m.cfg.levelW = !m.cfg.levelW
			return m, nil, filterOverlayResult{}
		case filterFocusLevelI:
			m.cfg.levelI = !m.cfg.levelI
			return m, nil, filterOverlayResult{}
		}
	}

	if m.focus == filterFocusInclude {
		m.includeInput.Focus()
		m.excludeInput.Blur()
		var cmd tea.Cmd
		m.includeInput, cmd = m.includeInput.Update(msg)
		return m, cmd, filterOverlayResult{}
	}
	if m.focus == filterFocusExclude {
		m.excludeInput.Focus()
		m.includeInput.Blur()
		var cmd tea.Cmd
		m.excludeInput, cmd = m.excludeInput.Update(msg)
		return m, cmd, filterOverlayResult{}
	}

	m.includeInput.Blur()
	m.excludeInput.Blur()
	return m, nil, filterOverlayResult{}
}

func (m filterOverlayModel) compile() (filterConfig, error) {
	cfg := m.cfg

	cfg.includeRaw = strings.TrimSpace(m.includeInput.Value())
	cfg.excludeRaw = strings.TrimSpace(m.excludeInput.Value())

	cfg.include = nil
	cfg.exclude = nil

	if cfg.includeRaw != "" {
		r, err := regexp.Compile(cfg.includeRaw)
		if err != nil {
			return filterConfig{}, fmt.Errorf("include regex: %w", err)
		}
		cfg.include = r
	}
	if cfg.excludeRaw != "" {
		r, err := regexp.Compile(cfg.excludeRaw)
		if err != nil {
			return filterConfig{}, fmt.Errorf("exclude regex: %w", err)
		}
		cfg.exclude = r
	}
	return cfg, nil
}

func (m filterOverlayModel) View(st styles) string {
	title := st.PanelTitle.Render("Filter Log Output")

	level := func(on bool, label string, focused bool) string {
		box := "[ ]"
		if on {
			box = "[✓]"
		}
		s := fmt.Sprintf("%s %s", box, label)
		if focused {
			return st.SelectedRow.Render(s)
		}
		return s
	}

	levels1 := lipgloss.JoinHorizontal(lipgloss.Top,
		level(m.cfg.levelE, "E  Error", m.focus == filterFocusLevelE),
		"   ",
		level(m.cfg.levelW, "W  Warning", m.focus == filterFocusLevelW),
		"   ",
		level(m.cfg.levelI, "I  Info", m.focus == filterFocusLevelI),
	)

	inc := "Include (regex): [" + m.includeInput.View() + "]"
	if m.focus == filterFocusInclude {
		inc = st.SelectedRow.Render(inc)
	}
	exc := "Exclude (regex): [" + m.excludeInput.View() + "]"
	if m.focus == filterFocusExclude {
		exc = st.SelectedRow.Render(exc)
	}

	button := func(label string, focused bool) string {
		s := "<" + label + ">"
		if focused {
			return st.SelectedRow.Render(s)
		}
		return s
	}
	btns := lipgloss.JoinHorizontal(lipgloss.Top,
		button("Apply", m.focus == filterFocusApply),
		"   ",
		button("Clear All", m.focus == filterFocusClear),
		"   ",
		button("Cancel", m.focus == filterFocusCancel),
	)
	btns = lipgloss.PlaceHorizontal(max(1, m.sz.W-4), lipgloss.Right, btns)

	errLine := ""
	if m.err != "" {
		errLine = st.ErrorBanner.Render(m.err)
	}

	hint := st.Hint.Render("Tab: next field  Space: toggle level  Enter: apply  Esc: cancel")

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		"",
		"Log Levels:",
		"  "+levels1,
		"",
		"────────────",
		"",
		padOrTrim(inc, m.sz.W),
		padOrTrim(exc, m.sz.W),
		"",
		"────────────",
		"",
		btns,
		"",
		hint,
		errLine,
	)

	return st.OverlayBox.Render(content)
}
