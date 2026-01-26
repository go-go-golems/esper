package monitor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (m *monitorModel) toggleSessionLogging() error {
	if m.sessionLogOn {
		m.closeSessionLogging()
		m.setToast("log: OFF", 2*time.Second)
		return nil
	}

	dir, err := os.UserCacheDir()
	if err != nil || strings.TrimSpace(dir) == "" {
		dir = os.TempDir()
	}
	dir = filepath.Join(dir, "esper")
	if mkErr := os.MkdirAll(dir, 0o755); mkErr != nil {
		return mkErr
	}
	path := filepath.Join(dir, fmt.Sprintf("session-%s.log", time.Now().Format("20060102-150405")))

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}

	m.sessionLogOn = true
	m.sessionLogFile = f
	m.sessionLogPath = path
	m.setToast("log: ON ("+filepath.Base(path)+")", 3*time.Second)
	return nil
}

func (m *monitorModel) closeSessionLogging() {
	if m.sessionLogFile != nil {
		_ = m.sessionLogFile.Close()
	}
	m.sessionLogOn = false
	m.sessionLogFile = nil
	m.sessionLogPath = ""
}

func (m *monitorModel) writeSessionLog(b []byte) {
	if !m.sessionLogOn || m.sessionLogFile == nil || len(b) == 0 {
		return
	}
	if _, err := m.sessionLogFile.Write(b); err != nil {
		// If the file goes bad (disk full, etc), stop logging to avoid repeated errors.
		m.closeSessionLogging()
		m.setToast(fmt.Sprintf("log write failed: %v", err), 3*time.Second)
	}
}
