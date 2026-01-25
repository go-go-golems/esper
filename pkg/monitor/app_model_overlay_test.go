package monitor

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestAppModel_OverlayDoesNotBlockNonKeyMsgs(t *testing.T) {
	app := newAppModel(context.Background(), Config{})
	app.screen = screenMonitor
	app.monitor = newMonitorModel(app.cfg, nil)

	// Size the app so innerSize is non-zero.
	_, _ = app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	// Open an overlay.
	_, _ = app.Update(openOverlayMsg{overlay: newHelpOverlay()})
	if app.overlay == nil {
		t.Fatalf("expected overlay to be open")
	}

	// While overlay is open, non-key messages must still reach the active screen.
	_, _ = app.Update(serialChunkMsg{b: []byte("I (1) test: hello\n")})

	if app.overlay == nil {
		t.Fatalf("expected overlay to remain open after non-key msg")
	}
	if !strings.Contains(stripANSI(app.monitor.out), "hello") {
		t.Fatalf("expected monitor output to be updated while overlay open; got %q", app.monitor.out)
	}
}
