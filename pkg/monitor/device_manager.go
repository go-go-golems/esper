package monitor

import (
	"context"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/go-go-golems/esper/pkg/devices"
	"github.com/go-go-golems/esper/pkg/scan"
)

type deviceManagerActionKind int

const (
	deviceManagerActionNone deviceManagerActionKind = iota
	deviceManagerActionBack
	deviceManagerActionRescan
	deviceManagerActionConnect
	deviceManagerActionOpenOverlay
)

type deviceManagerAction struct {
	kind        deviceManagerActionKind
	connect     connectParams
	overlay     overlayModel
	flashErrMsg string
}

type deviceManagerModel struct {
	sz size

	reg     *devices.Registry
	regPath string
	regErr  string

	ports          []scan.Port
	onlineBySerial map[string]scan.Port

	list selectList
	err  string
}

func newDeviceManagerModel() deviceManagerModel {
	m := deviceManagerModel{}
	m.reloadRegistry()
	return m
}

func (m *deviceManagerModel) setSize(sz size) {
	m.sz = sz
}

func (m *deviceManagerModel) open() {
	m.err = ""
	m.reloadRegistry()
	m.list.SetLen(len(m.devices()))
	if m.list.Selected < 0 {
		m.list.Selected = 0
	}
	if m.list.Selected >= len(m.devices()) {
		m.list.Selected = max(0, len(m.devices())-1)
	}
}

func (m *deviceManagerModel) reloadRegistry() {
	reg, path, err := devices.Load()
	if err != nil {
		m.regErr = err.Error()
		m.regPath = path
		m.reg = &devices.Registry{Version: 1}
		return
	}
	m.reg = reg
	m.regPath = path
	m.regErr = ""
}

func (m *deviceManagerModel) applyScanResult(msg portsScanResultMsg) {
	if msg.err != nil {
		m.err = fmt.Sprintf("scan failed: %v", msg.err)
		return
	}
	m.err = ""
	m.ports = msg.ports
	m.onlineBySerial = make(map[string]scan.Port, len(msg.ports))
	for _, p := range msg.ports {
		s := strings.TrimSpace(p.Serial)
		if s == "" {
			continue
		}
		m.onlineBySerial[s] = p
	}
}

func (m deviceManagerModel) scanPortsCmd(ctx context.Context) tea.Cmd {
	return newScanPortsCmd(ctx)
}

func (m deviceManagerModel) Update(msg tea.Msg) (deviceManagerModel, tea.Cmd, deviceManagerAction) {
	switch msg := msg.(type) {
	case devicesRegistryChangedMsg:
		m.reloadRegistry()
		m.list.SetLen(len(m.devices()))
		if m.list.Selected >= len(m.devices()) {
			m.list.Selected = max(0, len(m.devices())-1)
		}
		return m, nil, deviceManagerAction{}

	case tea.KeyMsg:
		switch msg.String() {
		case "q":
			return m, nil, deviceManagerAction{kind: deviceManagerActionBack}
		case "r":
			return m, nil, deviceManagerAction{kind: deviceManagerActionRescan}
		case "a":
			return m, nil, deviceManagerAction{
				kind:    deviceManagerActionOpenOverlay,
				overlay: newDeviceEditOverlay(deviceEditOverlayConfig{title: "Add Device", isEdit: false}),
			}
		case "e":
			if e := m.selectedEntry(); e != nil {
				return m, nil, deviceManagerAction{
					kind:    deviceManagerActionOpenOverlay,
					overlay: newDeviceEditOverlay(deviceEditOverlayConfig{title: "Edit Device", isEdit: true, entry: *e}),
				}
			}
			return m, nil, deviceManagerAction{}
		case "x":
			if e := m.selectedEntry(); e != nil {
				return m, nil, deviceManagerAction{
					kind:    deviceManagerActionOpenOverlay,
					overlay: newConfirmOverlay(confirmOverlayConfig{title: "Remove Device", message: fmt.Sprintf("Remove %q from registry?", e.Nickname), confirmLabel: "Remove", forward: removeDeviceEntryMsg{usbSerial: e.USBSerial}}),
				}
			}
			return m, nil, deviceManagerAction{}
		case "c":
			return m, nil, m.connectSelectedAction()
		}

		switch msg.Type {
		case tea.KeyEsc:
			return m, nil, deviceManagerAction{kind: deviceManagerActionBack}
		case tea.KeyUp:
			m.list.Move(-1, len(m.devices()))
		case tea.KeyDown:
			m.list.Move(1, len(m.devices()))
		case tea.KeyHome:
			m.list.Selected = 0
		case tea.KeyEnd:
			m.list.Selected = max(0, len(m.devices())-1)
		case tea.KeyEnter:
			if e := m.selectedEntry(); e != nil {
				return m, nil, deviceManagerAction{
					kind:    deviceManagerActionOpenOverlay,
					overlay: newDeviceEditOverlay(deviceEditOverlayConfig{title: "Edit Device", isEdit: true, entry: *e}),
				}
			}
		}
		return m, nil, deviceManagerAction{}
	}

	return m, nil, deviceManagerAction{}
}

func (m deviceManagerModel) View(st styles, sz size) string {
	if sz.W < 40 || sz.H < 10 {
		return st.Hint.Render("Terminal too small.")
	}

	title := padOrTrim(st.TitleBar.Render("esper — Device Manager"), sz.W)

	bodyH := max(1, sz.H-2)
	panelInnerW := max(0, sz.W-st.Panel.GetHorizontalBorderSize())
	panelInnerH := max(0, bodyH-st.Panel.GetVerticalBorderSize())

	content := m.renderPanel(st, size{W: panelInnerW, H: panelInnerH})
	panel := st.Panel.Copy().Width(panelInnerW).Height(panelInnerH).Render(content)

	footer := st.StatusBar.Render("↑↓ Navigate   e Edit   x Remove   c Connect   a Add   r Rescan   Esc Back   ? Help")
	footer = padOrTrim(footer, sz.W)

	return lipgloss.JoinVertical(lipgloss.Left, title, panel, footer)
}

func (m deviceManagerModel) renderPanel(st styles, inner size) string {
	lines := []string{
		st.PanelTitle.Render("Registered Devices"),
		"",
	}

	if m.regErr != "" {
		lines = append(lines, st.Hint.Render("Registry error: "+m.regErr))
		lines = append(lines, st.Hint.Render("Path: "+m.regPath))
		lines = append(lines, "")
	}

	if m.err != "" {
		lines = append(lines, st.Hint.Render(m.err))
		lines = append(lines, "")
	}

	header := m.renderTableHeader(st, inner.W)
	lines = append(lines, header)
	lines = append(lines, m.renderTable(st, inner.W, max(3, inner.H-10))...)
	lines = append(lines, "")
	lines = append(lines, m.renderDetails(st, inner.W)...)

	out := lipgloss.JoinVertical(lipgloss.Left, lines...)
	return lipgloss.NewStyle().Width(inner.W).Height(inner.H).Render(out)
}

func (m deviceManagerModel) renderTableHeader(st styles, w int) string {
	nickW := 12
	nameW := 22
	serialW := 20
	statusW := 10

	h := padOrTrim("NICKNAME", nickW) + " " +
		padOrTrim("NAME", nameW) + " " +
		padOrTrim("USB SERIAL", serialW) + " " +
		padOrTrim("STATUS", statusW)
	return st.Hint.Render(padOrTrim(h, w))
}

func (m deviceManagerModel) renderTable(st styles, w, h int) []string {
	devs := m.devices()
	start, end := m.list.Window(len(devs), h)

	nickW := 12
	nameW := 22
	serialW := 20
	statusW := 10

	var out []string
	for i := start; i < end; i++ {
		d := devs[i]
		cursor := "  "
		rowStyle := st.Row
		if i == m.list.Selected {
			cursor = "→ "
			rowStyle = st.SelectedRow
		}

		status := "○ offline"
		if m.isOnline(d.USBSerial) {
			status = "● online"
		}

		name := d.Name
		if strings.TrimSpace(name) == "" {
			name = "—"
		}

		row := cursor +
			padOrTrim(d.Nickname, nickW) + " " +
			padOrTrim(name, nameW) + " " +
			padOrTrim(d.USBSerial, serialW) + " " +
			padOrTrim(status, statusW)
		out = append(out, rowStyle.Render(padOrTrim(row, w)))
	}

	for len(out) < h {
		out = append(out, "")
	}

	if len(devs) == 0 && len(out) > 0 {
		out[0] = st.Hint.Render("No devices registered yet. Press 'a' to add one.")
		if len(out) > 2 {
			out[2] = st.Hint.Render("Tip: from Port Picker, press 'n' to nickname a connected device.")
		}
	}

	return out
}

func (m deviceManagerModel) renderDetails(st styles, w int) []string {
	e := m.selectedEntry()
	if e == nil {
		return []string{st.Hint.Render("Selected: —")}
	}
	desc := e.Description
	if strings.TrimSpace(desc) == "" {
		desc = "—"
	}
	pp := e.PreferredPath
	if strings.TrimSpace(pp) == "" {
		pp = "—"
	}
	return []string{
		st.PanelTitle.Render("Selected: " + e.Nickname),
		st.Hint.Render("Description: " + padOrTrim(desc, max(10, w-14))),
		st.Hint.Render("Preferred path: " + padOrTrim(pp, max(10, w-16))),
	}
}

func (m deviceManagerModel) devices() []devices.DeviceEntry {
	if m.reg == nil {
		return nil
	}
	return m.reg.Devices
}

func (m deviceManagerModel) selectedEntry() *devices.DeviceEntry {
	devs := m.devices()
	i := m.list.Selected
	if i < 0 || i >= len(devs) {
		return nil
	}
	return &devs[i]
}

func (m deviceManagerModel) isOnline(usbSerial string) bool {
	if m.onlineBySerial == nil {
		return false
	}
	_, ok := m.onlineBySerial[strings.TrimSpace(usbSerial)]
	return ok
}

func (m deviceManagerModel) connectSelectedAction() deviceManagerAction {
	e := m.selectedEntry()
	if e == nil {
		return deviceManagerAction{}
	}

	// Prefer PreferredPath if it exists.
	if strings.TrimSpace(e.PreferredPath) != "" {
		if _, err := os.Stat(e.PreferredPath); err == nil {
			return deviceManagerAction{
				kind: deviceManagerActionConnect,
				connect: connectParams{
					portPath:        e.PreferredPath,
					baud:            115200,
					elfPath:         "",
					toolchainPrefix: "",
				},
			}
		}
	}

	// Otherwise connect only if we can resolve it from current scan.
	if p, ok := m.onlineBySerial[strings.TrimSpace(e.USBSerial)]; ok {
		path := p.PreferredPath
		if path == "" {
			path = p.Device
		}
		if path != "" {
			return deviceManagerAction{
				kind: deviceManagerActionConnect,
				connect: connectParams{
					portPath:        path,
					baud:            115200,
					elfPath:         "",
					toolchainPrefix: "",
				},
			}
		}
	}

	return deviceManagerAction{kind: deviceManagerActionNone, flashErrMsg: "Device is offline; cannot connect"}
}
