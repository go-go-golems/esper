package monitor

import (
	tea "github.com/charmbracelet/bubbletea"
)

type helpOverlay struct {
	m helpOverlayModel
}

func newHelpOverlay() *helpOverlay {
	return &helpOverlay{m: newHelpOverlayModel()}
}

func (o *helpOverlay) setSize(sz size) {
	o.m.setSize(sz)
}

func (o *helpOverlay) open() {}

func (o *helpOverlay) Update(msg tea.Msg) (overlayModel, tea.Cmd, overlayOutcome) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return o, nil, overlayOutcome{}
	}
	if k.String() == "q" {
		return o, nil, overlayOutcome{close: true}
	}
	return o, nil, overlayOutcome{}
}

func (o *helpOverlay) View(st styles) string {
	return o.m.View(st)
}

type filterOverlay struct {
	m          filterOverlayModel
	initialCfg filterConfig
}

func newFilterOverlay(initialCfg filterConfig) *filterOverlay {
	return &filterOverlay{
		m:          newFilterOverlayModel(),
		initialCfg: initialCfg,
	}
}

func (o *filterOverlay) setSize(sz size) {
	o.m.setSize(sz)
}

func (o *filterOverlay) open() {
	o.m.openFrom(o.initialCfg)
}

func (o *filterOverlay) Update(msg tea.Msg) (overlayModel, tea.Cmd, overlayOutcome) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return o, nil, overlayOutcome{}
	}
	m2, cmd, res := o.m.Update(k)
	o.m = m2

	switch res.kind {
	case filterOverlayCancel:
		return o, cmd, overlayOutcome{close: true}
	case filterOverlayApply:
		return o, cmd, overlayOutcome{close: true, forward: filterSetMsg{cfg: res.cfg}}
	case filterOverlayClear:
		return o, cmd, overlayOutcome{close: true, forward: filterSetMsg{cfg: res.cfg}}
	default:
		return o, cmd, overlayOutcome{}
	}
}

func (o *filterOverlay) View(st styles) string {
	return o.m.View(st)
}

type paletteOverlay struct {
	m paletteOverlayModel
}

func newPaletteOverlay() *paletteOverlay {
	return &paletteOverlay{m: newPaletteOverlayModel()}
}

func (o *paletteOverlay) setSize(sz size) {
	o.m.setSize(sz)
}

func (o *paletteOverlay) open() {
	o.m.open()
}

func (o *paletteOverlay) Update(msg tea.Msg) (overlayModel, tea.Cmd, overlayOutcome) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return o, nil, overlayOutcome{}
	}
	m2, cmd, res := o.m.Update(k)
	o.m = m2

	switch res.kind {
	case paletteOverlayClose:
		return o, cmd, overlayOutcome{close: true}
	case paletteOverlayExec:
		return o, cmd, overlayOutcome{close: true, forward: paletteExecMsg{cmd: res.cmd}}
	default:
		return o, cmd, overlayOutcome{}
	}
}

func (o *paletteOverlay) View(st styles) string {
	return o.m.View(st)
}
