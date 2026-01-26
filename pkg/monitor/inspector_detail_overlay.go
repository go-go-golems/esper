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
	sz         size
	evt        monitorEvent
	eventIndex int
	vp         viewport.Model
}

func newInspectorDetailOverlay(evt monitorEvent, eventIndex int) *inspectorDetailOverlay {
	vp := viewport.New(0, 0)
	vp.MouseWheelEnabled = false
	vp.HighPerformanceRendering = false
	return &inspectorDetailOverlay{
		evt:        evt,
		eventIndex: eventIndex,
		vp:         vp,
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
	case tea.KeyTab:
		return o, nil, overlayOutcome{forward: inspectorDetailNextEventMsg{fromIndex: o.eventIndex}}
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
	case "c":
		return o, nil, overlayOutcome{forward: o.copyActionMsg()}
	case "C":
		return o, nil, overlayOutcome{forward: o.copyDecodedActionMsg()}
	case "s":
		return o, nil, overlayOutcome{forward: o.saveReportActionMsg()}
	case "r":
		return o, nil, overlayOutcome{forward: o.copyRawBase64ActionMsg()}
	case "j":
		return o, nil, overlayOutcome{close: true, forward: inspectorDetailJumpToLogMsg{anchor: o.jumpAnchor()}}
	case "q":
		return o, nil, overlayOutcome{close: true}
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

	footerText := "Esc Close   PgUp/PgDn scroll"
	switch strings.ToLower(o.evt.Kind) {
	case "panic":
		footerText = "c Copy raw   C Copy decoded   j Jump to log   Tab Next event   Esc Close"
	case "coredump":
		footerText = "c Copy report   s Save full report   r Copy raw base64   j Jump to log   Tab Next event   Esc Close"
	}
	footer := st.Hint.Render(footerText)

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

	contextBody := strings.TrimSpace(renderPanicContext(o.evt.Body))
	contextBox := o.renderTitledBox(box, st.PanelTitle.Render("Context"), contextBody, w)

	return lipgloss.JoinVertical(lipgloss.Left, rawBox, "", decodedBox, "", contextBox)
}

func (o *inspectorDetailOverlay) renderCoreDumpBody(st styles, w int) string {
	head, rest := splitTwoSections(o.evt.Body, "\n\n")
	head = wrapPreserveNewlines(strings.TrimSpace(head), w)
	rest = strings.TrimSpace(rest)

	// Render a truncated report to keep the detail view scannable. Full report is still available for save/copy.
	reportToShow := rest
	if reportToShow != "" {
		reportToShow, _ = truncateLines(reportToShow, 22)
	}

	out := []string{head}
	if rest != "" {
		box := st.Panel.Copy()
		out = append(out, "", o.renderTitledBox(box, st.PanelTitle.Render("Decoded Report (truncated)"), reportToShow, w))
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

func (o *inspectorDetailOverlay) copyActionMsg() tea.Msg {
	switch strings.ToLower(o.evt.Kind) {
	case "panic":
		raw, _ := o.panicSections()
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return inspectorDetailCopyTextMsg{label: "raw", text: ""}
		}
		return inspectorDetailCopyTextMsg{label: "raw", text: raw}
	case "coredump":
		_, reportFull := o.coreDumpSections()
		reportFull = strings.TrimSpace(reportFull)
		if reportFull == "" {
			return inspectorDetailCopyTextMsg{label: "report", text: ""}
		}
		reportShown, _ := truncateLines(reportFull, 22)
		return inspectorDetailCopyTextMsg{label: "report", text: reportShown}
	default:
		return nil
	}
}

func (o *inspectorDetailOverlay) copyDecodedActionMsg() tea.Msg {
	if strings.ToLower(o.evt.Kind) != "panic" {
		return nil
	}
	_, decoded := o.panicSections()
	decoded = strings.TrimSpace(decoded)
	if decoded == "" {
		return inspectorDetailCopyTextMsg{label: "decoded", text: ""}
	}
	return inspectorDetailCopyTextMsg{label: "decoded", text: decoded}
}

func (o *inspectorDetailOverlay) saveReportActionMsg() tea.Msg {
	if strings.ToLower(o.evt.Kind) != "coredump" {
		return nil
	}
	return inspectorDetailSaveTextMsg{label: "report", at: o.evt.At, text: strings.TrimSpace(o.evt.Body)}
}

func (o *inspectorDetailOverlay) copyRawBase64ActionMsg() tea.Msg {
	if strings.ToLower(o.evt.Kind) != "coredump" {
		return nil
	}
	if p := strings.TrimSpace(o.coreDumpSavedPath()); p != "" {
		return inspectorDetailCopyFileMsg{label: "raw", path: p}
	}
	return inspectorDetailCopyFileMsg{label: "raw", path: ""}
}

func (o *inspectorDetailOverlay) jumpAnchor() string {
	switch strings.ToLower(o.evt.Kind) {
	case "panic":
		raw, _ := o.panicSections()
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return "Backtrace:"
		}
		lines := strings.Split(raw, "\n")
		if len(lines) > 0 && strings.TrimSpace(lines[0]) != "" {
			return strings.TrimSpace(lines[0])
		}
		return raw
	case "coredump":
		if p := strings.TrimSpace(o.coreDumpSavedPath()); p != "" {
			return p
		}
		head, _ := o.coreDumpSections()
		head = strings.TrimSpace(head)
		if head != "" {
			lines := strings.Split(head, "\n")
			if len(lines) > 0 && strings.TrimSpace(lines[0]) != "" {
				return strings.TrimSpace(lines[0])
			}
		}
		return "Core dump"
	default:
		return ""
	}
}

func (o *inspectorDetailOverlay) panicSections() (string, string) {
	raw, decoded := splitTwoSections(o.evt.Body, "\n\nDecoded Frames:\n")
	raw = strings.TrimPrefix(raw, "Raw Backtrace:\n")
	decoded = strings.TrimPrefix(decoded, "Decoded Frames:\n")
	return raw, decoded
}

func (o *inspectorDetailOverlay) coreDumpSections() (string, string) {
	head, rest := splitTwoSections(o.evt.Body, "\n\n")
	return head, rest
}

func (o *inspectorDetailOverlay) coreDumpSavedPath() string {
	head, _ := o.coreDumpSections()
	for _, ln := range strings.Split(head, "\n") {
		ln = strings.TrimSpace(ln)
		if strings.HasPrefix(ln, "Saved to:") {
			return strings.TrimSpace(strings.TrimPrefix(ln, "Saved to:"))
		}
	}
	return ""
}

func renderPanicContext(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return "(no context parsed)"
	}

	errorStr, coreStr, regsLine := "", "", ""

	for _, ln := range strings.Split(body, "\n") {
		trim := strings.TrimSpace(ln)
		if strings.Contains(trim, "Guru Meditation Error:") && errorStr == "" && coreStr == "" {
			// Typical formats:
			// - "Guru Meditation Error: Core 0 panic'ed (LoadProhibited). ..."
			// - "Guru Meditation Error: Core 1 panic'ed (StoreProhibited). ..."
			if i := strings.Index(trim, "Core "); i >= 0 {
				rest := trim[i+len("Core "):]
				if j := strings.Index(rest, " "); j >= 0 {
					coreStr = rest[:j]
				}
				if k := strings.Index(trim, "panic'ed ("); k >= 0 {
					rest2 := trim[k+len("panic'ed ("):]
					if end := strings.Index(rest2, ")"); end >= 0 {
						errorStr = rest2[:end]
					}
				}
			}
		}
		if strings.Contains(trim, "PC:") && regsLine == "" {
			// Best-effort: capture a single-line register summary if present.
			regsLine = trim
		}
	}

	if errorStr == "" && coreStr == "" && regsLine == "" {
		return "(no context parsed)"
	}

	var out []string
	if errorStr != "" {
		out = append(out, "Error: "+errorStr)
	}
	if coreStr != "" {
		out = append(out, "Core: "+coreStr)
	}
	if regsLine != "" {
		out = append(out, regsLine)
	}
	return strings.Join(out, "\n")
}

func truncateLines(s string, maxLines int) (string, bool) {
	if maxLines <= 0 {
		return "", true
	}
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) <= maxLines {
		return s, false
	}
	return strings.Join(lines[:maxLines], "\n") + "\n…", true
}
