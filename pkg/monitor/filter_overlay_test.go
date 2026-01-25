package monitor

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestFilterOverlayApplyDoesNotBlockOnInvalidRegex(t *testing.T) {
	m := newFilterOverlayModel()
	m.setSize(size{W: 120, H: 40})
	m.openFrom(defaultFilterConfig())

	m.includeInput.SetValue("[")
	m.excludeInput.SetValue("(")
	m.validate()

	m.focus = filterFocusApply
	_, _, res := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if res.kind != filterOverlayApply {
		t.Fatalf("kind=%v, want %v", res.kind, filterOverlayApply)
	}
	if res.cfg.include != nil || res.cfg.exclude != nil {
		t.Fatalf("compiled include/exclude should be nil for invalid regexes")
	}
	if res.cfg.includeRaw != "[" || res.cfg.excludeRaw != "(" {
		t.Fatalf("raw include/exclude = %q/%q, want \"[\"/\"(\"", res.cfg.includeRaw, res.cfg.excludeRaw)
	}
}

func TestFilterOverlayClearCancelSemantics(t *testing.T) {
	m := newFilterOverlayModel()
	m.setSize(size{W: 120, H: 40})
	m.openFrom(defaultFilterConfig())

	// Clear.
	m.focus = filterFocusClear
	m2, _, res := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = m2
	if res.kind != filterOverlayClear {
		t.Fatalf("kind=%v, want %v", res.kind, filterOverlayClear)
	}
	def := defaultFilterConfig()
	if res.cfg.levelE != def.levelE || res.cfg.levelW != def.levelW || res.cfg.levelI != def.levelI || res.cfg.levelD != def.levelD || res.cfg.levelV != def.levelV {
		t.Fatalf("clear cfg levels not default")
	}
	if res.cfg.includeRaw != "" || res.cfg.excludeRaw != "" || len(res.cfg.rules) != 0 {
		t.Fatalf("clear cfg should wipe regex/rules")
	}

	// Cancel.
	_, _, res = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if res.kind != filterOverlayCancel {
		t.Fatalf("kind=%v, want %v", res.kind, filterOverlayCancel)
	}
}

func TestFilterOverlayRuleAddRemoveAndStyleCycle(t *testing.T) {
	m := newFilterOverlayModel()
	m.setSize(size{W: 120, H: 40})
	m.openFrom(defaultFilterConfig())

	m.focus = filterFocusAddRule
	m2, _, res := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = m2
	if res.kind != filterOverlayNone {
		t.Fatalf("kind=%v, want %v", res.kind, filterOverlayNone)
	}
	if len(m.rules) != 1 {
		t.Fatalf("rules=%d, want 1", len(m.rules))
	}

	// Cycle style.
	m.focus = filterFocusRuleStyle
	before := m.rules[0].style
	m2, _, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	m = m2
	if m.rules[0].style == before {
		t.Fatalf("style did not cycle")
	}

	// Remove.
	m.focus = filterFocusRuleStyle
	m2, _, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	m = m2
	if len(m.rules) != 0 {
		t.Fatalf("rules=%d, want 0", len(m.rules))
	}
}
