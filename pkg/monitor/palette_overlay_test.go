package monitor

import "testing"

func TestPaletteIncludesSeparatorsWhenNotFiltering(t *testing.T) {
	m := newPaletteOverlayModel()
	m.setSize(size{W: 120, H: 40})
	m.open()

	sepCount := 0
	for _, r := range m.filtered {
		if r.kind == paletteRowSeparator {
			sepCount++
		}
	}
	if sepCount == 0 {
		t.Fatalf("expected separators in unfiltered palette")
	}
	if len(m.filtered) == 0 || m.filtered[m.list.Selected].kind != paletteRowCommand {
		t.Fatalf("expected selection to land on a command row")
	}
}

func TestPaletteFilteringRemovesSeparators(t *testing.T) {
	m := newPaletteOverlayModel()
	m.setSize(size{W: 120, H: 40})
	m.open()

	m.input.SetValue("reset")
	m.refilter()

	if len(m.filtered) == 0 {
		t.Fatalf("expected at least one filtered row")
	}
	for _, r := range m.filtered {
		if r.kind != paletteRowCommand {
			t.Fatalf("expected only command rows when filtering; got kind=%v", r.kind)
		}
	}
}

func TestPaletteSelectionSkipsSeparators(t *testing.T) {
	m := newPaletteOverlayModel()
	m.setSize(size{W: 120, H: 40})
	m.open()

	// Move down through the list; ensure we never land on separators.
	for i := 0; i < 50; i++ {
		m.moveSelection(1)
		if len(m.filtered) == 0 {
			break
		}
		if m.filtered[m.list.Selected].kind != paletteRowCommand {
			t.Fatalf("selection landed on non-command row at idx=%d", m.list.Selected)
		}
	}
}
