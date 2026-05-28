package serialio

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"go.bug.st/serial"
)

type OpenConfig struct {
	Port string
	Baud int
}

func Open(cfg OpenConfig) (serial.Port, string, error) {
	if cfg.Port == "" {
		return nil, "", errors.New("missing port")
	}
	if cfg.Baud <= 0 {
		return nil, "", fmt.Errorf("invalid baud: %d", cfg.Baud)
	}

	portPath := cfg.Port
	if hasGlob(portPath) {
		matches, err := filepath.Glob(portPath)
		if err != nil {
			return nil, "", fmt.Errorf("glob port: %w", err)
		}
		if len(matches) == 0 {
			return nil, "", fmt.Errorf("no ports match: %q", portPath)
		}
		portPath = matches[0]
	}

	mode := &serial.Mode{BaudRate: cfg.Baud}
	p, err := serial.Open(portPath, mode)
	if err != nil {
		return nil, "", err
	}
	return p, portPath, nil
}

func hasGlob(s string) bool {
	return strings.ContainsAny(s, "*?[")
}
