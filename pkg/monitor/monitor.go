package monitor

import (
	"context"
	"errors"
	"fmt"
)

func Run(ctx context.Context, cfg Config) error {
	if cfg.Port == "" {
		return errors.New("missing -port")
	}
	_ = ctx
	return fmt.Errorf("not implemented yet (port=%q baud=%d)", cfg.Port, cfg.Baud)
}

