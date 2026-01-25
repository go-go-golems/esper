package monitor

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/go-go-golems/esper/pkg/decode"
	"github.com/go-go-golems/esper/pkg/parse"
	"github.com/go-go-golems/esper/pkg/render"
	"github.com/go-go-golems/esper/pkg/serialio"
	"go.bug.st/serial"
)

type serialChunkMsg struct{ b []byte }
type serialErrMsg struct{ err error }
type tickMsg struct{ t time.Time }

type model struct {
	cfg Config

	port serial.Port

	lineSplitter parse.LineSplitter
	autoColor    render.AutoColorer
	gdb          decode.GDBStubDetector
	panic        decode.PanicDecoder
	coredump     decode.CoreDumpDecoder

	lastDataAt time.Time

	out string
}

func Run(ctx context.Context, cfg Config) error {
	p, portPath, err := serialio.Open(serialio.OpenConfig{Port: cfg.Port, Baud: cfg.Baud})
	if err != nil {
		return err
	}
	defer p.Close()
	_ = portPath

	_ = p.SetReadTimeout(200 * time.Millisecond)

	m := &model{
		cfg: cfg,
		port: p,
		lastDataAt: time.Now(),
	}
	m.autoColor.DisableAutoColor = false
	m.panic = decode.PanicDecoder{ElfPath: cfg.ElfPath, ToolchainPrefix: cfg.ToolchainPrefix}
	m.coredump = decode.CoreDumpDecoder{ElfPath: cfg.ElfPath}

	prog := tea.NewProgram(m, tea.WithContext(ctx))
	_, runErr := prog.Run()
	return runErr
}

func (m *model) Init() tea.Cmd {
	return tea.Batch(m.readSerialCmd(), m.tickCmd())
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch t := msg.(type) {
	case tea.KeyMsg:
		if t.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}
		// Minimal: send printable runes and Enter to device; ignore other keys for now.
		switch t.Type {
		case tea.KeyEnter:
			_, _ = m.port.Write([]byte("\r\n"))
		default:
			if s := t.String(); len(s) == 1 {
				_, _ = m.port.Write([]byte(s))
			}
		}
		return m, nil
	case serialChunkMsg:
		if len(t.b) == 0 {
			return m, m.readSerialCmd()
		}
		m.lastDataAt = time.Now()

		if g := m.gdb.Push(t.b); g != nil {
			m.append([]byte("--- GDB stub detected\n"))
		}

		lines := m.lineSplitter.Push(t.b)
		for _, line := range lines {
			events, sendEnter := m.coredump.PushLine(line)
			if sendEnter {
				_, _ = m.port.Write([]byte("\n"))
			}
			if m.coredump.InProgress() {
				// suppress normal output while buffering
				continue
			}
			for _, e := range events {
				m.append(e)
			}

			// panic backtrace decode is opportunistic: if the line contains Backtrace:, emit extra decoded lines.
			if decoded, ok := m.panic.DecodeBacktraceLine(line); ok && len(decoded) > 0 {
				m.append(decoded)
			}

			m.append(m.autoColor.ColorizeLine(line))
		}

		return m, m.readSerialCmd()
	case serialErrMsg:
		m.append([]byte(fmt.Sprintf("--- serial error: %v\n", t.err)))
		return m, m.readSerialCmd()
	case tickMsg:
		// finalize tail if idle
		if time.Since(m.lastDataAt) > 250*time.Millisecond {
			if tail := m.lineSplitter.FinalizeTail(); len(tail) > 0 {
				m.append(m.autoColor.ColorizeLine(append(tail, '\n')))
			}
		}
		return m, m.tickCmd()
	default:
		return m, nil
	}
}

func (m *model) View() string {
	if m.out == "" {
		return "esper: connected (Ctrl-C to exit)\n"
	}
	return m.out
}

func (m *model) append(b []byte) {
	m.out += string(b)
	// Keep a bounded rolling buffer to avoid unbounded memory growth.
	const max = 1 << 20  // 1 MiB
	const keep = 1 << 19 // 512 KiB
	if len(m.out) > max {
		m.out = m.out[len(m.out)-keep:]
	}
}

func (m *model) readSerialCmd() tea.Cmd {
	return func() tea.Msg {
		buf := make([]byte, 4096)
		n, err := m.port.Read(buf)
		if err != nil {
			return serialErrMsg{err: err}
		}
		if n <= 0 {
			return serialChunkMsg{b: nil}
		}
		return serialChunkMsg{b: append([]byte{}, buf[:n]...)}
	}
}

func (m *model) tickCmd() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg{t: t}
	})
}
