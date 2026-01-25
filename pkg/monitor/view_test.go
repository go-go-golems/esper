package monitor

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func splitLinesExact(s string) []string {
	lines := strings.Split(s, "\n")
	// If the view ends with a newline, drop the final empty row for sizing assertions.
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func assertViewSized(t *testing.T, view string, w, h int) {
	t.Helper()

	lines := splitLinesExact(view)
	if len(lines) != h {
		t.Fatalf("expected %d lines, got %d (lipgloss.Height=%d)", h, len(lines), lipgloss.Height(view))
	}
	for i, line := range lines {
		if got := lipgloss.Width(line); got != w {
			t.Fatalf("line %d: expected width %d, got %d", i+1, w, got)
		}
	}
}

func TestAppViewSizing_PortPicker(t *testing.T) {
	m := newAppModel(context.Background(), Config{
		Baud:            115200,
		ToolchainPrefix: "xtensa-esp32s3-elf-",
	})
	m.winW = 120
	m.winH = 40
	t.Logf("frame vborder=%d hborder=%d inner=%+v", m.styles.ScreenFrame.GetVerticalBorderSize(), m.styles.ScreenFrame.GetHorizontalBorderSize(), m.innerSize())
	m.portPicker.setSize(m.innerSize())

	view := m.View()
	assertViewSized(t, view, 120, 40)
}

func TestAppViewSizing_Monitor(t *testing.T) {
	m := newAppModel(context.Background(), Config{
		Port:            "/dev/ttyFAKE",
		Baud:            115200,
		ToolchainPrefix: "xtensa-esp32s3-elf-",
	})
	m.winW = 120
	m.winH = 40
	m.screen = screenMonitor
	m.mode = modeDevice

	m.monitor = newMonitorModel(m.cfg, nil)
	m.monitor.setSize(m.innerSize())
	m.monitor.append([]byte("I (12) wifi: connected ssid=lab-001\n"))

	view := m.View()
	assertViewSized(t, view, 120, 40)
}
