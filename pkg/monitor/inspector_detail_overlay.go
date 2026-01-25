package monitor

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wrap"
)

type inspectorDetailOverlay struct {
	sz  size
	evt monitorEvent
	vp  viewport.Model
}

func newInspectorDetailOverlay(evt monitorEvent) *inspectorDetailOverlay {
	vp := viewport.New(0, 0)
	vp.MouseWheelEnabled = false
	vp.HighPerformanceRendering = false
	return &inspectorDetailOverlay{
		evt: evt,
		vp:  vp,
	}
}

func (o *inspectorDetailOverlay) setSize(sz size) {
	o.sz = sz
}

func (o *inspectorDetailOverlay) open() {
	o.vp.GotoTop()
}

func (o *inspectorDetailOverlay) Update(msg tea.Msg) (overlayModel, tea.Cmd, overlayOutcome) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return o, nil, overlayOutcome{}
	}

	switch k.Type {
	case tea.KeyEsc:
		return o, nil, overlayOutcome{close: true}
	case tea.KeyPgUp:
		o.vp.LineUp(max(1, o.vp.Height-1))
		return o, nil, overlayOutcome{}
	case tea.KeyPgDown:
		o.vp.LineDown(max(1, o.vp.Height-1))
		return o, nil, overlayOutcome{}
	case tea.KeyHome:
		o.vp.GotoTop()
		return o, nil, overlayOutcome{}
	case tea.KeyEnd:
		o.vp.GotoBottom()
		return o, nil, overlayOutcome{}
	case tea.KeyUp:
		o.vp.LineUp(1)
		return o, nil, overlayOutcome{}
	case tea.KeyDown:
		o.vp.LineDown(1)
		return o, nil, overlayOutcome{}
	}

	switch k.String() {
	case "q":
		return o, nil, overlayOutcome{close: true}
	case "k":
		o.vp.LineUp(1)
	case "j":
		o.vp.LineDown(1)
	}

	return o, nil, overlayOutcome{}
}

func (o *inspectorDetailOverlay) View(st styles) string {
	if o.sz.W < 30 || o.sz.H < 6 {
		return st.Hint.Render("Terminal too small.")
	}

	boxW := max(60, min(110, o.sz.W-6))
	boxH := max(16, min(o.sz.H-6, 34))

	innerW := max(0, boxW-st.OverlayBox.GetHorizontalFrameSize())
	innerH := max(0, boxH-st.OverlayBox.GetVerticalFrameSize())

	kind := strings.ToUpper(o.evt.Kind)
	header := st.PanelTitle.Render(fmt.Sprintf("Inspector — %s — %s", kind, o.evt.At.Format("15:04:05")))

	footer := st.Hint.Render("Esc Close   PgUp/PgDn scroll")

	body := o.renderBody(st, innerW)

	vp := o.vp
	vp.Width = innerW
	vp.Height = max(1, innerH-4) // header + blank + blank + footer
	vp.SetContent(body)

	content := lipgloss.JoinVertical(lipgloss.Left,
		header,
		"",
		"",
		vp.View(),
		footer,
	)

	// Defensive sizing: clamp to the inner box size. lipgloss Width/Height are minimums.
	lines := splitLinesN(content, innerH)
	for i := range lines {
		lines[i] = padOrTrim(lines[i], innerW)
	}
	content = strings.Join(lines, "\n")

	return st.OverlayBox.Copy().Width(boxW).Height(boxH).Render(content)
}

func (o *inspectorDetailOverlay) renderBody(st styles, w int) string {
	switch strings.ToLower(o.evt.Kind) {
	case "panic":
		return o.renderPanicBody(st, w)
	case "coredump":
		return o.renderCoreDumpBody(st, w)
	default:
		return wrapPreserveNewlines(strings.TrimSpace(o.evt.Body), w)
	}
}

func (o *inspectorDetailOverlay) renderPanicBody(st styles, w int) string {
	raw, decoded := splitTwoSections(o.evt.Body, "\n\nDecoded Frames:\n")
	raw = strings.TrimPrefix(raw, "Raw Backtrace:\n")
	decoded = strings.TrimPrefix(decoded, "Decoded Frames:\n")

	box := st.Panel.Copy()

	rawBox := o.renderTitledBox(box, st.PanelTitle.Render("Raw Backtrace"), strings.TrimSpace(raw), w)
	decodedBox := o.renderTitledBox(box, st.PanelTitle.Render("Decoded Frames"), strings.TrimSpace(decoded), w)

	return lipgloss.JoinVertical(lipgloss.Left, rawBox, "", decodedBox)
}

func (o *inspectorDetailOverlay) renderCoreDumpBody(st styles, w int) string {
	head, rest := splitTwoSections(o.evt.Body, "\n\n")
	head = wrapPreserveNewlines(strings.TrimSpace(head), w)
	rest = strings.TrimSpace(rest)

	out := []string{head}
	if rest != "" {
		box := st.Panel.Copy()
		out = append(out, "", o.renderTitledBox(box, st.PanelTitle.Render("Decoded Report (truncated)"), rest, w))
	}
	return lipgloss.JoinVertical(lipgloss.Left, out...)
}

func (o *inspectorDetailOverlay) renderTitledBox(box lipgloss.Style, title string, body string, w int) string {
	contentW := max(0, w-box.GetHorizontalFrameSize())
	body = wrapPreserveNewlines(body, contentW)
	content := lipgloss.JoinVertical(lipgloss.Left, title, "", body)

	lines := strings.Split(content, "\n")
	// lipgloss output sometimes has a trailing newline; drop it if it would add an extra empty row.
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	for i := range lines {
		lines[i] = padOrTrim(lines[i], contentW)
	}
	content = strings.Join(lines, "\n")

	return box.Width(w).Render(content)
}

func splitTwoSections(s, sep string) (string, string) {
	if sep == "" {
		return s, ""
	}
	i := strings.Index(s, sep)
	if i < 0 {
		return s, ""
	}
	return s[:i], s[i+len(sep):]
}

func wrapPreserveNewlines(s string, w int) string {
	if w <= 0 {
		return ""
	}
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return ""
	}

	var out []string
	for _, ln := range strings.Split(s, "\n") {
		if strings.TrimSpace(ln) == "" {
			out = append(out, "")
			continue
		}
		wrapped := wrap.String(ln, w)
		out = append(out, strings.Split(wrapped, "\n")...)
	}
	return strings.Join(out, "\n")
}
