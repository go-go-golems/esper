package monitor

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMakeTimestampedPath_UsesXDGCacheHomeAndAvoidsCollisions(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmp)

	at := time.Date(2026, 1, 26, 12, 34, 56, 0, time.UTC)
	p1, err := makeTimestampedPath("esper-coredump-report", "txt", at)
	if err != nil {
		t.Fatalf("makeTimestampedPath: %v", err)
	}
	wantPrefix := filepath.Join(tmp, "esper", "esper-coredump-report-20260126-123456")
	if !strings.HasPrefix(p1, wantPrefix) {
		t.Fatalf("unexpected path: %q (want prefix %q)", p1, wantPrefix)
	}
	if err := os.WriteFile(p1, []byte("x"), 0o644); err != nil {
		t.Fatalf("write p1: %v", err)
	}

	p2, err := makeTimestampedPath("esper-coredump-report", "txt", at)
	if err != nil {
		t.Fatalf("makeTimestampedPath(2): %v", err)
	}
	if p2 == p1 {
		t.Fatalf("expected collision-avoidance path; got same path %q", p2)
	}
	if !strings.Contains(p2, "20260126-123456-2") {
		t.Fatalf("expected -2 suffix for collision avoidance; got %q", p2)
	}
}

func TestInspectorDetailCopyTextMsg_CopyErrorShowsToast(t *testing.T) {
	orig := clipboardWriteAll
	clipboardWriteAll = func(string) error { return errors.New("boom") }
	t.Cleanup(func() { clipboardWriteAll = orig })

	m := newMonitorModel(Config{}, nil)
	m2, _, _ := m.Update(inspectorDetailCopyTextMsg{label: "raw", text: "hello"}, modeHost)
	if !strings.Contains(m2.toastText, "copy failed") {
		t.Fatalf("expected copy error toast, got %q", m2.toastText)
	}
}
