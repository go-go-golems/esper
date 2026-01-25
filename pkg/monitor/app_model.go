package monitor

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type screen int

const (
	screenPortPicker screen = iota
	screenMonitor
)

type mode int

const (
	modeDevice mode = iota
	modeHost
)

type overlayKind int

const (
	overlayNone overlayKind = iota
	overlayHelp
)

type appModel struct {
	ctx context.Context

	cfg Config

	winW int
	winH int

	screen  screen
	mode    mode
	overlay overlayKind

	styles styles

	session *serialSession

	portPicker portPickerModel
	monitor    monitorModel
	help       helpOverlayModel

	initialConnect *connectParams
}

func newAppModel(ctx context.Context, cfg Config) *appModel {
	m := &appModel{
		ctx:     ctx,
		cfg:     cfg,
		screen:  screenPortPicker,
		mode:    modeDevice,
		overlay: overlayNone,
		styles:  defaultStyles(),
		help:    newHelpOverlayModel(),
	}

	m.portPicker = newPortPickerModel(portPickerConfig{
		defaultBaud:      cfg.Baud,
		defaultElfPath:   cfg.ElfPath,
		defaultToolchain: cfg.ToolchainPrefix,
	})

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
		m.portPicker.setSize(m.innerSize())
		m.monitor.setSize(m.innerSize())
		m.help.setSize(m.innerSize())
		return m, nil
	}

	// Global keys.
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit
		}
		if k.String() == "?" {
			if m.overlay == overlayHelp {
				m.overlay = overlayNone
				return m, nil
			}
			m.overlay = overlayHelp
			return m, nil
		}
		if m.overlay == overlayHelp && (k.Type == tea.KeyEsc || k.String() == "q") {
			m.overlay = overlayNone
			return m, nil
		}
	}

	// Overlay captures input first.
	if m.overlay != overlayNone {
		if k, ok := msg.(tea.KeyMsg); ok {
			var cmd tea.Cmd
			m.help, cmd = m.help.Update(k)
			return m, cmd
		}
		return m, nil
	}

	switch msg := msg.(type) {
	case portsScanResultMsg:
		m.portPicker.applyScanResult(msg)
		return m, nil
	case connectResultMsg:
		if msg.err != nil {
			m.portPicker.setError(fmt.Sprintf("Connection failed: %v", msg.err))
			m.portPicker.setErrorHint("Try a different port, or fix permissions.")
			m.screen = screenPortPicker
			return m, nil
		}

		m.close()
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
		m.session = nil
		m.screen = screenPortPicker
		return m, m.portPicker.scanPortsCmd(m.ctx)
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
	switch m.screen {
	case screenPortPicker:
		inner = m.portPicker.View(m.styles, m.innerSize())
	case screenMonitor:
		inner = m.monitor.View(m.styles, m.innerSize(), m.mode)
	default:
		inner = "esper: unknown screen"
	}

	// Screen chrome: outer border around whole UI.
	frame := m.styles.ScreenFrame.
		Width(m.winW).
		Height(m.winH).
		Render(inner)

	if m.overlay == overlayHelp {
		return m.help.RenderOver(m.styles, m.winW, m.winH, frame)
	}
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
	default:
		return cmd
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
