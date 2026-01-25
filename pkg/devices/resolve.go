package devices

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/go-go-golems/esper/pkg/scan"
)

type ResolveResult struct {
	PortPath string
	Entry    DeviceEntry
}

func ResolveNicknameToPort(ctx context.Context, nickname string) (*ResolveResult, error) {
	reg, _, err := Load()
	if err != nil {
		return nil, err
	}

	entry := reg.FindByNickname(nickname)
	if entry == nil {
		return nil, fmt.Errorf("unknown device nickname %q", nickname)
	}

	if entry.PreferredPath != "" {
		if _, err := os.Stat(entry.PreferredPath); err == nil {
			return &ResolveResult{PortPath: entry.PreferredPath, Entry: *entry}, nil
		}
	}

	ports, err := scan.ScanLinux(ctx, scan.Options{
		All:          true,
		PreferByID:   true,
		ProbeEsptool: false,
	})
	if err != nil {
		return nil, err
	}

	for _, p := range ports {
		if strings.TrimSpace(p.Serial) == strings.TrimSpace(entry.USBSerial) {
			portPath := p.PreferredPath
			if portPath == "" {
				portPath = p.Device
			}
			if portPath != "" {
				return &ResolveResult{PortPath: portPath, Entry: *entry}, nil
			}
		}
	}

	return nil, errors.New("device not currently connected (run `esper scan`)")
}
