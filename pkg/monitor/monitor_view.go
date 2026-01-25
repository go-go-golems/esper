package monitor

import (
	"bytes"
	"fmt"
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
}

func newMonitorModel(cfg Config, session *serialSession) monitorModel {
	m := monitorModel{
		cfg:        cfg,
		session:    session,
		lastDataAt: time.Now(),
		follow:     true,
		now:        time.Now(),
		hostFocus:  hostFocusViewport,
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
}

func (m monitorModel) Update(msg tea.Msg, curMode mode) (monitorModel, tea.Cmd, monitorAction) {
	switch t := msg.(type) {
	case tea.KeyMsg:
		if t.String() == "ctrl+t" {
			if curMode == modeHost {
				m.follow = true
				m.showInspector = false
				m.hostFocus = hostFocusViewport
				return m, nil, monitorAction{kind: monitorActionModeChanged, mode: modeDevice}
			}
			m.follow = false
			m.hostFocus = hostFocusViewport
			return m, nil, monitorAction{kind: monitorActionModeChanged, mode: modeHost}
		}

		if curMode == modeHost {
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

		m.viewport.SetContent(m.out)
		if m.follow {
			m.viewport.GotoBottom()
		}
		return m, m.readSerialCmd(), monitorAction{}

	case serialErrMsg:
		m.append([]byte(fmt.Sprintf("--- serial error: %v\n", t.err)))
		m.viewport.SetContent(m.out)
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
		// finalize tail if idle
		if time.Since(m.lastDataAt) > 250*time.Millisecond {
			if tail := m.lineSplitter.FinalizeTail(); len(tail) > 0 {
				m.append(m.autoColor.ColorizeLine(append(tail, '\n')))
			}
		}
		m.viewport.SetContent(m.out)
		if m.follow {
			m.viewport.GotoBottom()
		}
		return m, m.tickCmd(), monitorAction{}

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
		footerText := "Ctrl-T: DEVICE   PgUp/PgDn scroll   G: resume follow   i: inspector"
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

	return lipgloss.JoinVertical(lipgloss.Left, title, main, status, footer)
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
	return fmt.Sprintf("Mode: %s │ Follow: %s │ Inspect: %s │ Capture: %s │ Filter: — │ Search: — │ Buf: %s │ %s",
		modeStr,
		followStr,
		ins,
		capture,
		buf,
		m.now.Format("15:04:05"),
	)
}

func (m *monitorModel) append(b []byte) {
	m.out += string(b)
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
