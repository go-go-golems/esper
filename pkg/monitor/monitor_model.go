package monitor

import (
	"os"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/go-go-golems/esper/pkg/decode"
	"github.com/go-go-golems/esper/pkg/parse"
	"github.com/go-go-golems/esper/pkg/render"
)

type monitorActionKind int

const (
	monitorActionNone monitorActionKind = iota
	monitorActionDisconnect
	monitorActionModeChanged
	monitorActionOpenOverlay
	monitorActionQuit
)

type monitorAction struct {
	kind    monitorActionKind
	reason  string
	mode    mode
	overlay overlayModel
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

	lastDataAt        time.Time
	coreDumpStartedAt time.Time

	out string
	log []string

	viewport viewport.Model
	follow   bool

	input textinput.Model

	now time.Time

	// Host-mode extras.
	hostFocus     hostFocus
	showInspector bool

	events    []monitorEvent
	eventList selectList

	toastUntil time.Time
	toastText  string

	searchActive bool
	searchInput  textinput.Model

	searchQuery   string
	searchMatches []int
	searchCur     int

	wrap bool

	sessionLogOn   bool
	sessionLogFile *os.File
	sessionLogPath string

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
	m.viewport.HighPerformanceRendering = false

	ti := textinput.New()
	ti.Prompt = ""
	ti.Placeholder = ""
	ti.Focus()
	m.input = ti

	si := textinput.New()
	si.Prompt = ""
	si.Placeholder = "Search..."
	si.Blur()
	m.searchInput = si

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
	m.searchInput.Width = max(10, sz.W-24)

	m.viewport.SetContent(m.out)
	if m.follow {
		m.viewport.GotoBottom()
	}
}

func (m monitorModel) viewportWidthFor(sz size) int {
	if m.showInspector && sz.W >= 100 {
		panelW := max(32, min(54, sz.W/3))
		return max(1, sz.W-panelW-1)
	}
	return max(1, sz.W)
}
