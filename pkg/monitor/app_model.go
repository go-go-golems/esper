package monitor

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/go-go-golems/esper/pkg/devices"
)

type screen int

const (
	screenPortPicker screen = iota
	screenMonitor
	screenDeviceManager
)

type mode int

const (
	modeDevice mode = iota
	modeHost
)

type appModel struct {
	ctx context.Context

	cfg Config

	winW int
	winH int

	screen  screen
	mode    mode
	overlay overlayModel

	styles styles

	session *serialSession

	portPicker portPickerModel
	monitor    monitorModel
	deviceMgr  deviceManagerModel

	initialConnect *connectParams
}

func newAppModel(ctx context.Context, cfg Config) *appModel {
	m := &appModel{
		ctx:    ctx,
		cfg:    cfg,
		screen: screenPortPicker,
		mode:   modeDevice,
		styles: defaultStyles(),
	}

	m.portPicker = newPortPickerModel(portPickerConfig{
		defaultBaud:      cfg.Baud,
		defaultElfPath:   cfg.ElfPath,
		defaultToolchain: cfg.ToolchainPrefix,
	})
	m.deviceMgr = newDeviceManagerModel()

	// If a port is provided, optimistically connect; on failure we fall back to port picker with error.
	if cfg.Port != "" {
		m.initialConnect = &connectParams{
			portPath:        cfg.Port,
			baud:            cfg.Baud,
			elfPath:         cfg.ElfPath,
			toolchainPrefix: cfg.ToolchainPrefix,
		}
	}

	return m
}

func (m *appModel) Init() tea.Cmd {
	// Start scanning immediately if we are in the port picker.
	if m.screen == screenPortPicker {
		if m.initialConnect != nil {
			// Attempt connect concurrently with initial scan so the port picker can still populate on failure.
			return tea.Batch(
				m.portPicker.scanPortsCmd(m.ctx),
				connectCmd(m.ctx, *m.initialConnect),
			)
		}
		return m.portPicker.scanPortsCmd(m.ctx)
	}
	return nil
}

func (m *appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Always handle resize first (layout depends on it).
	switch t := msg.(type) {
	case tea.WindowSizeMsg:
		m.winW, m.winH = t.Width, t.Height
		inner := m.innerSize()
		m.portPicker.setSize(inner)
		m.monitor.setSize(inner)
		m.deviceMgr.setSize(inner)
		if m.overlay != nil {
			m.overlay.setSize(inner)
		}
		return m, nil
	}

	// Overlay open/close requests should be handled regardless of current overlay state.
	switch msg := msg.(type) {
	case openOverlayMsg:
		if msg.overlay == nil {
			return m, nil
		}
		m.overlay = msg.overlay
		m.overlay.setSize(m.innerSize())
		m.overlay.open()
		return m, nil
	case closeOverlayMsg:
		m.overlay = nil
		return m, nil
	default:
		// fall through
	}

	// Global keys.
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.Type {
		case tea.KeyCtrlC:
			if m.screen == screenMonitor && m.monitor.coredump.InProgress() {
				// During core dump capture, Ctrl-C aborts capture (per UX spec) instead of quitting.
				break
			}
			return m, tea.Quit
		}
		if k.String() == "?" && m.overlay == nil {
			m.overlay = newHelpOverlay()
			m.overlay.setSize(m.innerSize())
			m.overlay.open()
			return m, nil
		}

		// Port picker shortcuts.
		if m.overlay == nil && m.screen == screenPortPicker {
			switch k.String() {
			case "d":
				m.screen = screenDeviceManager
				m.deviceMgr.setSize(m.innerSize())
				m.deviceMgr.open()
				return m, m.deviceMgr.scanPortsCmd(m.ctx)
			case "n":
				if p := m.portPicker.selectedPort(); p != nil && strings.TrimSpace(p.Serial) != "" {
					entry := devices.DeviceEntry{
						USBSerial:     strings.TrimSpace(p.Serial),
						Name:          strings.TrimSpace(p.Product),
						PreferredPath: strings.TrimSpace(p.PreferredPath),
					}
					m.overlay = newDeviceEditOverlay(deviceEditOverlayConfig{
						title:          "Assign Nickname",
						isEdit:         false,
						entry:          entry,
						fixedUSBSerial: entry.USBSerial,
					})
					m.overlay.setSize(m.innerSize())
					m.overlay.open()
					return m, nil
				}
			}
		}
	}

	// Handle non-key messages that must be processed regardless of overlays.
	switch msg := msg.(type) {
	case portsScanResultMsg:
		switch m.screen {
		case screenPortPicker:
			m.portPicker.applyScanResult(msg)
		case screenDeviceManager:
			m.deviceMgr.applyScanResult(msg)
		}
		return m, nil
	case connectResultMsg:
		if msg.err != nil {
			m.portPicker.setError(fmt.Sprintf("Connection failed: %v", msg.err))
			m.portPicker.setErrorHint("Try a different port, or fix permissions.")
			m.screen = screenPortPicker
			return m, nil
		}

		m.close()
		m.overlay = nil
		m.session = msg.session
		m.cfg.Port = msg.portPath
		m.cfg.Baud = msg.baud
		m.cfg.ElfPath = msg.elfPath
		m.cfg.ToolchainPrefix = msg.toolchainPrefix

		m.monitor = newMonitorModel(m.cfg, m.session)
		m.monitor.setSize(m.innerSize())

		m.screen = screenMonitor
		m.mode = modeDevice
		return m, tea.Batch(m.monitor.readSerialCmd(), m.monitor.tickCmd())
	case disconnectMsg:
		m.close()
		m.overlay = nil
		m.session = nil
		m.screen = screenPortPicker
		return m, m.portPicker.scanPortsCmd(m.ctx)
	case devicesRegistryChangedMsg:
		m.portPicker.reloadRegistry()
		m.deviceMgr.reloadRegistry()
		return m, nil
	case removeDeviceEntryMsg:
		reg, _, err := devices.Load()
		if err != nil {
			m.deviceMgr.err = fmt.Sprintf("registry load failed: %v", err)
			return m, nil
		}
		if !reg.RemoveByUSBSerial(msg.usbSerial) {
			m.deviceMgr.err = fmt.Sprintf("no entry with usb_serial=%q", msg.usbSerial)
			return m, nil
		}
		if _, err := devices.Save(reg); err != nil {
			m.deviceMgr.err = fmt.Sprintf("registry save failed: %v", err)
			return m, nil
		}
		m.portPicker.reloadRegistry()
		m.deviceMgr.reloadRegistry()
		return m, nil
	default:
		// fall through
	}

	// Overlay captures input first.
	if m.overlay != nil {
		if k, ok := msg.(tea.KeyMsg); ok {
			ov, cmd1, out := m.overlay.Update(k)
			m.overlay = ov
			if k.Type == tea.KeyEsc {
				out.close = true
			}
			if out.close {
				m.overlay = nil
			}

			cmd2 := m.routeToScreen(out.forward)
			return m, tea.Batch(cmd1, cmd2)
		}

		// Non-key messages must continue to flow to the active screen (e.g. serial/tick),
		// otherwise "auto overlays" (like core dump capture progress) would deadlock.
		return m, m.routeToScreen(msg)
	}

	// Screen-specific message routing.
	switch m.screen {
	case screenPortPicker:
		pm, cmd, act := m.portPicker.Update(msg)
		m.portPicker = pm
		return m, m.applyPortPickerAction(act, cmd)
	case screenMonitor:
		mm, cmd, act := m.monitor.Update(msg, m.mode)
		m.monitor = mm
		return m, m.applyMonitorAction(act, cmd)
	case screenDeviceManager:
		dm, cmd, act := m.deviceMgr.Update(msg)
		m.deviceMgr = dm
		return m, m.applyDeviceManagerAction(act, cmd)
	default:
		return m, nil
	}
}

func (m *appModel) View() string {
	if m.winW <= 0 || m.winH <= 0 {
		// Before first WindowSizeMsg.
		return "esper: starting..."
	}

	inner := ""
	innerSz := m.innerSize()
	switch m.screen {
	case screenPortPicker:
		inner = m.portPicker.View(m.styles, innerSz)
	case screenMonitor:
		inner = m.monitor.View(m.styles, innerSz, m.mode)
	case screenDeviceManager:
		inner = m.deviceMgr.View(m.styles, innerSz)
	default:
		inner = "esper: unknown screen"
	}

	// Defensive sizing: clamp inner to the exact content area size before applying the
	// outer frame. lipgloss Width/Height are minimums, not maximums.
	innerLines := splitLinesN(inner, innerSz.H)
	for i := range innerLines {
		innerLines[i] = padOrTrim(innerLines[i], innerSz.W)
	}
	inner = strings.Join(innerLines, "\n")

	if m.overlay != nil {
		box := m.overlay.View(m.styles)
		inner = renderOverlayOver(m.styles, innerSz, inner, box)
	}

	// Screen chrome: outer border around whole UI.
	frame := m.styles.ScreenFrame.
		Width(innerSz.W).
		Height(innerSz.H).
		Render(inner)
	return frame
}

func (m *appModel) innerSize() size {
	// Account for the outer frame border.
	w := m.winW - m.styles.ScreenFrame.GetHorizontalBorderSize()
	h := m.winH - m.styles.ScreenFrame.GetVerticalBorderSize()
	if w < 0 {
		w = 0
	}
	if h < 0 {
		h = 0
	}
	return size{W: w, H: h}
}

func (m *appModel) close() {
	m.monitor.closeSessionLogging()
	if m.session != nil {
		_ = m.session.Close()
		m.session = nil
	}
}

func (m *appModel) applyPortPickerAction(act portPickerAction, cmd tea.Cmd) tea.Cmd {
	switch act.kind {
	case portPickerActionNone:
		return cmd
	case portPickerActionRescan:
		return tea.Batch(cmd, m.portPicker.scanPortsCmd(m.ctx))
	case portPickerActionConnect:
		// Connect happens as a command that returns connectResultMsg.
		return tea.Batch(cmd, connectCmd(m.ctx, act.connectParams))
	case portPickerActionQuit:
		return tea.Quit
	default:
		return cmd
	}
}

func (m *appModel) applyMonitorAction(act monitorAction, cmd tea.Cmd) tea.Cmd {
	switch act.kind {
	case monitorActionNone:
		return cmd
	case monitorActionDisconnect:
		return tea.Batch(cmd, func() tea.Msg { return disconnectMsg{reason: act.reason} })
	case monitorActionModeChanged:
		m.mode = act.mode
		return cmd
	case monitorActionOpenOverlay:
		if act.overlay != nil {
			m.overlay = act.overlay
			m.overlay.setSize(m.innerSize())
			m.overlay.open()
		}
		return cmd
	case monitorActionQuit:
		return tea.Quit
	default:
		return cmd
	}
}

func (m *appModel) applyDeviceManagerAction(act deviceManagerAction, cmd tea.Cmd) tea.Cmd {
	switch act.kind {
	case deviceManagerActionNone:
		if strings.TrimSpace(act.flashErrMsg) != "" {
			m.deviceMgr.err = act.flashErrMsg
		}
		return cmd
	case deviceManagerActionBack:
		m.screen = screenPortPicker
		return tea.Batch(cmd, m.portPicker.scanPortsCmd(m.ctx))
	case deviceManagerActionRescan:
		return tea.Batch(cmd, m.deviceMgr.scanPortsCmd(m.ctx))
	case deviceManagerActionConnect:
		cp := act.connect
		if cp.baud == 0 {
			cp.baud = m.cfg.Baud
		}
		if strings.TrimSpace(cp.elfPath) == "" {
			cp.elfPath = m.cfg.ElfPath
		}
		if strings.TrimSpace(cp.toolchainPrefix) == "" {
			cp.toolchainPrefix = m.cfg.ToolchainPrefix
		}
		return tea.Batch(cmd, connectCmd(m.ctx, cp))
	case deviceManagerActionOpenOverlay:
		if act.overlay != nil {
			m.overlay = act.overlay
			m.overlay.setSize(m.innerSize())
			m.overlay.open()
		}
		return cmd
	default:
		return cmd
	}
}

func (m *appModel) routeToScreen(msg tea.Msg) tea.Cmd {
	if msg == nil {
		return nil
	}

	switch m.screen {
	case screenPortPicker:
		pm, cmd, act := m.portPicker.Update(msg)
		m.portPicker = pm
		return m.applyPortPickerAction(act, cmd)
	case screenMonitor:
		mm, cmd, act := m.monitor.Update(msg, m.mode)
		m.monitor = mm
		return m.applyMonitorAction(act, cmd)
	case screenDeviceManager:
		dm, cmd, act := m.deviceMgr.Update(msg)
		m.deviceMgr = dm
		return m.applyDeviceManagerAction(act, cmd)
	default:
		return nil
	}
}

type size struct {
	W int
	H int
}

func clamp(n, lo, hi int) int {
	if n < lo {
		return lo
	}
	if n > hi {
		return hi
	}
	return n
}

func (s size) PlaceCentered(str string) string {
	return lipgloss.Place(s.W, s.H, lipgloss.Center, lipgloss.Center, str)
}
