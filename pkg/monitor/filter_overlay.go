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
	levelD bool
	levelV bool

	include *regexp.Regexp
	exclude *regexp.Regexp

	includeRaw string
	excludeRaw string

	rules []highlightRule
}

func defaultFilterConfig() filterConfig {
	return filterConfig{levelE: true, levelW: true, levelI: true, levelD: false, levelV: false}
}

type filterOverlayFocus int

const (
	filterFocusLevelE filterOverlayFocus = iota
	filterFocusLevelW
	filterFocusLevelI
	filterFocusLevelD
	filterFocusLevelV
	filterFocusInclude
	filterFocusExclude
	filterFocusRulePattern
	filterFocusRuleStyle
	filterFocusAddRule
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

	includeErr string
	excludeErr string

	rules    []highlightRuleEdit
	ruleList selectList

	ruleInputW int
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
	w := max(10, sz.W-28)
	m.includeInput.Width = w
	m.excludeInput.Width = w
	m.ruleInputW = max(10, sz.W-46)

	for i := range m.rules {
		m.rules[i].pattern.Width = m.ruleInputW
	}
}

func (m *filterOverlayModel) openFrom(cfg filterConfig) {
	m.cfg = cfg
	m.includeInput.SetValue(cfg.includeRaw)
	m.excludeInput.SetValue(cfg.excludeRaw)
	m.focus = filterFocusLevelE
	m.includeErr = ""
	m.excludeErr = ""
	m.includeInput.Blur()
	m.excludeInput.Blur()

	m.rules = highlightRuleEditsFromConfig(cfg, m.ruleInputW)
	m.ruleList.Selected = 0
	m.ruleList.SetLen(len(m.rules))
	m.validate()
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
		m.focus = (m.focus + 1) % 13
		return m, nil, filterOverlayResult{kind: filterOverlayNone}
	case tea.KeyShiftTab:
		m.focus = (m.focus + 13 - 1) % 13
		return m, nil, filterOverlayResult{kind: filterOverlayNone}
	}

	switch msg.Type {
	case tea.KeyEnter:
		switch m.focus {
		case filterFocusApply:
			return m, nil, filterOverlayResult{kind: filterOverlayApply, cfg: m.buildConfig()}
		case filterFocusClear:
			return m, nil, filterOverlayResult{kind: filterOverlayClear, cfg: defaultFilterConfig()}
		case filterFocusCancel:
			return m, nil, filterOverlayResult{kind: filterOverlayCancel}
		case filterFocusAddRule:
			m.addRule()
			m.focus = filterFocusRulePattern
			return m, nil, filterOverlayResult{}
		default:
			// fallthrough: in fields, Enter applies
			return m, nil, filterOverlayResult{kind: filterOverlayApply, cfg: m.buildConfig()}
		}
	}

	// Rule list navigation.
	switch msg.String() {
	case "up", "k":
		if m.focus == filterFocusRulePattern || m.focus == filterFocusRuleStyle {
			m.ruleList.Move(-1, len(m.rules))
			return m, nil, filterOverlayResult{}
		}
	case "down", "j":
		if m.focus == filterFocusRulePattern || m.focus == filterFocusRuleStyle {
			m.ruleList.Move(1, len(m.rules))
			return m, nil, filterOverlayResult{}
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
		case filterFocusLevelD:
			m.cfg.levelD = !m.cfg.levelD
			return m, nil, filterOverlayResult{}
		case filterFocusLevelV:
			m.cfg.levelV = !m.cfg.levelV
			return m, nil, filterOverlayResult{}
		case filterFocusRuleStyle:
			if len(m.rules) > 0 && m.ruleList.Selected >= 0 && m.ruleList.Selected < len(m.rules) {
				m.rules[m.ruleList.Selected].style = highlightStyleNext(m.rules[m.ruleList.Selected].style)
				return m, nil, filterOverlayResult{}
			}
		}
	}

	switch msg.String() {
	case "a":
		if m.focus == filterFocusRulePattern || m.focus == filterFocusRuleStyle || m.focus == filterFocusAddRule {
			m.addRule()
			m.focus = filterFocusRulePattern
			return m, nil, filterOverlayResult{}
		}
	case "x":
		if m.focus == filterFocusRulePattern || m.focus == filterFocusRuleStyle {
			m.removeSelectedRule()
			return m, nil, filterOverlayResult{}
		}
	}

	if m.focus == filterFocusInclude {
		m.includeInput.Focus()
		m.excludeInput.Blur()
		m.blurRuleInputs()
		var cmd tea.Cmd
		m.includeInput, cmd = m.includeInput.Update(msg)
		m.validate()
		return m, cmd, filterOverlayResult{}
	}
	if m.focus == filterFocusExclude {
		m.excludeInput.Focus()
		m.includeInput.Blur()
		m.blurRuleInputs()
		var cmd tea.Cmd
		m.excludeInput, cmd = m.excludeInput.Update(msg)
		m.validate()
		return m, cmd, filterOverlayResult{}
	}

	m.includeInput.Blur()
	m.excludeInput.Blur()
	if m.focus == filterFocusRulePattern {
		m.focusRulePattern()
		var cmd tea.Cmd
		m.rules, cmd = updateSelectedRulePattern(m.rules, m.ruleList.Selected, msg)
		m.validate()
		return m, cmd, filterOverlayResult{}
	}
	m.blurRuleInputs()
	return m, nil, filterOverlayResult{}
}

func (m *filterOverlayModel) validate() {
	m.includeErr = ""
	m.excludeErr = ""

	inc := strings.TrimSpace(m.includeInput.Value())
	exc := strings.TrimSpace(m.excludeInput.Value())

	if inc != "" {
		if _, err := regexp.Compile(inc); err != nil {
			m.includeErr = err.Error()
		}
	}
	if exc != "" {
		if _, err := regexp.Compile(exc); err != nil {
			m.excludeErr = err.Error()
		}
	}

	for i := range m.rules {
		m.rules[i].err = ""
		p := strings.TrimSpace(m.rules[i].pattern.Value())
		if p == "" {
			continue
		}
		if _, err := regexp.Compile(p); err != nil {
			m.rules[i].err = err.Error()
		}
	}
}

func (m filterOverlayModel) buildConfig() filterConfig {
	cfg := m.cfg

	cfg.includeRaw = strings.TrimSpace(m.includeInput.Value())
	cfg.excludeRaw = strings.TrimSpace(m.excludeInput.Value())

	cfg.include = nil
	cfg.exclude = nil

	if cfg.includeRaw != "" {
		if r, err := regexp.Compile(cfg.includeRaw); err == nil {
			cfg.include = r
		}
	}
	if cfg.excludeRaw != "" {
		if r, err := regexp.Compile(cfg.excludeRaw); err == nil {
			cfg.exclude = r
		}
	}

	cfg.rules = nil
	for _, r := range m.rules {
		cfg.rules = append(cfg.rules, r.toConfig())
	}
	return cfg
}

func (m *filterOverlayModel) addRule() {
	ri := newHighlightRuleEdit(m.ruleInputW)
	m.rules = append(m.rules, ri)
	m.ruleList.SetLen(len(m.rules))
	m.ruleList.Selected = len(m.rules) - 1
}

func (m *filterOverlayModel) removeSelectedRule() {
	if len(m.rules) == 0 || m.ruleList.Selected < 0 || m.ruleList.Selected >= len(m.rules) {
		return
	}
	i := m.ruleList.Selected
	m.rules = append(m.rules[:i], m.rules[i+1:]...)
	m.ruleList.SetLen(len(m.rules))
	if m.ruleList.Selected >= len(m.rules) {
		m.ruleList.Selected = max(0, len(m.rules)-1)
	}
}

func (m *filterOverlayModel) blurRuleInputs() {
	for i := range m.rules {
		m.rules[i].pattern.Blur()
	}
}

func (m *filterOverlayModel) focusRulePattern() {
	m.includeInput.Blur()
	m.excludeInput.Blur()
	for i := range m.rules {
		if i == m.ruleList.Selected {
			m.rules[i].pattern.Focus()
			continue
		}
		m.rules[i].pattern.Blur()
	}
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
	levels2 := lipgloss.JoinHorizontal(lipgloss.Top,
		level(m.cfg.levelD, "D  Debug", m.focus == filterFocusLevelD),
		"   ",
		level(m.cfg.levelV, "V  Verbose", m.focus == filterFocusLevelV),
	)

	inc := "Include (regex): [" + m.includeInput.View() + "]"
	if m.focus == filterFocusInclude {
		inc = st.SelectedRow.Render(inc)
	}
	exc := "Exclude (regex): [" + m.excludeInput.View() + "]"
	if m.focus == filterFocusExclude {
		exc = st.SelectedRow.Render(exc)
	}

	var rules []string
	rules = append(rules, "Highlight Rules:")
	if len(m.rules) == 0 {
		rules = append(rules, st.Hint.Render("  (no rules yet)"))
	} else {
		for i := range m.rules {
			r := m.rules[i]
			styleLabel := highlightStyleLabel(r.style)
			row := fmt.Sprintf("  %d. [%s] → [%s ▼]", i+1, r.pattern.View(), styleLabel)
			focused := (m.focus == filterFocusRulePattern || m.focus == filterFocusRuleStyle) && i == m.ruleList.Selected
			if focused {
				row = st.SelectedRow.Render(row)
			}
			rules = append(rules, padOrTrim(row, m.sz.W))
			if strings.TrimSpace(r.err) != "" {
				rules = append(rules, st.InlineError.Render("     invalid regex: "+r.err))
			}
		}
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

	addRule := lipgloss.PlaceHorizontal(
		max(1, m.sz.W-4),
		lipgloss.Right,
		button("Add Rule", m.focus == filterFocusAddRule),
	)

	errLines := []string{}
	if strings.TrimSpace(m.includeErr) != "" {
		errLines = append(errLines, st.InlineError.Render("include regex invalid: "+m.includeErr))
	}
	if strings.TrimSpace(m.excludeErr) != "" {
		errLines = append(errLines, st.InlineError.Render("exclude regex invalid: "+m.excludeErr))
	}

	hint := st.Hint.Render("Tab: next field  Space: toggle/cycle  a: add rule  x: remove rule  ↑/↓: select rule  Enter: apply  Esc: cancel")

	divider := strings.Repeat("─", max(12, min(m.sz.W-4, m.sz.W)))

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		"",
		"Log Levels:",
		"  "+levels1,
		"  "+levels2,
		"",
		divider,
		"",
		padOrTrim(inc, m.sz.W),
		stringsJoinVertical(errLines),
		padOrTrim(exc, m.sz.W),
		"",
		divider,
		"",
		stringsJoinVertical(rules),
		addRule,
		"",
		btns,
		"",
		hint,
	)

	return st.OverlayBox.Render(content)
}

type highlightRule struct {
	patternRaw string
	rx         *regexp.Regexp
	style      highlightStyleKind
}

type highlightRuleEdit struct {
	pattern textinput.Model
	style   highlightStyleKind
	err     string
}

func newHighlightRuleEdit(patternW int) highlightRuleEdit {
	ti := textinput.New()
	ti.Prompt = ""
	ti.Placeholder = "error|fail|crash"
	ti.Width = max(10, patternW)
	ti.Blur()
	return highlightRuleEdit{pattern: ti, style: highlightStyleRedBackground}
}

func highlightRuleEditsFromConfig(cfg filterConfig, patternW int) []highlightRuleEdit {
	if len(cfg.rules) == 0 {
		return nil
	}
	out := make([]highlightRuleEdit, 0, len(cfg.rules))
	for _, r := range cfg.rules {
		ed := newHighlightRuleEdit(patternW)
		ed.pattern.SetValue(r.patternRaw)
		ed.style = r.style
		out = append(out, ed)
	}
	return out
}

func (r highlightRuleEdit) toConfig() highlightRule {
	p := strings.TrimSpace(r.pattern.Value())
	if p == "" {
		return highlightRule{patternRaw: "", rx: nil, style: r.style}
	}
	rx, _ := regexp.Compile(p)
	return highlightRule{patternRaw: p, rx: rx, style: r.style}
}

func updateSelectedRulePattern(rules []highlightRuleEdit, sel int, msg tea.KeyMsg) ([]highlightRuleEdit, tea.Cmd) {
	if sel < 0 || sel >= len(rules) {
		return rules, nil
	}
	var cmd tea.Cmd
	rules[sel].pattern, cmd = rules[sel].pattern.Update(msg)
	return rules, cmd
}
