package monitor

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/go-go-golems/esper/pkg/decode"
	"github.com/go-go-golems/esper/pkg/parse"
	"github.com/go-go-golems/esper/pkg/render"
)

type monitorActionKind int

const (
	monitorActionNone monitorActionKind = iota
	monitorActionDisconnect
	monitorActionModeChanged
	monitorActionShowHelp
	monitorActionQuit
)

type monitorAction struct {
	kind   monitorActionKind
	reason string
	mode   mode
}

type hostFocus int

const (
	hostFocusViewport hostFocus = iota
	hostFocusInspector
)

type monitorEvent struct {
	At    time.Time
	Kind  string
	Title string
	Body  string
}

type monitorModel struct {
	sz size

	cfg     Config
	session *serialSession

	lineSplitter parse.LineSplitter
	autoColor    render.AutoColorer
	gdb          decode.GDBStubDetector
	panic        decode.PanicDecoder
	coredump     decode.CoreDumpDecoder

	lastDataAt time.Time

	out string
	log []string

	viewport viewport.Model
	follow   bool

	input textinput.Model

	now time.Time

	// Host-mode extras.
	hostFocus     hostFocus
	showInspector bool

	events        []monitorEvent
	selectedEvent int

	toastUntil time.Time
	toastText  string

	overlay monitorOverlayKind
	search  searchOverlayModel
	filter  filterOverlayModel
	palette paletteOverlayModel

	ctrlTPending   bool
	ctrlTPendingID int

	filterCfg filterConfig
}

func newMonitorModel(cfg Config, session *serialSession) monitorModel {
	m := monitorModel{
		cfg:        cfg,
		session:    session,
		lastDataAt: time.Now(),
		follow:     true,
		now:        time.Now(),
		hostFocus:  hostFocusViewport,
		filterCfg:  defaultFilterConfig(),
	}
	m.autoColor.DisableAutoColor = false
	m.panic = decode.PanicDecoder{ElfPath: cfg.ElfPath, ToolchainPrefix: cfg.ToolchainPrefix}
	m.coredump = decode.CoreDumpDecoder{ElfPath: cfg.ElfPath}
	m.viewport = viewport.New(0, 0)
	m.viewport.MouseWheelEnabled = false
	m.viewport.HighPerformanceRendering = true

	ti := textinput.New()
	ti.Prompt = ""
	ti.Placeholder = ""
	ti.Focus()
	m.input = ti

	m.search = newSearchOverlayModel()
	m.filter = newFilterOverlayModel()
	m.palette = newPaletteOverlayModel()

	return m
}

func (m *monitorModel) setSize(sz size) {
	m.sz = sz

	// Layout:
	// - 1 line title
	// - 1 line status
	// - 1 line input/help
	// - remaining: viewport
	vh := max(1, sz.H-3)
	m.viewport.Width = max(1, m.viewportWidthFor(sz))
	m.viewport.Height = vh

	fieldW := max(1, sz.W-4) // "> " + "[ ]"
	m.input.Width = max(1, fieldW-2)

	m.viewport.SetContent(m.out)
	if m.follow {
		m.viewport.GotoBottom()
	}

	m.search.setSize(sz)
	m.filter.setSize(sz)
	m.palette.setSize(sz)
}

func (m monitorModel) Update(msg tea.Msg, curMode mode) (monitorModel, tea.Cmd, monitorAction) {
	switch t := msg.(type) {
	case tea.KeyMsg:
		// If a host-mode overlay is active, it captures keys first.
		if curMode == modeHost && m.overlay != monitorOverlayNone {
			return m.updateOverlay(t, curMode)
		}

		if t.String() == "ctrl+t" {
			if curMode == modeHost {
				// UX spec wants ctrl+t t for command palette, but ctrl+t alone exits host mode.
				// We implement this as a short prefix window: if the next key is 't' quickly, open palette;
				// otherwise, fall back to exiting host mode.
				m.ctrlTPending = true
				m.ctrlTPendingID++
				id := m.ctrlTPendingID
				return m, ctrlTPrefixTimeoutCmd(id, 350*time.Millisecond), monitorAction{}
			}
			m.follow = false
			m.hostFocus = hostFocusViewport
			return m, nil, monitorAction{kind: monitorActionModeChanged, mode: modeHost}
		}

		// Global disconnect (matches UX spec).
		if t.String() == "ctrl+d" {
			return m, nil, monitorAction{kind: monitorActionDisconnect, reason: "user disconnect"}
		}

		if curMode == modeHost {
			if m.ctrlTPending {
				switch t.Type {
				case tea.KeyEsc:
					m.ctrlTPending = false
					m.follow = true
					m.showInspector = false
					m.hostFocus = hostFocusViewport
					return m, nil, monitorAction{kind: monitorActionModeChanged, mode: modeDevice}
				}
				if t.String() == "t" {
					// ctrl+t t opens command palette.
					m.ctrlTPending = false
					m.overlay = monitorOverlayPalette
					m.palette.open()
					return m, nil, monitorAction{}
				}
				// During the ctrl+t prefix window, ignore other host shortcuts.
				return m, nil, monitorAction{}
			}

			// Host-mode overlays.
			switch t.String() {
			case "/":
				m.overlay = monitorOverlaySearch
				m.search.open()
				return m, nil, monitorAction{}
			case "f":
				m.overlay = monitorOverlayFilter
				m.filter.openFrom(m.filterCfg)
				return m, nil, monitorAction{}
			}

			// Exit to device mode.
			if t.Type == tea.KeyEsc {
				m.ctrlTPending = false
				m.follow = true
				m.showInspector = false
				m.hostFocus = hostFocusViewport
				return m, nil, monitorAction{kind: monitorActionModeChanged, mode: modeDevice}
			}

			// Host-mode inspector toggle.
			if t.String() == "i" {
				m.showInspector = !m.showInspector
				if !m.showInspector {
					m.hostFocus = hostFocusViewport
				}
				m.setSize(m.sz)
				return m, nil, monitorAction{}
			}

			if m.showInspector && t.Type == tea.KeyTab {
				if m.hostFocus == hostFocusViewport {
					m.hostFocus = hostFocusInspector
				} else {
					m.hostFocus = hostFocusViewport
				}
				return m, nil, monitorAction{}
			}

			switch t.Type {
			case tea.KeyPgUp:
				if m.hostFocus == hostFocusViewport {
					m.viewport.LineUp(10)
				} else {
					m.selectedEvent = clamp(m.selectedEvent-10, 0, max(0, len(m.events)-1))
				}
			case tea.KeyPgDown:
				if m.hostFocus == hostFocusViewport {
					m.viewport.LineDown(10)
				} else {
					m.selectedEvent = clamp(m.selectedEvent+10, 0, max(0, len(m.events)-1))
				}
			case tea.KeyUp:
				if m.hostFocus == hostFocusViewport {
					m.viewport.LineUp(1)
				} else {
					m.selectedEvent = clamp(m.selectedEvent-1, 0, max(0, len(m.events)-1))
				}
			case tea.KeyDown:
				if m.hostFocus == hostFocusViewport {
					m.viewport.LineDown(1)
				} else {
					m.selectedEvent = clamp(m.selectedEvent+1, 0, max(0, len(m.events)-1))
				}
			case tea.KeyHome:
				if m.hostFocus == hostFocusViewport {
					m.viewport.GotoTop()
				} else {
					m.selectedEvent = 0
				}
			case tea.KeyEnd:
				if m.hostFocus == hostFocusViewport {
					m.viewport.GotoBottom()
				} else {
					m.selectedEvent = max(0, len(m.events)-1)
				}
			}
			if t.String() == "G" || t.String() == "g" {
				m.follow = true
				m.viewport.GotoBottom()
				return m, nil, monitorAction{kind: monitorActionModeChanged, mode: modeDevice}
			}
			return m, nil, monitorAction{}
		}

		// Device mode: line-buffered send (Enter submits line, other keys ignored for now).
		if t.Type == tea.KeyEnter {
			line := m.input.Value()
			m.input.SetValue("")
			if m.session != nil {
				_ = m.session.WriteLine(line)
			}
			return m, nil, monitorAction{}
		}

		var cmd tea.Cmd
		m.input, cmd = m.input.Update(t)
		return m, cmd, monitorAction{}

	case serialChunkMsg:
		if len(t.b) == 0 {
			return m, m.readSerialCmd(), monitorAction{}
		}
		m.lastDataAt = time.Now()

		if g := m.gdb.Push(t.b); g != nil {
			m.append([]byte("--- GDB stub detected\n"))
			m.addEvent("gdb", "GDB stub detected", string(g.Payload))
			m.setToast("GDB stub detected (HOST mode: press i for inspector)", 3*time.Second)
		}

		lines := m.lineSplitter.Push(t.b)
		for _, line := range lines {
			events, sendEnter := m.coredump.PushLine(line)
			if sendEnter && m.session != nil {
				_, _ = m.session.port.Write([]byte("\n"))
			}
			if m.coredump.InProgress() {
				// suppress normal output while buffering
				continue
			}
			for _, e := range events {
				m.append(e)
				if bytes.HasPrefix(e, []byte("--- Core dump")) {
					m.addEvent("coredump", "Core dump event", string(e))
					m.setToast("Core dump event captured (HOST mode: press i for inspector)", 3*time.Second)
				}
			}

			// panic backtrace decode is opportunistic: if the line contains Backtrace:, emit extra decoded lines.
			if decoded, ok := m.panic.DecodeBacktraceLine(line); ok && len(decoded) > 0 {
				m.append(decoded)
				m.addEvent("panic", "Backtrace decoded", string(decoded))
				m.setToast("Backtrace decoded (HOST mode: press i for inspector)", 3*time.Second)
			}

			m.append(m.autoColor.ColorizeLine(line))
		}

		m.refreshViewportContent()
		if m.follow {
			m.viewport.GotoBottom()
		}
		return m, m.readSerialCmd(), monitorAction{}

	case serialErrMsg:
		m.append([]byte(fmt.Sprintf("--- serial error: %v\n", t.err)))
		m.refreshViewportContent()
		if m.follow {
			m.viewport.GotoBottom()
		}
		return m, m.readSerialCmd(), monitorAction{}

	case tickMsg:
		m.now = t.t
		if m.toastText != "" && !m.toastUntil.IsZero() && m.now.After(m.toastUntil) {
			m.toastText = ""
			m.toastUntil = time.Time{}
		}
		if m.ctrlTPending && m.ctrlTPendingID != 0 && m.now.After(m.toastUntil) {
			// no-op; ctrl-t timeout handled by ctrlTPrefixTimeoutMsg
		}
		// finalize tail if idle
		if time.Since(m.lastDataAt) > 250*time.Millisecond {
			if tail := m.lineSplitter.FinalizeTail(); len(tail) > 0 {
				m.append(m.autoColor.ColorizeLine(append(tail, '\n')))
			}
		}
		m.refreshViewportContent()
		if m.follow {
			m.viewport.GotoBottom()
		}
		return m, m.tickCmd(), monitorAction{}

	case ctrlTPrefixTimeoutMsg:
		if curMode == modeHost && m.ctrlTPending && t.id == m.ctrlTPendingID {
			m.ctrlTPending = false
			m.follow = true
			m.showInspector = false
			m.hostFocus = hostFocusViewport
			return m, nil, monitorAction{kind: monitorActionModeChanged, mode: modeDevice}
		}
		return m, nil, monitorAction{}

	default:
		return m, nil, monitorAction{}
	}
}

func (m monitorModel) View(st styles, sz size, curMode mode) string {
	if sz.W < 30 || sz.H < 6 {
		return st.Hint.Render("Terminal too small.")
	}

	title := padOrTrim(st.TitleBar.Render(m.renderTitle()), sz.W)

	status := padOrTrim(st.StatusBar.Render(m.renderStatus(curMode)), sz.W)

	bodyH := max(1, sz.H-3)
	vw := max(1, m.viewportWidthFor(sz))
	vp := m.viewport
	vp.Width = vw
	vp.Height = bodyH
	body := vp.View()
	body = lipgloss.NewStyle().Width(vw).Height(bodyH).Render(body)

	main := body
	if curMode == modeHost && m.showInspector && sz.W >= 100 {
		panelW := max(32, min(54, sz.W/3))
		main = lipgloss.JoinHorizontal(
			lipgloss.Top,
			lipgloss.NewStyle().Width(sz.W-panelW-1).Render(body),
			lipgloss.NewStyle().Width(panelW).Render(m.renderInspectorPanel(st, size{W: panelW, H: bodyH})),
		)
	}

	footer := ""
	if curMode == modeHost {
		footerText := "Ctrl-T: DEVICE   Ctrl-T T: commands   PgUp/PgDn scroll   G: resume follow   / search   f filter   i inspector"
		if m.showInspector {
			footerText += "   Tab: focus"
		}
		if m.toastText != "" {
			footerText += "   " + m.toastText
		}
		footer = padOrTrim(footerText, sz.W)
	} else {
		field := padOrTrim(m.input.View(), max(0, sz.W-4))
		footer = padOrTrim("> ["+field+"]", sz.W)
	}

	content := lipgloss.JoinVertical(lipgloss.Left, title, main, status, footer)
	if curMode == modeHost {
		content = m.renderOverlayIfNeeded(st, sz, content)
	}
	return content
}

func (m monitorModel) renderTitle() string {
	elfOK := "—"
	if m.cfg.ElfPath != "" {
		elfOK = "✓"
	}
	port := m.cfg.Port
	if m.session != nil && m.session.portPath != "" {
		port = m.session.portPath
	}
	return fmt.Sprintf("esper ── Connected: %s ── %d ── ELF: %s ──", port, m.cfg.Baud, elfOK)
}

func (m monitorModel) renderStatus(curMode mode) string {
	modeStr := "DEVICE"
	if curMode == modeHost {
		modeStr = "HOST"
	}
	followStr := "OFF"
	if m.follow {
		followStr = "ON"
	}
	capture := "—"
	if m.coredump.InProgress() {
		capture = "CORE"
	}
	buf := fmt.Sprintf("%dK/1M", len(m.out)/1024)
	ins := "OFF"
	if m.showInspector && curMode == modeHost {
		ins = "ON"
	}
	return fmt.Sprintf("Mode: %s │ Follow: %s │ Inspect: %s │ Capture: %s │ Filter: %s │ Search: %s │ Buf: %s │ %s",
		modeStr,
		followStr,
		ins,
		capture,
		m.filterSummary(),
		m.searchSummary(),
		buf,
		m.now.Format("15:04:05"),
	)
}

func (m monitorModel) filterSummary() string {
	cfg := m.filterCfg
	enabled := cfg.levelE != true || cfg.levelW != true || cfg.levelI != true || cfg.include != nil || cfg.exclude != nil
	if !enabled {
		return "—"
	}
	parts := []string{}
	if !cfg.levelE || !cfg.levelW || !cfg.levelI {
		lv := ""
		if cfg.levelE {
			lv += "E"
		}
		if cfg.levelW {
			lv += "W"
		}
		if cfg.levelI {
			lv += "I"
		}
		if lv == "" {
			lv = "∅"
		}
		parts = append(parts, "lvl:"+lv)
	}
	if strings.TrimSpace(cfg.includeRaw) != "" {
		parts = append(parts, "inc:"+padOrTrim(cfg.includeRaw, 16))
	}
	if strings.TrimSpace(cfg.excludeRaw) != "" {
		parts = append(parts, "exc:"+padOrTrim(cfg.excludeRaw, 16))
	}
	if len(parts) == 0 {
		return "ON"
	}
	return strings.Join(parts, " ")
}

func (m monitorModel) searchSummary() string {
	q := strings.TrimSpace(m.search.query)
	if q == "" {
		return "—"
	}
	return "/" + padOrTrim(q, 18)
}

func (m *monitorModel) append(b []byte) {
	s := string(b)
	m.out += s
	for _, line := range splitKeepNewline(s) {
		if line == "" {
			continue
		}
		m.log = append(m.log, line)
	}
	const maxLines = 4000
	if len(m.log) > maxLines {
		m.log = append([]string{}, m.log[len(m.log)-maxLines:]...)
	}
	// Keep a bounded rolling buffer to avoid unbounded memory growth.
	const maxBytes = 1 << 20  // 1 MiB
	const keepBytes = 1 << 19 // 512 KiB
	if len(m.out) > maxBytes {
		m.out = m.out[len(m.out)-keepBytes:]
	}
}

func (m monitorModel) readSerialCmd() tea.Cmd {
	return func() tea.Msg {
		if m.session == nil || m.session.port == nil {
			return serialErrMsg{err: fmt.Errorf("not connected")}
		}
		buf := make([]byte, 4096)
		n, err := m.session.port.Read(buf)
		if err != nil {
			return serialErrMsg{err: err}
		}
		if n <= 0 {
			return serialChunkMsg{b: nil}
		}
		return serialChunkMsg{b: append([]byte{}, buf[:n]...)}
	}
}

func (m monitorModel) tickCmd() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg{t: t}
	})
}

func (m *monitorModel) addEvent(kind, title, body string) {
	if body == "" {
		body = title
	}
	m.events = append(m.events, monitorEvent{
		At:    time.Now(),
		Kind:  kind,
		Title: title,
		Body:  body,
	})
	const maxEvents = 200
	if len(m.events) > maxEvents {
		m.events = append([]monitorEvent{}, m.events[len(m.events)-maxEvents:]...)
		m.selectedEvent = clamp(m.selectedEvent, 0, max(0, len(m.events)-1))
	}
	if len(m.events) == 1 {
		m.selectedEvent = 0
	}
}

func (m *monitorModel) setToast(text string, d time.Duration) {
	if d <= 0 {
		m.toastText = ""
		m.toastUntil = time.Time{}
		return
	}
	m.toastText = text
	m.toastUntil = time.Now().Add(d)
}

func (m monitorModel) renderInspectorPanel(st styles, sz size) string {
	if sz.W < 10 || sz.H < 4 {
		return ""
	}
	if len(m.events) == 0 {
		return st.Panel.Width(sz.W).Height(sz.H).Render(st.Hint.Render("No events yet."))
	}

	header := st.PanelTitle.Render("Inspector")
	focus := "view"
	if m.hostFocus == hostFocusInspector {
		focus = "inspector"
	}
	sub := st.Hint.Render(fmt.Sprintf("focus: %s  events:%d", focus, len(m.events)))

	listH := max(3, sz.H-6)
	start := 0
	if m.selectedEvent >= listH {
		start = m.selectedEvent - listH + 1
	}
	end := min(len(m.events), start+listH)

	var rows []string
	for i := start; i < end; i++ {
		e := m.events[i]
		prefix := fmt.Sprintf("%s %-7s ", e.At.Format("15:04:05"), e.Kind)
		line := padOrTrim(prefix+e.Title, max(0, sz.W-st.Panel.GetHorizontalBorderSize()-2))
		if i == m.selectedEvent {
			line = st.SelectedRow.Render(line)
		} else {
			line = st.Row.Render(line)
		}
		rows = append(rows, line)
	}
	for len(rows) < listH {
		rows = append(rows, "")
	}

	body := stringsJoinVertical(rows)

	detail := ""
	if m.selectedEvent >= 0 && m.selectedEvent < len(m.events) {
		detail = m.events[m.selectedEvent].Body
	}
	detail = padOrTrim(detail, max(0, sz.W-st.Panel.GetHorizontalBorderSize()-2))

	content := lipgloss.JoinVertical(lipgloss.Left, header, sub, "", body, "", detail)
	return st.Panel.Width(sz.W).Height(sz.H).Render(content)
}

func (m monitorModel) viewportWidthFor(sz size) int {
	if m.showInspector && sz.W >= 100 {
		panelW := max(32, min(54, sz.W/3))
		return max(1, sz.W-panelW-1)
	}
	return max(1, sz.W)
}

func stringsJoinVertical(lines []string) string {
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func splitKeepNewline(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.SplitAfter(s, "\n")
	// If string doesn't end with newline, last part won't include it.
	// Keep it anyway (viewport may still show partial).
	return parts
}

func (m *monitorModel) refreshViewportContent() {
	m.viewport.SetContent(strings.Join(m.filteredLines(), ""))
}

func (m monitorModel) filteredLines() []string {
	if len(m.log) == 0 {
		return nil
	}

	cfg := m.filterCfg
	enabled := cfg.levelE != true || cfg.levelW != true || cfg.levelI != true || cfg.include != nil || cfg.exclude != nil
	if !enabled {
		return m.log
	}

	var out []string
	for _, line := range m.log {
		stripped := stripANSI(line)

		// Level filter (ESP-IDF-ish prefix: I|W|E + space + '(').
		if len(stripped) >= 3 {
			lvl := stripped[0]
			if stripped[1] == ' ' && stripped[2] == '(' {
				switch lvl {
				case 'E':
					if !cfg.levelE {
						continue
					}
				case 'W':
					if !cfg.levelW {
						continue
					}
				case 'I':
					if !cfg.levelI {
						continue
					}
				}
			}
		}

		if cfg.include != nil && !cfg.include.MatchString(stripped) {
			continue
		}
		if cfg.exclude != nil && cfg.exclude.MatchString(stripped) {
			continue
		}

		out = append(out, line)
	}
	return out
}

func (m monitorModel) updateOverlay(k tea.KeyMsg, curMode mode) (monitorModel, tea.Cmd, monitorAction) {
	_ = curMode
	switch m.overlay {
	case monitorOverlaySearch:
		s, cmd, res := m.search.Update(k)
		m.search = s
		switch res.kind {
		case searchOverlayClose:
			m.overlay = monitorOverlayNone
		case searchOverlayJump:
			m.searchApplyJump()
			m.overlay = monitorOverlayNone
		case searchOverlayNext:
			m.searchApplyNext()
		case searchOverlayPrev:
			m.searchApplyPrev()
		}
		return m, cmd, monitorAction{}
	case monitorOverlayFilter:
		fm, cmd, res := m.filter.Update(k)
		m.filter = fm
		switch res.kind {
		case filterOverlayCancel:
			m.overlay = monitorOverlayNone
		case filterOverlayClear:
			m.filterCfg = res.cfg
			m.overlay = monitorOverlayNone
			m.refreshViewportContent()
		case filterOverlayApply:
			m.filterCfg = res.cfg
			m.overlay = monitorOverlayNone
			m.refreshViewportContent()
		}
		return m, cmd, monitorAction{}
	case monitorOverlayPalette:
		pm, cmd, res := m.palette.Update(k)
		m.palette = pm
		switch res.kind {
		case paletteOverlayClose:
			m.overlay = monitorOverlayNone
		case paletteOverlayExec:
			m.overlay = monitorOverlayNone
			return m, cmd, m.execPalette(res.cmd)
		}
		return m, cmd, monitorAction{}
	default:
		return m, nil, monitorAction{}
	}
}

func (m monitorModel) renderOverlayIfNeeded(st styles, sz size, content string) string {
	var box string
	switch m.overlay {
	case monitorOverlaySearch:
		box = m.search.View(st)
	case monitorOverlayFilter:
		box = m.filter.View(st)
	case monitorOverlayPalette:
		box = m.palette.View(st)
	default:
		return content
	}

	// Bubble Tea can't truly "layer" strings, but we can approximate an overlay by
	// rendering both at full screen size and then replacing the rows that contain
	// the overlay box.
	bg := st.OverlayDim.Render(content)
	ov := lipgloss.Place(sz.W, sz.H, lipgloss.Center, lipgloss.Center, box)

	bgLines := splitLinesN(bg, sz.H)
	ovLines := splitLinesN(ov, sz.H)
	for i := 0; i < sz.H && i < len(bgLines) && i < len(ovLines); i++ {
		if strings.TrimSpace(stripANSI(ovLines[i])) != "" {
			bgLines[i] = ovLines[i]
		}
	}
	return strings.Join(bgLines, "\n")
}

func splitLinesN(s string, n int) []string {
	lines := strings.Split(s, "\n")
	// lipgloss output sometimes has a trailing newline; drop it if it would add an extra empty row.
	if len(lines) == n+1 && lines[len(lines)-1] == "" {
		lines = lines[:n]
	}
	if len(lines) > n {
		lines = lines[:n]
	}
	for len(lines) < n {
		lines = append(lines, "")
	}
	return lines
}

func (m *monitorModel) searchApplyJump() {
	m.searchComputeMatches()
	if len(m.search.matches) == 0 {
		m.setToast("no matches", 2*time.Second)
		return
	}
	m.search.cur = clamp(m.search.cur, 0, len(m.search.matches)-1)
	m.viewport.YOffset = clamp(m.search.matches[m.search.cur], 0, max(0, m.viewport.TotalLineCount()-1))
}

func (m *monitorModel) searchApplyNext() {
	m.searchComputeMatches()
	if len(m.search.matches) == 0 {
		m.setToast("no matches", 2*time.Second)
		return
	}
	m.search.cur = (m.search.cur + 1) % len(m.search.matches)
	m.viewport.YOffset = clamp(m.search.matches[m.search.cur], 0, max(0, m.viewport.TotalLineCount()-1))
}

func (m *monitorModel) searchApplyPrev() {
	m.searchComputeMatches()
	if len(m.search.matches) == 0 {
		m.setToast("no matches", 2*time.Second)
		return
	}
	m.search.cur = (m.search.cur + len(m.search.matches) - 1) % len(m.search.matches)
	m.viewport.YOffset = clamp(m.search.matches[m.search.cur], 0, max(0, m.viewport.TotalLineCount()-1))
}

func (m *monitorModel) searchComputeMatches() {
	query := strings.TrimSpace(m.search.query)
	if query == "" {
		m.search.matches = nil
		m.search.cur = 0
		return
	}

	lines := m.filteredLines()
	m.search.matches = nil
	for i, line := range lines {
		if containsQuery(line, query) {
			m.search.matches = append(m.search.matches, i)
		}
	}
	if len(m.search.matches) == 0 {
		m.search.cur = 0
		return
	}
	if m.search.cur >= len(m.search.matches) {
		m.search.cur = 0
	}
}

func (m *monitorModel) execPalette(cmd paletteCommand) monitorAction {
	switch cmd.Kind {
	case cmdOpenSearch:
		m.overlay = monitorOverlaySearch
		m.search.open()
		return monitorAction{}
	case cmdOpenFilter:
		m.overlay = monitorOverlayFilter
		m.filter.openFrom(m.filterCfg)
		return monitorAction{}
	case cmdToggleInspector:
		m.showInspector = !m.showInspector
		m.setSize(m.sz)
		return monitorAction{}
	case cmdDisconnect:
		return monitorAction{kind: monitorActionDisconnect, reason: "disconnect"}
	case cmdClearViewport:
		m.out = ""
		m.log = nil
		m.viewport.SetContent("")
		m.events = nil
		m.selectedEvent = 0
		return monitorAction{}
	case cmdShowHelp:
		return monitorAction{kind: monitorActionShowHelp}
	case cmdQuit:
		return monitorAction{kind: monitorActionQuit}
	default:
		return monitorAction{}
	}
}
