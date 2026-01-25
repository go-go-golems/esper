package monitor

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

func Run(ctx context.Context, cfg Config) error {
	m := newAppModel(ctx, cfg)

	prog := tea.NewProgram(
		m,
		tea.WithContext(ctx),
		tea.WithAltScreen(),
	)

	final, err := prog.Run()

	// Best-effort cleanup (Bubble Tea has no destructor hook).
	if am, ok := final.(*appModel); ok {
		am.close()
	}

	if err != nil {
		return fmt.Errorf("esper tui: %w", err)
	}
	return nil
}
