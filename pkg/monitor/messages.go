package monitor

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/go-go-golems/esper/pkg/scan"
	"github.com/go-go-golems/esper/pkg/serialio"
	"go.bug.st/serial"
)

type portsScanResultMsg struct {
	ports []scan.Port
	err   error
}

func newScanPortsCmd(ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		ports, err := scan.ScanLinux(ctx, scan.Options{
			All:        true,
			PreferByID: true,
		})
		return portsScanResultMsg{ports: ports, err: err}
	}
}

type connectParams struct {
	portPath        string
	baud            int
	elfPath         string
	toolchainPrefix string

	probeEsptool bool
}

type connectResultMsg struct {
	session         *serialSession
	portPath        string
	baud            int
	elfPath         string
	toolchainPrefix string
	err             error
}

func connectCmd(ctx context.Context, params connectParams) tea.Cmd {
	return func() tea.Msg {
		_ = ctx

		p, openedPath, err := serialio.Open(serialio.OpenConfig{Port: params.portPath, Baud: params.baud})
		if err != nil {
			return connectResultMsg{err: err}
		}

		_ = p.SetReadTimeout(200 * time.Millisecond)

		// Note: probeEsptool behavior is intentionally not implemented here yet.
		// It will be wired in once we decide on confirmation UX and desired reset behavior.

		return connectResultMsg{
			session:         &serialSession{port: p, portPath: openedPath},
			portPath:        openedPath,
			baud:            params.baud,
			elfPath:         params.elfPath,
			toolchainPrefix: params.toolchainPrefix,
		}
	}
}

type disconnectMsg struct {
	reason string
}

type serialChunkMsg struct{ b []byte }
type serialErrMsg struct{ err error }
type tickMsg struct{ t time.Time }

type serialSession struct {
	port     serial.Port
	portPath string
}

func (s *serialSession) Close() error {
	if s == nil || s.port == nil {
		return nil
	}
	return s.port.Close()
}

func (s *serialSession) WriteLine(line string) error {
	if s == nil || s.port == nil {
		return fmt.Errorf("not connected")
	}
	line = strings.TrimRight(line, "\r\n")
	if line != "" {
		if _, err := s.port.Write([]byte(line)); err != nil {
			return err
		}
	}
	_, err := s.port.Write([]byte("\r\n"))
	return err
}

type searchActionKind int

const (
	searchActionJump searchActionKind = iota
	searchActionNext
	searchActionPrev
)

type searchActionMsg struct {
	kind  searchActionKind
	query string
}

type filterSetMsg struct {
	cfg filterConfig
}

type paletteExecMsg struct {
	cmd paletteCommand
}
