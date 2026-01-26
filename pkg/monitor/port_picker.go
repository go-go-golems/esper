package monitor

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/go-go-golems/esper/pkg/devices"
	"github.com/go-go-golems/esper/pkg/scan"
)

type portPickerFocus int

const (
	focusPortList portPickerFocus = iota
	focusBaud
	focusElf
	focusToolchain
	focusProbe
	focusButtons
)

type portPickerConfig struct {
	defaultBaud      int
	defaultElfPath   string
	defaultToolchain string
}

type portPickerModel struct {
	sz size

	ports    []scan.Port
	portList selectList
	focus    portPickerFocus

	baudIdx int
	bauds   []int

	elfPath         string
	toolchainPrefix string
	probeEsptool    bool

	nickByUSBSerial map[string]string

	errBanner string
	errHint   string
}

func newPortPickerModel(cfg portPickerConfig) portPickerModel {
	bauds := []int{9600, 19200, 38400, 57600, 115200, 230400, 460800, 921600}
	idx := 4
	if cfg.defaultBaud > 0 {
		for i, b := range bauds {
			if b == cfg.defaultBaud {
				idx = i
				break
			}
		}
	}

	m := portPickerModel{
		focus:           focusPortList,
		bauds:           bauds,
		baudIdx:         idx,
		elfPath:         cfg.defaultElfPath,
		toolchainPrefix: cfg.defaultToolchain,
	}
	m.reloadRegistry()
	return m
}

func (m *portPickerModel) setSize(sz size) {
	m.sz = sz
}

func (m *portPickerModel) setError(msg string) {
	m.errBanner = msg
}

func (m *portPickerModel) setErrorHint(msg string) {
	m.errHint = msg
}

func (m portPickerModel) scanPortsCmd(ctx context.Context) tea.Cmd {
	return newScanPortsCmd(ctx)
}

func (m *portPickerModel) applyScanResult(msg portsScanResultMsg) {
	if msg.err != nil {
		m.setError(fmt.Sprintf("Scan failed: %v", msg.err))
		m.setErrorHint("Is this supported OS? (Scan is Linux-only.)")
		return
	}
	m.errBanner = ""
	m.errHint = ""

	m.ports = msg.ports
	m.portList.SetLen(len(m.ports))
	m.reloadRegistry()
}

func (m *portPickerModel) reloadRegistry() {
	reg, _, err := devices.Load()
	if err != nil {
		m.nickByUSBSerial = nil
		return
	}
	m.nickByUSBSerial = make(map[string]string, len(reg.Devices))
	for _, d := range reg.Devices {
		usb := strings.TrimSpace(d.USBSerial)
		if usb == "" {
			continue
		}
		nn := strings.TrimSpace(d.Nickname)
		if nn == "" {
			continue
		}
		m.nickByUSBSerial[usb] = nn
	}
}

func (m portPickerModel) selectedPort() *scan.Port {
	i := m.portList.Selected
	if i < 0 || i >= len(m.ports) {
		return nil
	}
	return &m.ports[i]
}

type portPickerActionKind int

const (
	portPickerActionNone portPickerActionKind = iota
	portPickerActionRescan
	portPickerActionConnect
	portPickerActionQuit
)

type portPickerAction struct {
	kind          portPickerActionKind
	connectParams connectParams
}

func (m portPickerModel) Update(msg tea.Msg) (portPickerModel, tea.Cmd, portPickerAction) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyTab:
			m.focus = (m.focus + 1) % 6
			return m, nil, portPickerAction{kind: portPickerActionNone}
		case tea.KeyShiftTab:
			m.focus = (m.focus + 6 - 1) % 6
			return m, nil, portPickerAction{kind: portPickerActionNone}
		}

		switch msg.String() {
		case "q":
			return m, nil, portPickerAction{kind: portPickerActionQuit}
		case "r":
			return m, nil, portPickerAction{kind: portPickerActionRescan}
		}

		switch m.focus {
		case focusPortList:
			switch msg.Type {
			case tea.KeyUp:
				m.portList.Move(-1, len(m.ports))
				return m, nil, portPickerAction{}
			case tea.KeyDown:
				m.portList.Move(1, len(m.ports))
				return m, nil, portPickerAction{}
			case tea.KeyEnter:
				return m, nil, m.connectAction()
			}
		case focusBaud:
			switch msg.Type {
			case tea.KeyUp, tea.KeyRight:
				m.baudIdx = (m.baudIdx + 1) % len(m.bauds)
			case tea.KeyDown, tea.KeyLeft:
				m.baudIdx = (m.baudIdx + len(m.bauds) - 1) % len(m.bauds)
			case tea.KeyEnter:
				return m, nil, m.connectAction()
			}
		case focusElf:
			m.elfPath = updateSingleLineField(m.elfPath, msg)
			return m, nil, portPickerAction{}
		case focusToolchain:
			m.toolchainPrefix = updateSingleLineField(m.toolchainPrefix, msg)
			return m, nil, portPickerAction{}
		case focusProbe:
			if msg.String() == " " {
				m.probeEsptool = !m.probeEsptool
			}
			return m, nil, portPickerAction{}
		case focusButtons:
			if msg.Type == tea.KeyEnter {
				return m, nil, m.connectAction()
			}
		}
	}

	return m, nil, portPickerAction{}
}

func (m portPickerModel) connectAction() portPickerAction {
	portPath := ""
	if m.portList.Selected >= 0 && m.portList.Selected < len(m.ports) {
		portPath = m.ports[m.portList.Selected].PreferredPath
		if portPath == "" {
			portPath = m.ports[m.portList.Selected].Device
		}
	}
	if portPath == "" {
		m2 := m
		m2.setError("No port selected")
		m2.setErrorHint("Press r to rescan.")
		return portPickerAction{kind: portPickerActionNone}
	}

	return portPickerAction{
		kind: portPickerActionConnect,
		connectParams: connectParams{
			portPath:        portPath,
			baud:            m.bauds[m.baudIdx],
			elfPath:         strings.TrimSpace(m.elfPath),
			toolchainPrefix: strings.TrimSpace(m.toolchainPrefix),
			probeEsptool:    m.probeEsptool,
		},
	}
}

func (m portPickerModel) View(st styles, sz size) string {
	if sz.W < 30 || sz.H < 10 {
		return st.Hint.Render("Terminal too small.")
	}

	title := st.TitleBar.Render("esper")
	if sz.H == 1 {
		return padOrTrim(title, sz.W)
	}

	// Reserve 1 line for title, 1 line for footer
	contentH := sz.H - 2

	var errorSection string
	errH := 0
	if m.errBanner != "" {
		b := st.ErrorBanner.Width(max(0, sz.W-st.ErrorBanner.GetHorizontalBorderSize())).Render(m.errBanner)
		if m.errHint != "" {
			b = lipgloss.JoinVertical(lipgloss.Left, b, st.Hint.Render(m.errHint))
		}
		errorSection = b
		errH = lipgloss.Height(b)
	}

	// Calculate panel size, accounting for error banner
	panelW := min(sz.W, 78)
	panelH := min(contentH-errH, 18)

	panelInner := size{W: panelW - st.Panel.GetHorizontalBorderSize(), H: panelH - st.Panel.GetVerticalBorderSize()}
	panel := st.Panel.Width(panelInner.W).Height(panelInner.H).Render(m.renderPanel(st, panelInner))

	// Center the panel in the remaining content area (after title + error, before footer)
	centerAreaH := contentH - errH
	centeredPanel := lipgloss.Place(sz.W, centerAreaH, lipgloss.Center, lipgloss.Center, panel)

	// Build content area (error + centered panel)
	var content string
	if errorSection != "" {
		content = lipgloss.JoinVertical(lipgloss.Left, errorSection, centeredPanel)
	} else {
		content = centeredPanel
	}

	help := st.StatusBar.Render("↑↓ Navigate   Tab Next field   Enter Connect   n Nickname   d Device Manager   r Rescan   ? Help   q Quit")

	return lipgloss.JoinVertical(lipgloss.Left,
		padOrTrim(title, sz.W),
		content,
		padOrTrim(help, sz.W),
	)
}

func (m portPickerModel) renderPanel(st styles, inner size) string {
	lines := []string{
		st.PanelTitle.Render("Select Serial Port"),
		"",
	}

	listH := inner.H - 9
	if listH < 3 {
		listH = 3
	}
	lines = append(lines, m.renderPortList(st, inner.W, listH)...)
	lines = append(lines, "")
	lines = append(lines, m.renderForm(st, inner.W)...)

	out := lipgloss.JoinVertical(lipgloss.Left, lines...)
	return lipgloss.NewStyle().Width(inner.W).Height(inner.H).Render(out)
}

func (m portPickerModel) renderPortList(st styles, w, h int) []string {
	nickW := 10
	nameW := max(10, w-nickW-18)
	chipW := 10
	starW := 2

	start, end := m.portList.Window(len(m.ports), h)

	out := make([]string, 0, h)
	for i := start; i < end; i++ {
		p := m.ports[i]

		cursor := "  "
		rowStyle := st.Row
		if i == m.portList.Selected {
			cursor = "→ "
			rowStyle = st.SelectedRow
		}

		nick := "—"
		if m.nickByUSBSerial != nil {
			if nn := strings.TrimSpace(m.nickByUSBSerial[strings.TrimSpace(p.Serial)]); nn != "" {
				nick = nn
			}
		}
		name := portName(p)
		chip := portChip(p)
		star := " "
		if p.Score >= 80 {
			star = "★"
		}

		row := cursor +
			padOrTrim(nick, nickW) + " " +
			padOrTrim(name, nameW) + " " +
			padOrTrim(chip, chipW) + " " +
			padOrTrim(star, starW)

		out = append(out, rowStyle.Render(padOrTrim(row, w)))
	}

	for len(out) < h {
		out = append(out, "")
	}

	if len(m.ports) == 0 {
		out[0] = st.Hint.Render("No ports found. Press r to rescan.")
	}

	return out
}

func (m portPickerModel) renderForm(st styles, w int) []string {
	baudStr := fmt.Sprintf("%d", m.bauds[m.baudIdx])
	baudField := fmt.Sprintf("Baud: [%s ▼]", padOrTrim(baudStr, 8))
	baudFieldW := lipgloss.Width(baudField)

	// ELF field shares line with Baud field, so subtract baud width + separator
	elfFieldW := max(10, w-baudFieldW-10) // "ELF: [" = 6, "]" = 1, separator = 3
	elfField := fmt.Sprintf("ELF: [%s]", padOrTrim(m.elfPath, elfFieldW))

	// Toolchain field gets full width
	tcFieldW := max(10, w-22) // "Toolchain prefix: [" = 20, "]" = 1, margin = 1
	tcField := fmt.Sprintf("Toolchain prefix: [%s]", padOrTrim(m.toolchainPrefix, tcFieldW))

	if m.focus == focusBaud {
		baudField = st.SelectedRow.Render(baudField)
	}
	if m.focus == focusElf {
		elfField = st.SelectedRow.Render(elfField)
	}
	if m.focus == focusToolchain {
		tcField = st.SelectedRow.Render(tcField)
	}

	probe := "[ ] Probe with esptool (may reset device)"
	if m.probeEsptool {
		probe = "[x] Probe with esptool (may reset device)"
	}
	if m.focus == focusProbe {
		probe = st.SelectedRow.Render(probe)
	}

	buttons := "<Connect>    <Rescan>    <Quit>"
	if m.focus == focusButtons {
		buttons = st.SelectedRow.Render(buttons)
	}

	return []string{
		padOrTrim(baudField+"   "+elfField, w),
		padOrTrim(tcField, w),
		"",
		padOrTrim(probe, w),
		"",
		lipgloss.Place(w, 1, lipgloss.Center, lipgloss.Center, buttons),
	}
}

func portName(p scan.Port) string {
	if p.Product != "" {
		return p.Product
	}
	if p.ByID != "" {
		return filepath.Base(p.ByID)
	}
	return p.Device
}

func portChip(p scan.Port) string {
	prod := strings.ToLower(p.Product)
	if strings.Contains(prod, "usb jtag") || strings.Contains(prod, "serial debug") {
		return "USB-JTAG"
	}
	if strings.Contains(strings.ToLower(p.Manufacturer), "espressif") || strings.EqualFold(p.VID, "303a") {
		return "USB-JTAG"
	}
	if strings.Contains(strings.ToLower(p.Product), "cp210") {
		return "CP210x"
	}
	return "(unknown)"
}

func updateSingleLineField(cur string, k tea.KeyMsg) string {
	switch k.Type {
	case tea.KeyBackspace:
		if cur == "" {
			return cur
		}
		_, size := utf8.DecodeLastRuneInString(cur)
		if size <= 0 {
			size = 1
		}
		return cur[:len(cur)-size]
	case tea.KeyRunes:
		s := string(k.Runes)
		// Keep it single-line; trim control.
		s = strings.Map(func(r rune) rune {
			if r == '\n' || r == '\r' || r == '\t' {
				return -1
			}
			return r
		}, s)
		return cur + s
	case tea.KeyCtrlU:
		return ""
	case tea.KeyEnter:
		return cur
	}

	return cur
}

// padOrTrim, min, max moved to ui_helpers.go
